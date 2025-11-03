package blockchain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
)

// BitcoinClient implements the Client interface for Bitcoin
type BitcoinClient struct {
	rpcURL     string
	httpClient *http.Client
	requestID  uint64
}

// NewBitcoinClient creates a new Bitcoin client
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

// Execute executes a Bitcoin JSON-RPC method
func (c *BitcoinClient) Execute(ctx context.Context, method string, params any) (json.RawMessage, error) {
	// Generate a unique request ID
	id := atomic.AddUint64(&c.requestID, 1)

	// Create the JSON-RPC request
	request := RPCRequest{
		JSONRPC: "1.0", // Bitcoin uses JSON-RPC 1.0
		Method:  method,
		Params:  params,
		ID:      id,
	}

	// Marshal the request body
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, bytes.NewReader(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	// Add basic auth if needed (many Bitcoin nodes require it)
	// TODO: Add authentication mechanism for Bitcoin nodes

	// Execute the request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check for HTTP error
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse the response
	var rpcResponse RPCResponse
	if err := json.Unmarshal(respBody, &rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Check for RPC error
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("RPC error: %d - %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	return rpcResponse.Result, nil
}

// GetBlockCount returns the current block height
func (c *BitcoinClient) GetBlockCount(ctx context.Context) (int64, error) {
	result, err := c.Execute(ctx, "getblockcount", []any{})
	if err != nil {
		return 0, err
	}

	var blockCount int64
	if err := json.Unmarshal(result, &blockCount); err != nil {
		return 0, fmt.Errorf("failed to unmarshal block count: %w", err)
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
	if err := json.Unmarshal(result, &blockHash); err != nil {
		return "", fmt.Errorf("failed to unmarshal block hash: %w", err)
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
	if err := json.Unmarshal(result, &blockInfo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block info: %w", err)
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
		if err := json.Unmarshal(result, &txInfo); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction info: %w", err)
		}
		return txInfo, nil
	}

	var rawTx string
	if err := json.Unmarshal(result, &rawTx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal raw transaction: %w", err)
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
	if err := json.Unmarshal(result, &txid); err != nil {
		return "", fmt.Errorf("failed to unmarshal transaction id: %w", err)
	}

	return txid, nil
}

// Interface implementations for Client interface

// GetBalance returns the balance for the default account (interface implementation)
func (c *BitcoinClient) GetBalance(ctx context.Context, address string) (json.RawMessage, error) {
	// Bitcoin doesn't use addresses the same way as Ethereum
	// We get the default account balance
	result, err := c.Execute(ctx, "getbalance", []interface{}{})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetLatestBlock returns the latest block height (interface implementation)
func (c *BitcoinClient) GetLatestBlock(ctx context.Context) (json.RawMessage, error) {
	blockHeight, err := c.GetBlockCount(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to JSON-RPC response format
	result, _ := json.Marshal(blockHeight)
	return json.RawMessage(result), nil
}

// GetTransaction returns the raw transaction (interface implementation)
func (c *BitcoinClient) GetTransaction(ctx context.Context, hash string) (json.RawMessage, error) {
	tx, err := c.GetRawTransaction(ctx, hash, true)
	if err != nil {
		return nil, err
	}

	// Convert to JSON-RPC response format
	result, _ := json.Marshal(tx)
	return json.RawMessage(result), nil
}

// GetGasPrice is not applicable for Bitcoin
func (c *BitcoinClient) GetGasPrice(ctx context.Context) (json.RawMessage, error) {
	return nil, fmt.Errorf("GetGasPrice not applicable for Bitcoin")
}

// GetTransactionCount returns the transaction count for an account
func (c *BitcoinClient) GetTransactionCount(ctx context.Context, address string) (json.RawMessage, error) {
	// Bitcoin doesn't have nonce like Ethereum, return 0
	result, _ := json.Marshal(0)
	return json.RawMessage(result), nil
}
