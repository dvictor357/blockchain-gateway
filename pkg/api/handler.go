package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/marketdata"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
	"github.com/gin-gonic/gin"
)

// Handler manages API requests with improved error handling and validation
type Handler struct {
	clientManager     *blockchain.ClientManager
	logger            *log.Logger
	marketDataService *marketdata.Service
	validator         *validation.Validator
	errorHandler      *ErrorHandler
	apiConfig         *config.APIConfig
}

// NewHandler creates a new API handler with all dependencies
func NewHandler(clientManager *blockchain.ClientManager, logger *log.Logger, marketService *marketdata.Service) *Handler {
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
func (h *Handler) ListChains(c *gin.Context) {
	chains := h.clientManager.ListChains()

	c.JSON(http.StatusOK, gin.H{
		"chains": chains,
		"count":  len(chains),
	})
}

// QueryChain handles RPC queries to a specific blockchain with improved validation
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

	// Get gas price
	gasPrice, err := h.clientManager.GetGasPrice(ctx, chain)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get gas price", chain)
		return
	}

	// Format the response
	c.JSON(http.StatusOK, gin.H{
		"chain":         chain,
		"gas_price":     gasPrice.String(),
		"gas_price_hex": fmt.Sprintf("0x%x", gasPrice),
	})
}

// GetTransactionCount retrieves the transaction count with improved validation
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

	// Get transaction count
	count, err := h.clientManager.GetTransactionCount(ctx, chain, address)
	if err != nil {
		h.errorHandler.HandleBlockchainError(c, err, "get transaction count", chain)
		return
	}

	// Format the response
	c.JSON(http.StatusOK, gin.H{
		"chain":             chain,
		"address":           address,
		"transaction_count": count,
		"nonce":             fmt.Sprintf("0x%x", count),
	})
}

// GetCoinMarkets handles requests for coin market data with improved validation
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

	c.JSON(http.StatusOK, gin.H{
		"data": markets,
		"pagination": gin.H{
			"total_records": total,
			"limit":         limit,
			"offset":        offset,
			"current_page":  (offset / limit) + 1,
			"total_pages":   (total + limit - 1) / limit, // Ceiling division
		},
		"meta": gin.H{
			"last_data_update_from_source": latestFetchTime.Format(time.RFC3339Nano),
		},
	})
}
