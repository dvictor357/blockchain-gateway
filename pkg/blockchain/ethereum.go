package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
)

// EthereumClient implements the Client interface for Ethereum using the base EVM client
type EthereumClient struct {
	*EVMClient
}

// NewEthereumClient creates a new Ethereum client
func NewEthereumClient(rpcURL string, httpClient *http.Client) *EthereumClient {
	return &EthereumClient{
		EVMClient: NewEVMClient("ethereum", rpcURL, httpClient),
	}
}

// Name returns the name of the blockchain
func (c *EthereumClient) Name() string {
	return "ethereum"
}

// Execute delegates to the base EVM client
func (c *EthereumClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	return c.EVMClient.Execute(ctx, method, params)
}

// GetBalance delegates to the base EVM client
func (c *EthereumClient) GetBalance(ctx context.Context, address string) (json.RawMessage, error) {
	return c.EVMClient.GetBalance(ctx, address)
}

// GetLatestBlock delegates to the base EVM client
func (c *EthereumClient) GetLatestBlock(ctx context.Context) (json.RawMessage, error) {
	return c.EVMClient.GetLatestBlock(ctx)
}

// GetTransaction delegates to the base EVM client
func (c *EthereumClient) GetTransaction(ctx context.Context, hash string) (json.RawMessage, error) {
	return c.EVMClient.GetTransaction(ctx, hash)
}

// GetGasPrice delegates to the base EVM client
func (c *EthereumClient) GetGasPrice(ctx context.Context) (json.RawMessage, error) {
	return c.EVMClient.GetGasPrice(ctx)
}

// GetTransactionCount delegates to the base EVM client
func (c *EthereumClient) GetTransactionCount(ctx context.Context, address string) (json.RawMessage, error) {
	return c.EVMClient.GetTransactionCount(ctx, address)
}
