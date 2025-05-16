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

// PolygonClient implements the Client interface for Polygon.
type PolygonClient struct {
	rpcURL     string
	httpClient *http.Client
	requestID  uint64
}

// NewPolygonClient creates a new PolygonClient instance.
func NewPolygonClient(rpcURL string, httpClient *http.Client) *PolygonClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &PolygonClient{
		rpcURL:     rpcURL,
		httpClient: httpClient,
		requestID:  0,
	}
}

// Name returns the name of the blockchain
func (c *PolygonClient) Name() string {
	return "polygon"
}

// Execute executes on Polygon JSON-RPC method
func (c *PolygonClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	// Generate a unique request ID
	id := atomic.AddUint64(&c.requestID, 1)

	// Create the JSON-RPC request
	request := RPCRequest{
		JSONRPC: "2.0",
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

	// Execute HTTP request
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

// GetLatestBlockNumber returns the latest block number
func (c *PolygonClient) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	result, err := c.Execute(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var hexBlock string
	if err := json.Unmarshal(result, &hexBlock); err != nil {
		return 0, fmt.Errorf("failed to unmarshal block number: %w", err)
	}

	// Convert hex string to uint64
	var blockNumber uint64
	fmt.Sscanf(hexBlock, "0x%x", &blockNumber)

	return blockNumber, nil
}

// GetBalance returns the balance of the specified address
func (c *PolygonClient) GetBalance(ctx context.Context, address string, blockNumber string) (string, error) {
	if blockNumber == "" {
		blockNumber = "latest"
	}

	result, err := c.Execute(ctx, "eth_getBalance", []interface{}{address, blockNumber})
	if err != nil {
		return "", err
	}

	var balance string
	if err := json.Unmarshal(result, &balance); err != nil {
		return "", fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	return balance, nil
}

// GetTransactionCount returns the transaction count for the specified address
func (c *PolygonClient) GetTransactionCount(ctx context.Context, address string, blockNumber string) (uint64, error) {
	if blockNumber == "" {
		blockNumber = "latest"
	}

	result, err := c.Execute(ctx, "eth_getTransactionCount", []interface{}{address, blockNumber})
	if err != nil {
		return 0, err
	}

	var hexCount string
	if err := json.Unmarshal(result, &hexCount); err != nil {
		return 0, fmt.Errorf("failed to unmarshal transaction count: %w", err)
	}

	// Convert hex string to uint64
	var count uint64
	fmt.Sscanf(hexCount, "0x%x", &count)

	return count, nil
}

// SendRawTransaction sends a signed transaction to the network
func (c *PolygonClient) SendRawTransaction(ctx context.Context, signedTxData string) (string, error) {
	// Ensure the transaction data is properly prefixed with "0x"
	if len(signedTxData) >= 2 && signedTxData[:2] != "0x" {
		signedTxData = "0x" + signedTxData
	}

	result, err := c.Execute(ctx, "eth_sendRawTransaction", []interface{}{signedTxData})
	if err != nil {
		return "", err
	}

	var txHash string
	if err := json.Unmarshal(result, &txHash); err != nil {
		return "", fmt.Errorf("failed to unmarshal transaction hash: %w", err)
	}

	return txHash, nil
}
