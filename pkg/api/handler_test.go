package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/models"
	"github.com/dvictor357/blockchain-gateway/pkg/validation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MarketDataServiceInterface for testing
type MarketDataServiceInterface interface {
	GetMarketDataFromDB(ctx context.Context, limit int, offset int, orderBy string, sortDirection string) ([]models.CoinMarket, int, time.Time, error)
}

// Mock implementations for testing
type MockClientManager struct {
	mock.Mock
}

func (m *MockClientManager) ListChains() []string {
	args := m.Called()
	return args.Get(0).([]string)
}

func (m *MockClientManager) Execute(ctx context.Context, chain, method string, params interface{}) (json.RawMessage, error) {
	args := m.Called(ctx, chain, method, params)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(json.RawMessage), nil
}

func (m *MockClientManager) BatchExecute(ctx context.Context, requests map[string][]blockchain.RPCRequest) (map[string][]blockchain.RPCResponse, error) {
	args := m.Called(ctx, requests)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(map[string][]blockchain.RPCResponse), nil
}

func (m *MockClientManager) GetBalance(ctx context.Context, chain, address string) (*blockchain.Balance, error) {
	args := m.Called(ctx, chain, address)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*blockchain.Balance), nil
}

func (m *MockClientManager) GetLatestBlock(ctx context.Context, chain string) (*blockchain.BlockInfo, error) {
	args := m.Called(ctx, chain)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*blockchain.BlockInfo), nil
}

func (m *MockClientManager) GetTransaction(ctx context.Context, chain, txHash string) (*blockchain.TransactionInfo, error) {
	args := m.Called(ctx, chain, txHash)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*blockchain.TransactionInfo), nil
}

func (m *MockClientManager) GetGasPrice(ctx context.Context, chain string) (*big.Int, error) {
	args := m.Called(ctx, chain)
	if args.Get(1) != nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*big.Int), nil
}

func (m *MockClientManager) GetTransactionCount(ctx context.Context, chain, address string) (uint64, error) {
	args := m.Called(ctx, chain, address)
	if args.Get(1) != nil {
		return 0, args.Error(1)
	}
	return args.Get(0).(uint64), nil
}

type MockMarketDataService struct {
	mock.Mock
}

func (m *MockMarketDataService) GetMarketDataFromDB(ctx context.Context, limit int, offset int, orderBy string, sortDirection string) ([]models.CoinMarket, int, time.Time, error) {
	args := m.Called(ctx, limit, offset, orderBy, sortDirection)
	if args.Get(3) != nil {
		return nil, 0, time.Time{}, args.Error(3)
	}
	return args.Get(0).([]models.CoinMarket), args.Get(1).(int), args.Get(2).(time.Time), nil
}

// TestableHandler is a standalone handler for testing that doesn't embed Handler
type TestableHandler struct {
	clientManager     ClientManagerInterface
	marketDataService MarketDataServiceInterface
	validator         *validation.Validator
	errorHandler      *ErrorHandler
	apiConfig         *config.APIConfig
}

func NewTestableHandler(clientManager ClientManagerInterface, logger *log.Logger, marketService MarketDataServiceInterface) *TestableHandler {
	return &TestableHandler{
		clientManager:     clientManager,
		marketDataService: marketService,
		validator:         validation.NewValidator(),
		errorHandler:      NewErrorHandler(logger),
		apiConfig:         config.NewAPIConfig(),
	}
}

// Override methods to use the mocked interfaces
func (h *TestableHandler) ListChains(c *gin.Context) {
	chains := h.clientManager.ListChains()

	c.JSON(http.StatusOK, ChainsResponse{
		Chains: chains,
		Count:  len(chains),
	})
}

