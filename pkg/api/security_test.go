package api

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func securityRequest(router http.Handler, method, target, body string, keys ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, strings.NewReader(body))
	for _, key := range keys {
		r.Header.Add("X-API-Key", key)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestRouterAPIKeyProtection(t *testing.T) {
	handler, manager, _ := setupTestHandler()
	manager.On("ListChains").Return([]string{"ethereum"})
	server := config.ServerConfig{Mode: "debug", RateLimit: 1000, SecurityConfig: config.SecurityConfig{APIKey: "exact-secret"}}
	router := NewRouter(handler, log.New(io.Discard, "", 0), server)
	for _, path := range []string{"/api/v1/chains", "/swagger", "/swagger/index.html", "/swagger/doc.json", "/debug/routes", "/not-found", "/health/", "/health/extra", "/api/v1/chains?api_key=exact-secret", "/api/v1/chains?X-API-Key=exact-secret"} {
		t.Run(path, func(t *testing.T) {
			for _, keys := range [][]string{nil, {"wrong"}, {"exact-secret", "exact-secret"}, {"wrong", "exact-secret"}, {"exact-secret, exact-secret"}} {
				w := securityRequest(router, http.MethodGet, path, "", keys...)
				require.Equal(t, http.StatusUnauthorized, w.Code, w.Body.String())
				assert.Equal(t, "UNAUTHORIZED", errorCode(t, w))
				assert.NotContains(t, w.Body.String(), "exact-secret")
			}
		})
	}
	for _, path := range []string{"/api/v1/chains", "/swagger/index.html", "/swagger/doc.json", "/debug/routes"} {
		w := securityRequest(router, http.MethodGet, path, "", "exact-secret")
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	for _, keys := range [][]string{nil, {"wrong"}, {"exact-secret", "wrong"}} {
		w := securityRequest(router, http.MethodGet, "/health", "", keys...)
		assert.Equal(t, http.StatusOK, w.Code)
	}
}

func TestRouterAuthenticationFailuresConsumeRateLimit(t *testing.T) {
	handler, _, _ := setupTestHandler()
	router := NewRouter(handler, log.New(io.Discard, "", 0), config.ServerConfig{RateLimit: 1, SecurityConfig: config.SecurityConfig{APIKey: "secret"}})
	assert.Equal(t, http.StatusUnauthorized, securityRequest(router, http.MethodGet, "/api/v1/chains", "").Code)
	assert.Equal(t, http.StatusTooManyRequests, securityRequest(router, http.MethodGet, "/api/v1/chains", "", "secret").Code)
}

func TestRouterDefaultRPCPolicyRejectsWritesAndMixedBatchesBeforeUpstream(t *testing.T) {
	evm, evmHits := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0x1"`} })
	btc, btcHits := startNode(t, true, func(string) fakeReply { return fakeReply{result: `1`} })
	handler := newUpstreamHandler(t, evm.URL, btc.URL)
	router := NewRouter(handler, log.New(io.Discard, "", 0), config.ServerConfig{RateLimit: 1000})
	for _, method := range []string{"eth_sendRawTransaction", "eth_sendTransaction", "eth_sign", "eth_signTransaction", "eth_accounts", "eth_requestAccounts", "personal_unlockAccount", "admin_peers", "debug_traceTransaction", "sendrawtransaction", "getbalance", "listwallets", "walletpassphrase", "ETH_blockNumber", "totally_unknown"} {
		chain := testEVMChain
		if method == "sendrawtransaction" || method == "getbalance" || method == "listwallets" || method == "walletpassphrase" {
			chain = "bitcoin"
		}
		t.Run(method, func(t *testing.T) {
			w := securityRequest(router, http.MethodPost, "/api/v1/chains/"+chain+"/query", fmt.Sprintf(`{"method":%q,"params":[],"id":1}`, method))
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Equal(t, "RPC_METHOD_NOT_ALLOWED", errorCode(t, w))
			batch := fmt.Sprintf(`{%q:[{"method":"eth_blockNumber","params":[],"id":1},{"method":%q,"params":[],"id":2}],"bitcoin":[{"method":"getblockcount","params":[],"id":3}]}`, testEVMChain, method)
			w = securityRequest(router, http.MethodPost, "/api/v1/batch", batch)
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Equal(t, "RPC_METHOD_NOT_ALLOWED", errorCode(t, w))
		})
	}
	assert.Zero(t, evmHits.Load())
	assert.Zero(t, btcHits.Load())
	for _, chain := range []string{testEVMChain, "bitcoin"} {
		method := "eth_blockNumber"
		if chain == "bitcoin" {
			method = "getblockcount"
		}
		w := securityRequest(router, http.MethodPost, "/api/v1/chains/"+chain+"/query", fmt.Sprintf(`{"method":%q,"params":[]}`, method))
		assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	}
	assert.EqualValues(t, 1, evmHits.Load())
	assert.EqualValues(t, 1, btcHits.Load())
}

func TestRouterExplicitRPCPolicyReplacesDefaultsAndOptsIntoWrites(t *testing.T) {
	evm, evmHits := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0xaccepted"`} })
	btc, btcHits := startNode(t, true, func(string) fakeReply { return fakeReply{result: `"accepted"`} })
	t.Setenv("API_KEY", "")
	t.Setenv("RPC_METHOD_ALLOWLIST", " eth_sendRawTransaction , sendrawtransaction ")
	security := config.LoadSecurityConfig()
	require.NoError(t, security.Validate())
	server := config.ServerConfig{RateLimit: 1000, SecurityConfig: security}
	handler := newUpstreamHandler(t, evm.URL, btc.URL)
	handler.apiConfig = config.NewAPIConfigFromServer(server)
	router := NewRouter(handler, log.New(io.Discard, "", 0), server)
	for _, tc := range []struct {
		chain, method string
		status        int
	}{
		{testEVMChain, "eth_sendRawTransaction", http.StatusOK},
		{"bitcoin", "sendrawtransaction", http.StatusOK},
		{testEVMChain, "eth_blockNumber", http.StatusForbidden},
		{"bitcoin", "getblockcount", http.StatusForbidden},
		{testEVMChain, "ETH_sendRawTransaction", http.StatusForbidden},
	} {
		w := securityRequest(router, http.MethodPost, "/api/v1/chains/"+tc.chain+"/query", fmt.Sprintf(`{"method":%q,"params":["0x01"]}`, tc.method))
		assert.Equal(t, tc.status, w.Code, w.Body.String())
	}
	w := securityRequest(router, http.MethodPost, "/api/v1/batch", fmt.Sprintf(`{%q:[{"method":"eth_sendRawTransaction","params":["0x01"]}],"bitcoin":[{"method":"sendrawtransaction","params":["01"]}]}`, testEVMChain))
	assert.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 2, evmHits.Load())
	assert.EqualValues(t, 2, btcHits.Load())
}

func TestRouterRESTHelpersCannotBypassRPCPolicy(t *testing.T) {
	evm, evmHits := startNode(t, false, func(string) fakeReply { return fakeReply{result: `"0x1"`} })
	btc, btcHits := startNode(t, true, func(string) fakeReply { return fakeReply{result: `1`} })
	hash := "0x" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		path    string
		allowed []string
	}{
		{"/chains/" + testEVMChain + "/address/" + testAddress + "/balance", []string{"eth_blockNumber"}},
		{"/chains/" + testEVMChain + "/gas-price", []string{"eth_blockNumber"}},
		{"/chains/" + testEVMChain + "/address/" + testAddress + "/nonce", []string{"eth_blockNumber"}},
		{"/chains/" + testEVMChain + "/block/latest", []string{"eth_blockNumber"}},
		{"/chains/" + testEVMChain + "/block/latest", []string{"eth_getBlockByNumber"}},
		{"/chains/" + testEVMChain + "/tx/" + hash, []string{"eth_getTransactionByHash"}},
		{"/chains/" + testEVMChain + "/tx/" + hash, []string{"eth_getTransactionReceipt"}},
		{"/chains/bitcoin/block/latest", []string{"getblockcount", "getblockhash"}},
		{"/chains/bitcoin/block/latest", []string{"getblockcount", "getblock"}},
		{"/chains/bitcoin/block/latest", []string{"getblockhash", "getblock"}},
		{"/chains/bitcoin/tx/" + strings.Repeat("a", 64), []string{"getrawtransaction"}},
	} {
		t.Run(tc.path+strings.Join(tc.allowed, ","), func(t *testing.T) {
			handler := newUpstreamHandler(t, evm.URL, btc.URL)
			server := config.ServerConfig{RateLimit: 1000, SecurityConfig: config.SecurityConfig{RPCMethodAllowlist: tc.allowed}}
			handler.apiConfig = config.NewAPIConfigFromServer(server)
			router := NewRouter(handler, log.New(io.Discard, "", 0), server)
			w := securityRequest(router, http.MethodGet, "/api/v1"+tc.path, "")
			require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
			assert.Equal(t, "RPC_METHOD_NOT_ALLOWED", errorCode(t, w))
		})
	}
	assert.Zero(t, evmHits.Load(), "preflight must precede even allowed helper calls")
	assert.Zero(t, btcHits.Load(), "optional enrichment must not bypass policy")
}

func TestRouterRestrictedPolicyAllowsPermittedRESTHelper(t *testing.T) {
	evm, hits := startNode(t, false, func(method string) fakeReply {
		assert.Equal(t, "eth_getBalance", method)
		return fakeReply{result: `"0x1"`}
	})
	handler := newUpstreamHandler(t, evm.URL, "")
	server := config.ServerConfig{RateLimit: 1000, SecurityConfig: config.SecurityConfig{
		RPCMethodAllowlist: []string{"eth_getBalance"},
	}}
	handler.apiConfig = config.NewAPIConfigFromServer(server)
	router := NewRouter(handler, log.New(io.Discard, "", 0), server)
	w := securityRequest(router, http.MethodGet, "/api/v1/chains/"+testEVMChain+"/address/"+testAddress+"/balance", "")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.EqualValues(t, 1, hits.Load())
	w = securityRequest(router, http.MethodGet, "/api/v1/chains/"+testEVMChain+"/gas-price", "")
	assert.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.EqualValues(t, 1, hits.Load())
}
