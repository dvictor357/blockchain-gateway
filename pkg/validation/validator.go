package validation

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
)

// Validator provides validation methods for API inputs
type Validator struct{}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	return &Validator{}
}

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validation error for field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}

	var messages []string
	for _, err := range e {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// Ethereum address regex pattern
var ethereumAddressRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{40}$`)

// Bitcoin address regex patterns
var bitcoinAddressRegex = regexp.MustCompile(`^[13][a-km-zA-HJ-NP-Z1-9]{25,34}$|^bc1[a-z0-9]{39,59}$`)

// Transaction hash regex pattern
var transactionHashRegex = regexp.MustCompile(`^0x[a-fA-F0-9]{64}$`)

// Bitcoin transaction hashes are 64 hex characters without a 0x prefix
var bitcoinTransactionHashRegex = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)

// Hash format for chains of unknown type: 64 hex characters, optionally 0x-prefixed
var genericTransactionHashRegex = regexp.MustCompile(`^(0x)?[a-fA-F0-9]{64}$`)

var chainNameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

var rpcMethodRegex = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// chainType returns the registry type of a chain. Ethereum and polygon are treated as EVM
// chains even before the registry has been loaded from configuration.
func chainType(chain string) blockchain.ChainType {
	if info, err := blockchain.GetChainInfo(chain); err == nil {
		return info.Type
	}
	switch chain {
	case "ethereum", "polygon":
		return blockchain.ChainTypeEVM
	}
	return ""
}

// ValidateChainName validates a blockchain name
func (v *Validator) ValidateChainName(chain string) ValidationError {
	if chain == "" {
		return ValidationError{Field: "chain", Message: "chain name is required"}
	}

	// Normalize to lowercase
	chain = strings.ToLower(chain)

	// Check if it's a valid chain name format
	if !chainNameRegex.MatchString(chain) {
		return ValidationError{Field: "chain", Message: "invalid chain name format"}
	}

	return ValidationError{}
}

// ValidateAddress validates a blockchain address based on the chain type
func (v *Validator) ValidateAddress(address, chain string) ValidationError {
	if address == "" {
		return ValidationError{Field: "address", Message: "address is required"}
	}

	chain = strings.ToLower(chain)

	switch chainType(chain) {
	case blockchain.ChainTypeEVM:
		if !ethereumAddressRegex.MatchString(address) {
			return ValidationError{Field: "address", Message: "invalid Ethereum/EVM address format"}
		}
	case blockchain.ChainTypeBitcoin:
		if !bitcoinAddressRegex.MatchString(address) {
			return ValidationError{Field: "address", Message: "invalid Bitcoin address format"}
		}
	default:
		// For unknown chains, just check it's not empty and has reasonable length
		if len(address) < 10 || len(address) > 100 {
			return ValidationError{Field: "address", Message: "address length must be between 10 and 100 characters"}
		}
	}

	return ValidationError{}
}

// ValidateTransactionHash validates a transaction hash
func (v *Validator) ValidateTransactionHash(hash, chain string) ValidationError {
	if hash == "" {
		return ValidationError{Field: "hash", Message: "transaction hash is required"}
	}

	chain = strings.ToLower(chain)

	switch chainType(chain) {
	case blockchain.ChainTypeEVM:
		if !transactionHashRegex.MatchString(hash) {
			return ValidationError{Field: "hash", Message: "invalid Ethereum/EVM transaction hash format"}
		}
	case blockchain.ChainTypeBitcoin:
		// Bitcoin transaction hashes are also 64 character hex strings, but without 0x prefix
		if !bitcoinTransactionHashRegex.MatchString(hash) {
			return ValidationError{Field: "hash", Message: "invalid Bitcoin transaction hash format"}
		}
	default:
		// For unknown chains, basic hex validation
		if !genericTransactionHashRegex.MatchString(hash) {
			return ValidationError{Field: "hash", Message: "invalid transaction hash format"}
		}
	}

	return ValidationError{}
}

// ValidatePaginationParams validates pagination parameters
func (v *Validator) ValidatePaginationParams(limit, offset int) ValidationErrors {
	var errors ValidationErrors

	if limit < 1 {
		errors = append(errors, ValidationError{Field: "limit", Message: "limit must be greater than 0"})
	}

	if limit > 100 {
		errors = append(errors, ValidationError{Field: "limit", Message: "limit cannot exceed 100"})
	}

	if offset < 0 {
		errors = append(errors, ValidationError{Field: "offset", Message: "offset cannot be negative"})
	}

	return errors
}

// ValidateOrderBy validates order by field
func (v *Validator) ValidateOrderBy(orderBy string, allowedFields []string) ValidationError {
	if orderBy == "" {
		return ValidationError{} // Empty is allowed, will use default
	}

	orderBy = strings.ToLower(orderBy)

	for _, field := range allowedFields {
		if orderBy == strings.ToLower(field) {
			return ValidationError{}
		}
	}

	return ValidationError{Field: "orderBy", Message: fmt.Sprintf("invalid orderBy field, allowed: %v", allowedFields)}
}

// ValidateSortDirection validates sort direction
func (v *Validator) ValidateSortDirection(direction string) ValidationError {
	if direction == "" {
		return ValidationError{} // Empty is allowed, will use default
	}

	direction = strings.ToLower(direction)

	if direction != "asc" && direction != "desc" {
		return ValidationError{Field: "sortDirection", Message: "sortDirection must be 'asc' or 'desc'"}
	}

	return ValidationError{}
}

// ValidateRPCRequest validates a JSON-RPC request
func (v *Validator) ValidateRPCRequest(method string, params interface{}) ValidationErrors {
	var errors ValidationErrors

	if method == "" {
		errors = append(errors, ValidationError{Field: "method", Message: "RPC method is required"})
		return errors
	}

	// Basic method name validation
	if !rpcMethodRegex.MatchString(method) {
		errors = append(errors, ValidationError{Field: "method", Message: "invalid RPC method format"})
	}

	return errors
}

// ValidateChainAndAddress validates both chain and address together
func (v *Validator) ValidateChainAndAddress(chain, address string) ValidationErrors {
	var errors ValidationErrors

	if chainErr := v.ValidateChainName(chain); chainErr.Message != "" {
		errors = append(errors, chainErr)
	}

	if addressErr := v.ValidateAddress(address, chain); addressErr.Message != "" {
		errors = append(errors, addressErr)
	}

	return errors
}

// ValidateChainAndHash validates both chain and transaction hash together
func (v *Validator) ValidateChainAndHash(chain, hash string) ValidationErrors {
	var errors ValidationErrors

	if chainErr := v.ValidateChainName(chain); chainErr.Message != "" {
		errors = append(errors, chainErr)
	}

	if hashErr := v.ValidateTransactionHash(hash, chain); hashErr.Message != "" {
		errors = append(errors, hashErr)
	}

	return errors
}
