package resilience

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
)

// ProtectedEVMClient wraps an EVM client with circuit breaker protection
type ProtectedEVMClient struct {
	name           string
	rpcURL         string
	httpClient     *http.Client
	requestID      uint64
	circuitBreaker *CircuitBreaker
	config         CircuitBreakerConfig
}

// ProtectedClientConfig holds configuration for protected clients
type ProtectedClientConfig struct {
	CircuitBreakerConfig CircuitBreakerConfig
	HTTPClient           *http.Client
}

// NewProtectedEVMClient creates a new protected EVM client
func NewProtectedEVMClient(name, rpcURL string, cfg ProtectedClientConfig) *ProtectedEVMClient {
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
			Timeout: 30 * time.Second,
		}
	}

	circuitBreakerConfig := cfg.CircuitBreakerConfig
	if circuitBreakerConfig == (CircuitBreakerConfig{}) {
		circuitBreakerConfig = DefaultCircuitBreakerConfig()
	}

	return &ProtectedEVMClient{
		name:           name,
		rpcURL:         rpcURL,
		httpClient:     httpClient,
		requestID:      0,
		circuitBreaker: NewCircuitBreaker(circuitBreakerConfig),
		config:         circuitBreakerConfig,
	}
}

// Execute executes an RPC method with circuit breaker protection
func (c *ProtectedEVMClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	operation := func() (interface{}, error) {
		return c.executeRPC(ctx, method, params)
	}

	result, err := c.circuitBreaker.Execute(operation)
	if err != nil {
		return nil, fmt.Errorf("%s RPC error (circuit breaker: %v): %w", c.name, c.circuitBreaker.GetState(), err)
	}

	return result.(json.RawMessage), nil
}

// executeRPC performs the actual RPC call
func (c *ProtectedEVMClient) executeRPC(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
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

	// Execute HTTP request with timeout
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

// GetCircuitBreakerState returns the current state of the circuit breaker
func (c *ProtectedEVMClient) GetCircuitBreakerState() State {
	return c.circuitBreaker.GetState()
}

// GetCircuitBreakerStats returns statistics about the circuit breaker
func (c *ProtectedEVMClient) GetCircuitBreakerStats() map[string]interface{} {
	return c.circuitBreaker.GetStats()
}

// ResetCircuitBreaker resets the circuit breaker
func (c *ProtectedEVMClient) ResetCircuitBreaker() {
	c.circuitBreaker.Reset()
}

// ProtectedClientManager wraps the ClientManager with circuit breaker protection
type ProtectedClientManager struct {
	clientManager      *blockchain.ClientManager
	circuitBreakers    map[string]*CircuitBreaker
	circuitBreakerPool *CircuitBreakerPool
	config             CircuitBreakerConfig
}

// NewProtectedClientManager creates a new protected client manager
func NewProtectedClientManager(clientManager *blockchain.ClientManager, cbConfig CircuitBreakerConfig) *ProtectedClientManager {
	pool := NewCircuitBreakerPool(cbConfig)

	return &ProtectedClientManager{
		clientManager:      clientManager,
		circuitBreakers:    make(map[string]*CircuitBreaker),
		circuitBreakerPool: pool,
		config:             cbConfig,
	}
}

// Execute executes an RPC method with circuit breaker protection
func (cm *ProtectedClientManager) Execute(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error) {
	operation := func() (interface{}, error) {
		return cm.clientManager.Execute(ctx, chain, method, params)
	}

	result, err := cm.circuitBreakerPool.Execute(chain, operation)
	if err != nil {
		return nil, fmt.Errorf("chain %s: %w", chain, err)
	}

	return result.(json.RawMessage), nil
}

// BatchExecute executes multiple RPC requests with circuit breaker protection
func (cm *ProtectedClientManager) BatchExecute(ctx context.Context, requests map[string][]blockchain.RPCRequest) (map[string][]blockchain.RPCResponse, error) {
	return cm.clientManager.BatchExecute(ctx, requests)
}

// GetClient gets a client for the specified blockchain
func (cm *ProtectedClientManager) GetClient(chain string) (blockchain.Client, error) {
	return cm.clientManager.GetClient(chain)
}

// ListChains returns a list of all supported blockchain names
func (cm *ProtectedClientManager) ListChains() []string {
	return cm.clientManager.ListChains()
}

// GetCircuitBreakerStats returns statistics for all circuit breakers
func (cm *ProtectedClientManager) GetCircuitBreakerStats() map[string]map[string]interface{} {
	return cm.circuitBreakerPool.GetStats()
}

// ResetCircuitBreaker resets the circuit breaker for a specific chain
func (cm *ProtectedClientManager) ResetCircuitBreaker(chain string) {
	cm.circuitBreakerPool.GetOrCreate(chain).Reset()
}

// RPCClient interface represents a blockchain RPC client
type RPCClient interface {
	Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error)
	Name() string
}

// Wrapper wraps an existing client with circuit breaker protection
func WrapWithCircuitBreaker(client RPCClient, cbConfig CircuitBreakerConfig) *WrappedClient {
	return &WrappedClient{
		client:         client,
		circuitBreaker: NewCircuitBreaker(cbConfig),
	}
}

// WrappedClient wraps an RPC client with circuit breaker protection
type WrappedClient struct {
	client         RPCClient
	circuitBreaker *CircuitBreaker
}

// Execute executes an RPC method with circuit breaker protection
func (w *WrappedClient) Execute(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	operation := func() (interface{}, error) {
		return w.client.Execute(ctx, method, params)
	}

	result, err := w.circuitBreaker.Execute(operation)
	if err != nil {
		return nil, fmt.Errorf("client %s (circuit breaker: %v): %w", w.client.Name(), w.circuitBreaker.GetState(), err)
	}

	return result.(json.RawMessage), nil
}

// Name returns the name of the client
func (w *WrappedClient) Name() string {
	return w.client.Name()
}

// GetCircuitBreakerState returns the current state of the circuit breaker
func (w *WrappedClient) GetCircuitBreakerState() State {
	return w.circuitBreaker.GetState()
}

// GetCircuitBreakerStats returns statistics about the circuit breaker
func (w *WrappedClient) GetCircuitBreakerStats() map[string]interface{} {
	return w.circuitBreaker.GetStats()
}

// ResetCircuitBreaker resets the circuit breaker
func (w *WrappedClient) ResetCircuitBreaker() {
	w.circuitBreaker.Reset()
}
