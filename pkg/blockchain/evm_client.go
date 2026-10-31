package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

// Execute executes an RPC method on the EVM blockchain. The response must be a valid
// JSON-RPC 2.0 envelope for this request; node-reported errors are returned as *RPCError.
func (c *EVMClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	id := atomic.AddUint64(&c.requestID, 1)
	return callJSONRPC(ctx, c.httpClient, c.rpcURL, "2.0", false, id, method, params)
}

// GetLatestBlockNumber returns the latest block number
func (c *EVMClient) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	result, err := c.Execute(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var blockNumberHex string
	if err := decodeResult(result, &blockNumberHex, "block number"); err != nil {
		return 0, err
	}

	return parseHexToUint64(blockNumberHex)
}

// GetBalance returns the balance of an address at a specific block ("latest" when blockNumber is empty)
func (c *EVMClient) GetBalance(ctx context.Context, address string, blockNumber string) (string, error) {
	params := []interface{}{address, normalizeBlockSelector(blockNumber)}
	result, err := c.Execute(ctx, "eth_getBalance", params)
	if err != nil {
		return "", err
	}

	var balance string
	if err := decodeResult(result, &balance, "balance"); err != nil {
		return "", err
	}

	return balance, nil
}

// GetTransactionCount returns the transaction count (nonce) for an address ("latest" when blockNumber is empty)
func (c *EVMClient) GetTransactionCount(ctx context.Context, address string, blockNumber string) (uint64, error) {
	params := []interface{}{address, normalizeBlockSelector(blockNumber)}
	result, err := c.Execute(ctx, "eth_getTransactionCount", params)
	if err != nil {
		return 0, err
	}

	var countHex string
	if err := decodeResult(result, &countHex, "transaction count"); err != nil {
		return 0, err
	}

	return parseHexToUint64(countHex)
}

// SendRawTransaction broadcasts a signed transaction; the data is sent with a 0x prefix.
func (c *EVMClient) SendRawTransaction(ctx context.Context, signedTxData string) (string, error) {
	signedTxData = normalizeHexPrefix(signedTxData)
	if signedTxData == "0x" {
		return "", fmt.Errorf("%w: signed transaction data is empty", ErrInvalidRequest)
	}

	params := []interface{}{signedTxData}
	result, err := c.Execute(ctx, "eth_sendRawTransaction", params)
	if err != nil {
		return "", err
	}

	var txHash string
	if err := decodeResult(result, &txHash, "transaction hash"); err != nil {
		return "", err
	}

	return txHash, nil
}

// normalizeBlockSelector defaults an empty block selector to "latest".
func normalizeBlockSelector(blockNumber string) string {
	blockNumber = strings.TrimSpace(blockNumber)
	if blockNumber == "" {
		return "latest"
	}
	return blockNumber
}

// normalizeHexPrefix trims whitespace and ensures a lowercase 0x prefix.
func normalizeHexPrefix(data string) string {
	data = strings.TrimSpace(data)
	if len(data) >= 2 && data[0] == '0' && (data[1] == 'x' || data[1] == 'X') {
		data = data[2:]
	}
	return "0x" + data
}
