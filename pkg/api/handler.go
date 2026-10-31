package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dvictor357/blaze"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
)

// BlockchainService is the blockchain functionality used by the API.
type BlockchainService interface {
	ListChains() []string
	Execute(context.Context, string, string, interface{}) (json.RawMessage, error)
	BatchExecute(context.Context, map[string][]blockchain.RPCRequest) (map[string][]blockchain.RPCResponse, error)
	GetBalance(context.Context, string, string) (*blockchain.Balance, error)
	GetLatestBlock(context.Context, string) (*blockchain.BlockInfo, error)
	GetTransaction(context.Context, string, string) (*blockchain.TransactionInfo, error)
	GetGasPrice(context.Context, string) (*big.Int, error)
	GetTransactionCount(context.Context, string, string) (uint64, error)
}

// MarketDataService is the market data functionality used by the API.
type MarketDataService interface {
	GetMarketDataFromDB(context.Context, int, int, string, string) ([]models.CoinMarket, int, time.Time, error)
}

// Handler manages API requests with improved error handling and validation
type Handler struct {
	clientManager     BlockchainService
	logger            *log.Logger
	marketDataService MarketDataService
	validator         *validation.Validator
	errorHandler      *ErrorHandler
	apiConfig         *config.APIConfig
}

// NewHandler creates a new API handler with all dependencies
func NewHandler(clientManager BlockchainService, logger *log.Logger, marketService MarketDataService, apiConfig *config.APIConfig) *Handler {
	return &Handler{
		clientManager:     clientManager,
		logger:            logger,
		marketDataService: marketService,
		validator:         validation.NewValidator(),
		errorHandler:      NewErrorHandler(logger),
		apiConfig:         apiConfig,
	}
}

// ListChains returns a list of all supported blockchains
// @Summary      List Supported Blockchains
// @Description  Get a list of all blockchain networks supported by this gateway
// @Tags         chains
// @Security     APIKeyAuth
// @Failure      401  {object} api.SwaggerErrorResponse
// @Accept       json
// @Produce      json
// @Success      200  {object}  api.ChainsResponse
// @Failure      500  {object}  api.SwaggerErrorResponse
// @Router       /api/v1/chains [get]
func (h *Handler) ListChains(c *blaze.Context) error {
	chains := h.clientManager.ListChains()

	return c.JSON(http.StatusOK, ChainsResponse{
		Chains: chains,
		Count:  len(chains),
	})
}

// QueryChain handles RPC queries to a specific blockchain with improved validation
// @Summary      Execute RPC Query
// @Description  Execute a JSON-RPC query on a specific blockchain network
// @Description  Only exact methods in RPC_METHOD_ALLOWLIST are exposed; defaults are finite read-only methods. Denied methods return 403 RPC_METHOD_NOT_ALLOWED.
// @Tags         chains
// @Security     APIKeyAuth
// @Accept       json
// @Produce      json
// @Param        chain  path      string                    true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Param        request body     api.SwaggerRPCRequest     true  "RPC Request"
// @Success      200    {object}  api.SwaggerRPCResponse
// @Failure      400    {object}  api.SwaggerErrorResponse
// @Failure      401    {object} api.SwaggerErrorResponse
// @Failure      403    {object} api.SwaggerErrorResponse
// @Failure      404    {object}  api.SwaggerErrorResponse
// @Failure      413    {object}  api.SwaggerErrorResponse
// @Failure      500    {object}  api.SwaggerErrorResponse
// @Failure      502    {object}  api.SwaggerErrorResponse
// @Failure      504    {object}  api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/query [post]
func (h *Handler) QueryChain(c *blaze.Context) error {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
	}

	// Parse the request body
	var request blockchain.RPCRequest
	if err := h.decodeRequestJSON(c, &request); err != nil {
		return h.errorHandler.HandleBindingError(c, err)
	}

	// Validate RPC request
	if validationErrors := h.validator.ValidateRPCRequest(request.Method, request.Params); validationErrors.HasErrors() {
		return h.errorHandler.HandleValidationErrors(c, validationErrors)
	}
	if !h.apiConfig.IsRPCMethodAllowed(request.Method) {
		return h.rpcMethodDenied(c, request.Method)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Execute the RPC request
	result, err := h.clientManager.Execute(ctx, chain, request.Method, request.Params)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "execute RPC request", chain)
	}

	// Create a JSON-RPC response
	response := blockchain.RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      request.ID,
	}

	return c.JSON(http.StatusOK, response)
}

