package blockchain

import (
	"fmt"
	"strings"
	"sync"

	"github.com/dvictor357/blockchain-gateway/pkg/config"
)

type ChainType string

const (
	ChainTypeEVM     ChainType = "evm"
	ChainTypeBitcoin ChainType = "bitcoin"
	ChainTypeOther   ChainType = "other"
)

type ChainInfo struct {
	Name        string
	DisplayName string
	NativeToken string
	Decimals    int
	ChainID     int64
	Type        ChainType
	DefaultRPC  string
	Explorer    string
}

var (
	chainRegistry = map[string]ChainInfo{
		// Bitcoin is still hardcoded as it's not EVM
		"bitcoin": {
			Name:        "bitcoin",
			DisplayName: "Bitcoin",
			NativeToken: "BTC",
			Decimals:    8,
			ChainID:     0,
			Type:        ChainTypeBitcoin,
			DefaultRPC:  "https://btc.getblock.io/mainnet",
			Explorer:    "https://www.blockchain.com/explorer",
		},
	}

	registryMutex sync.RWMutex
)

// LoadChainsFromConfig loads blockchain configurations and registers them
func LoadChainsFromConfig(chainsConfig config.ChainsConfig) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	// Load EVM chains from configuration
	for _, chainConfig := range chainsConfig.EVMChains {
		if !chainConfig.Enabled {
			continue // Skip disabled chains
		}

		chainType := ChainTypeEVM
		if chainConfig.Type == "bitcoin" {
			chainType = ChainTypeBitcoin
		} else if chainConfig.Type == "other" {
			chainType = ChainTypeOther
		}

		chainInfo := ChainInfo{
			Name:        strings.ToLower(chainConfig.Name),
			DisplayName: chainConfig.DisplayName,
			NativeToken: chainConfig.NativeToken,
			Decimals:    chainConfig.Decimals,
			ChainID:     chainConfig.ChainID,
			Type:        chainType,
			DefaultRPC:  chainConfig.RPCURL,
			Explorer:    chainConfig.Explorer,
		}

		// Register the chain (overwrite if exists)
		chainRegistry[chainInfo.Name] = chainInfo
	}

	return nil
}

// GetChainInfo returns metadata for the specified blockchain
func GetChainInfo(chain string) (ChainInfo, error) {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	chain = strings.ToLower(chain)
	info, exists := chainRegistry[chain]
	if !exists {
		return ChainInfo{}, fmt.Errorf("chain not supported: %s", chain)
	}

	return info, nil
}

// IsChainSupported checks if a blockchain is supported
func IsChainSupported(chain string) bool {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	_, exists := chainRegistry[strings.ToLower(chain)]
	return exists
}

// ListSupportedChains returns a list of all supported blockchain names
func ListSupportedChains() []string {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	chains := make([]string, 0, len(chainRegistry))
	for chain := range chainRegistry {
		chains = append(chains, chain)
	}
	return chains
}

// ListEVMChains returns a list of all supported EVM blockchain names
func ListEVMChains() []string {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	chains := make([]string, 0)
	for chain, info := range chainRegistry {
		if info.Type == ChainTypeEVM {
			chains = append(chains, chain)
		}
	}
	return chains
}

// RegisterChain adds a new blockchain to the registry
func RegisterChain(info ChainInfo) error {
	registryMutex.Lock()
	defer registryMutex.Unlock()

	name := strings.ToLower(info.Name)
	if _, exists := chainRegistry[name]; exists {
		return fmt.Errorf("chain already registered: %s", name)
	}

	chainRegistry[name] = info
	return nil
}

// GetDefaultRPCEndpoint returns the default RPC endpoint for a chain
func GetDefaultRPCEndpoint(chain string) (string, error) {
	info, err := GetChainInfo(chain)
	if err != nil {
		return "", err
	}
	return info.DefaultRPC, nil
}

// IsEVMCompatible checks if a chain is EVM compatible
func IsEVMCompatible(chain string) bool {
	info, err := GetChainInfo(chain)
	if err != nil {
		return false
	}
	return info.Type == ChainTypeEVM
}
