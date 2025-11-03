package blockchain

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

type Balance struct {
	Address    string   `json:"address"`
	Balance    *big.Int `json:"balance"`
	HexBalance string   `json:"hex_balance,omitempty"`
	Decimals   int      `json:"decimals"`
	Symbol     string   `json:"symbol,omitempty"`
	Chain      string   `json:"chain"`
}

// MarshalJSON implements custom JSON marshaling for Balance
func (b Balance) MarshalJSON() ([]byte, error) {
	type Alias Balance
	return json.Marshal(&struct {
		Balance string `json:"balance"`
		*Alias
	}{
		Balance: b.Balance.String(),
		Alias:   (*Alias)(&b),
	})
}

type BlockInfo struct {
	Number           uint64 `json:"number"`
	Hash             string `json:"hash"`
	ParentHash       string `json:"parent_hash,omitempty"`
	Timestamp        uint64 `json:"timestamp,omitempty"`
	TransactionCount int    `json:"transaction_count,omitempty"`
	Chain            string `json:"chain"`
}

type TransactionInfo struct {
	Hash        string   `json:"hash"`
	From        string   `json:"from,omitempty"`
	To          string   `json:"to,omitempty"`
	Value       *big.Int `json:"value,omitempty"`
	BlockNumber uint64   `json:"block_number,omitempty"`
	BlockHash   string   `json:"block_hash,omitempty"`
	Status      string   `json:"status,omitempty"`
	Chain       string   `json:"chain"`
}

// MarshalJSON implements custom JSON marshaling for TransactionInfo
func (t TransactionInfo) MarshalJSON() ([]byte, error) {
	type Alias TransactionInfo
	var value *string
	if t.Value != nil {
		v := t.Value.String()
		value = &v
	}
	return json.Marshal(&struct {
		Value *string `json:"value,omitempty"`
		*Alias
	}{
		Value: value,
		Alias: (*Alias)(&t),
	})
}

// GetBalance retrieves an account balance from the specified blockchain
// DEPRECATED: Moved to client_manager.go
/*
func (cm *ClientManager) GetBalance(ctx context.Context, chain, address string) (*Balance, error) {
	chain = strings.ToLower(chain)

	var balance *Balance

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// For EVM-compatible chains
		result, err := cm.Execute(ctx, chain, "eth_getBalance", []interface{}{address, "latest"})
		if err != nil {
			return nil, err
		}

		var hexBalance string
		if err := json.Unmarshal(result, &hexBalance); err != nil {
			return nil, fmt.Errorf("failed to unmarshal balance: %w", err)
		}

		// Convert hex to big.Int
		balanceInt := new(big.Int)
		balanceInt.SetString(strings.TrimPrefix(hexBalance, "0x"), 16)

		balance = &Balance{
			Address:    address,
			Balance:    balanceInt,
			HexBalance: hexBalance,
			Decimals:   chainInfo.Decimals,
			Symbol:     chainInfo.NativeToken,
			Chain:      chain,
		}

	case ChainTypeBitcoin:
		// Bitcoin has a different approach to addresses
		// First, we need to check if the server supports the account
		// For public nodes, we'll just return a fallback value or error

		result, err := cm.Execute(ctx, chain, "getbalance", []interface{}{})
		if err != nil {
			return nil, fmt.Errorf("%s RPC error: %w", chainInfo.DisplayName, err)
		}

		var btcBalance float64
		if err := json.Unmarshal(result, &btcBalance); err != nil {
			return nil, fmt.Errorf("failed to unmarshal %s balance: %w", chainInfo.NativeToken, err)
		}

		// Convert to smallest units
		factor := math.Pow10(chainInfo.Decimals)
		units := big.NewInt(int64(btcBalance * factor))

		balance = &Balance{
			Address:  address,
			Balance:  units,
			Decimals: chainInfo.Decimals,
			Symbol:   chainInfo.NativeToken,
			Chain:    chain,
		}

	default:
		return nil, fmt.Errorf("balance fetch not implemented for %s", chain)
	}

	return balance, nil
}
*/