// BatchQuery handles batch RPC requests with improved validation
// @Summary      Execute Batch RPC Queries
// @Description  Execute multiple RPC queries across different blockchain networks in a single request.
// @Description  The total number of RPC requests across all chains is limited (10 by default); requests on one chain run sequentially.
// @Description  Per-request node errors are returned inside the response with their original code and data.
// @Description  All methods are checked against RPC_METHOD_ALLOWLIST before any upstream call; one denied method rejects the entire batch with 403 RPC_METHOD_NOT_ALLOWED.
// @Tags         chains
// @Security     APIKeyAuth
// @Accept       json
// @Produce      json
// @Param        request body     map[string][]api.SwaggerRPCRequest     true  "Batch RPC Request"
// @Success      200     {object} map[string][]api.SwaggerRPCResponse
// @Failure      400     {object} api.SwaggerErrorResponse
// @Failure      401     {object} api.SwaggerErrorResponse
// @Failure      403     {object} api.SwaggerErrorResponse
// @Failure      404     {object} api.SwaggerErrorResponse
// @Failure      413     {object} api.SwaggerErrorResponse
// @Failure      500     {object} api.SwaggerErrorResponse
// @Router       /api/v1/batch [post]
func (h *Handler) BatchQuery(c *blaze.Context) error {
	// Parse the request body
	var batchRequest map[string][]blockchain.RPCRequest
	if err := h.decodeRequestJSON(c, &batchRequest); err != nil {
		return h.errorHandler.HandleBindingError(c, err)
	}

	// The limit applies to the total RPC count, not the number of chains. Reject before any upstream work.
	totalRequests := 0
	for _, requests := range batchRequest {
		totalRequests += len(requests)
	}

	if totalRequests == 0 {
		return h.errorHandler.RespondWithError(c, http.StatusBadRequest, "EMPTY_BATCH",
			"Batch request must include at least one RPC request")
	}

	if totalRequests > h.apiConfig.MaxBatchSize {
		return h.errorHandler.RespondWithError(c, http.StatusBadRequest, "BATCH_TOO_LARGE",
			"Batch cannot exceed %d RPC requests in total", h.apiConfig.MaxBatchSize)
	}

	// Validate each chain and request in a stable order
	chains := make([]string, 0, len(batchRequest))
	for chain := range batchRequest {
		chains = append(chains, chain)
	}
	sort.Strings(chains)

	for _, chain := range chains {
		requests := batchRequest[chain]
		if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
			return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		}

		for _, req := range requests {
			if validationErrors := h.validator.ValidateRPCRequest(req.Method, req.Params); validationErrors.HasErrors() {
				return h.errorHandler.HandleValidationErrors(c, validationErrors)
			}
			if !h.apiConfig.IsRPCMethodAllowed(req.Method) {
				return h.rpcMethodDenied(c, req.Method)
			}
		}
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("batch"))
	defer cancel()

	// Execute batch request
	results, err := h.clientManager.BatchExecute(ctx, batchRequest)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "execute batch request", "")
	}

	return c.JSON(http.StatusOK, results)
}

