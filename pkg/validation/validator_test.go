package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidator_ValidateChainName(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		chain     string
		wantError bool
		wantMsg   string
	}{
		{
			name:      "valid ethereum chain",
			chain:     "ethereum",
			wantError: false,
		},
		{
			name:      "valid polygon chain",
			chain:     "polygon",
			wantError: false,
		},
		{
			name:      "valid bitcoin chain",
			chain:     "bitcoin",
			wantError: false,
		},
		{
			name:      "valid chain with underscore",
			chain:     "test_chain",
			wantError: false,
		},
		{
			name:      "valid chain with dash",
			chain:     "test-chain",
			wantError: false,
		},
		{
			name:      "empty chain name",
			chain:     "",
			wantError: true,
			wantMsg:   "chain name is required",
		},
		{
			name:      "chain starting with number",
			chain:     "1ethereum",
			wantError: true,
			wantMsg:   "invalid chain name format",
		},
		{
			name:      "chain with special characters",
			chain:     "ethereum@test",
			wantError: true,
			wantMsg:   "invalid chain name format",
		},
		{
			name:      "chain with spaces",
			chain:     "ethereum test",
			wantError: true,
			wantMsg:   "invalid chain name format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateChainName(tt.chain)

			if tt.wantError {
				assert.NotEmpty(t, err.Message, "Expected validation error")
				assert.Contains(t, err.Message, tt.wantMsg)
				assert.Equal(t, "chain", err.Field)
			} else {
				assert.Empty(t, err.Message, "Expected no validation error")
			}
		})
	}
}

func TestValidator_ValidateAddress(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		address   string
		chain     string
		wantError bool
		wantMsg   string
	}{
		// Ethereum/EVM addresses
		{
			name:      "valid ethereum address",
			address:   "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			chain:     "ethereum",
			wantError: false,
		},
		{
			name:      "valid polygon address",
			address:   "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			chain:     "polygon",
			wantError: false,
		},
		{
			name:      "ethereum address without 0x prefix",
			address:   "742d35Cc6634C0532925a3b844Bc454e4438f44e",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM address format",
		},
		{
			name:      "ethereum address too short",
			address:   "0x742d35Cc6634C0532925a3b844Bc454e4438f4",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM address format",
		},
		{
			name:      "ethereum address too long",
			address:   "0x742d35Cc6634C0532925a3b844Bc454e4438f44e1",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM address format",
		},
		{
			name:      "ethereum address with invalid characters",
			address:   "0x742d35Cc6634C0532925a3b844Bc454e4438f44g",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM address format",
		},
		// Bitcoin addresses
		{
			name:      "valid bitcoin legacy address",
			address:   "1A1zP1eP5QGefi2DMPTfTL5SLmv7DivfNa",
			chain:     "bitcoin",
			wantError: false,
		},
		{
			name:      "valid bitcoin P2SH address",
			address:   "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy",
			chain:     "bitcoin",
			wantError: false,
		},
		{
			name:      "valid bitcoin bech32 address",
			address:   "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4",
			chain:     "bitcoin",
			wantError: false,
		},
		{
			name:      "invalid bitcoin address",
			address:   "invalid_bitcoin_address",
			chain:     "bitcoin",
			wantError: true,
			wantMsg:   "invalid Bitcoin address format",
		},
		// Generic addresses
		{
			name:      "valid generic address",
			address:   "cosmos1abc123def456",
			chain:     "cosmos",
			wantError: false,
		},
		{
			name:      "generic address too short",
			address:   "short",
			chain:     "cosmos",
			wantError: true,
			wantMsg:   "address length must be between 10 and 100 characters",
		},
		{
			name:      "generic address too long",
			address:   "very_long_address_that_exceeds_the_maximum_allowed_length_of_one_hundred_characters_and_should_fail_validation",
			chain:     "cosmos",
			wantError: true,
			wantMsg:   "address length must be between 10 and 100 characters",
		},
		// Empty address
		{
			name:      "empty address",
			address:   "",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "address is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateAddress(tt.address, tt.chain)

			if tt.wantError {
				assert.NotEmpty(t, err.Message, "Expected validation error")
				assert.Contains(t, err.Message, tt.wantMsg)
				assert.Equal(t, "address", err.Field)
			} else {
				assert.Empty(t, err.Message, "Expected no validation error")
			}
		})
	}
}