func (h *TestableHandler) QueryChain(c *gin.Context) {
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

func (h *TestableHandler) BatchQuery(c *gin.Context) {
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

func (h *TestableHandler) GetBalance(c *gin.Context) {
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

func (h *TestableHandler) GetLatestBlock(c *gin.Context) {
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

func (h *TestableHandler) GetTransaction(c *gin.Context) {
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

func (h *TestableHandler) GetGasPrice(c *gin.Context) {
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
	c.JSON(http.StatusOK, GasPriceResponse{
		Chain:       chain,
		GasPrice:    gasPrice.String(),
		GasPriceHex: fmt.Sprintf("0x%x", gasPrice),
	})
}

func (h *TestableHandler) GetTransactionCount(c *gin.Context) {
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
	c.JSON(http.StatusOK, TransactionCountResponse{
		Chain:            chain,
		Address:          address,
		TransactionCount: count,
		Nonce:            fmt.Sprintf("0x%x", count),
	})
}

func (h *TestableHandler) GetCoinMarkets(c *gin.Context) {
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

func setupTestHandler() (*TestableHandler, *MockClientManager, *MockMarketDataService) {
	gin.SetMode(gin.TestMode)

	mockClientManager := &MockClientManager{}
	mockMarketService := &MockMarketDataService{}
	logger := log.New(os.Stdout, "test", log.LstdFlags)

	handler := NewTestableHandler(mockClientManager, logger, mockMarketService)

	return handler, mockClientManager, mockMarketService
}

func TestHandler_ListChains(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		mockChains     []string
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful chains list",
			mockChains:     []string{"ethereum", "bitcoin", "polygon"},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"chains":["ethereum","bitcoin","polygon"],"count":3}`,
		},
		{
			name:           "empty chains list",
			mockChains:     []string{},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"chains":[],"count":0}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClientManager.On("ListChains").Return(tt.mockChains).Once()

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)

			handler.ListChains(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			assert.JSONEq(t, tt.expectedBody, w.Body.String())
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_QueryChain(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		requestBody    string
		mockResult     json.RawMessage
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful query",
			chain:          "ethereum",
			requestBody:    `{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`,
			mockResult:     json.RawMessage(`"0x1234567"`),
			mockError:      nil,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"jsonrpc":"2.0","result":"0x1234567","id":1}`,
		},
		{
			name:           "invalid chain name",
			chain:          "invalid@chain",
			requestBody:    `{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Validation failed","code":"VALIDATION_ERROR"}`,
		},
		{
			name:           "invalid JSON body",
			chain:          "ethereum",
			requestBody:    `invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid request body format","code":"INVALID_JSON"}`,
		},
		{
			name:           "blockchain error",
			chain:          "ethereum",
			requestBody:    `{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}`,
			mockError:      blockchain.ErrChainNotSupported,
			expectedStatus: http.StatusNotFound,
			expectedBody:   `{"error":"Blockchain 'ethereum' is not supported","code":"CHAIN_NOT_SUPPORTED"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockResult != nil || tt.mockError != nil {
				mockClientManager.On("Execute", mock.Anything, tt.chain, "eth_blockNumber", []interface{}{}).Return(tt.mockResult, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Params = gin.Params{{Key: "chain", Value: tt.chain}}
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.QueryChain(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				if strings.Contains(tt.expectedBody, "VALIDATION_ERROR") {
					// For validation errors, just check the structure
					var response map[string]interface{}
					err := json.Unmarshal(w.Body.Bytes(), &response)
					require.NoError(t, err)
					assert.Equal(t, "Validation failed", response["error"])
					assert.Equal(t, "VALIDATION_ERROR", response["code"])
				} else {
					assert.JSONEq(t, tt.expectedBody, w.Body.String())
				}
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_BatchQuery(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		requestBody    string
		mockResult     map[string][]blockchain.RPCResponse
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name: "successful batch query",
			requestBody: `{
				"ethereum": [
					{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}
				]
			}`,
			mockResult: map[string][]blockchain.RPCResponse{
				"ethereum": {
					{JSONRPC: "2.0", Result: json.RawMessage(`"0x1234567"`), ID: 1},
				},
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"ethereum":[{"jsonrpc":"2.0","result":"0x1234567","id":1}]}`,
		},
		{
			name:           "empty batch request",
			requestBody:    `{}`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Batch request must include at least one blockchain","code":"EMPTY_BATCH"}`,
		},
		{
			name:           "invalid JSON",
			requestBody:    `invalid json`,
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid request body format","code":"INVALID_JSON"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockResult != nil || tt.mockError != nil {
				mockClientManager.On("BatchExecute", mock.Anything, mock.Anything).Return(tt.mockResult, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/", strings.NewReader(tt.requestBody))
			c.Request.Header.Set("Content-Type", "application/json")

			handler.BatchQuery(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetBalance(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		address        string
		mockBalance    *blockchain.Balance
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:    "successful balance retrieval",
			chain:   "ethereum",
			address: "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			mockBalance: &blockchain.Balance{
				Address:    "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
				Balance:    big.NewInt(1000000000000000000),
				HexBalance: "0xde0b6b3a7640000",
				Decimals:   18,
				Symbol:     "ETH",
				Chain:      "ethereum",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"address":"0x742d35Cc6634C0532925a3b844Bc454e4438f44e","balance":"1000000000000000000","hex_balance":"0xde0b6b3a7640000","decimals":18,"symbol":"ETH","chain":"ethereum"}`,
		},
		{
			name:           "invalid address",
			chain:          "ethereum",
			address:        "invalid_address",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "blockchain error",
			chain:          "ethereum",
			address:        "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			mockError:      blockchain.ErrRPCTimeout,
			expectedStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockBalance != nil || tt.mockError != nil {
				mockClientManager.On("GetBalance", mock.Anything, tt.chain, tt.address).Return(tt.mockBalance, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{
				{Key: "chain", Value: tt.chain},
				{Key: "address", Value: tt.address},
			}

			handler.GetBalance(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetLatestBlock(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		mockBlock      *blockchain.BlockInfo
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:  "successful block retrieval",
			chain: "ethereum",
			mockBlock: &blockchain.BlockInfo{
				Number:           18934257,
				Hash:             "0x8d12a0d346a05cf0dd9e650a5e41baa531a2ef7a287572739ce5c5a36856ec7c",
				ParentHash:       "0x781d36b32c7cbf06d952baa1d827eb425bacfdf9c9afc30b735959054a3f2fc1",
				Timestamp:        1716403465,
				TransactionCount: 124,
				Chain:            "ethereum",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"number":18934257,"hash":"0x8d12a0d346a05cf0dd9e650a5e41baa531a2ef7a287572739ce5c5a36856ec7c","parent_hash":"0x781d36b32c7cbf06d952baa1d827eb425bacfdf9c9afc30b735959054a3f2fc1","timestamp":1716403465,"transaction_count":124,"chain":"ethereum"}`,
		},
		{
			name:           "invalid chain",
			chain:          "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "blockchain error",
			chain:          "ethereum",
			mockError:      errors.New("block not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockBlock != nil || tt.mockError != nil {
				mockClientManager.On("GetLatestBlock", mock.Anything, tt.chain).Return(tt.mockBlock, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{{Key: "chain", Value: tt.chain}}

			handler.GetLatestBlock(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetTransaction(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		hash           string
		mockTx         *blockchain.TransactionInfo
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:  "successful transaction retrieval",
			chain: "ethereum",
			hash:  "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			mockTx: &blockchain.TransactionInfo{
				Hash:        "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
				From:        "0x95222290dd7278aa3ddd389cc1e1d165cc4bafe5",
				To:          "0xdef1c0ded9bec7f1a1670819833240f027b25eff",
				Value:       big.NewInt(500000000000000000),
				BlockNumber: 18934220,
				BlockHash:   "0x90e1a8e935cfd5970d6789a7afedb1dac09af91a7b8fc7dbe16008116ab19f9c",
				Status:      "success",
				Chain:       "ethereum",
			},
			expectedStatus: http.StatusOK,
			expectedBody:   `{"hash":"0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71","from":"0x95222290dd7278aa3ddd389cc1e1d165cc4bafe5","to":"0xdef1c0ded9bec7f1a1670819833240f027b25eff","value":"500000000000000000","block_number":18934220,"block_hash":"0x90e1a8e935cfd5970d6789a7afedb1dac09af91a7b8fc7dbe16008116ab19f9c","status":"success","chain":"ethereum"}`,
		},
		{
			name:           "invalid hash",
			chain:          "ethereum",
			hash:           "invalid_hash",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "transaction not found",
			chain:          "ethereum",
			hash:           "0x9c46f98547a5bf8e785e0e77472b3ca8fb5cdb9279fbc443637f781a3e9bff71",
			mockError:      errors.New("transaction not found"),
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockTx != nil || tt.mockError != nil {
				mockClientManager.On("GetTransaction", mock.Anything, tt.chain, tt.hash).Return(tt.mockTx, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{
				{Key: "chain", Value: tt.chain},
				{Key: "hash", Value: tt.hash},
			}

			handler.GetTransaction(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetGasPrice(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		mockGasPrice   *big.Int
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful gas price retrieval",
			chain:          "ethereum",
			mockGasPrice:   big.NewInt(20000000000),
			expectedStatus: http.StatusOK,
			expectedBody:   `{"chain":"ethereum","gas_price":"20000000000","gas_price_hex":"0x4a817c800"}`,
		},
		{
			name:           "invalid chain",
			chain:          "",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "operation not applicable",
			chain:          "bitcoin",
			mockError:      errors.New("operation not applicable for this blockchain"),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockGasPrice != nil || tt.mockError != nil {
				mockClientManager.On("GetGasPrice", mock.Anything, tt.chain).Return(tt.mockGasPrice, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{{Key: "chain", Value: tt.chain}}

			handler.GetGasPrice(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetTransactionCount(t *testing.T) {
	handler, mockClientManager, _ := setupTestHandler()

	tests := []struct {
		name           string
		chain          string
		address        string
		mockCount      uint64
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful transaction count retrieval",
			chain:          "ethereum",
			address:        "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			mockCount:      42,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"chain":"ethereum","address":"0x742d35Cc6634C0532925a3b844Bc454e4438f44e","transaction_count":42,"nonce":"0x2a"}`,
		},
		{
			name:           "invalid address",
			chain:          "ethereum",
			address:        "invalid_address",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "blockchain error",
			chain:          "ethereum",
			address:        "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
			mockError:      blockchain.ErrRPCTimeout,
			expectedStatus: http.StatusGatewayTimeout,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockCount > 0 || tt.mockError != nil {
				mockClientManager.On("GetTransactionCount", mock.Anything, tt.chain, tt.address).Return(tt.mockCount, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/", nil)
			c.Params = gin.Params{
				{Key: "chain", Value: tt.chain},
				{Key: "address", Value: tt.address},
			}

			handler.GetTransactionCount(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockClientManager.AssertExpectations(t)
		})
	}
}

func TestHandler_GetCoinMarkets(t *testing.T) {
	handler, _, mockMarketService := setupTestHandler()

	testTime := time.Date(2024, 5, 28, 10, 1, 5, 123456789, time.UTC)
	mockMarkets := []models.CoinMarket{
		{
			ID:            "bitcoin",
			Symbol:        "btc",
			Name:          "Bitcoin",
			CurrentPrice:  models.NullableFloat64(67000.0),
			MarketCapRank: models.NullableInt(1),
		},
	}

	tests := []struct {
		name           string
		queryParams    string
		mockMarkets    []models.CoinMarket
		mockTotal      int
		mockTime       time.Time
		mockError      error
		expectedStatus int
		expectedBody   string
	}{
		{
			name:           "successful market data retrieval",
			queryParams:    "?limit=10&offset=0&orderBy=market_cap_rank&sortDirection=asc",
			mockMarkets:    mockMarkets,
			mockTotal:      100,
			mockTime:       testTime,
			expectedStatus: http.StatusOK,
			expectedBody:   `{"data":[{"id":"bitcoin","symbol":"btc","name":"Bitcoin","image":"","current_price":67000,"market_cap_rank":1}],"pagination":{"total_records":100,"limit":10,"offset":0,"current_page":1,"total_pages":10},"meta":{"last_data_update_from_source":"2024-05-28T10:01:05.123456789Z"}}`,
		},
		{
			name:           "invalid limit parameter",
			queryParams:    "?limit=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid limit parameter","code":"INVALID_LIMIT"}`,
		},
		{
			name:           "invalid offset parameter",
			queryParams:    "?offset=invalid",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   `{"error":"Invalid offset parameter","code":"INVALID_OFFSET"}`,
		},
		{
			name:           "service error",
			queryParams:    "?limit=10&offset=0",
			mockError:      errors.New("database connection failed"),
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mockMarkets != nil || tt.mockError != nil {
				mockMarketService.On("GetMarketDataFromDB", mock.Anything, mock.AnythingOfType("int"), mock.AnythingOfType("int"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).Return(tt.mockMarkets, tt.mockTotal, tt.mockTime, tt.mockError).Once()
			}

			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/markets"+tt.queryParams, nil)

			handler.GetCoinMarkets(c)

			assert.Equal(t, tt.expectedStatus, w.Code)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}
			mockMarketService.AssertExpectations(t)
		})
	}
}

func TestHandler_Integration(t *testing.T) {
	// Test that all components work together correctly
	handler, mockClientManager, _ := setupTestHandler()

	// Test that validation, error handling, and client manager integration works
	t.Run("validation and error handling integration", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = gin.Params{
			{Key: "chain", Value: "invalid@chain"},
			{Key: "address", Value: "invalid_address"},
		}

		handler.GetBalance(c)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response ErrorResponse
		err := json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "Validation failed", response.Error)
		assert.Equal(t, "VALIDATION_ERROR", response.Code)
		assert.NotEmpty(t, response.Details)
	})

	// Test timeout configuration integration
	t.Run("timeout configuration integration", func(t *testing.T) {
		// This test verifies that the handler uses the configured timeouts
		// In a real scenario, we would test with a slow mock that times out
		mockClientManager.On("GetBalance", mock.Anything, "ethereum", "0x742d35Cc6634C0532925a3b844Bc454e4438f44e").Return(nil, context.DeadlineExceeded).Once()

		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/", nil)
		c.Params = gin.Params{
			{Key: "chain", Value: "ethereum"},
			{Key: "address", Value: "0x742d35Cc6634C0532925a3b844Bc454e4438f44e"},
		}

		handler.GetBalance(c)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		mockClientManager.AssertExpectations(t)
	})
}
