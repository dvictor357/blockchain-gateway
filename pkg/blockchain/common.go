package blockchain

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

const (
	// maxBTCNumberLen and maxBTCExponent bound provider-supplied decimal text before any
	// big-number arithmetic, so a short input such as "1e999999" cannot force huge allocations.
	maxBTCNumberLen = 64
	maxBTCExponent  = 32
	// maxSatoshi is Bitcoin's 21,000,000 BTC supply cap in satoshis.
	maxSatoshi = 2_100_000_000_000_000
	// satoshiDigits is the number of decimal places in a BTC amount.
	satoshiDigits = 8
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

// GetBalance retrieves an account balance from the specified blockchain.
// Bitcoin address balances are not available: there is no address index configured, and the
// node's wallet-wide getbalance must never be reported as an arbitrary address's balance.
func (cm *ClientManager) GetBalance(ctx context.Context, chain, address string) (*Balance, error) {
	chain = strings.ToLower(chain)

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, &UnsupportedChainError{Chain: chain}
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// For EVM-compatible chains
		result, err := cm.Execute(ctx, chain, "eth_getBalance", []interface{}{address, "latest"})
		if err != nil {
			return nil, err
		}

		var hexBalance string
		if err := decodeResult(result, &hexBalance, "balance"); err != nil {
			return nil, err
		}

		balanceInt, err := parseHexBigInt(hexBalance)
		if err != nil {
			return nil, err
		}

		return &Balance{
			Address:    address,
			Balance:    balanceInt,
			HexBalance: hexBalance,
			Decimals:   chainInfo.Decimals,
			Symbol:     chainInfo.NativeToken,
			Chain:      chain,
		}, nil

	case ChainTypeBitcoin:
		return nil, fmt.Errorf("%w: address balance lookup for %s requires an address index, which is not configured",
			ErrOperationNotSupported, chainInfo.DisplayName)

	default:
		return nil, fmt.Errorf("%w: balance fetch for %s", ErrOperationNotSupported, chain)
	}
}

// GetLatestBlock retrieves the latest block information for the specified blockchain
func (cm *ClientManager) GetLatestBlock(ctx context.Context, chain string) (*BlockInfo, error) {
	chain = strings.ToLower(chain)

	var blockInfo *BlockInfo

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, &UnsupportedChainError{Chain: chain}
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// Get latest block number
		result, err := cm.Execute(ctx, chain, "eth_blockNumber", []interface{}{})
		if err != nil {
			return nil, err
		}

		var hexBlock string
		if err := decodeResult(result, &hexBlock, "block number"); err != nil {
			return nil, err
		}

		blockNumber, err := parseHexToUint64(hexBlock)
		if err != nil {
			return nil, err
		}

		// Get block details
		blockResult, err := cm.Execute(ctx, chain, "eth_getBlockByNumber",
			[]interface{}{hexBlock, false})
		if err != nil {
			return nil, err
		}

		var block map[string]interface{}
		if err := decodeNullableResult(blockResult, &block, "block details"); err != nil {
			return nil, err
		}
		if block == nil {
			return nil, fmt.Errorf("%w: node returned no block for its latest block number", ErrUpstream)
		}

		// Extract transaction count
		txCount := 0
		if txs, ok := block["transactions"].([]interface{}); ok {
			txCount = len(txs)
		}

		timestamp, err := optionalHexUint64(block, "timestamp")
		if err != nil {
			return nil, err
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

		var blockCount uint64
		if err := decodeResult(result, &blockCount, "block count"); err != nil {
			return nil, err
		}

		// Get block hash
		hashResult, err := cm.Execute(ctx, chain, "getblockhash", []interface{}{blockCount})
		if err != nil {
			return nil, err
		}

		var blockHash string
		if err := decodeResult(hashResult, &blockHash, "block hash"); err != nil {
			return nil, err
		}

		// Get block details
		blockResult, err := cm.Execute(ctx, chain, "getblock", []interface{}{blockHash})
		if err != nil {
			return nil, err
		}

		var block struct {
			Time              uint64            `json:"time"`
			Tx                []json.RawMessage `json:"tx"`
			PreviousBlockHash string            `json:"previousblockhash"`
		}
		if err := decodeResult(blockResult, &block, "block details"); err != nil {
			return nil, err
		}

		blockInfo = &BlockInfo{
			Number:           blockCount,
			Hash:             blockHash,
			ParentHash:       block.PreviousBlockHash,
			Timestamp:        block.Time,
			TransactionCount: len(block.Tx),
			Chain:            chain,
		}

	default:
		return nil, fmt.Errorf("%w: latest block for %s", ErrOperationNotSupported, chain)
	}

	return blockInfo, nil
}

