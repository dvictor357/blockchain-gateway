package blockchain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const managerTestChain = "managertestevm"

func newTestManager(t *testing.T, evmURL, bitcoinURL string) *ClientManager {
	t.Helper()
	manager, err := NewClientManager(&config.AppConfig{Chains: config.ChainsConfig{
		BitcoinRPCURL: bitcoinURL,
		EVMChains: []config.ChainConfig{{
			Name: managerTestChain, DisplayName: "Manager Test EVM", NativeToken: "TST", Decimals: 18,
			Type: "evm", RPCURL: evmURL, Enabled: true,
		}},
	}})
	require.NoError(t, err)
	return manager
}

// byMethod builds a node handler that answers from a method->result table (unknown methods get an RPC error).
func byMethod(results map[string]string) func(nodeCall) nodeReply {
	return func(call nodeCall) nodeReply {
		if result, ok := results[call.Method]; ok {
			return nodeReply{result: result}
		}
		return nodeReply{rpcError: `{"code":-32601,"message":"method not found"}`}
	}
}

func TestNewClientManager_UsesConfiguredBitcoinRPCURL(t *testing.T) {
	node, hits := newNode(t, true, byMethod(map[string]string{"getblockcount": "800000"}))
	manager := newTestManager(t, "http://127.0.0.1:1", node.URL)

	result, err := manager.Execute(context.Background(), "bitcoin", "getblockcount", []any{})

	require.NoError(t, err)
	assert.Equal(t, "800000", string(result))
	assert.EqualValues(t, 1, hits.Load())
}

func TestClientManager_BitcoinAddressBalanceIsUnsupported(t *testing.T) {
	node, hits := newNode(t, true, byMethod(map[string]string{"getbalance": "12.5"}))
	manager := newTestManager(t, "http://127.0.0.1:1", node.URL)

	balance, err := manager.GetBalance(context.Background(), "bitcoin", "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa")

	assert.ErrorIs(t, err, ErrOperationNotSupported)
	assert.Nil(t, balance)
	assert.Zero(t, hits.Load(), "the wallet-wide balance must never be requested or reported for an address")
}

func TestClientManager_EVMBalance(t *testing.T) {
	tests := []struct {
		name    string
		result  string
		want    string
		wantErr bool
	}{
		{name: "one ether", result: `"0xde0b6b3a7640000"`, want: "1000000000000000000"},
		{name: "zero", result: `"0x0"`, want: "0"},
		{name: "beyond uint64", result: `"0x1000000000000000000000000"`, want: "79228162514264337593543950336"},
		{name: "malformed", result: `"0xnothex"`, wantErr: true},
		{name: "prefix only", result: `"0x"`, wantErr: true},
		{name: "null", result: `null`, wantErr: true},
		{name: "number instead of string", result: `5`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node, _ := newNode(t, false, byMethod(map[string]string{"eth_getBalance": tt.result}))
			manager := newTestManager(t, node.URL, "")

			balance, err := manager.GetBalance(context.Background(), managerTestChain, "0x742d35Cc6634C0532925a3b844Bc454e4438f44e")

			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUpstream)
				assert.Nil(t, balance)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, balance.Balance.String())
		})
	}
}

func TestClientManager_BitcoinTransactionValueIsExact(t *testing.T) {
	const blockHash = "00000000000000000001abc"
	tx := func(vouts string) map[string]string {
		return map[string]string{
			"getrawtransaction": `{"blockhash":"` + blockHash + `","vout":[` + vouts + `]}`,
			"getblock":          `{"height":800000}`,
		}
	}

	t.Run("sum of outputs is exact", func(t *testing.T) {
		node, _ := newNode(t, true, byMethod(tx(`{"value":0.00000029,"n":0},{"value":0.1,"n":1},{"value":2e-1,"n":2}`)))
		manager := newTestManager(t, "http://127.0.0.1:1", node.URL)

		info, err := manager.GetTransaction(context.Background(), "bitcoin", strings.Repeat("a", 64))

		require.NoError(t, err)
		assert.Equal(t, "30000029", info.Value.String())
		assert.Equal(t, blockHash, info.BlockHash)
		assert.Equal(t, uint64(800000), info.BlockNumber)
		assert.Equal(t, "confirmed", info.Status)
	})

	t.Run("unconfirmed transaction is pending", func(t *testing.T) {
		node, _ := newNode(t, true, byMethod(map[string]string{"getrawtransaction": `{"vout":[{"value":1.5}]}`}))
		manager := newTestManager(t, "http://127.0.0.1:1", node.URL)

		info, err := manager.GetTransaction(context.Background(), "bitcoin", strings.Repeat("a", 64))

		require.NoError(t, err)
		assert.Equal(t, "150000000", info.Value.String())
		assert.Equal(t, "pending", info.Status)
	})

	malformed := map[string]string{
		"fractional satoshi output": `{"vout":[{"value":0.000000005}]}`,
		"negative output":           `{"vout":[{"value":-1}]}`,
		"string output":             `{"vout":[{"value":"0.1"}]}`,
		"missing output value":      `{"vout":[{"n":0}]}`,
		"output beyond supply":      `{"vout":[{"value":22000000}]}`,
		"no outputs member":         `{"blockhash":"abc"}`,
		"null transaction":          `null`,
		"transaction not an object": `"deadbeef"`,
	}
	for name, result := range malformed {
		t.Run(name, func(t *testing.T) {
			node, _ := newNode(t, true, byMethod(map[string]string{"getrawtransaction": result}))
			manager := newTestManager(t, "http://127.0.0.1:1", node.URL)

			info, err := manager.GetTransaction(context.Background(), "bitcoin", strings.Repeat("a", 64))

			assert.ErrorIs(t, err, ErrUpstream)
			assert.Nil(t, info)
		})
	}
}

