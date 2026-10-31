package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/dvictor357/blaze"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testEVMChain = "apitestevm"
	testAddress  = "0x742d35Cc6634C0532925a3b844Bc454e4438f44e"
)

// fakeReply is one answer from a fake node; body is the JSON-RPC member to embed.
type fakeReply struct {
	status int
	result string // raw JSON result
	err    string // raw JSON error object
	raw    string // sent verbatim instead of an envelope when set
}

// startNode runs a local node that echoes request ids. legacy selects Bitcoin Core's 1.0 envelope.
func startNode(t *testing.T, legacy bool, respond func(method string) fakeReply) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		body, _ := io.ReadAll(r.Body)
		var call struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
		}
		_ = json.Unmarshal(body, &call)

		reply := respond(call.Method)
		if reply.status != 0 {
			w.WriteHeader(reply.status)
		}
		switch {
		case reply.raw != "":
			_, _ = io.WriteString(w, reply.raw)
		case legacy && reply.err != "":
			_, _ = fmt.Fprintf(w, `{"result":null,"error":%s,"id":%s}`, reply.err, call.ID)
		case legacy:
			_, _ = fmt.Fprintf(w, `{"result":%s,"error":null,"id":%s}`, reply.result, call.ID)
		case reply.err != "":
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"error":%s}`, call.ID, reply.err)
		default:
			_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":%s,"result":%s}`, call.ID, reply.result)
		}
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

// newUpstreamHandler wires the production handler to a real ClientManager talking to local fake nodes.
func newUpstreamHandler(t *testing.T, evmURL, bitcoinURL string) *Handler {
	t.Helper()
	manager, err := blockchain.NewClientManager(&config.AppConfig{Chains: config.ChainsConfig{
		BitcoinRPCURL: bitcoinURL,
		EVMChains: []config.ChainConfig{{
			Name: testEVMChain, DisplayName: "API Test EVM", NativeToken: "TST", Decimals: 18,
			Type: "evm", RPCURL: evmURL, Enabled: true,
		}},
	}})
	require.NoError(t, err)
	return NewHandler(manager, log.New(io.Discard, "", 0), nil, config.NewAPIConfig())
}

func serveWithContext(ctx context.Context, method, route, target, body string, handler blaze.HandlerFunc) *httptest.ResponseRecorder {
	router := blaze.New()
	router.Handle(method, route, handler)
	request := httptest.NewRequest(method, target, strings.NewReader(body)).WithContext(ctx)
	writer := httptest.NewRecorder()
	router.ServeHTTP(writer, request)
	return writer
}

func TestUpstream_BitcoinAddressBalanceIsUnsupportedAndNeverQueriesTheWallet(t *testing.T) {
	btc, btcHits := startNode(t, true, func(string) fakeReply { return fakeReply{result: `12.5`} })
	handler := newUpstreamHandler(t, "http://127.0.0.1:1", btc.URL)

	w := serveHandler(http.MethodGet, "/chains/:chain/address/:address/balance",
		"/chains/bitcoin/address/1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa/balance", "", handler.GetBalance)

	assert.Equal(t, http.StatusNotImplemented, w.Code)
	assert.Equal(t, "OPERATION_NOT_SUPPORTED", errorCode(t, w))
	assert.NotContains(t, w.Body.String(), "12.5")
	assert.Zero(t, btcHits.Load(), "the wallet-wide balance must not be fetched")
}