// GetBalance retrieves the balance for an address with improved validation
// @Summary      Get Account Balance
// @Description  Get the native token balance for a specific address on a blockchain.
// @Description  Bitcoin is not supported (HTTP 501, code OPERATION_NOT_SUPPORTED): no address index is configured.
// @Tags         chains
// @Security     APIKeyAuth
// @Description  Helper RPC dependencies must all be permitted by RPC_METHOD_ALLOWLIST.
// @Accept       json
// @Produce      json
// @Param        chain    path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Param        address  path     string  true  "Wallet address"
// @Success      200      {object} api.SwaggerBalance
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      403      {object} api.SwaggerErrorResponse
// @Failure      404      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Failure      501      {object} api.SwaggerErrorResponse
// @Failure      502      {object} api.SwaggerErrorResponse
// @Failure      504      {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/address/{address}/balance [get]
func (h *Handler) GetBalance(c *blaze.Context) error {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate chain and address together
	if validationErrors := h.validator.ValidateChainAndAddress(chain, address); validationErrors.HasErrors() {
		return h.errorHandler.HandleValidationErrors(c, validationErrors)
	}
	if denied := h.deniedHelperMethod(chain, "balance"); denied != "" {
		return h.rpcMethodDenied(c, denied)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get balance
	balance, err := h.clientManager.GetBalance(ctx, chain, address)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "get balance", chain)
	}

	return c.JSON(http.StatusOK, balance)
}

// GetLatestBlock retrieves the latest block information with improved validation
// @Summary      Get Latest Block
// @Description  Get information about the latest block on a blockchain
// @Tags         chains
// @Security     APIKeyAuth
// @Description  Helper RPC dependencies must all be permitted by RPC_METHOD_ALLOWLIST.
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Success      200    {object} api.SwaggerBlockInfo
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      401    {object} api.SwaggerErrorResponse
// @Failure      403    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Failure      502    {object} api.SwaggerErrorResponse
// @Failure      504    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/block/latest [get]
func (h *Handler) GetLatestBlock(c *blaze.Context) error {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
	}
	if denied := h.deniedHelperMethod(chain, "block"); denied != "" {
		return h.rpcMethodDenied(c, denied)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get latest block
	block, err := h.clientManager.GetLatestBlock(ctx, chain)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "get latest block", chain)
	}

	return c.JSON(http.StatusOK, block)
}

// GetTransaction retrieves transaction details with improved validation
// @Summary      Get Transaction Details
// @Description  Get detailed information about a specific transaction by its hash
// @Tags         chains
// @Security     APIKeyAuth
// @Description  All helper RPC dependencies, including optional receipt/block enrichment, must be permitted by RPC_METHOD_ALLOWLIST.
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Param        hash   path     string  true  "Transaction hash"
// @Success      200    {object} api.SwaggerTransactionInfo
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      401    {object} api.SwaggerErrorResponse
// @Failure      403    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Failure      502    {object} api.SwaggerErrorResponse
// @Failure      504    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/tx/{hash} [get]
func (h *Handler) GetTransaction(c *blaze.Context) error {
	chain := c.Param("chain")
	hash := c.Param("hash")

	// Validate chain and hash together
	if validationErrors := h.validator.ValidateChainAndHash(chain, hash); validationErrors.HasErrors() {
		return h.errorHandler.HandleValidationErrors(c, validationErrors)
	}
	if denied := h.deniedHelperMethod(chain, "transaction"); denied != "" {
		return h.rpcMethodDenied(c, denied)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get transaction
	tx, err := h.clientManager.GetTransaction(ctx, chain, hash)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "get transaction", chain)
	}

	return c.JSON(http.StatusOK, tx)
}

// GetGasPrice retrieves the current gas price with improved validation
// @Summary      Get Gas Price
// @Description  Get the current gas price for EVM-compatible blockchains
// @Tags         chains
// @Security     APIKeyAuth
// @Description  Helper RPC dependencies must all be permitted by RPC_METHOD_ALLOWLIST.
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Success      200    {object} api.GasPriceResponse
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      401    {object} api.SwaggerErrorResponse
// @Failure      403    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Failure      502    {object} api.SwaggerErrorResponse
// @Failure      504    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/gas-price [get]
func (h *Handler) GetGasPrice(c *blaze.Context) error {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
	}
	if denied := h.deniedHelperMethod(chain, "gas"); denied != "" {
		return h.rpcMethodDenied(c, denied)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get gas price
	gasPrice, err := h.clientManager.GetGasPrice(ctx, chain)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "get gas price", chain)
	}

	// Format the response
	return c.JSON(http.StatusOK, GasPriceResponse{
		Chain:       chain,
		GasPrice:    gasPrice.String(),
		GasPriceHex: fmt.Sprintf("0x%x", gasPrice),
	})
}

