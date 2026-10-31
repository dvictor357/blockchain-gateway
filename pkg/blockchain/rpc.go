package blockchain

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

const (
	// maxErrorBodyBytes bounds how much of a non-2xx upstream body is read.
	maxErrorBodyBytes = 64 << 10
	// maxSnippetBytes bounds the upstream text included in (log-only) error messages.
	maxSnippetBytes = 256
)

// rpcEnvelope is the shape shared by EVM and Bitcoin JSON-RPC responses. Raw
// members distinguish an absent member from an explicit null.
type rpcEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

// callJSONRPC performs one JSON-RPC call over httpClient and validates the response envelope.
// legacy selects the Bitcoin Core 1.0 envelope rules (no jsonrpc member required, result:null
// permitted next to an error); otherwise strict JSON-RPC 2.0 rules apply.
// Failures are *RPCError for node-reported errors and wrap ErrUpstream for everything else.
// Errors never contain the endpoint URL, which may embed credentials.
func callJSONRPC(ctx context.Context, httpClient *http.Client, endpoint, version string, legacy bool, id uint64, method string, params any) (json.RawMessage, error) {
	requestBody, err := json.Marshal(RPCRequest{JSONRPC: version, Method: method, Params: params, ID: id})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", stripURL(err))
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: request failed: %w", ErrUpstream, stripURL(err))
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// Bitcoin Core reports RPC errors with non-2xx statuses, so a well-formed error
		// envelope is still surfaced as a structured error. Nothing here is ever a success.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBodyBytes))
		if _, rpcErr, parseErr := parseEnvelope(bytes.NewReader(body), id, legacy); parseErr == nil && rpcErr != nil {
			return nil, rpcErr
		}
		return nil, fmt.Errorf("%w: HTTP status %d: %s", ErrUpstream, resp.StatusCode, snippet(body))
	}

	result, rpcErr, err := parseEnvelope(resp.Body, id, legacy)
	if err != nil {
		return nil, err
	}
	if rpcErr != nil {
		return nil, rpcErr
	}
	return result, nil
}

// parseEnvelope decodes and validates one response envelope.
func parseEnvelope(r io.Reader, wantID uint64, legacy bool) (json.RawMessage, *RPCError, error) {
	decoder := json.NewDecoder(r)
	var env rpcEnvelope
	if err := decoder.Decode(&env); err != nil {
		return nil, nil, fmt.Errorf("%w: invalid response: %w", ErrUpstream, err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("unexpected data after response")
		}
		return nil, nil, fmt.Errorf("%w: invalid response: %w", ErrUpstream, err)
	}

	// The gateway always sends a numeric id, so any other id (including null) cannot be
	// correlated with this request.
	gotID, idErr := strconv.ParseUint(string(env.ID), 10, 64)
	if idErr != nil || gotID != wantID {
		return nil, nil, fmt.Errorf("%w: invalid response: id does not match request", ErrUpstream)
	}

	errorPresent := len(env.Error) > 0 && !isJSONNull(env.Error)
	resultPresent := len(env.Result) > 0

	if !legacy && env.JSONRPC != "2.0" {
		return nil, nil, fmt.Errorf("%w: invalid response: jsonrpc version is not 2.0", ErrUpstream)
	}

	if errorPresent {
		if !legacy && resultPresent {
			return nil, nil, fmt.Errorf("%w: invalid response: both result and error present", ErrUpstream)
		}
		rpcErr, err := parseRPCError(env.Error)
		if err != nil {
			return nil, nil, err
		}
		return nil, rpcErr, nil
	}

	if !resultPresent {
		return nil, nil, fmt.Errorf("%w: invalid response: neither result nor error present", ErrUpstream)
	}
	return env.Result, nil, nil
}

// parseRPCError requires an object with an integer code and a string message.
func parseRPCError(raw json.RawMessage) (*RPCError, error) {
	var obj struct {
		Code    *int            `json:"code"`
		Message *string         `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &obj); err != nil || obj.Code == nil || obj.Message == nil {
		return nil, fmt.Errorf("%w: invalid response: malformed error object", ErrUpstream)
	}
	return &RPCError{Code: *obj.Code, Message: *obj.Message, Data: obj.Data}, nil
}

func isJSONNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// stripURL drops the request URL (which may contain credentials) from transport errors
// while keeping the cause available to errors.Is/As.
func stripURL(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return urlErr.Err
	}
	return err
}

// snippet returns a bounded, printable excerpt of an upstream body for diagnostics.
func snippet(body []byte) string {
	if len(body) > maxSnippetBytes {
		body = body[:maxSnippetBytes]
	}
	text := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, strings.ToValidUTF8(string(body), "?"))
	return strings.Join(strings.Fields(text), " ")
}

// IsTimeout reports whether err is an actual deadline expiry (request context or HTTP
// client timeout). Client cancellation is not a timeout.
func IsTimeout(err error) bool {
	if errors.Is(err, ErrRPCTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// decodeResult unmarshals a required RPC result. A null result is rejected rather than being
// silently decoded into a zero value, and shape mismatches are classified as upstream failures.
func decodeResult(result json.RawMessage, destination any, what string) error {
	if isJSONNull(result) {
		return fmt.Errorf("%w: %s result is null", ErrUpstream, what)
	}
	return decodeNullableResult(result, destination, what)
}

// decodeNullableResult is decodeResult for methods where null means "does not exist"; a null
// result leaves the destination at its zero value (for example a nil map).
func decodeNullableResult(result json.RawMessage, destination any, what string) error {
	if err := json.Unmarshal(result, destination); err != nil {
		return fmt.Errorf("%w: unexpected %s result: %w", ErrUpstream, what, err)
	}
	return nil
}
