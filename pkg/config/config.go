package config

import (
	"os"
	"strconv"
)

// Constants for default values
const (
	defaultPort           = "8080"
	defaultHost           = "0.0.0.0"
	defaultGinMode        = "release"
	defaultLogLevel       = "info"
	defaultRequestTimeout = 30

	defaultDBHost     = "localhost"
	defaultDBPort     = "5432"
	defaultDBUser     = "postgres"
	defaultDBPassword = ""
	defaultDBName     = "blockchain_gateway"
	defaultDBSSLMode  = "disable"

	defaultCoinGeckoBaseURL    = "https://api.coingecko.com/api/v3"
	defaultCoinGeckoPerPage    = 100
	defaultCoinGeckoOrder      = "market_cap_desc"
	defaultCoinGeckoVsCurrency = "usd"

	migrationsDir = "migrations"
)

// ServerConfig holds server-specific configurations
type ServerConfig struct {
	Port      string
	Host      string
	GinMode   string
	RateLimit int
	// RequestTimeout time.Duration // For http.Server, if needed directly here
}

// DatabaseConfig holds database connection parameters
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// CoinGeckoConfig holds CoinGecko API client configurations
type CoinGeckoConfig struct {
	BaseURL    string
	PerPage    int
	Order      string
	VsCurrency string
	// Add API Key if needed in future
}

// AppConfig holds all application configurations
type AppConfig struct {
	Server        ServerConfig
	Database      DatabaseConfig
	CoinGecko     CoinGeckoConfig
	LogLevel      string
	MigrationsDir string
}

// LoadConfig loads application configurations from environment variables with defaults
func LoadConfig() *AppConfig {
	cfg := &AppConfig{
		Server: ServerConfig{
			Port:      GetStringEnv("PORT", defaultPort),
			Host:      defaultHost,
			GinMode:   GetStringEnv("GIN_MODE", defaultGinMode),
			RateLimit: GetIntEnv("RATE_LIMIT", 120),
		},
		Database: DatabaseConfig{
			Host:     GetStringEnv("DB_HOST", defaultDBHost),
			Port:     GetStringEnv("DB_PORT", defaultDBPort),
			User:     GetStringEnv("DB_USER", defaultDBUser),
			Password: GetStringEnv("DB_PASSWORD", defaultDBPassword),
			DBName:   GetStringEnv("DB_NAME", defaultDBName),
			SSLMode:  GetStringEnv("DB_SSLMODE", defaultDBSSLMode),
		},
		CoinGecko: CoinGeckoConfig{
			BaseURL:    GetStringEnv("COINGECKO_BASE_URL", defaultCoinGeckoBaseURL),
			PerPage:    GetIntEnv("COINGECKO_PER_PAGE", defaultCoinGeckoPerPage),
			Order:      GetStringEnv("COINGECKO_ORDER", defaultCoinGeckoOrder),
			VsCurrency: GetStringEnv("COINGECKO_VS_CURRENCY", defaultCoinGeckoVsCurrency),
		},
		LogLevel:      GetStringEnv("LOG_LEVEL", defaultLogLevel),
		MigrationsDir: migrationsDir, // Static for now
	}
	return cfg
}

// Helper function to get string environment variable or default
func GetStringEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// Helper function to get integer environment variable or default
func GetIntEnv(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}
	value, err := strconv.Atoi(valueStr)
	if err != nil {
		// Log or handle error appropriately, for now, fallback to default
		// log.Printf("Warning: Invalid value for %s: %s. Using default: %d", key, valueStr, defaultValue)
		return defaultValue
	}
	return value
}