// GetTransactionCount retrieves the transaction count with improved validation
// @Summary      Get Transaction Count
// @Description  Get the number of transactions for a specific address on a blockchain
// @Tags         chains
// @Security     APIKeyAuth
// @Description  Helper RPC dependencies must all be permitted by RPC_METHOD_ALLOWLIST.
// @Accept       json
// @Produce      json
// @Param        chain    path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon, bsc, arbitrum, optimism, base, avalanche, fantom)
// @Param        address  path     string  true  "Wallet address"
// @Success      200      {object} api.TransactionCountResponse
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      401      {object} api.SwaggerErrorResponse
// @Failure      403      {object} api.SwaggerErrorResponse
// @Failure      404      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Failure      502      {object} api.SwaggerErrorResponse
// @Failure      504      {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/address/{address}/nonce [get]
func (h *Handler) GetTransactionCount(c *blaze.Context) error {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate chain and address together
	if validationErrors := h.validator.ValidateChainAndAddress(chain, address); validationErrors.HasErrors() {
		return h.errorHandler.HandleValidationErrors(c, validationErrors)
	}
	if denied := h.deniedHelperMethod(chain, "nonce"); denied != "" {
		return h.rpcMethodDenied(c, denied)
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get transaction count
	count, err := h.clientManager.GetTransactionCount(ctx, chain, address)
	if err != nil {
		return h.errorHandler.HandleBlockchainError(c, err, "get transaction count", chain)
	}

	// Format the response
	return c.JSON(http.StatusOK, TransactionCountResponse{
		Chain:            chain,
		Address:          address,
		TransactionCount: count,
		Nonce:            fmt.Sprintf("0x%x", count),
	})
}

func (h *Handler) rpcMethodDenied(c *blaze.Context, method string) error {
	return h.errorHandler.RespondWithError(c, http.StatusForbidden, "RPC_METHOD_NOT_ALLOWED",
		"RPC method %q is not allowed", method)
}

// Preflight the complete helper, including optional enrichment lookups, before
// any upstream call so a restrictive policy cannot be bypassed via REST.
func (h *Handler) deniedHelperMethod(chain, operation string) string {
	var methods []string
	if strings.EqualFold(chain, "bitcoin") {
		switch operation {
		case "block":
			methods = []string{"getblockcount", "getblockhash", "getblock"}
		case "transaction":
			methods = []string{"getrawtransaction", "getblock"}
		}
	} else {
		switch operation {
		case "balance":
			methods = []string{"eth_getBalance"}
		case "block":
			methods = []string{"eth_blockNumber", "eth_getBlockByNumber"}
		case "transaction":
			methods = []string{"eth_getTransactionByHash", "eth_getTransactionReceipt"}
		case "gas":
			methods = []string{"eth_gasPrice"}
		case "nonce":
			methods = []string{"eth_getTransactionCount"}
		}
	}
	for _, method := range methods {
		if !h.apiConfig.IsRPCMethodAllowed(method) {
			return method
		}
	}
	return ""
}

// GetCoinMarkets handles requests for coin market data with improved validation
// @Summary      Get Coin Markets
// @Description  Get a list of cryptocurrency markets with pagination and sorting
// @Tags         markets
// @Accept       json
// @Produce      json
// @Security     APIKeyAuth
// @Param        limit      query     string  true  "Number of items per page"
// @Param        offset     query     string  true  "Page number"
// @Param        orderBy    query     string  true  "Field to order by (case-insensitive)" Enums(market_cap_rank, name, current_price, last_updated, data_fetched_at)
// @Param        sortDirection query     string  true  "Sort direction" Enums(asc, desc)
// @Success      200          {object} api.MarketDataResponse
// @Failure      400          {object} api.SwaggerErrorResponse
// @Failure      401          {object} api.SwaggerErrorResponse
// @Failure      500          {object} api.SwaggerErrorResponse
// @Failure      503          {object} api.SwaggerErrorResponse
// @Failure      504          {object} api.SwaggerErrorResponse
// @Router       /api/v1/markets [get]
func (h *Handler) GetCoinMarkets(c *blaze.Context) error {
	if h.marketDataService == nil {
		return h.errorHandler.RespondWithError(c, http.StatusServiceUnavailable, "MARKET_DATA_DISABLED",
			"Market data is disabled; enable DATABASE_ENABLED to serve stored market data")
	}

	// Parse and validate pagination parameters
	query := c.Request.URL.Query()
	limitStr := queryDefault(query, "limit", strconv.Itoa(h.apiConfig.DefaultPageLimit))
	offsetStr := queryDefault(query, "offset", "0")
	orderBy := queryDefault(query, "orderBy", "market_cap_rank")
	sortDirection := queryDefault(query, "sortDirection", "asc")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		return h.errorHandler.RespondWithError(c, http.StatusBadRequest, "INVALID_LIMIT",
			"Invalid limit parameter")
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		return h.errorHandler.RespondWithError(c, http.StatusBadRequest, "INVALID_OFFSET",
			"Invalid offset parameter")
	}

	// Validate pagination parameters
	if validationErrors := h.validator.ValidatePaginationParams(limit, offset); validationErrors.HasErrors() {
		return h.errorHandler.HandleValidationErrors(c, validationErrors)
	}

	// Validate and normalize limit
	limit = h.apiConfig.ValidatePageLimit(limit)

	// Validate orderBy field
	if validationErr := h.validator.ValidateOrderBy(orderBy, h.apiConfig.AllowedOrderByFields); validationErr.Message != "" {
		return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
	}

	// Validate sort direction
	if validationErr := h.validator.ValidateSortDirection(sortDirection); validationErr.Message != "" {
		return h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
	}

	// Fetch market data within the request budget
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	markets, total, latestFetchTime, err := h.marketDataService.GetMarketDataFromDB(ctx, limit, offset, strings.ToLower(orderBy), sortDirection)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return h.errorHandler.RespondWithError(c, http.StatusGatewayTimeout, "REQUEST_TIMEOUT",
				"Retrieving market data timed out")
		}
		return h.errorHandler.HandleGenericError(c, err, "retrieve market data")
	}

	return c.JSON(http.StatusOK, MarketDataResponse{
		Data: markets,
		Pagination: PaginationInfo{
			TotalRecords: total,
			Limit:        limit,
			Offset:       offset,
			CurrentPage:  (offset / limit) + 1,
			TotalPages:   (total + limit - 1) / limit,
		},
		Meta: MetaInfo{
			LastDataUpdateFromSource: latestFetchTime.Format(time.RFC3339Nano),
		},
	})
}

// errTrailingJSON reports content after the single JSON value in a request body.
var errTrailingJSON = errors.New("unexpected data after JSON body")

// decodeRequestJSON decodes exactly one JSON value from the body, enforcing the configured byte
// limit over the whole body. It preserves numeric RPC params and IDs without float64 rounding.
// Oversized bodies produce *http.MaxBytesError; trailing content (other than whitespace) is an error.
func (h *Handler) decodeRequestJSON(c *blaze.Context, destination interface{}) error {
	limit := h.apiConfig.MaxRequestBodyBytes
	if c.Request.ContentLength > limit {
		return &http.MaxBytesError{Limit: limit}
	}

	decoder := json.NewDecoder(http.MaxBytesReader(c.ResponseWriter, c.Request.Body, limit))
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return err
	}

	var trailing json.RawMessage
	switch err := decoder.Decode(&trailing); {
	case errors.Is(err, io.EOF):
		return nil
	case err == nil:
		return errTrailingJSON
	default:
		return err
	}
}

// queryDefault distinguishes an absent query parameter from an explicit empty value.
func queryDefault(query map[string][]string, key, fallback string) string {
	if values, ok := query[key]; ok && len(values) > 0 {
		return values[0]
	}
	return fallback
}