// GetTransaction retrieves transaction details for the specified transaction hash
func (cm *ClientManager) GetTransaction(ctx context.Context, chain, txHash string) (*TransactionInfo, error) {
	chain = strings.ToLower(chain)

	var txInfo *TransactionInfo

	// Get chain info
	chainInfo, err := GetChainInfo(chain)
	if err != nil {
		return nil, &UnsupportedChainError{Chain: chain}
	}

	switch chainInfo.Type {
	case ChainTypeEVM:
		// Get transaction by hash
		result, err := cm.Execute(ctx, chain, "eth_getTransactionByHash", []interface{}{txHash})
		if err != nil {
			return nil, err
		}

		var tx map[string]interface{}
		if err := decodeNullableResult(result, &tx, "transaction"); err != nil {
			return nil, err
		}

		// Check if transaction exists
		if tx["hash"] == nil {
			return nil, fmt.Errorf("%w: transaction %s", ErrNotFound, txHash)
		}

		// Pending transactions have no block number
		blockNumber, err := optionalHexUint64(tx, "blockNumber")
		if err != nil {
			return nil, err
		}

		// A real transaction always carries a value quantity ("0x0" is a valid zero).
		rawValue, ok := tx["value"].(string)
		if !ok {
			return nil, fmt.Errorf("%w: transaction value is missing or not a string", ErrUpstream)
		}
		value, err := parseHexBigInt(rawValue)
		if err != nil {
			return nil, err
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

		// The receipt only supplies the status; a failed lookup leaves it empty unless the request is over.
		receiptResult, err := cm.Execute(ctx, chain, "eth_getTransactionReceipt", []interface{}{txHash})
		if err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
		} else {
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

		var tx struct {
			BlockHash string `json:"blockhash"`
			Vout      []struct {
				Value json.RawMessage `json:"value"`
			} `json:"vout"`
		}
		if err := decodeResult(result, &tx, "transaction"); err != nil {
			return nil, err
		}
		if tx.Vout == nil {
			return nil, fmt.Errorf("%w: transaction has no outputs member", ErrUpstream)
		}

		// Bitcoin has no from/to; Value is the exact sum of all outputs in satoshis.
		satoshis := new(big.Int)
		for _, vout := range tx.Vout {
			amount, err := btcToSatoshi(vout.Value)
			if err != nil {
				return nil, err
			}
			satoshis.Add(satoshis, amount)
		}

		txInfo = &TransactionInfo{
			Hash:  txHash,
			Value: satoshis,
			Chain: chain,
		}

		// Get block info if available
		if tx.BlockHash != "" {
			txInfo.BlockHash = tx.BlockHash

			// Try to get block info to determine block number
			blockResult, err := cm.Execute(ctx, chain, "getblock", []interface{}{tx.BlockHash})
			if err != nil {
				if ctx.Err() != nil {
					return nil, err
				}
			} else {
				var block struct {
					Height *uint64 `json:"height"`
				}
				if err := json.Unmarshal(blockResult, &block); err == nil && block.Height != nil {
					txInfo.BlockNumber = *block.Height
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
		return nil, fmt.Errorf("%w: transaction lookup for %s", ErrOperationNotSupported, chain)
	}

	return txInfo, nil
}

// GetGasPrice returns the current gas price for EVM-compatible chains
func (cm *ClientManager) GetGasPrice(ctx context.Context, chain string) (*big.Int, error) {
	chain = strings.ToLower(chain)

	if !IsEVMCompatible(chain) {
		return nil, fmt.Errorf("%w: gas price for %s", ErrNotApplicable, chain)
	}

	result, err := cm.Execute(ctx, chain, "eth_gasPrice", []interface{}{})
	if err != nil {
		return nil, err
	}

	var hexGasPrice string
	if err := decodeResult(result, &hexGasPrice, "gas price"); err != nil {
		return nil, err
	}

	return parseHexBigInt(hexGasPrice)
}

// GetTransactionCount returns the number of transactions sent from an address
func (cm *ClientManager) GetTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	chain = strings.ToLower(chain)

	if !IsEVMCompatible(chain) {
		return 0, fmt.Errorf("%w: transaction count for %s", ErrNotApplicable, chain)
	}

	result, err := cm.Execute(ctx, chain, "eth_getTransactionCount", []interface{}{address, "latest"})
	if err != nil {
		return 0, err
	}

	var hexCount string
	if err := decodeResult(result, &hexCount, "transaction count"); err != nil {
		return 0, err
	}

	return parseHexToUint64(hexCount)
}

// Helper functions

// getString safely extracts a string value from a map
func getString(data map[string]interface{}, key string) string {
	if val, ok := data[key].(string); ok {
		return val
	}
	return ""
}

// optionalHexUint64 reads a hex quantity from a decoded object; a missing or null member is 0,
// but a present member that is not a well-formed quantity is an upstream error.
func optionalHexUint64(data map[string]interface{}, key string) (uint64, error) {
	switch v := data[key].(type) {
	case nil:
		return 0, nil
	case string:
		return parseHexToUint64(v)
	default:
		return 0, fmt.Errorf("%w: %s is not a hex quantity", ErrUpstream, key)
	}
}

// hexQuantityDigits validates an EVM hex quantity and returns its digits without the 0x prefix.
// The prefix is optional, but at least one hex digit is required; signs, whitespace and
// separators are rejected, and the value is bounded to 256 bits.
func hexQuantityDigits(value string) (string, error) {
	digits := strings.TrimPrefix(value, "0x")
	if digits == "" {
		return "", fmt.Errorf("%w: malformed hex quantity %q", ErrUpstream, snippet([]byte(value)))
	}
	for _, r := range digits {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return "", fmt.Errorf("%w: malformed hex quantity %q", ErrUpstream, snippet([]byte(value)))
		}
	}
	if significant := strings.TrimLeft(digits, "0"); len(significant) > 64 {
		return "", fmt.Errorf("%w: hex quantity exceeds 256 bits", ErrUpstream)
	}
	return digits, nil
}

// parseHexToUint64 converts a hex quantity to uint64
func parseHexToUint64(hexStr string) (uint64, error) {
	digits, err := hexQuantityDigits(hexStr)
	if err != nil {
		return 0, err
	}
	n, err := strconv.ParseUint(digits, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: hex quantity out of range for uint64", ErrUpstream)
	}
	return n, nil
}

// parseHexBigInt converts a hex quantity to a big.Int
func parseHexBigInt(hexStr string) (*big.Int, error) {
	digits, err := hexQuantityDigits(hexStr)
	if err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(digits, 16)
	if !ok {
		return nil, fmt.Errorf("%w: malformed hex quantity", ErrUpstream)
	}
	return n, nil
}

// btcToSatoshi converts a JSON number holding a BTC amount to exact satoshis without
// floating point. Decimal and scientific notation are supported. Negative, malformed,
// fractional-satoshi and over-supply values are rejected, and the text length and exponent
// are bounded before any big-number arithmetic.
func btcToSatoshi(raw json.RawMessage) (*big.Int, error) {
	text := string(raw)
	malformed := func(reason string) (*big.Int, error) {
		return nil, fmt.Errorf("%w: invalid BTC amount: %s", ErrUpstream, reason)
	}
	if text == "" || len(text) > maxBTCNumberLen {
		return malformed("missing or too long")
	}

	pos := 0
	digitsFrom := func() string {
		start := pos
		for pos < len(text) && text[pos] >= '0' && text[pos] <= '9' {
			pos++
		}
		return text[start:pos]
	}

	intDigits := digitsFrom()
	if intDigits == "" {
		return malformed("not a non-negative number")
	}
	fracDigits := ""
	if pos < len(text) && text[pos] == '.' {
		pos++
		if fracDigits = digitsFrom(); fracDigits == "" {
			return malformed("empty fraction")
		}
	}
	exponent := 0
	if pos < len(text) && (text[pos] == 'e' || text[pos] == 'E') {
		pos++
		sign := 1
		if pos < len(text) && (text[pos] == '+' || text[pos] == '-') {
			if text[pos] == '-' {
				sign = -1
			}
			pos++
		}
		expDigits := digitsFrom()
		if expDigits == "" || len(expDigits) > 3 {
			return malformed("bad exponent")
		}
		exponent, _ = strconv.Atoi(expDigits)
		exponent *= sign
		if exponent > maxBTCExponent || exponent < -maxBTCExponent {
			return malformed("exponent out of range")
		}
	}
	if pos != len(text) {
		return malformed("not a number")
	}

	// value = mantissa * 10^scale satoshis
	mantissa := strings.TrimLeft(intDigits+fracDigits, "0")
	if mantissa == "" {
		return new(big.Int), nil
	}
	scale := exponent - len(fracDigits) + satoshiDigits
	trimmed := strings.TrimRight(mantissa, "0")
	scale += len(mantissa) - len(trimmed)
	mantissa = trimmed

	if scale < 0 {
		return malformed("fractional satoshi")
	}
	if len(mantissa)+scale > 16 {
		return malformed("exceeds maximum supply")
	}

	satoshis, ok := new(big.Int).SetString(mantissa, 10)
	if !ok {
		return malformed("not a number")
	}
	satoshis.Mul(satoshis, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil))
	if satoshis.Cmp(big.NewInt(maxSatoshi)) > 0 {
		return malformed("exceeds maximum supply")
	}
	return satoshis, nil
}
