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
