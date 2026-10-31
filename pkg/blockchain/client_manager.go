package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
)

// Common errors
var (
	ErrChainNotSupported = errors.New("blockchain not supported")
	ErrInvalidRequest    = errors.New("invalid RPC request")
	ErrRPCTimeout        = errors.New("RPC request timeout")

	// ErrUpstream marks failures of the upstream node exchange that are not node-reported
	// JSON-RPC errors: transport errors, bad HTTP statuses, invalid envelopes or malformed results.
	ErrUpstream = errors.New("upstream blockchain node failure")
	// ErrOperationNotSupported marks operations this gateway cannot provide for a chain.
	ErrOperationNotSupported = errors.New("operation not supported")
	// ErrNotApplicable marks operations that do not apply to a chain type.
	ErrNotApplicable = errors.New("operation not applicable")
	// ErrNotFound marks a requested resource that does not exist on the chain.
	ErrNotFound = errors.New("resource not found")
)

// defaultRequestTimeout is the upstream HTTP client timeout when none is configured.
const defaultRequestTimeout = 30 * time.Second

// UnsupportedChainError reports which requested chain is unsupported.
type UnsupportedChainError struct {
	Chain string
}

func (e *UnsupportedChainError) Error() string {
	return fmt.Sprintf("%s: %s", ErrChainNotSupported, e.Chain)
}

func (e *UnsupportedChainError) Is(target error) bool {
	return target == ErrChainNotSupported
}

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