// GetLatestBlock retrieves the latest block information for the specified blockchain
// DEPRECATED: Moved to client_manager.go
/*
func (cm *ClientManager) GetLatestBlock(ctx context.Context, chain string) (*BlockInfo, error) {
	chain = strings.ToLower(chain)

	var blockInfo *BlockInfo

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// Get latest block number
		result, err := cm.Execute(ctx, chain, "eth_blockNumber", []interface{}{})
		if err != nil {
			return nil, err
		}

		var hexBlock string
		if err := json.Unmarshal(result, &hexBlock); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block number: %w", err)
		}

		// Convert hex string to uint64
		var blockNumber uint64
		fmt.Sscanf(hexBlock, "0x%x", &blockNumber)

		// Get block details
		blockResult, err := cm.Execute(ctx, chain, "eth_getBlockByNumber",
			[]interface{}{hexBlock, false})
		if err != nil {
			return nil, err
		}

		var block map[string]interface{}
		if err := json.Unmarshal(blockResult, &block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block details: %w", err)
		}

		// Extract transaction count
		txCount := 0
		if txs, ok := block["transactions"].([]interface{}); ok {
			txCount = len(txs)
		}

		// Extract timestamp
		var timestamp uint64
		if ts, ok := block["timestamp"].(string); ok {
			fmt.Sscanf(ts, "0x%x", &timestamp)
		}

		blockInfo = &BlockInfo{
			Number:           blockNumber,
			Hash:             getString(block, "hash"),
			ParentHash:       getString(block, "parentHash"),
			Timestamp:        timestamp,
			TransactionCount: txCount,
			Chain:            chain,
		}

	case ChainTypeBitcoin:
		// Get block count
		result, err := cm.Execute(ctx, chain, "getblockcount", []interface{}{})
		if err != nil {
			return nil, err
		}

		var blockCount int64
		if err := json.Unmarshal(result, &blockCount); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block count: %w", err)
		}

		// Get block hash
		hashResult, err := cm.Execute(ctx, chain, "getblockhash", []interface{}{blockCount})
		if err != nil {
			return nil, err
		}

		var blockHash string
		if err := json.Unmarshal(hashResult, &blockHash); err != nil {
			return nil, fmt.Errorf("failed to unmarshal block hash: %w", err)
		}

		// Get block details
		blockResult, err := cm.Execute(ctx, chain, "getblock", []interface{}{blockHash})
		if err != nil {
			return nil, err
		}

		var block map[string]interface{}
		if err := json.Unmarshal(blockResult, &block); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BTC block: %w", err)
		}

		// Get transaction count
		txCount := 0
		if txs, ok := block["tx"].([]interface{}); ok {
			txCount = len(txs)
		}

		// Get timestamp
		timestamp := uint64(0)
		if ts, ok := block["time"].(float64); ok {
			timestamp = uint64(ts)
		}

		blockInfo = &BlockInfo{
			Number:           uint64(blockCount),
			Hash:             blockHash,
			ParentHash:       getString(block, "previousblockhash"),
			Timestamp:        timestamp,
			TransactionCount: txCount,
			Chain:            chain,
		}

	default:
		return nil, fmt.Errorf("get latest block not implemented for %s", chain)
	}

	return blockInfo, nil
}
*/

