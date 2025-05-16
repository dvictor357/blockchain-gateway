package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Common errors
var (
	ErrChainNotSupported = errors.New("blockchain not supported")
	ErrInvalidRequest    = errors.New("invalid RPC request")
	ErrRPCTimeout        = errors.New("RPC request timeout")
)

// ClientManager manages all blockchain clients
type ClientManager struct {
	clients    map[string]Client
	mu         sync.RWMutex
	httpClient *http.Client
}

// Client represents a blockchain RPC client
type Client interface {
	Name() string
	Execute(ctx context.Context, method string, params any) (json.RawMessage, error)
}

// RPCRequest represents a JSON-RPC request
type RPCRequest struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params"`
	ID      any    `json:"id"`
}

// RPCResponse represents a JSON-RPC response
type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
	ID      any             `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

// NewClientManager creates a new client manager
func NewClientManager() (*ClientManager, error) {
	manager := &ClientManager{
		clients: make(map[string]Client),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Register default clients
	if err := manager.registerDefaultClients(); err != nil {
		return nil, fmt.Errorf("failed to register default clients: %w", err)
	}

	return manager, nil
}

// registerDefaultClients registers the default supported blockchain clients
func (cm *ClientManager) registerDefaultClients() error {
	// Get all supported chains from registry
	chains := ListSupportedChains()

	for _, chain := range chains {
		chainInfo, err := GetChainInfo(chain)
		if err != nil {
			return fmt.Errorf("failed to get chain info for %s: %w", chain, err)
		}

		// Create appropriate client based on chain type
		var client Client
		switch chainInfo.Type {
		case ChainTypeEVM:
			if chain == "ethereum" {
				client = NewEthereumClient(chainInfo.DefaultRPC, cm.httpClient)
			} else if chain == "polygon" {
				client = NewPolygonClient(chainInfo.DefaultRPC, cm.httpClient)
			} else {
				// Generic EVM client for other EVM chains
				// This would be implemented as needed
				continue
			}
		case ChainTypeBitcoin:
			client = NewBitcoinClient(chainInfo.DefaultRPC, cm.httpClient)
		default:
			// Skip chains we don't have client implementations for yet
			continue
		}

		// Register the client
		if err := cm.RegisterClient(client); err != nil {
			return err
		}
	}

	return nil
}

// RegisterClient registers a new blockchain client
func (cm *ClientManager) RegisterClient(client Client) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	name := strings.ToLower(client.Name())
	if _, exists := cm.clients[name]; exists {
		return fmt.Errorf("client for %s already registered", name)
	}

	cm.clients[name] = client
	return nil
}

// GetClient returns a client for the specified blockchain
func (cm *ClientManager) GetClient(chain string) (Client, error) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	chain = strings.ToLower(chain)

	// Check if the chain is supported in our registry
	if !IsChainSupported(chain) {
		return nil, ErrChainNotSupported
	}

	// Check if we have a client implementation
	client, exists := cm.clients[chain]
	if !exists {
		return nil, ErrChainNotSupported
	}

	return client, nil
}

// ListChains returns a list of all supported blockchain names
func (cm *ClientManager) ListChains() []string {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	// Filter to only return chains that have a registered client
	availableChains := make([]string, 0, len(cm.clients))
	for chain := range cm.clients {
		availableChains = append(availableChains, chain)
	}

	return availableChains
}

// Execute executes an RPC method on the specified blockchain
func (cm *ClientManager) Execute(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return nil, err
	}

	return client.Execute(ctx, method, params)
}

// BatchExecute executes multiple RPC requests, potentially across different blockchains
func (cm *ClientManager) BatchExecute(ctx context.Context, requests map[string][]RPCRequest) (map[string][]RPCResponse, error) {
	results := make(map[string][]RPCResponse)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	for chain, chainRequests := range requests {
		wg.Add(1)
		go func(chain string, reqs []RPCRequest) {
			defer wg.Done()

			client, err := cm.GetClient(chain)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("chain %s: %w", chain, err))
				mu.Unlock()
				return
			}

			responses := make([]RPCResponse, len(reqs))
			for i, req := range reqs {
				result, err := client.Execute(ctx, req.Method, req.Params)

				// Create response
				response := RPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
				}

				if err != nil {
					// Handle error
					response.Error = &RPCError{
						Code:    -32000,
						Message: err.Error(),
					}
				} else {
					// Set result
					response.Result = result
				}

				responses[i] = response
			}

			mu.Lock()
			results[chain] = responses
			mu.Unlock()
		}(chain, chainRequests)
	}

	wg.Wait()

	if len(errors) > 0 {
		return results, fmt.Errorf("batch execution had %d errors: %v", len(errors), errors[0])
	}

	return results, nil
}