func TestUpstream_FailuresAreClassifiedWithoutLeakingNodeDetails(t *testing.T) {
	tests := []struct {
		name       string
		reply      fakeReply
		wantStatus int
		wantCode   string
	}{
		{name: "node RPC error", reply: fakeReply{err: `{"code":-32000,"message":"leaky internal detail","data":"leaky data"}`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_RPC_ERROR"},
		{name: "HTTP 503 from node", reply: fakeReply{status: 503, raw: `leaky backend exploded`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_ERROR"},
		{name: "HTTP 500 with a success-shaped body", reply: fakeReply{status: 500, raw: `{"jsonrpc":"2.0","id":1,"result":"0x1"}`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_ERROR"},
		{name: "empty envelope", reply: fakeReply{raw: `{}`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_ERROR"},
		{name: "malformed balance quantity", reply: fakeReply{result: `"0xleaky"`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_ERROR"},
		{name: "null balance", reply: fakeReply{result: `null`}, wantStatus: http.StatusBadGateway, wantCode: "UPSTREAM_ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evm, _ := startNode(t, false, func(string) fakeReply { return tt.reply })
			handler := newUpstreamHandler(t, evm.URL+"/v2/leaky-key", "")

			w := serveHandler(http.MethodGet, "/chains/:chain/address/:address/balance",
				"/chains/"+testEVMChain+"/address/"+testAddress+"/balance", "", handler.GetBalance)

			assert.Equal(t, tt.wantStatus, w.Code)
			assert.Equal(t, tt.wantCode, errorCode(t, w))
			assert.NotContains(t, w.Body.String(), "leaky")
		})
	}
}

func TestUpstream_ValidBalanceIsParsedExactly(t *testing.T) {
	evm, _ := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0xde0b6b3a7640000"`} })
	handler := newUpstreamHandler(t, evm.URL, "")

	w := serveHandler(http.MethodGet, "/chains/:chain/address/:address/balance",
		"/chains/"+testEVMChain+"/address/"+testAddress+"/balance", "", handler.GetBalance)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"address":"`+testAddress+`","balance":"1000000000000000000","hex_balance":"0xde0b6b3a7640000","decimals":18,"symbol":"TST","chain":"`+testEVMChain+`"}`, w.Body.String())
}

func TestUpstream_RequestContextDecidesBetweenTimeoutAndCancellation(t *testing.T) {
	evm, hits := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0x1"`} })
	handler := newUpstreamHandler(t, evm.URL, "")
	target := "/chains/" + testEVMChain + "/address/" + testAddress + "/nonce"

	t.Run("expired deadline is a gateway timeout", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()

		w := serveWithContext(ctx, http.MethodGet, "/chains/:chain/address/:address/nonce", target, "", handler.GetTransactionCount)

		assert.Equal(t, http.StatusGatewayTimeout, w.Code)
		assert.Equal(t, "RPC_TIMEOUT", errorCode(t, w))
	})

	t.Run("client cancellation is not a gateway timeout", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		w := serveWithContext(ctx, http.MethodGet, "/chains/:chain/address/:address/nonce", target, "", handler.GetTransactionCount)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Equal(t, "INTERNAL_ERROR", errorCode(t, w))
	})

	assert.Zero(t, hits.Load(), "neither request should have reached the node")
}

func TestUpstream_BatchKeepsNodeErrorsAndHidesTransportDetails(t *testing.T) {
	evm, _ := startNode(t, false, func(method string) fakeReply {
		if method == "eth_call" {
			return fakeReply{err: `{"code":3,"message":"execution reverted","data":{"reason":"0x08c379a0","nested":[1,{"a":null}]}}`}
		}
		return fakeReply{result: `"0x10"`}
	})
	// A Bitcoin endpoint with a credential in its path that nothing listens on.
	deadBTC := httptest.NewServer(http.NotFoundHandler())
	deadURL := deadBTC.URL + "/secret-token"
	deadBTC.Close()
	handler := newUpstreamHandler(t, evm.URL, deadURL)

	body := `{"` + testEVMChain + `":[
		{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1},
		{"jsonrpc":"2.0","method":"eth_call","params":[],"id":2}],
		"bitcoin":[{"jsonrpc":"1.0","method":"getblockcount","params":[],"id":"a"}]}`
	w := serveHandler(http.MethodPost, "/batch", "/batch", body, handler.BatchQuery)

	require.Equal(t, http.StatusOK, w.Code)
	assert.NotContains(t, w.Body.String(), "secret-token")

	decoder := json.NewDecoder(w.Body)
	decoder.UseNumber()
	var results map[string][]blockchain.RPCResponse
	require.NoError(t, decoder.Decode(&results))

	require.Len(t, results[testEVMChain], 2)
	assert.JSONEq(t, `"0x10"`, string(results[testEVMChain][0].Result))
	require.NotNil(t, results[testEVMChain][1].Error)
	assert.Equal(t, 3, results[testEVMChain][1].Error.Code)
	assert.Equal(t, "execution reverted", results[testEVMChain][1].Error.Message)
	assert.JSONEq(t, `{"reason":"0x08c379a0","nested":[1,{"a":null}]}`, string(results[testEVMChain][1].Error.Data))

	require.Len(t, results["bitcoin"], 1)
	require.NotNil(t, results["bitcoin"][0].Error)
	assert.Equal(t, "a", results["bitcoin"][0].ID)
	assert.Nil(t, results["bitcoin"][0].Result)
}

func TestUpstream_BatchWithUnsupportedChainFailsBeforeAnyNodeCall(t *testing.T) {
	evm, hits := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0x1"`} })
	handler := newUpstreamHandler(t, evm.URL, "")

	body := `{"` + testEVMChain + `":[{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}],
		"solanaish":[{"jsonrpc":"2.0","method":"getSlot","params":[],"id":1}]}`
	w := serveHandler(http.MethodPost, "/batch", "/batch", body, handler.BatchQuery)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.JSONEq(t, `{"error":"Blockchain 'solanaish' is not supported","code":"CHAIN_NOT_SUPPORTED"}`, w.Body.String())
	assert.Zero(t, hits.Load(), "valid chains must not be queried when the batch is rejected")
}
