package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/marketdata"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
	"github.com/gin-gonic/gin"
)

// ClientManagerInterface defines the interface for client manager operations
type ClientManagerInterface interface {
	Execute(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error)
	BatchExecute(ctx context.Context, requests map[string][]blockchain.RPCRequest) (map[string][]blockchain.RPCResponse, error)

	// Strongly-typed methods for common operations
	GetBalance(ctx context.Context, chain, address string) (*blockchain.Balance, error)
	GetLatestBlock(ctx context.Context, chain string) (*blockchain.BlockInfo, error)
	GetTransaction(ctx context.Context, chain, hash string) (*blockchain.TransactionInfo, error)
	GetGasPrice(ctx context.Context, chain string) (*big.Int, error)
	GetTransactionCount(ctx context.Context, chain, address string) (uint64, error)

	ListChains() []string
}

// Handler manages API requests with improved error handling and validation
type Handler struct {
	clientManager     ClientManagerInterface
	logger            *log.Logger
	marketDataService *marketdata.Service
	validator         *validation.Validator
	errorHandler      *ErrorHandler
	apiConfig         *config.APIConfig
}

// NewHandler creates a new API handler with all dependencies
func NewHandler(clientManager ClientManagerInterface, logger *log.Logger, marketService *marketdata.Service) *Handler {
	return &Handler{
		clientManager:     clientManager,
		logger:            logger,
		marketDataService: marketService,
		validator:         validation.NewValidator(),
		errorHandler:      NewErrorHandler(logger),
		apiConfig:         config.NewAPIConfig(),
	}
}

// ListChains returns a list of all supported blockchains
// @Summary      List Supported Blockchains
// @Description  Get a list of all blockchain networks supported by this gateway
// @Tags         chains
// @Accept       json
// @Produce      json
// @Success      200  {object}  api.ChainsResponse
// @Failure      500  {object}  api.SwaggerErrorResponse
// @Router       /api/v1/chains [get]
func (h *Handler) ListChains(c *gin.Context) {
	chains := h.clientManager.ListChains()

	c.JSON(http.StatusOK, ChainsResponse{
		Chains: chains,
		Count:  len(chains),
	})
}

// QueryChain handles RPC queries to a specific blockchain with improved validation
// @Summary      Execute RPC Query
// @Description  Execute a JSON-RPC query on a specific blockchain network
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain  path      string                    true  "Blockchain name" Enums(ethereum, bitcoin, polygon)
// @Param        request body     api.SwaggerRPCRequest     true  "RPC Request"
// @Success      200    {object}  api.SwaggerRPCResponse
// @Failure      400    {object}  api.SwaggerErrorResponse
// @Failure      404    {object}  api.SwaggerErrorResponse
// @Failure      500    {object}  api.SwaggerErrorResponse
// @Failure      504    {object}  api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/query [post]
func (h *Handler) QueryChain(c *gin.Context) {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		return
	}

	// Parse the request body
	var request blockchain.RPCRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		h.errorHandler.HandleBindingError(c, err)
		return
	}

	// Validate RPC request
	if validationErrors := h.validator.ValidateRPCRequest(request.Method, request.Params); validationErrors.HasErrors() {
		h.errorHandler.HandleValidationErrors(c, validationErrors)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Execute the RPC request
	result, err := h.clientManager.Execute(ctx, chain, request.Method, request.Params)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "execute RPC request", chain)
		return
	}

	// Create a JSON-RPC response
	response := blockchain.RPCResponse{
		JSONRPC: "2.0",
		Result:  result,
		ID:      request.ID,
	}

	c.JSON(http.StatusOK, response)
}

