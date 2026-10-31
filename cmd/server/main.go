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
	"errors"
	"fmt"
	"io/fs"
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

const (
	// responseWriteMargin is added to the longest handler budget to form the server write timeout.
	responseWriteMargin = 10 * time.Second
	shutdownTimeout     = 30 * time.Second
	migrationTimeout    = 60 * time.Second
)

func main() {
	logger := log.New(os.Stdout, "[BLOCKCHAIN-GATEWAY] ", log.LstdFlags|log.Lshortfile)
	if err := run(logger); err != nil {
		logger.Fatalf("%v", err)
	}
}

// run starts the service and blocks until shutdown. It returns only after the market worker
// has stopped and the database is closed, so callers may exit the process afterwards.
func run(logger *log.Logger) error {
	// A .env file is optional: environment-only startup is supported. A file that exists but
	// cannot be read or parsed is still an error.
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("error loading .env file: %w", err)
	}

	appConfig := config.LoadConfig()
	if err := appConfig.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

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

	db, err := database.ConnectDB(context.Background(), dbConnectionCfg)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	// Deferred first so it runs last: the market worker is stopped before the database closes.
	defer db.Close()

	migrationCtx, cancelMigration := context.WithTimeout(context.Background(), migrationTimeout)
	err = database.RunMigrations(migrationCtx, db, appConfig.MigrationsDir)
	cancelMigration()
	if err != nil {
		return fmt.Errorf("failed to run database migrations: %w", err)
	}
	logger.Println("Database migrations completed successfully.")

	clientManager, err := blockchain.NewClientManager(appConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize blockchain client manager: %w", err)
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

	// The worker stops on every exit path, and its loop is awaited before the database closes.
	workerCtx, cancelWorker := context.WithCancel(context.Background())
	workerDone := make(chan struct{})
	if appConfig.CoinGecko.FetcherEnabled {
		go func() {
			defer close(workerDone)
			marketServ.StartFetchingLoop(workerCtx)
		}()
	} else {
		logger.Println("Market data fetcher disabled (MARKET_FETCHER_ENABLED=false); serving stored data only.")
		close(workerDone)
	}
	defer func() {
		cancelWorker()
		<-workerDone
	}()

	apiConfig := config.NewAPIConfigFromServer(appConfig.Server)
	apiHandler := api.NewHandler(clientManager, logger, marketServ, apiConfig)

	router := api.NewRouter(apiHandler, logger, appConfig.Server)

	addr := fmt.Sprintf("%s:%s", appConfig.Server.Host, appConfig.Server.Port)
	readTimeout := 15 * time.Second
	idleTimeout := 60 * time.Second
	if appConfig.Server.Mode == "debug" {
		readTimeout = 30 * time.Second
		idleTimeout = 120 * time.Second
	}

	server := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       readTimeout,
		// Must outlast the longest handler budget so a slow-but-successful response can still be written.
		WriteTimeout: apiConfig.LongestTimeout() + responseWriteMargin,
		IdleTimeout:  idleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Printf("Server listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var runErr error
	select {
	case <-quit:
		logger.Println("Shutting down server...")
	case err := <-serverErr:
		runErr = fmt.Errorf("server failed: %w", err)
	}

	// Stop the fetcher as draining begins; its deferred wait above completes before db.Close.
	cancelWorker()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Printf("Server did not drain cleanly, closing connections: %v", err)
		server.Close()
	} else {
		logger.Println("Server gracefully stopped")
	}

	return runErr
}