// RPCError represents a JSON-RPC error object reported by a node. Data may be any JSON value.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// NewClientManager creates a new client manager with configuration
func NewClientManager(appConfig *config.AppConfig) (*ClientManager, error) {
	timeout := appConfig.Server.RequestTimeout
	if timeout <= 0 {
		timeout = defaultRequestTimeout
	}

	manager := &ClientManager{
		clients: make(map[string]Client),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	// Load chains from configuration first
	if err := LoadChainsFromConfig(appConfig.Chains); err != nil {
		return nil, fmt.Errorf("failed to load chains from config: %w", err)
	}

	// Register clients for all configured chains
	if err := manager.registerClientsFromConfig(appConfig.Chains); err != nil {
		return nil, fmt.Errorf("failed to register clients from config: %w", err)
	}

	// Register Bitcoin client (not EVM, so it is not part of the EVM chain list)
	if err := manager.registerBitcoinClient(appConfig.Chains.BitcoinRPCURL); err != nil {
		return nil, fmt.Errorf("failed to register Bitcoin client: %w", err)
	}

	return manager, nil
}

// NewClientManagerLegacy creates a new client manager with legacy hardcoded clients
// This is kept for backward compatibility
func NewClientManagerLegacy() (*ClientManager, error) {
	manager := &ClientManager{
		clients: make(map[string]Client),
		httpClient: &http.Client{
			Timeout: defaultRequestTimeout,
		},
	}

	// Register default clients
	if err := manager.registerDefaultClients(); err != nil {
		return nil, fmt.Errorf("failed to register default clients: %w", err)
	}

	return manager, nil
}

// registerClientsFromConfig registers blockchain clients based on configuration
func (cm *ClientManager) registerClientsFromConfig(chainsConfig config.ChainsConfig) error {
	for _, chainConfig := range chainsConfig.EVMChains {
		if !chainConfig.Enabled {
			continue
		}

		client := NewGenericEVMClient(chainConfig.Name, chainConfig.RPCURL, cm.httpClient)

		if err := cm.RegisterClient(client); err != nil {
			return fmt.Errorf("failed to register client for %s: %w", chainConfig.Name, err)
		}
	}

	return nil
}

// registerBitcoinClient registers the Bitcoin client using rpcURL, or the registry default when empty.
func (cm *ClientManager) registerBitcoinClient(rpcURL string) error {
	if rpcURL == "" {
		chainInfo, err := GetChainInfo("bitcoin")
		if err != nil {
			return fmt.Errorf("failed to get Bitcoin chain info: %w", err)
		}
		rpcURL = chainInfo.DefaultRPC
	}

	return cm.RegisterClient(NewBitcoinClient(rpcURL, cm.httpClient))
}

// registerDefaultClients registers the default supported blockchain clients
func (cm *ClientManager) registerDefaultClients() error {
	chains := ListSupportedChains()

	for _, chain := range chains {
		chainInfo, err := GetChainInfo(chain)
		if err != nil {
			return fmt.Errorf("failed to get chain info for %s: %w", chain, err)
		}

		var client Client
		switch chainInfo.Type {
		case ChainTypeEVM:
			if chain == "ethereum" {
				client = NewEthereumClient(chainInfo.DefaultRPC, cm.httpClient)
			} else if chain == "polygon" {
				client = NewPolygonClient(chainInfo.DefaultRPC, cm.httpClient)
			} else {
				client = NewGenericEVMClient(chain, chainInfo.DefaultRPC, cm.httpClient)
			}
		case ChainTypeBitcoin:
			client = NewBitcoinClient(chainInfo.DefaultRPC, cm.httpClient)
		default:
			continue
		}

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
		return nil, &UnsupportedChainError{Chain: chain}
	}

	// Check if we have a client implementation
	client, exists := cm.clients[chain]
	if !exists {
		return nil, &UnsupportedChainError{Chain: chain}
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

// BatchExecute executes multiple RPC requests, potentially across different blockchains.
// Every chain is resolved before any upstream call, so an unsupported chain fails the whole batch
// with an error matching ErrChainNotSupported. Afterwards each request yields one response in
// order: node RPC errors keep their code and data, other failures carry a generic message, and
// once ctx is done the remaining requests are answered without contacting the node.
func (cm *ClientManager) BatchExecute(ctx context.Context, requests map[string][]RPCRequest) (map[string][]RPCResponse, error) {
	chains := make([]string, 0, len(requests))
	for chain := range requests {
		chains = append(chains, chain)
	}
	sort.Strings(chains)

	clients := make(map[string]Client, len(chains))
	for _, chain := range chains {
		client, err := cm.GetClient(chain)
		if err != nil {
			return nil, fmt.Errorf("chain %s: %w", chain, err)
		}
		clients[chain] = client
	}

	results := make(map[string][]RPCResponse, len(chains))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, chain := range chains {
		wg.Add(1)
		go func(chain string, client Client, reqs []RPCRequest) {
			defer wg.Done()

			responses := make([]RPCResponse, len(reqs))
			for i, req := range reqs {
				response := RPCResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
				}

				var err error
				var result json.RawMessage
				if err = ctx.Err(); err == nil {
					result, err = client.Execute(ctx, req.Method, req.Params)
				}

				if err != nil {
					response.Error = batchRPCError(err)
				} else {
					response.Result = result
				}

				responses[i] = response
			}

			mu.Lock()
			results[chain] = responses
			mu.Unlock()
		}(chain, clients[chain], requests[chain])
	}

	wg.Wait()

	return results, nil
}

// batchRPCError converts an execution error into the JSON-RPC error returned to the caller.
// Node-reported errors pass through unchanged; anything else gets a fixed message so transport
// details (URLs, upstream bodies) are never exposed.
func batchRPCError(err error) *RPCError {
	var rpcErr *RPCError
	switch {
	case errors.As(err, &rpcErr):
		return &RPCError{Code: rpcErr.Code, Message: rpcErr.Message, Data: rpcErr.Data}
	case IsTimeout(err):
		return &RPCError{Code: -32000, Message: "upstream request timed out"}
	case errors.Is(err, context.Canceled):
		return &RPCError{Code: -32000, Message: "request canceled"}
	default:
		return &RPCError{Code: -32000, Message: "upstream request failed"}
	}
}
