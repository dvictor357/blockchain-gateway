package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEVMClient_Execute_SendsJSONRPCRequest(t *testing.T) {
	var got struct {
		JSONRPC string          `json:"jsonrpc"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
		ID      json.RawMessage `json:"id"`
	}
	var method, contentType string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, contentType = r.Method, r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = io.WriteString(w, `{"jsonrpc":"2.0","id":1,"result":"0x1"}`)
	}))
	defer server.Close()

	_, err := NewEVMClient("test", server.URL, nil).Execute(context.Background(), "eth_getBalance", []any{"0xabc", "latest"})

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, method)
	assert.Equal(t, "application/json", contentType)
	assert.Equal(t, "2.0", got.JSONRPC)
	assert.Equal(t, "eth_getBalance", got.Method)
	assert.JSONEq(t, `["0xabc","latest"]`, string(got.Params))
	assert.Equal(t, "1", string(got.ID))
}

func TestEVMClient_Execute_ValidResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "string result", body: `{"jsonrpc":"2.0","result":"0x1234567","id":1}`, want: `"0x1234567"`},
		{name: "null result is preserved", body: `{"jsonrpc":"2.0","result":null,"id":1}`, want: `null`},
		{name: "object result", body: `{"jsonrpc":"2.0","id":1,"result":{"a":[1,2]}}`, want: `{"a":[1,2]}`},
		{name: "null error next to a result", body: `{"jsonrpc":"2.0","id":1,"result":"0x1","error":null}`, want: `"0x1"`},
		{name: "whitespace after the envelope", body: "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":true}\n\n", want: `true`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRawServer(t, http.StatusOK, tt.body)

			result, err := NewEVMClient("test", server.URL, nil).Execute(context.Background(), "m", []any{})

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(result))
		})
	}
}

func TestEVMClient_Execute_RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "HTTP 500 with a success body", status: 500, body: `{"jsonrpc":"2.0","id":1,"result":"0x1"}`},
		{name: "HTTP 503 with a success body", status: 503, body: `{"jsonrpc":"2.0","id":1,"result":"0x1"}`},
		{name: "HTTP 404", status: 404, body: `not found`},
		{name: "empty body", status: 200, body: ``},
		{name: "empty envelope", status: 200, body: `{}`},
		{name: "invalid JSON", status: 200, body: `invalid json`},
		{name: "array instead of envelope", status: 200, body: `[{"jsonrpc":"2.0","id":1,"result":"0x1"}]`},
		{name: "second JSON value", status: 200, body: `{"jsonrpc":"2.0","id":1,"result":"0x1"}{"x":1}`},
		{name: "trailing garbage", status: 200, body: `{"jsonrpc":"2.0","id":1,"result":"0x1"} junk`},
		{name: "wrong version", status: 200, body: `{"jsonrpc":"1.0","id":1,"result":"0x1"}`},
		{name: "missing version", status: 200, body: `{"id":1,"result":"0x1"}`},
		{name: "missing id", status: 200, body: `{"jsonrpc":"2.0","result":"0x1"}`},
		{name: "null id", status: 200, body: `{"jsonrpc":"2.0","id":null,"result":"0x1"}`},
		{name: "wrong id", status: 200, body: `{"jsonrpc":"2.0","id":2,"result":"0x1"}`},
		{name: "string id", status: 200, body: `{"jsonrpc":"2.0","id":"1","result":"0x1"}`},
		{name: "error response with null id", status: 200, body: `{"jsonrpc":"2.0","id":null,"error":{"code":-32600,"message":"bad"}}`},
		{name: "error response with wrong id", status: 200, body: `{"jsonrpc":"2.0","id":9,"error":{"code":-32600,"message":"bad"}}`},
		{name: "neither result nor error", status: 200, body: `{"jsonrpc":"2.0","id":1}`},
		{name: "null error and no result", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":null}`},
		{name: "error together with result", status: 200, body: `{"jsonrpc":"2.0","id":1,"result":"0x1","error":{"code":1,"message":"m"}}`},
		{name: "error together with null result", status: 200, body: `{"jsonrpc":"2.0","id":1,"result":null,"error":{"code":1,"message":"m"}}`},
		{name: "error without code", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":{"message":"m"}}`},
		{name: "error without message", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":{"code":1}}`},
		{name: "error with string code", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":{"code":"1","message":"m"}}`},
		{name: "error with fractional code", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":{"code":1.5,"message":"m"}}`},
		{name: "error with non-string message", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":{"code":1,"message":7}}`},
		{name: "error that is not an object", status: 200, body: `{"jsonrpc":"2.0","id":1,"error":"boom"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRawServer(t, tt.status, tt.body)

			result, err := NewEVMClient("test", server.URL, nil).Execute(context.Background(), "m", []any{})

			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ErrUpstream)
			var rpcErr *RPCError
			assert.False(t, errors.As(err, &rpcErr), "malformed envelopes are not node-reported errors")
		})
	}
}

func TestEVMClient_Execute_PreservesStructuredRPCErrors(t *testing.T) {
	tests := []struct {
		name     string
		errorObj string
		code     int
		message  string
		data     string
	}{
		{name: "object data", errorObj: `{"code":3,"message":"execution reverted","data":{"reason":"0x08c379a0","list":[1,{"a":null}]}}`, code: 3, message: "execution reverted", data: `{"reason":"0x08c379a0","list":[1,{"a":null}]}`},
		{name: "string data", errorObj: `{"code":-32000,"message":"boom","data":"0xdeadbeef"}`, code: -32000, message: "boom", data: `"0xdeadbeef"`},
		{name: "number data", errorObj: `{"code":-32602,"message":"bad params","data":42}`, code: -32602, message: "bad params", data: `42`},
		{name: "no data", errorObj: `{"code":-32601,"message":"method not found"}`, code: -32601, message: "method not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{rpcError: tt.errorObj} })

			_, err := NewEVMClient("test", server.URL, nil).Execute(context.Background(), "m", []any{})

			var rpcErr *RPCError
			require.ErrorAs(t, err, &rpcErr)
			assert.Equal(t, tt.code, rpcErr.Code)
			assert.Equal(t, tt.message, rpcErr.Message)
			assert.Equal(t, tt.data, string(rpcErr.Data))
			assert.NotErrorIs(t, err, ErrUpstream, "a node error is not an upstream protocol failure")
		})
	}
}

func TestEVMClient_Execute_NonSuccessStatusWithErrorEnvelopeIsStillAnError(t *testing.T) {
	server := newRawServer(t, http.StatusBadRequest, `{"jsonrpc":"2.0","id":1,"error":{"code":-32602,"message":"bad","data":{"x":1}}}`)

	_, err := NewEVMClient("test", server.URL, nil).Execute(context.Background(), "m", []any{})

	var rpcErr *RPCError
	require.ErrorAs(t, err, &rpcErr)
	assert.Equal(t, -32602, rpcErr.Code)
	assert.JSONEq(t, `{"x":1}`, string(rpcErr.Data))
}

func TestEVMClient_Execute_ErrorsDoNotExposeURLOrBody(t *testing.T) {
	const secretPath = "/v2/secret-api-key"
	oversized := strings.Repeat("x", 2000) + "SECRET-BODY-TAIL"

	t.Run("bad status with a large body", func(t *testing.T) {
		server := newRawServer(t, http.StatusInternalServerError, oversized)

		_, err := NewEVMClient("test", server.URL+secretPath, nil).Execute(context.Background(), "m", nil)

		require.ErrorIs(t, err, ErrUpstream)
		assert.Less(t, len(err.Error()), 600, "diagnostic text must be bounded")
		assert.NotContains(t, err.Error(), "secret-api-key")
		assert.NotContains(t, err.Error(), "SECRET-BODY-TAIL")
	})

	t.Run("transport failure", func(t *testing.T) {
		server := httptest.NewServer(http.NotFoundHandler())
		url := server.URL + secretPath
		server.Close() // nothing listens on this loopback port any more

		_, err := NewEVMClient("test", url, nil).Execute(context.Background(), "m", nil)

		require.ErrorIs(t, err, ErrUpstream)
		assert.NotContains(t, err.Error(), "secret-api-key")
	})
}

// timeoutNetError mimics the error an http.Client returns when its own Timeout elapses.
type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "client timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

var _ net.Error = timeoutNetError{}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestEVMClient_Execute_TimeoutAndCancellationAreDistinct(t *testing.T) {
	t.Run("expired context deadline", func(t *testing.T) {
		server, hits := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: `"0x1"`} })
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		_, err := NewEVMClient("test", server.URL, nil).Execute(ctx, "m", nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.DeadlineExceeded)
		assert.True(t, IsTimeout(err))
		assert.NotErrorIs(t, err, context.Canceled)
		assert.Zero(t, hits.Load())
	})

	t.Run("http client timeout", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, timeoutNetError{}
		})}

		_, err := NewEVMClient("test", "http://node.invalid", client).Execute(context.Background(), "m", nil)

		require.Error(t, err)
		assert.True(t, IsTimeout(err))
		assert.NotErrorIs(t, err, context.Canceled)
	})

	t.Run("cancellation while the node is busy", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		release := make(chan struct{})
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Drain the request so the server can notice the client disconnecting.
			_, _ = io.Copy(io.Discard, r.Body)
			cancel()
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		defer server.Close()
		defer close(release) // runs before server.Close so a stuck handler can never block it

		_, err := NewEVMClient("test", server.URL, nil).Execute(ctx, "m", nil)

		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.False(t, IsTimeout(err), "client cancellation is not a timeout")
	})
}

func TestEVMClient_Execute_ConcurrentCallsUseDistinctIDs(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]int{}
	server, _ := newNode(t, false, func(call nodeCall) nodeReply {
		mu.Lock()
		seen[string(call.ID)]++
		mu.Unlock()
		return nodeReply{result: string(call.ID)}
	})
	client := NewEVMClient("test", server.URL, nil)

	const callers = 25
	var wg sync.WaitGroup
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.Execute(context.Background(), "m", nil)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		assert.NoError(t, err, "each response must carry its own request's id")
	}
	assert.Len(t, seen, callers)
}

func TestEVMClient_GetLatestBlockNumber(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		want    uint64
		wantErr bool
	}{
		{name: "valid block number", result: `"0x1234567"`, want: 0x1234567},
		{name: "zero block number", result: `"0x0"`, want: 0},
		{name: "invalid hex", result: `"invalid"`, wantErr: true},
		{name: "prefix only", result: `"0x"`, wantErr: true},
		{name: "null result is not zero", result: `null`, wantErr: true},
		{name: "non-string result", result: `123`, wantErr: true},
		{name: "overflow", result: `"0x1ffffffffffffffff"`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, _ := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: tt.result} })

			got, err := NewEVMClient("test", server.URL, nil).GetLatestBlockNumber(context.Background())

			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUpstream)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEVMClient_BlockSelectorDefaultsToLatest(t *testing.T) {
	tests := []struct {
		name     string
		selector string
		want     string
	}{
		{name: "empty", selector: "", want: "latest"},
		{name: "blank", selector: "  ", want: "latest"},
		{name: "latest", selector: "latest", want: "latest"},
		{name: "explicit block", selector: "0x1234", want: "0x1234"},
	}

	for _, tt := range tests {
		for _, method := range []string{"eth_getBalance", "eth_getTransactionCount"} {
			t.Run(method+"/"+tt.name, func(t *testing.T) {
				var params []string
				server, _ := newNode(t, false, func(call nodeCall) nodeReply {
					assert.Equal(t, method, call.Method)
					assert.NoError(t, json.Unmarshal(call.Params, &params))
					return nodeReply{result: `"0x1"`}
				})
				client := NewEVMClient("test", server.URL, nil)

				var err error
				if method == "eth_getBalance" {
					_, err = client.GetBalance(context.Background(), "0x123", tt.selector)
				} else {
					_, err = client.GetTransactionCount(context.Background(), "0x123", tt.selector)
				}

				require.NoError(t, err)
				assert.Equal(t, []string{"0x123", tt.want}, params)
			})
		}
	}
}

func TestEVMClient_GetTransactionCount_RejectsMalformedQuantity(t *testing.T) {
	for _, result := range []string{`"invalid"`, `"0x"`, `null`, `"-0x1"`} {
		server, _ := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: result} })

		_, err := NewEVMClient("test", server.URL, nil).GetTransactionCount(context.Background(), "0x123", "latest")

		assert.ErrorIs(t, err, ErrUpstream, result)
	}
}

func TestEVMClient_SendRawTransaction(t *testing.T) {
	const body = "f86c808504a817c800825208"

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "0x prefix kept", input: "0x" + body, want: "0x" + body},
		{name: "prefix added", input: body, want: "0x" + body},
		{name: "uppercase prefix normalized", input: "0X" + body, want: "0x" + body},
		{name: "whitespace trimmed", input: "  " + body + "\n", want: "0x" + body},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var params []string
			server, _ := newNode(t, false, func(call nodeCall) nodeReply {
				assert.Equal(t, "eth_sendRawTransaction", call.Method)
				assert.NoError(t, json.Unmarshal(call.Params, &params))
				return nodeReply{result: `"0xabc123"`}
			})

			hash, err := NewEVMClient("test", server.URL, nil).SendRawTransaction(context.Background(), tt.input)

			require.NoError(t, err)
			assert.Equal(t, "0xabc123", hash)
			assert.Equal(t, []string{tt.want}, params)
		})
	}

	t.Run("empty data is rejected before contacting the node", func(t *testing.T) {
		server, hits := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: `"0x"`} })

		_, err := NewEVMClient("test", server.URL, nil).SendRawTransaction(context.Background(), " ")

		assert.ErrorIs(t, err, ErrInvalidRequest)
		assert.Zero(t, hits.Load())
	})
}

func TestParseHexToUint64(t *testing.T) {
	valid := map[string]uint64{
		"0x1234":             0x1234,
		"1234":               0x1234, // the prefix is optional
		"0x0":                0,
		"0x0000000000000001": 1,
		"0xFFFFFFFFFFFFFFFF": 0xffffffffffffffff,
		"0xffffffffffffffff": 0xffffffffffffffff,
	}
	for input, want := range valid {
		got, err := parseHexToUint64(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, got, input)
	}

	for _, input := range []string{"", "0x", "0xghij", "not_hex", "-1", "+1", "-0x1", "0x 1", " 0x1", "0x1_0", "0x1ffffffffffffffff", "0X1"} {
		_, err := parseHexToUint64(input)
		assert.ErrorIs(t, err, ErrUpstream, "%q", input)
	}
}

func TestParseHexBigInt(t *testing.T) {
	got, err := parseHexBigInt("0xde0b6b3a7640000")
	require.NoError(t, err)
	assert.Equal(t, "1000000000000000000", got.String())

	max := "0x" + strings.Repeat("f", 64)
	got, err = parseHexBigInt(max)
	require.NoError(t, err)
	assert.Equal(t, 256, got.BitLen())

	got, err = parseHexBigInt("0x" + strings.Repeat("0", 70) + "1")
	require.NoError(t, err, "leading zeros are not significant digits")
	assert.Equal(t, "1", got.String())

	for _, input := range []string{"", "0x", "-0x1", "-1", "0xzz", "0x1" + strings.Repeat("0", 64)} {
		_, err := parseHexBigInt(input)
		assert.ErrorIs(t, err, ErrUpstream, "%q", input)
	}
}
