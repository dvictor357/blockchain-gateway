package blockchain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
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
	GetBalance(ctx context.Context, address string) (json.RawMessage, error)
	GetLatestBlock(ctx context.Context) (json.RawMessage, error)
	GetTransaction(ctx context.Context, hash string) (json.RawMessage, error)
	GetGasPrice(ctx context.Context) (json.RawMessage, error)
	GetTransactionCount(ctx context.Context, address string) (json.RawMessage, error)
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

// NewClientManager creates a new client manager with configuration
func NewClientManager(appConfig *config.AppConfig) (*ClientManager, error) {
	manager := &ClientManager{
		clients: make(map[string]Client),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
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

	// Register Bitcoin client (still hardcoded)
	if err := manager.registerBitcoinClient(); err != nil {
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
			Timeout: 30 * time.Second,
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

// registerBitcoinClient registers the Bitcoin client
func (cm *ClientManager) registerBitcoinClient() error {
	chainInfo, err := GetChainInfo("bitcoin")
	if err != nil {
		return fmt.Errorf("failed to get Bitcoin chain info: %w", err)
	}

	client := NewBitcoinClient(chainInfo.DefaultRPC, cm.httpClient)

	return cm.RegisterClient(client)
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

// GetBalance retrieves the balance for an address
func (cm *ClientManager) GetBalance(ctx context.Context, chain, address string) (*Balance, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return nil, err
	}

	result, err := client.GetBalance(ctx, address)
	if err != nil {
		return nil, err
	}

	var hexBalance string
	if err := json.Unmarshal(result, &hexBalance); err != nil {
		return nil, fmt.Errorf("failed to unmarshal balance: %w", err)
	}

	// Get chain info for decimals and symbol
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	// Convert hex to big.Int
	balanceInt := new(big.Int)
	balanceInt.SetString(strings.TrimPrefix(hexBalance, "0x"), 16)

	return &Balance{
		Address:    address,
		Balance:    balanceInt,
		HexBalance: hexBalance,
		Decimals:   chainInfo.Decimals,
		Symbol:     chainInfo.NativeToken,
		Chain:      chain,
	}, nil
}

// GetLatestBlock retrieves the latest block
func (cm *ClientManager) GetLatestBlock(ctx context.Context, chain string) (*BlockInfo, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return nil, err
	}

	result, err := client.GetLatestBlock(ctx)
	if err != nil {
		return nil, err
	}

	var block map[string]interface{}
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block: %w", err)
	}

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	var blockNumber uint64
	var timestamp uint64
	var txCount int

	if chainInfo.Type == ChainTypeEVM {
		// Extract block number
		if bn, ok := block["number"].(string); ok && bn != "" {
			fmt.Sscanf(bn, "0x%x", &blockNumber)
		}

		// Extract timestamp
		if ts, ok := block["timestamp"].(string); ok && ts != "" {
			fmt.Sscanf(ts, "0x%x", &timestamp)
		}

		// Extract transaction count
		if txs, ok := block["transactions"].([]interface{}); ok {
			txCount = len(txs)
		}
	} else if chainInfo.Type == ChainTypeBitcoin {
		// For Bitcoin, we need to get block count first
		if height, ok := block["height"].(float64); ok {
			blockNumber = uint64(height)
		}

		if time, ok := block["time"].(float64); ok {
			timestamp = uint64(time)
		}

		if txs, ok := block["tx"].([]interface{}); ok {
			txCount = len(txs)
		}
	}

	return &BlockInfo{
		Number:           blockNumber,
		Hash:             getString(block, "hash"),
		ParentHash:       getString(block, "parentHash"),
		Timestamp:        timestamp,
		TransactionCount: txCount,
		Chain:            chain,
	}, nil
}

// GetTransaction retrieves a transaction
func (cm *ClientManager) GetTransaction(ctx context.Context, chain, hash string) (*TransactionInfo, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return nil, err
	}

	result, err := client.GetTransaction(ctx, hash)
	if err != nil {
		return nil, err
	}

	var tx map[string]interface{}
	if err := json.Unmarshal(result, &tx); err != nil {
		return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
	}

	// Check if transaction exists
	if tx["hash"] == nil {
		return nil, fmt.Errorf("transaction not found: %s", hash)
	}

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	var blockNumber uint64
	var value *big.Int

	if chainInfo.Type == ChainTypeEVM {
		// Get block number
		if bn, ok := tx["blockNumber"].(string); ok && bn != "" {
			fmt.Sscanf(bn, "0x%x", &blockNumber)
		}

		// Get value
		if val, ok := tx["value"].(string); ok && val != "" {
			value = new(big.Int)
			value.SetString(strings.TrimPrefix(val, "0x"), 16)
		}

		// Get transaction receipt for status
		receiptResult, err := cm.Execute(ctx, chain, "eth_getTransactionReceipt", []interface{}{hash})
		if err == nil {
			var receipt map[string]interface{}
			if err := json.Unmarshal(receiptResult, &receipt); err == nil {
				if status, ok := receipt["status"].(string); ok {
					if status == "0x1" {
						tx["status"] = "success"
					} else {
						tx["status"] = "failed"
					}
				}
			}
		}
	} else if chainInfo.Type == ChainTypeBitcoin {
		// For Bitcoin, calculate total value from outputs
		var outputValue float64
		if vouts, ok := tx["vout"].([]interface{}); ok {
			for _, v := range vouts {
				if vout, ok := v.(map[string]interface{}); ok {
					if val, ok := vout["value"].(float64); ok {
						outputValue += val
					}
				}
			}
		}

		// Convert BTC to satoshis
		value = big.NewInt(int64(outputValue * 100000000))

		// Get block info if available
		if blockHash, ok := tx["blockhash"].(string); ok {
			// Try to get block info to determine block number
			blockResult, err := cm.Execute(ctx, chain, "getblock", []interface{}{blockHash})
			if err == nil {
				var block map[string]interface{}
				if err := json.Unmarshal(blockResult, &block); err == nil {
					if height, ok := block["height"].(float64); ok {
						blockNumber = uint64(height)
					}
				}
			}
		}
	}

	return &TransactionInfo{
		Hash:        getString(tx, "hash"),
		From:        getString(tx, "from"),
		To:          getString(tx, "to"),
		Value:       value,
		BlockNumber: blockNumber,
		BlockHash:   getString(tx, "blockHash"),
		Status:      getString(tx, "status"),
		Chain:       chain,
	}, nil
}

// GetGasPrice retrieves the gas price
func (cm *ClientManager) GetGasPrice(ctx context.Context, chain string) (*big.Int, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return nil, err
	}

	result, err := client.GetGasPrice(ctx)
	if err != nil {
		return nil, err
	}

	var hexGasPrice string
	if err := json.Unmarshal(result, &hexGasPrice); err != nil {
		return nil, fmt.Errorf("failed to unmarshal gas price: %w", err)
	}

	// Convert hex to big.Int
	gasPrice := new(big.Int)
	gasPrice.SetString(strings.TrimPrefix(hexGasPrice, "0x"), 16)

	return gasPrice, nil
}

// GetTransactionCount retrieves the transaction count
func (cm *ClientManager) GetTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	client, err := cm.GetClient(chain)
	if err != nil {
		return 0, err
	}

	result, err := client.GetTransactionCount(ctx, address)
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