// GetTransaction retrieves transaction details for the specified transaction hash
// DEPRECATED: Moved to client_manager.go
/*
func (cm *ClientManager) GetTransaction(ctx context.Context, chain, txHash string) (*TransactionInfo, error) {
	chain = strings.ToLower(chain)

	var txInfo *TransactionInfo

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, err
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// Get transaction by hash
		result, err := cm.Execute(ctx, chain, "eth_getTransactionByHash", []interface{}{txHash})
		if err != nil {
			return nil, err
		}

		var tx map[string]interface{}
		if err := json.Unmarshal(result, &tx); err != nil {
			return nil, fmt.Errorf("failed to unmarshal transaction: %w", err)
		}

		// Check if transaction exists
		if tx["hash"] == nil {
			return nil, fmt.Errorf("transaction not found: %s", txHash)
		}

		// Get block number
		var blockNumber uint64
		if bn, ok := tx["blockNumber"].(string); ok && bn != "" {
			fmt.Sscanf(bn, "0x%x", &blockNumber)
		}

		// Get value as big.Int
		value := new(big.Int)
		if val, ok := tx["value"].(string); ok && val != "" {
			value.SetString(strings.TrimPrefix(val, "0x"), 16)
		}

		txInfo = &TransactionInfo{
			Hash:        getString(tx, "hash"),
			From:        getString(tx, "from"),
			To:          getString(tx, "to"),
			Value:       value,
			BlockNumber: blockNumber,
			BlockHash:   getString(tx, "blockHash"),
			Chain:       chain,
		}

		// Get transaction receipt for status
		receiptResult, err := cm.Execute(ctx, chain, "eth_getTransactionReceipt", []interface{}{txHash})
		if err == nil {
			var receipt map[string]interface{}
			if err := json.Unmarshal(receiptResult, &receipt); err == nil {
				if status, ok := receipt["status"].(string); ok {
					if status == "0x1" {
						txInfo.Status = "success"
					} else {
						txInfo.Status = "failed"
					}
				}
			}
		}

	case ChainTypeBitcoin:
		// Get transaction details
		result, err := cm.Execute(ctx, chain, "getrawtransaction", []interface{}{txHash, true})
		if err != nil {
			return nil, err
		}

		var tx map[string]interface{}
		if err := json.Unmarshal(result, &tx); err != nil {
			return nil, fmt.Errorf("failed to unmarshal BTC transaction: %w", err)
		}

		// Bitcoin doesn't have from/to in the same way as Ethereum
		// We need to process inputs and outputs to determine this

		// Calculate total value (simplified)
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
		satoshis := big.NewInt(int64(outputValue * 100000000))

		txInfo = &TransactionInfo{
			Hash:  txHash,
			Value: satoshis,
			Chain: chain,
		}

		// Get block info if available
		if blockHash, ok := tx["blockhash"].(string); ok {
			txInfo.BlockHash = blockHash

			// Try to get block info to determine block number
			blockResult, err := cm.Execute(ctx, chain, "getblock", []interface{}{blockHash})
			if err == nil {
				var block map[string]interface{}
				if err := json.Unmarshal(blockResult, &block); err == nil {
					if height, ok := block["height"].(float64); ok {
						txInfo.BlockNumber = uint64(height)
					}
				}
			}
		}

		// Bitcoin transactions are considered confirmed if they're in a block
		if txInfo.BlockHash != "" {
			txInfo.Status = "confirmed"
		} else {
			txInfo.Status = "pending"
		}

	default:
		return nil, fmt.Errorf("get transaction not implemented for %s", chain)
	}

	return txInfo, nil
}
*/

// GetGasPrice returns the current gas price for EVM-compatible chains
// DEPRECATED: Moved to client_manager.go
/*
func (cm *ClientManager) GetGasPrice(ctx context.Context, chain string) (*big.Int, error) {
	chain = strings.ToLower(chain)

	// Check if chain is EVM-compatible
	if IsEVMCompatible(chain) {
		result, err := cm.Execute(ctx, chain, "eth_gasPrice", []interface{}{})
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

	} else {
		return nil, fmt.Errorf("gas price not applicable for %s", chain)
	}
}
*/

// GetTransactionCount returns the number of transactions sent from an address
// DEPRECATED: Moved to client_manager.go
/*
func (cm *ClientManager) GetTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	chain = strings.ToLower(chain)

	// Check if chain is EVM-compatible
	if IsEVMCompatible(chain) {
		result, err := cm.Execute(ctx, chain, "eth_getTransactionCount", []interface{}{address, "latest"})
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

	} else {
		return 0, fmt.Errorf("get transaction count not implemented for %s", chain)
	}
}
*/

// Helper functions

// getString safely extracts a string value from a map
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// getUint64 safely extracts a uint64 value from a map
func getUint64(data map[string]interface{}, key string) uint64 {
	switch v := data[key].(type) {
	case float64:
		return uint64(v)
	case string:
		// Try to parse hex
		if strings.HasPrefix(v, "0x") {
			var val uint64
			fmt.Sscanf(v, "0x%x", &val)
			return val
		}
		// Try to parse decimal
		if val, err := strconv.ParseUint(v, 10, 64); err == nil {
			return val
		}
	case int64:
		return uint64(v)
	case uint64:
		return v
	}
	return 0
}
