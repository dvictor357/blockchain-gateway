package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// EVMClient represents a generic EVM-compatible blockchain client
type EVMClient struct {
	name       string
	rpcURL     string
	httpClient *http.Client
	requestID  uint64
}

// NewEVMClient creates a new EVM client instance
func NewEVMClient(name, rpcURL string, httpClient *http.Client) *EVMClient {
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	return &EVMClient{
		name:       name,
		rpcURL:     rpcURL,
		httpClient: httpClient,
		requestID:  0,
	}
}

// NewGenericEVMClient creates a new EVM client for any EVM-compatible chain
func NewGenericEVMClient(chainName, rpcURL string, httpClient *http.Client) *EVMClient {
	return NewEVMClient(chainName, rpcURL, httpClient)
}

// Name returns the name of the blockchain
func (c *EVMClient) Name() string {
	return c.name
}

// Execute executes an RPC method on the EVM blockchain
func (c *EVMClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	// Generate unique request ID
	id := atomic.AddUint64(&c.requestID, 1)

	// Create JSON-RPC request
	request := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
		"id":      id,
	}

	// Marshal request
	requestBody, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.rpcURL, strings.NewReader(string(requestBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

	// Execute HTTP request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var rpcResponse struct {
		JSONRPC string          `json:"jsonrpc"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Data    string `json:"data,omitempty"`
		} `json:"error"`
		ID uint64 `json:"id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rpcResponse); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Check for RPC error
	if rpcResponse.Error != nil {
		return nil, fmt.Errorf("RPC error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	return rpcResponse.Result, nil
}

// GetLatestBlockNumber returns the latest block number
func (c *EVMClient) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	result, err := c.Execute(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var blockNumberHex string
	if err := json.Unmarshal(result, &blockNumberHex); err != nil {
		return 0, fmt.Errorf("failed to unmarshal block number: %w", err)
	}

	blockNumber, err := parseHexToUint64(blockNumberHex)
	if err != nil {
		return 0, fmt.Errorf("failed to parse block number: %w", err)
	}

	return blockNumber, nil
}

// SendRawTransaction broadcasts a signed transaction
func (c *EVMClient) SendRawTransaction(ctx context.Context, signedTxData string) (string, error) {
	params := []interface{}{signedTxData}
	result, err := c.Execute(ctx, "eth_sendRawTransaction", params)
	if err != nil {
		return "", err
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", fmt.Errorf("failed to unmarshal transaction hash: %w", err)
	}

	return txHash, nil
}

// parseHexToUint64 converts a hex string to uint64
func parseHexToUint64(hexStr string) (uint64, error) {
	// Remove 0x prefix if present
	if after, ok := strings.CutPrefix(hexStr, "0x"); ok {
		hexStr = after
	}

	return strconv.ParseUint(hexStr, 16, 64)
}

// Interface implementations for Client interface

// GetBalance implements the Client interface
func (c *EVMClient) GetBalance(ctx context.Context, address string) (json.RawMessage, error) {
	return c.Execute(ctx, "eth_getBalance", []any{address, "latest"})
}

// GetLatestBlock implements the Client interface
func (c *EVMClient) GetLatestBlock(ctx context.Context) (json.RawMessage, error) {
	return c.Execute(ctx, "eth_blockNumber", []any{})
}

// GetTransaction implements the Client interface
func (c *EVMClient) GetTransaction(ctx context.Context, hash string) (json.RawMessage, error) {
	return c.Execute(ctx, "eth_getTransactionByHash", []any{hash})
}

// GetGasPrice implements the Client interface
func (c *EVMClient) GetGasPrice(ctx context.Context) (json.RawMessage, error) {
	return c.Execute(ctx, "eth_gasPrice", []any{})
}

// GetTransactionCount implements the Client interface
func (c *EVMClient) GetTransactionCount(ctx context.Context, address string) (json.RawMessage, error) {
	return c.Execute(ctx, "eth_getTransactionCount", []any{address, "latest"})
}