func TestClientManager_EVMTransaction(t *testing.T) {
	const hash = "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71"
	tx := func(fields string) map[string]string {
		return map[string]string{
			"eth_getTransactionByHash":  `{"hash":"` + hash + `","from":"0xaa","to":"0xbb",` + fields + `}`,
			"eth_getTransactionReceipt": `{"status":"0x1"}`,
		}
	}

	t.Run("confirmed transaction", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(tx(`"value":"0xde0b6b3a7640000","blockNumber":"0x10","blockHash":"0xbh"`)))

		info, err := newTestManager(t, node.URL, "").GetTransaction(context.Background(), managerTestChain, hash)

		require.NoError(t, err)
		assert.Equal(t, "1000000000000000000", info.Value.String())
		assert.Equal(t, uint64(16), info.BlockNumber)
		assert.Equal(t, "success", info.Status)
	})

	t.Run("zero value transaction", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(tx(`"value":"0x0","blockNumber":"0x1"`)))

		info, err := newTestManager(t, node.URL, "").GetTransaction(context.Background(), managerTestChain, hash)

		require.NoError(t, err)
		assert.Equal(t, "0", info.Value.String())
	})

	t.Run("pending transaction has no block", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(tx(`"value":"0x1","blockNumber":null`)))

		info, err := newTestManager(t, node.URL, "").GetTransaction(context.Background(), managerTestChain, hash)

		require.NoError(t, err)
		assert.Zero(t, info.BlockNumber)
	})

	t.Run("unknown transaction is not found", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(map[string]string{"eth_getTransactionByHash": `null`}))

		info, err := newTestManager(t, node.URL, "").GetTransaction(context.Background(), managerTestChain, hash)

		assert.ErrorIs(t, err, ErrNotFound)
		assert.Nil(t, info)
	})

	malformed := map[string]string{
		"empty value":      `"value":"","blockNumber":"0x1"`,
		"missing value":    `"blockNumber":"0x1"`,
		"null value":       `"value":null,"blockNumber":"0x1"`,
		"non-hex value":    `"value":"not-hex","blockNumber":"0x1"`,
		"numeric value":    `"value":5,"blockNumber":"0x1"`,
		"malformed block":  `"value":"0x1","blockNumber":"0xzz"`,
		"non-string block": `"value":"0x1","blockNumber":7`,
		"negative value":   `"value":"-0x1","blockNumber":"0x1"`,
	}
	for name, fields := range malformed {
		t.Run(name, func(t *testing.T) {
			node, _ := newNode(t, false, byMethod(tx(fields)))

			info, err := newTestManager(t, node.URL, "").GetTransaction(context.Background(), managerTestChain, hash)

			assert.ErrorIs(t, err, ErrUpstream)
			assert.Nil(t, info)
		})
	}
}

