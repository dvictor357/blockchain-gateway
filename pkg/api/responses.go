package api

import (
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
)

// HealthResponse represents the health check response
type HealthResponse struct {
	Status string `json:"status" example:"ok"`
	Time   string `json:"time" example:"2023-05-15T14:30:45Z"`
}

// ChainsResponse represents the list of supported chains
type ChainsResponse struct {
	Chains []string `json:"chains" example:"ethereum,bitcoin,polygon"`
	Count  int      `json:"count" example:"3"`
}

// GasPriceResponse represents the gas price response
type GasPriceResponse struct {
	Chain       string `json:"chain" example:"ethereum"`
	GasPrice    string `json:"gas_price" example:"20000000000"`
	GasPriceHex string `json:"gas_price_hex" example:"0x4a817c800"`
}

// TransactionCountResponse represents the transaction count response
type TransactionCountResponse struct {
	Chain            string `json:"chain" example:"ethereum"`
	Address          string `json:"address" example:"0x742d35Cc6634C0532925a3b844Bc454e4438f44e"`
	TransactionCount uint64 `json:"transaction_count" example:"42"`
	Nonce            string `json:"nonce" example:"0x2a"`
}

// MarketDataResponse represents the paginated market data response
type MarketDataResponse struct {
	Data       []models.CoinMarket `json:"data"`
	Pagination PaginationInfo      `json:"pagination"`
	Meta       MetaInfo            `json:"meta"`
}

// PaginationInfo represents pagination information
type PaginationInfo struct {
	TotalRecords int `json:"total_records" example:"100"`
	Limit        int `json:"limit" example:"20"`
	Offset       int `json:"offset" example:"0"`
	CurrentPage  int `json:"current_page" example:"1"`
	TotalPages   int `json:"total_pages" example:"5"`
}

// MetaInfo represents metadata about the response
type MetaInfo struct {
	LastDataUpdateFromSource string `json:"last_data_update_from_source" example:"2024-05-28T10:01:05.123456789Z"`
}

// BatchQueryRequest represents a batch query request
type BatchQueryRequest map[string][]blockchain.RPCRequest

// BatchQueryResponse represents a batch query response
type BatchQueryResponse map[string][]blockchain.RPCResponse

// SwaggerRPCRequest represents an RPC request for Swagger documentation
type SwaggerRPCRequest struct {
	JSONRPC string   `json:"jsonrpc" example:"2.0"`
	Method  string   `json:"method" example:"eth_blockNumber"`
	Params  []string `json:"params" example:"[]"`
	ID      int      `json:"id" example:"1"`
}

// SwaggerRPCResponse represents an RPC response for Swagger documentation
type SwaggerRPCResponse struct {
	JSONRPC string    `json:"jsonrpc" example:"2.0"`
	Result  string    `json:"result,omitempty" example:"0x1234567"`
	Error   *RPCError `json:"error,omitempty"`
	ID      int       `json:"id" example:"1"`
}

// RPCError represents a JSON-RPC error for Swagger documentation
type RPCError struct {
	Code    int    `json:"code" example:"-32000"`
	Message string `json:"message" example:"Server error"`
	Data    string `json:"data,omitempty" example:"Additional error details"`
}

// SwaggerBalance represents a balance response for Swagger documentation
type SwaggerBalance struct {
	Address    string `json:"address" example:"0x742d35Cc6634C0532925a3b844Bc454e4438f44e"`
	Balance    string `json:"balance" example:"1000000000000000000"`
	HexBalance string `json:"hex_balance,omitempty" example:"0xde0b6b3a7640000"`
	Decimals   int    `json:"decimals" example:"18"`
	Symbol     string `json:"symbol,omitempty" example:"ETH"`
	Chain      string `json:"chain" example:"ethereum"`
}

// SwaggerBlockInfo represents block information for Swagger documentation
type SwaggerBlockInfo struct {
	Number           uint64 `json:"number" example:"18934257"`
	Hash             string `json:"hash" example:"0x8d12a0d346a05cf0dd9e650a5e41baa531a2ef7a287572739ce5c5a36856ec7c"`
	ParentHash       string `json:"parent_hash,omitempty" example:"0x781d36b32c7cbf06d952baa1d827eb425bacfdf9c9afc30b735959054a3f2fc1"`
	Timestamp        uint64 `json:"timestamp,omitempty" example:"1716403465"`
	TransactionCount int    `json:"transaction_count,omitempty" example:"124"`
	Chain            string `json:"chain" example:"ethereum"`
}

// SwaggerTransactionInfo represents transaction information for Swagger documentation
type SwaggerTransactionInfo struct {
	Hash        string `json:"hash" example:"0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71"`
	From        string `json:"from,omitempty" example:"0x95222290dd7278aa3ddd389cc1e1d165cc4bafe5"`
	To          string `json:"to,omitempty" example:"0xdef1c0ded9bec7f1a1670819833240f027b25eff"`
	Value       string `json:"value,omitempty" example:"500000000000000000"`
	BlockNumber uint64 `json:"block_number,omitempty" example:"18934220"`
	BlockHash   string `json:"block_hash,omitempty" example:"0x90e1a8e935cfd5970d6789a7afedb1dac09af91a7b8fc7dbe16008116ab19f9c"`
	Status      string `json:"status,omitempty" example:"success"`
	Chain       string `json:"chain" example:"ethereum"`
}

// ValidationErrorDetail represents a single validation error
type ValidationErrorDetail struct {
	Field   string `json:"field" example:"address"`
	Message string `json:"message" example:"invalid Ethereum address format"`
}

// ErrorResponse represents a standardized error response (already defined in errors.go)
// We'll reference it here for swagger documentation
type SwaggerErrorResponse struct {
	Error   string                       `json:"error" example:"Validation failed"`
	Code    string                       `json:"code" example:"VALIDATION_ERROR"`
	Details []validation.ValidationError `json:"details,omitempty"`
}
