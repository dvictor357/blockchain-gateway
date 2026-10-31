package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitcoinClient_Execute_LegacyEnvelopes(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   string // expected result, when the call succeeds
	}{
		{name: "legacy success without jsonrpc", status: 200, body: `{"result":800000,"error":null,"id":1}`, want: `800000`},
		{name: "legacy null result with null error", status: 200, body: `{"result":null,"error":null,"id":1}`, want: `null`},
		{name: "jsonrpc 2.0 success", status: 200, body: `{"jsonrpc":"2.0","result":"abc","id":1}`, want: `"abc"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRawServer(t, tt.status, tt.body)

			result, err := NewBitcoinClient(server.URL, nil).Execute(context.Background(), "getblockcount", []any{})

			require.NoError(t, err)
			assert.Equal(t, tt.want, string(result))
		})
	}
}

func TestBitcoinClient_Execute_StructuredErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		code   int
		data   string
	}{
		{name: "null result next to error", status: 200, body: `{"result":null,"error":{"code":-5,"message":"No such tx"},"id":1}`, code: -5},
		{name: "Bitcoin Core HTTP 500 error envelope", status: 500, body: `{"result":null,"error":{"code":-8,"message":"bad","data":{"k":"v"}},"id":1}`, code: -8, data: `{"k":"v"}`},
		{name: "HTTP 404 method not found envelope", status: 404, body: `{"result":null,"error":{"code":-32601,"message":"Method not found"},"id":1}`, code: -32601},
		{name: "non-null result next to error is tolerated", status: 200, body: `{"result":"x","error":{"code":-1,"message":"m"},"id":1}`, code: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRawServer(t, tt.status, tt.body)

			_, err := NewBitcoinClient(server.URL, nil).Execute(context.Background(), "m", nil)

			var rpcErr *RPCError
			require.ErrorAs(t, err, &rpcErr)
			assert.Equal(t, tt.code, rpcErr.Code)
			assert.Equal(t, tt.data, string(rpcErr.Data))
		})
	}
}

func TestBitcoinClient_Execute_RejectsInvalidResponses(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
	}{
		{name: "HTTP 500 without an error envelope", status: 500, body: `<html>oops</html>`},
		{name: "HTTP 503 with a success body", status: 503, body: `{"result":1,"error":null,"id":1}`},
		{name: "HTTP 401", status: 401, body: ``},
		{name: "empty envelope", status: 200, body: `{}`},
		{name: "null error without any result", status: 200, body: `{"error":null,"id":1}`},
		{name: "invalid JSON", status: 200, body: `nope`},
		{name: "missing id", status: 200, body: `{"result":1,"error":null}`},
		{name: "null id", status: 200, body: `{"result":1,"error":null,"id":null}`},
		{name: "wrong id", status: 200, body: `{"result":1,"error":null,"id":3}`},
		{name: "error with null id", status: 200, body: `{"result":null,"error":{"code":-1,"message":"m"},"id":null}`},
		{name: "error without code", status: 200, body: `{"result":null,"error":{"message":"m"},"id":1}`},
		{name: "error with non-integer code", status: 200, body: `{"result":null,"error":{"code":"x","message":"m"},"id":1}`},
		{name: "error without message", status: 200, body: `{"result":null,"error":{"code":1},"id":1}`},
		{name: "trailing data", status: 200, body: `{"result":1,"error":null,"id":1} {}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newRawServer(t, tt.status, tt.body)

			_, err := NewBitcoinClient(server.URL, nil).Execute(context.Background(), "m", nil)

			require.Error(t, err)
			assert.ErrorIs(t, err, ErrUpstream)
			var rpcErr *RPCError
			assert.False(t, errors.As(err, &rpcErr))
		})
	}
}

func TestBitcoinClient_CredentialsInURLAuthenticateButNeverAppearInErrors(t *testing.T) {
	var user, password string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, password, _ = r.BasicAuth()
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, "internal failure")
	}))
	defer server.Close()
	authURL := strings.Replace(server.URL, "http://", "http://rpcuser:hunter2@", 1)

	_, err := NewBitcoinClient(authURL, nil).Execute(context.Background(), "getblockcount", []any{})

	assert.Equal(t, "rpcuser", user)
	assert.Equal(t, "hunter2", password)
	require.ErrorIs(t, err, ErrUpstream)
	assert.NotContains(t, err.Error(), "hunter2")
}

func TestBitcoinClient_SendsLegacyRequest(t *testing.T) {
	var got struct {
		JSONRPC string `json:"jsonrpc"`
		Method  string `json:"method"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &got)
		_, _ = io.WriteString(w, `{"result":800000,"error":null,"id":1}`)
	}))
	defer server.Close()

	count, err := NewBitcoinClient(server.URL, nil).GetBlockCount(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(800000), count)
	assert.Equal(t, "1.0", got.JSONRPC)
	assert.Equal(t, "getblockcount", got.Method)
}
