package blockchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

// nodeReply describes how a fake node answers one call. result and rpcError are raw JSON.
type nodeReply struct {
	result   string
	rpcError string
	status   int
}

type nodeCall struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	ID     json.RawMessage `json:"id"`
}

// newNode starts a local JSON-RPC node that echoes the request id. legacy selects Bitcoin Core's
// 1.0 envelope (no jsonrpc member, result:null next to an error) instead of JSON-RPC 2.0.
// The returned counter holds the number of requests received.
func newNode(t *testing.T, legacy bool, handle func(call nodeCall) nodeReply) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var call nodeCall
		if err := json.Unmarshal(body, &call); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		reply := handle(call)
		var envelope string
		switch {
		case legacy && reply.rpcError != "":
			envelope = fmt.Sprintf(`{"result":null,"error":%s,"id":%s}`, reply.rpcError, call.ID)
		case legacy:
			envelope = fmt.Sprintf(`{"result":%s,"error":null,"id":%s}`, reply.result, call.ID)
		case reply.rpcError != "":
			envelope = fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"error":%s}`, call.ID, reply.rpcError)
		default:
			envelope = fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"result":%s}`, call.ID, reply.result)
		}

		w.Header().Set("Content-Type", "application/json")
		if reply.status != 0 {
			w.WriteHeader(reply.status)
		}
		_, _ = io.WriteString(w, envelope)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

// newRawServer answers every request with a fixed status and body.
func newRawServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

// expired returns a deadline that has already passed, for deterministic timeout tests.
func expired() time.Time {
	return time.Now().Add(-time.Second)
}
