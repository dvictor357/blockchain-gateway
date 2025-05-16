package blockchain

import (
	"fmt"
	"strings"
	"sync"
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
		"ethereum": {
			Name:        "ethereum",
			DisplayName: "Ethereum",
			NativeToken: "ETH",
			Decimals:    18,
			ChainID:     1,
			Type:        ChainTypeEVM,
			DefaultRPC:  "https://ethereum.publicnode.com",
			Explorer:    "https://etherscan.io",
		},
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
		"polygon": {
			Name:        "polygon",
			DisplayName: "Polygon",
			NativeToken: "POL",
			Decimals:    18,
			ChainID:     137,
			Type:        ChainTypeEVM,
			DefaultRPC:  "https://polygon-bor-rpc.publicnode.com",
			Explorer:    "https://polygonscan.com",
		},
	}

	registryMutex sync.RWMutex
)

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
