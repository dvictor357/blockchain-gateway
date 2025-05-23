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