// BatchQuery handles batch RPC requests with improved validation
// @Summary      Execute Batch RPC Queries
// @Description  Execute multiple RPC queries across different blockchain networks in a single request
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        request body     map[string][]api.SwaggerRPCRequest     true  "Batch RPC Request"
// @Success      200     {object} map[string][]api.SwaggerRPCResponse
// @Failure      400     {object} api.SwaggerErrorResponse
// @Failure      500     {object} api.SwaggerErrorResponse
// @Router       /api/v1/batch [post]
func (h *Handler) BatchQuery(c *gin.Context) {
	// Parse the request body
	var batchRequest map[string][]blockchain.RPCRequest
	if err := c.ShouldBindJSON(&batchRequest); err != nil {
		h.errorHandler.HandleBindingError(c, err)
		return
	}

	// Validate the request
	if len(batchRequest) == 0 {
		h.errorHandler.RespondWithError(c, http.StatusBadRequest, "EMPTY_BATCH",
			"Batch request must include at least one blockchain")
		return
	}

	if len(batchRequest) > h.apiConfig.MaxBatchSize {
		h.errorHandler.RespondWithError(c, http.StatusBadRequest, "BATCH_TOO_LARGE",
			"Batch size cannot exceed %d blockchains", h.apiConfig.MaxBatchSize)
		return
	}

	// Validate each chain and request
	for chain, requests := range batchRequest {
		if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
			h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
			return
		}

		for _, req := range requests {
			if validationErrors := h.validator.ValidateRPCRequest(req.Method, req.Params); validationErrors.HasErrors() {
				h.errorHandler.HandleValidationErrors(c, validationErrors)
				return
			}
		}
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("batch"))
	defer cancel()

	// Execute batch request
	results, err := h.clientManager.BatchExecute(ctx, batchRequest)
	if err != nil {
		h.errorHandler.HandleGenericError(c, err, "execute batch request")
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetBalance retrieves the balance for an address with improved validation
// @Summary      Get Account Balance
// @Description  Get the native token balance for a specific address on a blockchain
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain    path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon)
// @Param        address  path     string  true  "Wallet address"
// @Success      200      {object} api.SwaggerBalance
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      404      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/address/{address}/balance [get]
func (h *Handler) GetBalance(c *gin.Context) {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate chain and address together
	if validationErrors := h.validator.ValidateChainAndAddress(chain, address); validationErrors.HasErrors() {
		h.errorHandler.HandleValidationErrors(c, validationErrors)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get balance
	balance, err := h.clientManager.GetBalance(ctx, chain, address)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get balance", chain)
		return
	}

	c.JSON(http.StatusOK, balance)
}

// GetLatestBlock retrieves the latest block information with improved validation
// @Summary      Get Latest Block
// @Description  Get information about the latest block on a blockchain
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon)
// @Success      200    {object} api.SwaggerBlockInfo
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/block/latest [get]
func (h *Handler) GetLatestBlock(c *gin.Context) {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get latest block
	block, err := h.clientManager.GetLatestBlock(ctx, chain)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get latest block", chain)
		return
	}

	c.JSON(http.StatusOK, block)
}

// GetTransaction retrieves transaction details with improved validation
// @Summary      Get Transaction Details
// @Description  Get detailed information about a specific transaction by its hash
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon)
// @Param        hash   path     string  true  "Transaction hash"
// @Success      200    {object} api.SwaggerTransactionInfo
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/tx/{hash} [get]
func (h *Handler) GetTransaction(c *gin.Context) {
	chain := c.Param("chain")
	hash := c.Param("hash")

	// Validate chain and hash together
	if validationErrors := h.validator.ValidateChainAndHash(chain, hash); validationErrors.HasErrors() {
		h.errorHandler.HandleValidationErrors(c, validationErrors)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get transaction
	tx, err := h.clientManager.GetTransaction(ctx, chain, hash)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get transaction", chain)
		return
	}

	c.JSON(http.StatusOK, tx)
}

// GetGasPrice retrieves the current gas price with improved validation
// @Summary      Get Gas Price
// @Description  Get the current gas price for EVM-compatible blockchains
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain  path     string  true  "Blockchain name" Enums(ethereum, polygon)
// @Success      200    {object} api.GasPriceResponse
// @Failure      400    {object} api.SwaggerErrorResponse
// @Failure      404    {object} api.SwaggerErrorResponse
// @Failure      500    {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/gas-price [get]
func (h *Handler) GetGasPrice(c *gin.Context) {
	chain := c.Param("chain")

	// Validate chain name
	if validationErr := h.validator.ValidateChainName(chain); validationErr.Message != "" {
		h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get gas price (now returns *big.Int directly)
	gasPrice, err := h.clientManager.GetGasPrice(ctx, chain)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get gas price", chain)
		return
	}

	// Convert to hex string
	gasPriceHex := fmt.Sprintf("0x%x", gasPrice)

	// Format the response
	c.JSON(http.StatusOK, GasPriceResponse{
		Chain:       chain,
		GasPrice:    gasPrice.String(),
		GasPriceHex: gasPriceHex,
	})
}

