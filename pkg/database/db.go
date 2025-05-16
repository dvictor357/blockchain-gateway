package database

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// DBConfig holds database connection parameters
type DBConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// ConnectDB establishes a connection to the PostgreSQL database
func ConnectDB(cfg DBConfig) (*sql.DB, error) {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if err = db.Ping(); err != nil {
		db.Close() // Close the connection if ping fails
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to the database")
	return db, nil
}

// RunMigrations executes SQL migration files from a specified directory.
// For simplicity in this project, it just checks for the existence of the main table
// and applies the first migration if it doesn't exist. A more robust system
// would track applied migrations in a database table.
func RunMigrations(db *sql.DB, migrationsDir string) error {
	// Check if the main table already exists
	var tableName string
	err := db.QueryRow("SELECT to_regclass('public.coin_markets')").Scan(&tableName)

	if err != nil && err != sql.ErrNoRows { // Genuine error during check
		log.Printf("Error checking if 'coin_markets' table exists: %v", err)
		// If it's not sql.ErrNoRows, it could be a connection issue or other DB problem.
		// We might not want to proceed with migrations if the check itself fails badly.
		if !strings.Contains(err.Error(), "relation \"coin_markets\" does not exist") && !strings.Contains(err.Error(), "NULL") { // to_regclass returns NULL if not exists, which Scan might treat as ErrNoRows or convert to <nil> string
			return fmt.Errorf("error verifying 'coin_markets' table existence: %w", err)
		}
		// If it is sql.ErrNoRows or the specific string error, it means the table doesn't exist, so we proceed.
	}

	if tableName == "coin_markets" { // Check if scan found the table name
		log.Println("'coin_markets' table already exists. No migrations needed from initial script.")
		return nil
	}

	// If tableName is empty (or was nil from scan) or err was sql.ErrNoRows, the table likely doesn't exist.
	log.Println("'coin_markets' table does not exist or check was inconclusive, attempting to apply initial migration...")

	migrationFile := filepath.Join(migrationsDir, "001_create_coin_markets_table.sql")
	if _, err := os.Stat(migrationFile); os.IsNotExist(err) {
		return fmt.Errorf("migration file not found: %s", migrationFile)
	}

	sqlBytes, err := ioutil.ReadFile(migrationFile)
	if err != nil {
		return fmt.Errorf("failed to read migration file %s: %w", migrationFile, err)
	}

	sqlScript := string(sqlBytes)
	_, err = db.Exec(sqlScript)
	if err != nil {
		return fmt.Errorf("failed to execute migration script %s: %w", migrationFile, err)
	}

	log.Printf("Successfully applied migration: %s", migrationFile)
	return nil
}
