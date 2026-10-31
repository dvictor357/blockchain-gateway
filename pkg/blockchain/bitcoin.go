package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"sync/atomic"
)

// BitcoinClient implements the Client interface for Bitcoin
type BitcoinClient struct {
	rpcURL     string
	httpClient *http.Client
	requestID  uint64
}

// NewBitcoinClient creates a new Bitcoin client. The RPC URL may embed basic-auth credentials
// (http://user:password@host:8332); it is never included in returned errors.
func NewBitcoinClient(rpcURL string, httpClient *http.Client) *BitcoinClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &BitcoinClient{
		rpcURL:     rpcURL,
		httpClient: httpClient,
		requestID:  0,
	}
}

// Name returns the name of the blockchain
func (c *BitcoinClient) Name() string {
	return "bitcoin"
}

// Execute executes a Bitcoin JSON-RPC method. Bitcoin Core's legacy (1.0) response envelope
// is accepted: the jsonrpc member may be absent and result may be null next to an error.
func (c *BitcoinClient) Execute(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := atomic.AddUint64(&c.requestID, 1)
	return callJSONRPC(ctx, c.httpClient, c.rpcURL, "1.0", true, id, method, params)
}

// GetBlockCount returns the current block height
func (c *BitcoinClient) GetBlockCount(ctx context.Context) (int64, error) {
	result, err := c.Execute(ctx, "getblockcount", []any{})
	if err != nil {
		return 0, err
	}

	var blockCount int64
	if err := decodeResult(result, &blockCount, "block count"); err != nil {
		return 0, err
	}

	return blockCount, nil
}

// GetBlockHash returns the hash of the block at the specified height
func (c *BitcoinClient) GetBlockHash(ctx context.Context, height int64) (string, error) {
	result, err := c.Execute(ctx, "getblockhash", []any{height})
	if err != nil {
		return "", err
	}

	var blockHash string
	if err := decodeResult(result, &blockHash, "block hash"); err != nil {
		return "", err
	}

	return blockHash, nil
}

// GetBlock returns information about the block with the given hash
func (c *BitcoinClient) GetBlock(ctx context.Context, blockHash string) (map[string]any, error) {
	result, err := c.Execute(ctx, "getblock", []any{blockHash})
	if err != nil {
		return nil, err
	}

	var blockInfo map[string]any
	if err := decodeResult(result, &blockInfo, "block info"); err != nil {
		return nil, err
	}

	return blockInfo, nil
}

// GetRawTransaction returns the raw transaction data
func (c *BitcoinClient) GetRawTransaction(ctx context.Context, txid string, verbose bool) (interface{}, error) {
	result, err := c.Execute(ctx, "getrawtransaction", []interface{}{txid, verbose})
	if err != nil {
		return nil, err
	}

	if verbose {
		var txInfo map[string]interface{}
		if err := decodeResult(result, &txInfo, "transaction info"); err != nil {
			return nil, err
		}
		return txInfo, nil
	}

	var rawTx string
	if err := decodeResult(result, &rawTx, "raw transaction"); err != nil {
		return nil, err
	}

	return rawTx, nil
}

// SendRawTransaction broadcasts a signed transaction to the network
func (c *BitcoinClient) SendRawTransaction(ctx context.Context, signedTxHex string) (string, error) {
	result, err := c.Execute(ctx, "sendrawtransaction", []interface{}{signedTxHex})
	if err != nil {
		return "", err
	}

	var txid string
	if err := decodeResult(result, &txid, "transaction id"); err != nil {
		return "", err
	}

	return txid, nil
}
