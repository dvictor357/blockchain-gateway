// @title           Blockchain Gateway API
// @version         1.0
// @description     A high-performance Go API gateway for interacting with multiple blockchain networks through a unified interface.
// @description
// @description     This API provides simplified access to various blockchain RPC endpoints through a single, consistent interface.
// @description     It abstracts away the differences between blockchain implementations, allowing developers to focus on building their applications.
//
// @contact.name   API Support
// @contact.url    https://github.com/dvictor357/blockchain-gateway
// @contact.email  support@example.com
//
// @license.name  MIT
// @license.url   https://opensource.org/licenses/MIT
//
// @host      localhost:8080
// @BasePath  /
//
// @schemes   http https
//
// @tag.name health
// @tag.description Health check operations
//
// @tag.name chains
// @tag.description Blockchain operations and queries
//
// @tag.name markets
// @tag.description Cryptocurrency market data operations
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/dvictor357/blockchain-gateway/docs"
	"github.com/dvictor357/blockchain-gateway/pkg/api"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/coingecko"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/database"
	"github.com/dvictor357/blockchain-gateway/pkg/marketdata"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Fatal("Error loading .env file")
	}

	appConfig := config.LoadConfig()

	logger := log.New(os.Stdout, "[BLOCKCHAIN-GATEWAY] ", log.LstdFlags|log.Lshortfile)
	logger.Printf("Starting blockchain RPC gateway service with log level: %s...", appConfig.LogLevel)

	startupMessage := `
   ____  _            _        _           _         ______      _
  |  _ \| |          | |      | |         (_)       / _____)    | |
  | |_) | | ___   ___| |  ____| |__  _____ _ ____  | /  ___  ___| |_ _____
  |  _ <| |/ _ \ / __| | / ___)  _ \(____ | |  _ \ | | (___)/ _ |  _) ___ |
  | |_) ) | |_| | (__| |( (___| | | / ___ | | | | || \____( |_| | |_| ____|
  |____/|_|\___/ \___)\_)____)_| |__\_____|_|_| |_| \_____/ \___|\___)____)
	`
	logger.Println(startupMessage)

	dbConnectionCfg := database.DBConfig{
		Host:     appConfig.Database.Host,
		Port:     appConfig.Database.Port,
		User:     appConfig.Database.User,
		Password: appConfig.Database.Password,
		DBName:   appConfig.Database.DBName,
		SSLMode:  appConfig.Database.SSLMode,
	}

	db, err := database.ConnectDB(dbConnectionCfg)
	if err != nil {
		logger.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := database.RunMigrations(db, appConfig.MigrationsDir); err != nil {
		logger.Fatalf("Failed to run database migrations: %v", err)
	}
	logger.Println("Database migrations completed successfully.")

	clientManager, err := blockchain.NewClientManager(appConfig)
	if err != nil {
		logger.Fatalf("Failed to initialize blockchain client manager: %v", err)
	}

	cgClient := coingecko.NewClient(nil, appConfig.CoinGecko.BaseURL)
	marketRepo := marketdata.NewPostgresMarketRepository(db)

	marketServiceConfig := marketdata.ServiceConfig{
		CoinGeckoVsCurrency:      appConfig.CoinGecko.VsCurrency,
		CoinGeckoOrder:           appConfig.CoinGecko.Order,
		CoinGeckoPerPage:         appConfig.CoinGecko.PerPage,
		CoinGeckoPage:            1,
		CoinGeckoSparkline:       false,
		CoinGeckoPriceChangePerc: []string{"24h"},
	}
	marketServ := marketdata.NewService(logger, cgClient, marketRepo, marketServiceConfig)

	appCtx, cancelAppCtx := context.WithCancel(context.Background())
	defer cancelAppCtx()

	go marketServ.StartFetchingLoop(appCtx)

	apiHandler := api.NewHandler(clientManager, logger, marketServ)

	gin.SetMode(appConfig.Server.GinMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(api.LoggingMiddleware(logger))
	router.Use(api.RateLimit(appConfig.Server.RateLimit))

	router.GET("/health", healthCheckHandler)

	// Swagger documentation endpoint
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	if gin.Mode() == gin.DebugMode {
		router.Use(gin.Logger())
		logger.Println("Running in development mode (debug) with enhanced logging")

		router.GET("/debug/routes", func(c *gin.Context) {
			routes := []string{}
			for _, r := range router.Routes() {
				routes = append(routes, fmt.Sprintf("%s %s", r.Method, r.Path))
			}
			c.JSON(http.StatusOK, gin.H{
				"routes": routes,
				"count":  len(routes),
				"mode":   gin.Mode(),
			})
		})
	}

	apiV1 := router.Group("/api/v1")
	{
		apiV1.GET("/chains", apiHandler.ListChains)
		apiV1.POST("/chains/:chain/query", apiHandler.QueryChain)
		apiV1.POST("/batch", apiHandler.BatchQuery)
		apiV1.GET("/chains/:chain/address/:address/balance", apiHandler.GetBalance)
		apiV1.GET("/chains/:chain/block/latest", apiHandler.GetLatestBlock)
		apiV1.GET("/chains/:chain/tx/:hash", apiHandler.GetTransaction)
		apiV1.GET("/chains/:chain/gas-price", apiHandler.GetGasPrice)
		apiV1.GET("/chains/:chain/address/:address/nonce", apiHandler.GetTransactionCount)
		apiV1.GET("/markets", apiHandler.GetCoinMarkets)
	}

	addr := fmt.Sprintf("%s:%s", appConfig.Server.Host, appConfig.Server.Port)
	readTimeout := 15 * time.Second
	writeTimeout := 15 * time.Second
	idleTimeout := 60 * time.Second
	if gin.Mode() == gin.DebugMode {
		readTimeout = 30 * time.Second
		writeTimeout = 30 * time.Second
		idleTimeout = 120 * time.Second
	}

	server := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	go func() {
		logger.Printf("Server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatalf("Failed to start server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Println("Shutting down server...")

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Fatalf("Server forced to shutdown: %v", err)
	}

	logger.Println("Server gracefully stopped")
}

// healthCheckHandler handles health check requests
// @Summary      Health Check
// @Description  Check if the API is running and healthy
// @Tags         health
// @Accept       json
// @Produce      json
// @Success      200  {object}  api.HealthResponse
// @Router       /health [get]
func healthCheckHandler(c *gin.Context) {
	c.JSON(http.StatusOK, api.HealthResponse{
		Status: "ok",
		Time:   time.Now().Format(time.RFC3339),
	})
}
