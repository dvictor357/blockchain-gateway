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

	"github.com/dvictor357/blockchain-gateway/pkg/api"
	"github.com/dvictor357/blockchain-gateway/pkg/blockchain"
	"github.com/dvictor357/blockchain-gateway/pkg/coingecko"
	"github.com/dvictor357/blockchain-gateway/pkg/config"
	"github.com/dvictor357/blockchain-gateway/pkg/database"
	"github.com/dvictor357/blockchain-gateway/pkg/marketdata"
	"github.com/joho/godotenv"
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

	router := api.NewRouter(apiHandler, logger, appConfig.Server)

	addr := fmt.Sprintf("%s:%s", appConfig.Server.Host, appConfig.Server.Port)
	readTimeout := 15 * time.Second
	writeTimeout := 15 * time.Second
	idleTimeout := 60 * time.Second
	if appConfig.Server.Mode == "debug" {
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