func TestValidator_ValidateTransactionHash(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		hash      string
		chain     string
		wantError bool
		wantMsg   string
	}{
		// Ethereum/EVM transaction hashes
		{
			name:      "valid ethereum transaction hash",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "ethereum",
			wantError: false,
		},
		{
			name:      "valid polygon transaction hash",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "polygon",
			wantError: false,
		},
		{
			name:      "ethereum hash without 0x prefix",
			hash:      "9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM transaction hash format",
		},
		{
			name:      "ethereum hash too short",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff7",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM transaction hash format",
		},
		{
			name:      "ethereum hash with invalid characters",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bffg1",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "invalid Ethereum/EVM transaction hash format",
		},
		// Bitcoin transaction hashes
		{
			name:      "valid bitcoin transaction hash",
			hash:      "9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "bitcoin",
			wantError: false,
		},
		{
			name:      "bitcoin hash with 0x prefix (should fail)",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "bitcoin",
			wantError: true,
			wantMsg:   "invalid Bitcoin transaction hash format",
		},
		{
			name:      "bitcoin hash too short",
			hash:      "9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff7",
			chain:     "bitcoin",
			wantError: true,
			wantMsg:   "invalid Bitcoin transaction hash format",
		},
		// Generic transaction hashes
		{
			name:      "valid generic hash with 0x prefix",
			hash:      "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "cosmos",
			wantError: false,
		},
		{
			name:      "valid generic hash without 0x prefix",
			hash:      "9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			chain:     "cosmos",
			wantError: false,
		},
		{
			name:      "invalid generic hash",
			hash:      "invalid_hash",
			chain:     "cosmos",
			wantError: true,
			wantMsg:   "invalid transaction hash format",
		},
		// Empty hash
		{
			name:      "empty hash",
			hash:      "",
			chain:     "ethereum",
			wantError: true,
			wantMsg:   "transaction hash is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateTransactionHash(tt.hash, tt.chain)

			if tt.wantError {
				assert.NotEmpty(t, err.Message, "Expected validation error")
				assert.Contains(t, err.Message, tt.wantMsg)
				assert.Equal(t, "hash", err.Field)
			} else {
				assert.Empty(t, err.Message, "Expected no validation error")
			}
		})
	}
}

func TestValidator_ValidatePaginationParams(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		limit      int
		offset     int
		wantErrors int
		wantMsgs   []string
	}{
		{
			name:       "valid pagination",
			limit:      20,
			offset:     0,
			wantErrors: 0,
		},
		{
			name:       "valid pagination with offset",
			limit:      50,
			offset:     100,
			wantErrors: 0,
		},
		{
			name:       "limit too small",
			limit:      0,
			offset:     0,
			wantErrors: 1,
			wantMsgs:   []string{"limit must be greater than 0"},
		},
		{
			name:       "limit too large",
			limit:      150,
			offset:     0,
			wantErrors: 1,
			wantMsgs:   []string{"limit cannot exceed 100"},
		},
		{
			name:       "negative offset",
			limit:      20,
			offset:     -1,
			wantErrors: 1,
			wantMsgs:   []string{"offset cannot be negative"},
		},
		{
			name:       "multiple errors",
			limit:      0,
			offset:     -1,
			wantErrors: 2,
			wantMsgs:   []string{"limit must be greater than 0", "offset cannot be negative"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidatePaginationParams(tt.limit, tt.offset)

			assert.Len(t, errors, tt.wantErrors, "Expected %d validation errors", tt.wantErrors)

			if tt.wantErrors > 0 {
				for i, expectedMsg := range tt.wantMsgs {
					assert.Contains(t, errors[i].Message, expectedMsg)
				}
			}
		})
	}
}

func TestValidator_ValidateOrderBy(t *testing.T) {
	validator := NewValidator()
	allowedFields := []string{"name", "price", "date"}

	tests := []struct {
		name      string
		orderBy   string
		wantError bool
		wantMsg   string
	}{
		{
			name:      "valid order by field",
			orderBy:   "name",
			wantError: false,
		},
		{
			name:      "valid order by field case insensitive",
			orderBy:   "NAME",
			wantError: false,
		},
		{
			name:      "empty order by (allowed)",
			orderBy:   "",
			wantError: false,
		},
		{
			name:      "invalid order by field",
			orderBy:   "invalid_field",
			wantError: true,
			wantMsg:   "invalid orderBy field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateOrderBy(tt.orderBy, allowedFields)

			if tt.wantError {
				assert.NotEmpty(t, err.Message, "Expected validation error")
				assert.Contains(t, err.Message, tt.wantMsg)
				assert.Equal(t, "orderBy", err.Field)
			} else {
				assert.Empty(t, err.Message, "Expected no validation error")
			}
		})
	}
}

func TestValidator_ValidateSortDirection(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name      string
		direction string
		wantError bool
		wantMsg   string
	}{
		{
			name:      "valid asc direction",
			direction: "asc",
			wantError: false,
		},
		{
			name:      "valid desc direction",
			direction: "desc",
			wantError: false,
		},
		{
			name:      "valid ASC direction (case insensitive)",
			direction: "ASC",
			wantError: false,
		},
		{
			name:      "valid DESC direction (case insensitive)",
			direction: "DESC",
			wantError: false,
		},
		{
			name:      "empty direction (allowed)",
			direction: "",
			wantError: false,
		},
		{
			name:      "invalid direction",
			direction: "invalid",
			wantError: true,
			wantMsg:   "sortDirection must be 'asc' or 'desc'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.ValidateSortDirection(tt.direction)

			if tt.wantError {
				assert.NotEmpty(t, err.Message, "Expected validation error")
				assert.Contains(t, err.Message, tt.wantMsg)
				assert.Equal(t, "sortDirection", err.Field)
			} else {
				assert.Empty(t, err.Message, "Expected no validation error")
			}
		})
	}
}