func TestClientManager_LatestBlock(t *testing.T) {
	t.Run("EVM", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(map[string]string{
			"eth_blockNumber":      `"0x10"`,
			"eth_getBlockByNumber": `{"hash":"0xh","parentHash":"0xp","timestamp":"0x65","transactions":["0x1","0x2"]}`,
		}))

		info, err := newTestManager(t, node.URL, "").GetLatestBlock(context.Background(), managerTestChain)

		require.NoError(t, err)
		assert.Equal(t, BlockInfo{Number: 16, Hash: "0xh", ParentHash: "0xp", Timestamp: 101, TransactionCount: 2, Chain: managerTestChain}, *info)
	})

	t.Run("EVM node returns no block", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(map[string]string{"eth_blockNumber": `"0x10"`, "eth_getBlockByNumber": `null`}))

		info, err := newTestManager(t, node.URL, "").GetLatestBlock(context.Background(), managerTestChain)

		assert.ErrorIs(t, err, ErrUpstream)
		assert.Nil(t, info)
	})

	t.Run("EVM malformed timestamp", func(t *testing.T) {
		node, _ := newNode(t, false, byMethod(map[string]string{"eth_blockNumber": `"0x10"`, "eth_getBlockByNumber": `{"hash":"0xh","timestamp":"soon"}`}))

		_, err := newTestManager(t, node.URL, "").GetLatestBlock(context.Background(), managerTestChain)

		assert.ErrorIs(t, err, ErrUpstream)
	})

	t.Run("Bitcoin", func(t *testing.T) {
		node, _ := newNode(t, true, byMethod(map[string]string{
			"getblockcount": `800000`,
			"getblockhash":  `"00000000hash"`,
			"getblock":      `{"time":1700000000,"previousblockhash":"0000prev","tx":["a","b","c"]}`,
		}))

		info, err := newTestManager(t, "http://127.0.0.1:1", node.URL).GetLatestBlock(context.Background(), "bitcoin")

		require.NoError(t, err)
		assert.Equal(t, BlockInfo{Number: 800000, Hash: "00000000hash", ParentHash: "0000prev", Timestamp: 1700000000, TransactionCount: 3, Chain: "bitcoin"}, *info)
	})

	malformedBitcoin := map[string]map[string]string{
		"null block count":     {"getblockcount": `null`},
		"negative block count": {"getblockcount": `-1`},
		"fractional count":     {"getblockcount": `1.5`},
		"null block hash":      {"getblockcount": `5`, "getblockhash": `null`},
		"null block":           {"getblockcount": `5`, "getblockhash": `"h"`, "getblock": `null`},
		"negative time":        {"getblockcount": `5`, "getblockhash": `"h"`, "getblock": `{"time":-5}`},
	}
	for name, results := range malformedBitcoin {
		t.Run("Bitcoin "+name, func(t *testing.T) {
			node, _ := newNode(t, true, byMethod(results))

			info, err := newTestManager(t, "http://127.0.0.1:1", node.URL).GetLatestBlock(context.Background(), "bitcoin")

			assert.ErrorIs(t, err, ErrUpstream)
			assert.Nil(t, info)
		})
	}
}

func TestClientManager_GasPriceAndNonce(t *testing.T) {
	node, _ := newNode(t, false, byMethod(map[string]string{"eth_gasPrice": `"0x4a817c800"`, "eth_getTransactionCount": `"0x2a"`}))
	manager := newTestManager(t, node.URL, "")

	price, err := manager.GetGasPrice(context.Background(), managerTestChain)
	require.NoError(t, err)
	assert.Equal(t, "20000000000", price.String())

	count, err := manager.GetTransactionCount(context.Background(), managerTestChain, "0xabc")
	require.NoError(t, err)
	assert.Equal(t, uint64(42), count)

	t.Run("malformed quantities are upstream errors", func(t *testing.T) {
		bad, _ := newNode(t, false, byMethod(map[string]string{"eth_gasPrice": `"cheap"`, "eth_getTransactionCount": `""`}))
		badManager := newTestManager(t, bad.URL, "")

		_, err := badManager.GetGasPrice(context.Background(), managerTestChain)
		assert.ErrorIs(t, err, ErrUpstream)
		_, err = badManager.GetTransactionCount(context.Background(), managerTestChain, "0xabc")
		assert.ErrorIs(t, err, ErrUpstream)
	})

	t.Run("not applicable to Bitcoin", func(t *testing.T) {
		_, err := manager.GetGasPrice(context.Background(), "bitcoin")
		assert.ErrorIs(t, err, ErrNotApplicable)
		_, err = manager.GetTransactionCount(context.Background(), "bitcoin", "addr")
		assert.ErrorIs(t, err, ErrNotApplicable)
	})
}

func TestBitcoinClient_RequiredResultsRejectNull(t *testing.T) {
	node, _ := newNode(t, true, func(nodeCall) nodeReply { return nodeReply{result: `null`} })
	client := NewBitcoinClient(node.URL, nil)
	ctx := context.Background()

	_, err := client.GetBlockCount(ctx)
	assert.ErrorIs(t, err, ErrUpstream)
	_, err = client.GetBlockHash(ctx, 1)
	assert.ErrorIs(t, err, ErrUpstream)
	_, err = client.GetBlock(ctx, "h")
	assert.ErrorIs(t, err, ErrUpstream)
	_, err = client.GetRawTransaction(ctx, "t", true)
	assert.ErrorIs(t, err, ErrUpstream)
	_, err = client.SendRawTransaction(ctx, "00")
	assert.ErrorIs(t, err, ErrUpstream)
}

