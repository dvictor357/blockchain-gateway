package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// SecurityConfig controls HTTP API authentication and the RPC methods exposed by it.
type SecurityConfig struct {
	APIKey string
	// RPCMethodAllowlist replaces the built-in read-only defaults when nonempty.
	RPCMethodAllowlist []string
}

var rpcMethodName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func LoadSecurityConfig() SecurityConfig {
	cfg := SecurityConfig{APIKey: os.Getenv("API_KEY")}
	if raw := os.Getenv("RPC_METHOD_ALLOWLIST"); raw != "" {
		cfg.RPCMethodAllowlist = strings.Split(raw, ",")
		for i := range cfg.RPCMethodAllowlist {
			cfg.RPCMethodAllowlist[i] = strings.TrimSpace(cfg.RPCMethodAllowlist[i])
		}
	}
	return cfg
}

func (c SecurityConfig) Validate() error {
	if c.APIKey != "" && strings.TrimSpace(c.APIKey) == "" {
		return fmt.Errorf("API_KEY must not contain only whitespace")
	}
	for _, method := range c.RPCMethodAllowlist {
		if !rpcMethodName.MatchString(method) {
			return fmt.Errorf("RPC_METHOD_ALLOWLIST must contain nonempty exact RPC method names (letters, digits, underscore; starting with a letter or underscore)")
		}
	}
	return nil
}

// Finite read-only defaults deliberately exclude wallet/account, signing, node
// administration, debug, subscriptions, filters, and transaction submission RPCs.
var defaultRPCMethods = map[string]struct{}{
	"eth_chainId": {}, "eth_blockNumber": {}, "eth_getBalance": {},
	"eth_getBlockByNumber": {}, "eth_getBlockByHash": {},
	"eth_getTransactionByHash": {}, "eth_getTransactionReceipt": {},
	"eth_getTransactionCount": {}, "eth_gasPrice": {}, "eth_estimateGas": {},
	"eth_call": {}, "eth_getCode": {}, "eth_getStorageAt": {}, "eth_getLogs": {},
	"eth_getBlockTransactionCountByHash": {}, "eth_getBlockTransactionCountByNumber": {},
	"eth_getTransactionByBlockHashAndIndex": {}, "eth_getTransactionByBlockNumberAndIndex": {},
	"eth_feeHistory": {}, "eth_maxPriorityFeePerGas": {}, "eth_syncing": {},
	"net_version": {}, "web3_clientVersion": {},
	"getblockcount": {}, "getblockhash": {}, "getblock": {}, "getrawtransaction": {},
}
