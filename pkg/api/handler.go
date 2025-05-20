package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/user/blockchain-gateway/pkg/blockchain"
	"github.com/user/blockchain-gateway/pkg/marketdata"
)

var (
	allowedOrderBy = map[string]bool{
		"market_cap_rank": true,
		"name":            true,
		"current_price":   true,
		"last_updated":    true,
		"data_fetched_at": true,
	}
)

// Handler manages API requests
type Handler struct {
	clientManager     *blockchain.ClientManager
	logger            *log.Logger
	marketDataService *marketdata.Service
}

// NewHandler creates a new API handler
func NewHandler(clientManager *blockchain.ClientManager, logger *log.Logger, marketService *marketdata.Service) *Handler {
	return &Handler{
		clientManager:     clientManager,
		logger:            logger,
		marketDataService: marketService,
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

// QueryChain handles RPC queries to a specific blockchain
func (h *Handler) QueryChain(c *gin.Context) {
	// Extract chain name from URL parameters
	chain := c.Param("chain")

	// Parse the request body
	var request blockchain.RPCRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Execute the RPC request
	result, err := h.clientManager.Execute(ctx, chain, request.Method, request.Params)
	if err != nil {
		// Handle specific errors
		switch err {
		case blockchain.ErrChainNotSupported:
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		case blockchain.ErrInvalidRequest:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid RPC request"})
		case blockchain.ErrRPCTimeout:
			c.JSON(http.StatusGatewayTimeout, gin.H{"error": "RPC request timed out"})
		default:
			// Log the detailed error but return a generic message to the client
			h.logger.Printf("Error executing RPC request for %s: %v", chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute RPC request"})
		}
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

// BatchQuery handles batch RPC requests, potentially across multiple blockchains
func (h *Handler) BatchQuery(c *gin.Context) {
	// Parse the request body
	var batchRequest map[string][]blockchain.RPCRequest
	if err := c.ShouldBindJSON(&batchRequest); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid batch request format"})
		return
	}

	// Validate the request
	if len(batchRequest) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Batch request must include at least one blockchain"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	// Execute batch request
	results, err := h.clientManager.BatchExecute(ctx, batchRequest)
	if err != nil {
		h.logger.Printf("Error executing batch request: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to execute batch request"})
		return
	}

	c.JSON(http.StatusOK, results)
}

// GetBalance retrieves the balance for an address on the specified blockchain
func (h *Handler) GetBalance(c *gin.Context) {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate parameters
	if chain == "" || address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chain and address are required"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get balance
	balance, err := h.clientManager.GetBalance(ctx, chain, address)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrChainNotSupported):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		default:
			h.logger.Printf("Error getting balance for %s on %s: %v", address, chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get balance: %v", err)})
		}
		return
	}

	c.JSON(http.StatusOK, balance)
}

// GetLatestBlock retrieves the latest block information from the specified blockchain
func (h *Handler) GetLatestBlock(c *gin.Context) {
	chain := c.Param("chain")

	// Validate parameters
	if chain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chain is required"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get latest block
	block, err := h.clientManager.GetLatestBlock(ctx, chain)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrChainNotSupported):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		default:
			h.logger.Printf("Error getting latest block for %s: %v", chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get latest block: %v", err)})
		}
		return
	}

	c.JSON(http.StatusOK, block)
}

// GetTransaction retrieves transaction details for the specified transaction hash
func (h *Handler) GetTransaction(c *gin.Context) {
	chain := c.Param("chain")
	hash := c.Param("hash")

	// Validate parameters
	if chain == "" || hash == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chain and transaction hash are required"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get transaction
	tx, err := h.clientManager.GetTransaction(ctx, chain, hash)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrChainNotSupported):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		case strings.Contains(err.Error(), "not found"):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Transaction not found: %s", hash)})
		default:
			h.logger.Printf("Error getting transaction %s on %s: %v", hash, chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get transaction: %v", err)})
		}
		return
	}

	c.JSON(http.StatusOK, tx)
}

// GetGasPrice retrieves the current gas price for EVM-compatible chains
func (h *Handler) GetGasPrice(c *gin.Context) {
	chain := c.Param("chain")

	// Validate parameters
	if chain == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chain is required"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get gas price
	gasPrice, err := h.clientManager.GetGasPrice(ctx, chain)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrChainNotSupported):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		case strings.Contains(err.Error(), "not applicable"):
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Gas price not applicable for blockchain: %s", chain)})
		default:
			h.logger.Printf("Error getting gas price for %s: %v", chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get gas price: %v", err)})
		}
		return
	}

	// Format the response
	c.JSON(http.StatusOK, gin.H{
		"chain":         chain,
		"gas_price":     gasPrice.String(),
		"gas_price_hex": fmt.Sprintf("0x%x", gasPrice),
	})
}

// GetTransactionCount retrieves the number of transactions sent from an address
func (h *Handler) GetTransactionCount(c *gin.Context) {
	chain := c.Param("chain")
	address := c.Param("address")

	// Validate parameters
	if chain == "" || address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Chain and address are required"})
		return
	}

	// Create a context with timeout
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	// Get transaction count
	count, err := h.clientManager.GetTransactionCount(ctx, chain, address)
	if err != nil {
		switch {
		case errors.Is(err, blockchain.ErrChainNotSupported):
			c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("Blockchain '%s' not supported", chain)})
		default:
			h.logger.Printf("Error getting transaction count for %s on %s: %v", address, chain, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to get transaction count: %v", err)})
		}
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

// GetCoinMarkets handles requests for coin market data
func (h *Handler) GetCoinMarkets(c *gin.Context) {
	// Default pagination and sorting parameters
	defaultLimit := 20
	defaultOffset := 0
	defaultOrderBy := "market_cap_rank"
	defaultSortDirection := "asc"

	limitStr := c.DefaultQuery("limit", strconv.Itoa(defaultLimit))
	offsetStr := c.DefaultQuery("offset", strconv.Itoa(defaultOffset))
	orderBy := c.DefaultQuery("orderBy", defaultOrderBy)
	sortDirection := c.DefaultQuery("sortDirection", defaultSortDirection)

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = defaultLimit
	}
	// Max limit to prevent abuse
	if limit > 100 {
		limit = 100
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = defaultOffset
	}

	// Basic validation for orderBy and sortDirection (more robust validation can be added)
	if !allowedOrderBy[strings.ToLower(orderBy)] {
		orderBy = defaultOrderBy
	}
	if strings.ToLower(sortDirection) != "asc" && strings.ToLower(sortDirection) != "desc" {
		sortDirection = defaultSortDirection
	}

	markets, total, latestFetchTime, err := h.marketDataService.GetMarketDataFromDB(c.Request.Context(), limit, offset, orderBy, sortDirection)
	if err != nil {
		h.logger.Printf("Error fetching market data from service: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve market data"})
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
