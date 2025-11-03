package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
)

// PolygonClient implements the Client interface for Polygon using the base EVM client
type PolygonClient struct {
	*EVMClient
}

// NewPolygonClient creates a new PolygonClient instance
func NewPolygonClient(rpcURL string, httpClient *http.Client) *PolygonClient {
	return &PolygonClient{
		EVMClient: NewEVMClient("polygon", rpcURL, httpClient),
	}
}

// Name returns the name of the blockchain
func (c *PolygonClient) Name() string {
	return "polygon"
}

// Execute delegates to the base EVM client
func (c *PolygonClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	return c.EVMClient.Execute(ctx, method, params)
}

// GetBalance delegates to the base EVM client
func (c *PolygonClient) GetBalance(ctx context.Context, address string) (json.RawMessage, error) {
	return c.EVMClient.GetBalance(ctx, address)
}

// GetLatestBlock delegates to the base EVM client
func (c *PolygonClient) GetLatestBlock(ctx context.Context) (json.RawMessage, error) {
	return c.EVMClient.GetLatestBlock(ctx)
}

// GetTransaction delegates to the base EVM client
func (c *PolygonClient) GetTransaction(ctx context.Context, hash string) (json.RawMessage, error) {
	return c.EVMClient.GetTransaction(ctx, hash)
}

// GetGasPrice delegates to the base EVM client
func (c *PolygonClient) GetGasPrice(ctx context.Context) (json.RawMessage, error) {
	return c.EVMClient.GetGasPrice(ctx)
}

// GetTransactionCount delegates to the base EVM client
func (c *PolygonClient) GetTransactionCount(ctx context.Context, address string) (json.RawMessage, error) {
	return c.EVMClient.GetTransactionCount(ctx, address)
}
