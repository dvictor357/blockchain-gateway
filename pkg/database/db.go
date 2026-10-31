package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

const (
	connectTimeout  = 10 * time.Second
	maxOpenConns    = 10
	maxIdleConns    = 5
	connMaxLifetime = 30 * time.Minute
	connMaxIdleTime = 5 * time.Minute

	// migrationLockID is the PostgreSQL advisory lock key that serializes migration runners.
	migrationLockID int64 = 7_330_501_872_145
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

// connectionString builds a libpq keyword/value connection string. Every value is single-quoted
// with backslashes and quotes escaped, so spaces and quotes in passwords are safe.
func connectionString(cfg DBConfig) string {
	quote := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	pairs := []struct{ key, value string }{
		{"host", cfg.Host},
		{"port", cfg.Port},
		{"user", cfg.User},
		{"password", cfg.Password},
		{"dbname", cfg.DBName},
		{"sslmode", cfg.SSLMode},
	}

	parts := make([]string, 0, len(pairs)+1)
	for _, pair := range pairs {
		if pair.value == "" {
			continue
		}
		parts = append(parts, pair.key+"='"+quote.Replace(pair.value)+"'")
	}
	parts = append(parts, "connect_timeout="+strconv.Itoa(int(connectTimeout/time.Second)))
	return strings.Join(parts, " ")
}

// ConnectDB establishes a bounded connection pool to the PostgreSQL database. The initial
// connectivity check is limited by ctx and by a built-in connect timeout.
func ConnectDB(ctx context.Context, cfg DBConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", connectionString(cfg))
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(connMaxLifetime)
	db.SetConnMaxIdleTime(connMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()
	if err = db.PingContext(pingCtx); err != nil {
		db.Close() // Close the connection if ping fails
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to the database")
	return db, nil
}

var migrationFileRegex = regexp.MustCompile(`^(\d+)_.+\.sql$`)

type migration struct {
	version uint64
	name    string
	sql     string
}

// loadMigrations reads the numbered *.sql files in dir in ascending numeric order.
// Files must not contain their own transaction control: each runs inside a transaction.
func loadMigrations(dir string) ([]migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory %s: %w", dir, err)
	}

	var migrations []migration
	seen := make(map[uint64]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		match := migrationFileRegex.FindStringSubmatch(entry.Name())
		if match == nil {
			return nil, fmt.Errorf("migration file %q must be named <number>_<description>.sql", entry.Name())
		}
		version, err := strconv.ParseUint(match[1], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("migration file %q has an invalid number: %w", entry.Name(), err)
		}
		if other, dup := seen[version]; dup {
			return nil, fmt.Errorf("migration files %q and %q share version %d", other, entry.Name(), version)
		}
		seen[version] = entry.Name()

		contents, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read migration file %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents)})
	}

	if len(migrations) == 0 {
		return nil, fmt.Errorf("no migration files found in %s", dir)
	}
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}

// RunMigrations applies the pending SQL migrations from migrationsDir in order and records each
// in the schema_migrations table. Every migration runs in its own transaction guarded by a
// PostgreSQL advisory lock, so concurrent runners apply each migration exactly once and a failed
// migration leaves no partial changes. Databases created before tracking existed are handled by
// the idempotent initial script, which is re-run once and then recorded. ctx cancels lock waits.
func RunMigrations(ctx context.Context, db *sql.DB, migrationsDir string) error {
	migrations, err := loadMigrations(migrationsDir)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		applied, err := applyMigration(ctx, db, m)
		if err != nil {
			return fmt.Errorf("failed to apply migration %s: %w", m.name, err)
		}
		if applied {
			log.Printf("Successfully applied migration: %s", m.name)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) (bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	// The transaction-scoped lock is released on commit or rollback, including connection loss.
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", migrationLockID); err != nil {
		return false, fmt.Errorf("failed to acquire migration lock: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`); err != nil {
		return false, err
	}

	var alreadyApplied bool
	if err := tx.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)", m.name).Scan(&alreadyApplied); err != nil {
		return false, err
	}
	if alreadyApplied {
		return false, tx.Commit()
	}

	if _, err := tx.ExecContext(ctx, m.sql); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", m.name); err != nil {
		return false, err
	}
	return true, tx.Commit()
}
