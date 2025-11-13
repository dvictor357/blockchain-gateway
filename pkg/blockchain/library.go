package blockchain

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// LibraryConfig provides minimal configuration for library usage
type LibraryConfig struct {
	// HTTP client configuration
	Timeout time.Duration

	// Optional custom HTTP client
	HTTPClient *http.Client

	// Optional cache configuration
	CacheEnabled bool
	CacheTTL     time.Duration
}

// LibraryOption represents a configuration option for library usage
type LibraryOption func(*LibraryConfig)

// WithTimeout sets the HTTP timeout for blockchain clients
func WithTimeout(timeout time.Duration) LibraryOption {
	return func(c *LibraryConfig) {
		c.Timeout = timeout
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) LibraryOption {
	return func(c *LibraryConfig) {
		c.HTTPClient = client
	}
}

// WithCache enables caching with specified TTL
func WithCache(ttl time.Duration) LibraryOption {
	return func(c *LibraryConfig) {
		c.CacheEnabled = true
		c.CacheTTL = ttl
	}
}

// defaultLibraryConfig returns default configuration for library usage
func defaultLibraryConfig() *LibraryConfig {
	return &LibraryConfig{
		Timeout:      30 * time.Second,
		CacheEnabled: false,
		CacheTTL:     5 * time.Minute,
	}
}

// NewEVMClientLibrary creates a new EVM blockchain client for library usage
// This is a simplified constructor that doesn't require the full application config
func NewEVMClientLibrary(name, rpcURL string, opts ...LibraryOption) (Client, error) {
	config := defaultLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Create HTTP client
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	// Create and return the EVM client
	return NewGenericEVMClient(name, rpcURL, httpClient), nil
}

// NewBitcoinClientLibrary creates a new Bitcoin blockchain client for library usage
func NewBitcoinClientLibrary(rpcURL string, opts ...LibraryOption) (Client, error) {
	config := defaultLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Create HTTP client
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	// Create and return the Bitcoin client
	return NewBitcoinClient(rpcURL, httpClient), nil
}

// NewClientManagerLibrary creates a new client manager for library usage
// This is a simplified version that doesn't require the full application config
func NewClientManagerLibrary(opts ...LibraryOption) (*ClientManager, error) {
	config := defaultLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	// Create HTTP client
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	manager := &ClientManager{
		clients:    make(map[string]Client),
		httpClient: httpClient,
		cache:      nil, // Cache is optional for library usage
		enabled:    config.CacheEnabled,
	}

	return manager, nil
}

// SimpleClientManager provides a simplified interface for common blockchain operations
type SimpleClientManager struct {
	*ClientManager
}

// NewSimpleClientManager creates a simple client manager with common chains pre-configured
func NewSimpleClientManager(opts ...LibraryOption) (*SimpleClientManager, error) {
	config := defaultLibraryConfig()
	for _, opt := range opts {
		opt(config)
	}

	manager, err := NewClientManagerLibrary(opts...)
	if err != nil {
		return nil, err
	}

	// Add common EVM chains
	commonChains := map[string]string{
		"ethereum": "https://ethereum.publicnode.com",
		"polygon":  "https://polygon-bor-rpc.publicnode.com",
		"bsc":      "https://bsc-dataseed.binance.org",
		"arbitrum": "https://arb1.arbitrum.io/rpc",
		"optimism": "https://mainnet.optimism.io",
		"base":     "https://mainnet.base.org",
	}

	for name, rpcURL := range commonChains {
		client, err := NewEVMClientLibrary(name, rpcURL, opts...)
		if err == nil {
			manager.RegisterClient(client)
		}
	}

	// Add Bitcoin
	bitcoinClient, err := NewBitcoinClientLibrary("https://btc.getblock.io/mainnet", opts...)
	if err == nil {
		manager.RegisterClient(bitcoinClient)
	}

	return &SimpleClientManager{ClientManager: manager}, nil
}

// QuickQuery performs a quick RPC query without needing to manage the client directly
func (scm *SimpleClientManager) QuickQuery(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error) {
	return scm.Execute(ctx, chain, method, params)
}

// QuickBalance retrieves balance for an address
func (scm *SimpleClientManager) QuickBalance(ctx context.Context, chain, address string) (*Balance, error) {
	return scm.GetBalance(ctx, chain, address)
}

// QuickLatestBlock retrieves the latest block
func (scm *SimpleClientManager) QuickLatestBlock(ctx context.Context, chain string) (*BlockInfo, error) {
	return scm.GetLatestBlock(ctx, chain)
}

// QuickTransaction retrieves transaction details
func (scm *SimpleClientManager) QuickTransaction(ctx context.Context, chain, hash string) (*TransactionInfo, error) {
	return scm.GetTransaction(ctx, chain, hash)
}

// QuickGasPrice retrieves current gas price
func (scm *SimpleClientManager) QuickGasPrice(ctx context.Context, chain string) (json.RawMessage, error) {
	client, err := scm.GetClient(chain)
	if err != nil {
		return nil, err
	}
	return client.GetGasPrice(ctx)
}

// QuickTransactionCount retrieves transaction count (nonce) for an address
func (scm *SimpleClientManager) QuickTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	return scm.GetTransactionCount(ctx, chain, address)
}