func TestValidator_ValidateRPCRequest(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		method     string
		params     interface{}
		wantErrors int
		wantMsgs   []string
	}{
		{
			name:       "valid RPC method",
			method:     "eth_getBalance",
			params:     []interface{}{"0x123", "latest"},
			wantErrors: 0,
		},
		{
			name:       "valid RPC method with underscore",
			method:     "eth_block_number",
			params:     []interface{}{},
			wantErrors: 0,
		},
		{
			name:       "empty method",
			method:     "",
			params:     []interface{}{},
			wantErrors: 1,
			wantMsgs:   []string{"RPC method is required"},
		},
		{
			name:       "invalid method format",
			method:     "123invalid",
			params:     []interface{}{},
			wantErrors: 1,
			wantMsgs:   []string{"invalid RPC method format"},
		},
		{
			name:       "method with special characters",
			method:     "eth-getBalance",
			params:     []interface{}{},
			wantErrors: 1,
			wantMsgs:   []string{"invalid RPC method format"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateRPCRequest(tt.method, tt.params)

			assert.Len(t, errors, tt.wantErrors, "Expected %d validation errors", tt.wantErrors)

			if tt.wantErrors > 0 {
				for i, expectedMsg := range tt.wantMsgs {
					assert.Contains(t, errors[i].Message, expectedMsg)
				}
			}
		})
	}
}

func TestValidator_ValidateChainAndAddress(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		chain      string
		address    string
		wantErrors int
	}{
		{
			name:       "valid chain and address",
			chain:      "ethereum",
			address:    "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			wantErrors: 0,
		},
		{
			name:       "invalid chain, valid address",
			chain:      "",
			address:    "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			wantErrors: 1,
		},
		{
			name:       "valid chain, invalid address",
			chain:      "ethereum",
			address:    "invalid_address",
			wantErrors: 1,
		},
		{
			name:       "both invalid",
			chain:      "",
			address:    "",
			wantErrors: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateChainAndAddress(tt.chain, tt.address)
			assert.Len(t, errors, tt.wantErrors, "Expected %d validation errors", tt.wantErrors)
		})
	}
}

func TestValidator_ValidateChainAndHash(t *testing.T) {
	validator := NewValidator()

	tests := []struct {
		name       string
		chain      string
		hash       string
		wantErrors int
	}{
		{
			name:       "valid chain and hash",
			chain:      "ethereum",
			hash:       "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			wantErrors: 0,
		},
		{
			name:       "invalid chain, valid hash",
			chain:      "",
			hash:       "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			wantErrors: 1,
		},
		{
			name:       "valid chain, invalid hash",
			chain:      "ethereum",
			hash:       "invalid_hash",
			wantErrors: 1,
		},
		{
			name:       "both invalid",
			chain:      "",
			hash:       "",
			wantErrors: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := validator.ValidateChainAndHash(tt.chain, tt.hash)
			assert.Len(t, errors, tt.wantErrors, "Expected %d validation errors", tt.wantErrors)
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := ValidationError{
		Field:   "test_field",
		Message: "test message",
	}

	expected := "validation error for field 'test_field': test message"
	assert.Equal(t, expected, err.Error())
}

func TestValidationErrors_Error(t *testing.T) {
	tests := []struct {
		name     string
		errors   ValidationErrors
		expected string
	}{
		{
			name:     "no errors",
			errors:   ValidationErrors{},
			expected: "no validation errors",
		},
		{
			name: "single error",
			errors: ValidationErrors{
				{Field: "field1", Message: "message1"},
			},
			expected: "validation error for field 'field1': message1",
		},
		{
			name: "multiple errors",
			errors: ValidationErrors{
				{Field: "field1", Message: "message1"},
				{Field: "field2", Message: "message2"},
			},
			expected: "validation error for field 'field1': message1; validation error for field 'field2': message2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errors.Error())
		})
	}
}

func TestValidationErrors_HasErrors(t *testing.T) {
	tests := []struct {
		name     string
		errors   ValidationErrors
		expected bool
	}{
		{
			name:     "no errors",
			errors:   ValidationErrors{},
			expected: false,
		},
		{
			name: "has errors",
			errors: ValidationErrors{
				{Field: "field1", Message: "message1"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.errors.HasErrors())
		})
	}
}