func TestClientManager_BatchExecute(t *testing.T) {
	t.Run("node errors keep code and data, transport failures are generic", func(t *testing.T) {
		evm, _ := newNode(t, false, func(call nodeCall) nodeReply {
			if call.Method == "eth_call" {
				return nodeReply{rpcError: `{"code":3,"message":"execution reverted","data":{"reason":"0x08c3","list":[1,null]}}`}
			}
			return nodeReply{result: `"0x1"`}
		})
		dead := httptest.NewServer(http.NotFoundHandler())
		deadURL := dead.URL + "/secret-token"
		dead.Close()
		manager := newTestManager(t, evm.URL, deadURL)

		results, err := manager.BatchExecute(context.Background(), map[string][]RPCRequest{
			managerTestChain: {{JSONRPC: "2.0", Method: "eth_blockNumber", ID: 1}, {JSONRPC: "2.0", Method: "eth_call", ID: "two"}},
			"bitcoin":        {{JSONRPC: "1.0", Method: "getblockcount", ID: 3}},
		})

		require.NoError(t, err)
		require.Len(t, results[managerTestChain], 2)
		assert.Equal(t, `"0x1"`, string(results[managerTestChain][0].Result))
		require.NotNil(t, results[managerTestChain][1].Error)
		assert.Equal(t, 3, results[managerTestChain][1].Error.Code)
		assert.Equal(t, "execution reverted", results[managerTestChain][1].Error.Message)
		assert.JSONEq(t, `{"reason":"0x08c3","list":[1,null]}`, string(results[managerTestChain][1].Error.Data))
		assert.Equal(t, "two", results[managerTestChain][1].ID)

		require.Len(t, results["bitcoin"], 1)
		require.NotNil(t, results["bitcoin"][0].Error)
		assert.NotContains(t, results["bitcoin"][0].Error.Message, "secret-token")
		assert.Empty(t, results["bitcoin"][0].Error.Data)
	})

	t.Run("an unsupported chain fails the batch before any node call", func(t *testing.T) {
		evm, hits := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: `"0x1"`} })
		manager := newTestManager(t, evm.URL, "")

		results, err := manager.BatchExecute(context.Background(), map[string][]RPCRequest{
			managerTestChain: {{Method: "eth_blockNumber", ID: 1}},
			"nosuchchain":    {{Method: "x", ID: 1}},
		})

		assert.ErrorIs(t, err, ErrChainNotSupported)
		var unsupported *UnsupportedChainError
		require.ErrorAs(t, err, &unsupported)
		assert.Equal(t, "nosuchchain", unsupported.Chain)
		assert.Nil(t, results)
		assert.Zero(t, hits.Load())
	})

	t.Run("cancellation stops further requests but keeps one response per request", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		evm, hits := newNode(t, false, func(nodeCall) nodeReply {
			cancel() // the caller goes away while the first request is being served
			return nodeReply{result: `"0x1"`}
		})
		manager := newTestManager(t, evm.URL, "")

		results, err := manager.BatchExecute(ctx, map[string][]RPCRequest{
			managerTestChain: {{Method: "m", ID: 1}, {Method: "m", ID: 2}, {Method: "m", ID: 3}},
		})

		require.NoError(t, err)
		require.Len(t, results[managerTestChain], 3)
		for _, response := range results[managerTestChain][1:] {
			require.NotNil(t, response.Error)
			assert.Equal(t, "request canceled", response.Error.Message)
			assert.Nil(t, response.Result)
		}
		assert.EqualValues(t, 1, hits.Load(), "requests after the cancellation must not reach the node")
	})

	t.Run("an expired deadline is reported as a timeout", func(t *testing.T) {
		evm, hits := newNode(t, false, func(nodeCall) nodeReply { return nodeReply{result: `"0x1"`} })
		manager := newTestManager(t, evm.URL, "")
		ctx, cancel := context.WithDeadline(context.Background(), expired())
		defer cancel()

		results, err := manager.BatchExecute(ctx, map[string][]RPCRequest{managerTestChain: {{Method: "m", ID: 1}}})

		require.NoError(t, err)
		require.NotNil(t, results[managerTestChain][0].Error)
		assert.Equal(t, "upstream request timed out", results[managerTestChain][0].Error.Message)
		assert.Zero(t, hits.Load())
	})
}