// GetTransactionCount retrieves the transaction count with improved validation
// @Summary      Get Transaction Count
// @Description  Get the number of transactions for a specific address on a blockchain
// @Tags         chains
// @Accept       json
// @Produce      json
// @Param        chain    path     string  true  "Blockchain name" Enums(ethereum, bitcoin, polygon)
// @Param        address  path     string  true  "Wallet address"
// @Success      200      {object} api.TransactionCountResponse
// @Failure      400      {object} api.SwaggerErrorResponse
// @Failure      404      {object} api.SwaggerErrorResponse
// @Failure      500      {object} api.SwaggerErrorResponse
// @Router       /api/v1/chains/{chain}/address/{address}/nonce [get]
func (h *Handler) GetTransactionCount(c *gin.Context) {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate chain and address together
	if validationErrors := h.validator.ValidateChainAndAddress(chain, address); validationErrors.HasErrors() {
		h.errorHandler.HandleValidationErrors(c, validationErrors)
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.apiConfig.GetTimeout("default"))
	defer cancel()

	// Get transaction count (now returns uint64 directly)
	count, err := h.clientManager.GetTransactionCount(ctx, chain, address)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get transaction count", chain)
		return
	}

	// Format the response
	c.JSON(http.StatusOK, TransactionCountResponse{
		Chain:            chain,
		Address:          address,
		TransactionCount: count,
		Nonce:            fmt.Sprintf("0x%x", count),
	})
}

// GetCoinMarkets handles requests for coin market data with improved validation
// @Summary      Get Coin Markets
// @Description  Get a list of cryptocurrency markets with pagination and sorting
// @Tags         markets
// @Accept       json
// @Produce      json
// @Param        limit      query     string  true  "Number of items per page"
// @Param        offset     query     string  true  "Page number"
// @Param        orderBy    query     string  true  "Field to order by" Enums(market_cap_rank, price, volume_24h, price_change_percentage_24h)
// @Param        sortDirection query     string  true  "Sort direction" Enums(asc, desc)
// @Success      200          {object} api.MarketDataResponse
// @Failure      400          {object} api.SwaggerErrorResponse
// @Failure      500          {object} api.SwaggerErrorResponse
// @Router       /api/v1/markets [get]
func (h *Handler) GetCoinMarkets(c *gin.Context) {
	// Parse and validate pagination parameters
	limitStr := c.DefaultQuery("limit", strconv.Itoa(h.apiConfig.DefaultPageLimit))
	offsetStr := c.DefaultQuery("offset", "0")
	orderBy := c.DefaultQuery("orderBy", "market_cap_rank")
	sortDirection := c.DefaultQuery("sortDirection", "asc")

	limit, err := strconv.Atoi(limitStr)
	if err != nil {
		h.errorHandler.RespondWithError(c, http.StatusBadRequest, "INVALID_LIMIT",
			"Invalid limit parameter")
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil {
		h.errorHandler.RespondWithError(c, http.StatusBadRequest, "INVALID_OFFSET",
			"Invalid offset parameter")
		return
	}

	// Validate pagination parameters
	if validationErrors := h.validator.ValidatePaginationParams(limit, offset); validationErrors.HasErrors() {
		h.errorHandler.HandleValidationErrors(c, validationErrors)
		return
	}

	// Validate and normalize limit
	limit = h.apiConfig.ValidatePageLimit(limit)

	// Validate orderBy field
	if validationErr := h.validator.ValidateOrderBy(orderBy, h.apiConfig.AllowedOrderByFields); validationErr.Message != "" {
		h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		return
	}

	// Validate sort direction
	if validationErr := h.validator.ValidateSortDirection(sortDirection); validationErr.Message != "" {
		h.errorHandler.HandleValidationErrors(c, validation.ValidationErrors{validationErr})
		return
	}

	// Fetch market data
	markets, total, latestFetchTime, err := h.marketDataService.GetMarketDataFromDB(c.Request.Context(), limit, offset, orderBy, sortDirection)
	if err != nil {
		h.errorHandler.HandleGenericError(c, err, "retrieve market data")
		return
	}

	c.JSON(http.StatusOK, MarketDataResponse{
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
