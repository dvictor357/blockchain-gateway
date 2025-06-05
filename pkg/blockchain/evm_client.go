package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
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
	// Parse RPC URL
	parsedURL, err := url.Parse(c.rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse RPC URL: %w", err)
	}

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

	switch parsedURL.Scheme {
	case "http", "https":
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

		// Decode response
		if err := json.NewDecoder(resp.Body).Decode(&rpcResponse); err != nil {
			return nil, fmt.Errorf("failed to decode HTTP response: %w", err)
		}
	case "ws", "wss":
		// Establish WebSocket connection
		dialer := websocket.Dialer{
			Proxy:            http.ProxyFromEnvironment,
			HandshakeTimeout: 10 * time.Second, // Timeout for WebSocket handshake
		}
		conn, _, err := dialer.DialContext(ctx, c.rpcURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to WebSocket: %w", err)
		}
		defer conn.Close()

		// Set write deadline
		if err := conn.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return nil, fmt.Errorf("failed to set write deadline: %w", err)
		}

		// Send request over WebSocket
		if err := conn.WriteMessage(websocket.TextMessage, requestBody); err != nil {
			return nil, fmt.Errorf("failed to write message to WebSocket: %w", err)
		}

		// Set read deadline
		if err := conn.SetReadDeadline(time.Now().Add(10 * time.Second)); err != nil {
			return nil, fmt.Errorf("failed to set read deadline: %w", err)
		}

		// Read response
		_, responseBody, err := conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("failed to read message from WebSocket: %w", err)
		}

		// Unmarshal response
		if err := json.Unmarshal(responseBody, &rpcResponse); err != nil {
			return nil, fmt.Errorf("failed to unmarshal WebSocket response: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported RPC scheme: %s", parsedURL.Scheme)
	}

	// Check for RPC error
	if rpcResponse.Error != nil {
		log.Printf("RPC Error: Code %d, Message: %s, Data: %s\n", rpcResponse.Error.Code, rpcResponse.Error.Message, rpcResponse.Error.Data)
		return nil, fmt.Errorf("RPC error %d: %s", rpcResponse.Error.Code, rpcResponse.Error.Message)
	}

	// Check if response ID matches request ID
	if rpcResponse.ID != id {
		return nil, fmt.Errorf("response ID (%d) does not match request ID (%d)", rpcResponse.ID, id)
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

// GetBalance returns the balance of an address at a specific block
func (c *EVMClient) GetBalance(ctx context.Context, address string, blockNumber string) (string, error) {
	params := []interface{}{address, blockNumber}
	result, err := c.Execute(ctx, "eth_getBalance", params)
	if err != nil {
		return "", err
	}

	var balance string
	if err := json.Unmarshal(result, &balance); err != nil {
		return "", fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	return balance, nil
}

// GetTransactionCount returns the transaction count (nonce) for an address
func (c *EVMClient) GetTransactionCount(ctx context.Context, address string, blockNumber string) (uint64, error) {
	params := []interface{}{address, blockNumber}
	result, err := c.Execute(ctx, "eth_getTransactionCount", params)
	if err != nil {
		return 0, err
	}

	var countHex string
	if err := json.Unmarshal(result, &countHex); err != nil {
		return 0, fmt.Errorf("failed to unmarshal transaction count: %w", err)
	}

	count, err := parseHexToUint64(countHex)
	if err != nil {
		return 0, fmt.Errorf("failed to parse transaction count: %w", err)
	}

	return count, nil
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
	if strings.HasPrefix(hexStr, "0x") {
		hexStr = strings.TrimPrefix(hexStr, "0x")
	}

	return strconv.ParseUint(hexStr, 16, 64)
}
