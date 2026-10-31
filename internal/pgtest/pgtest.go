// Package pgtest is the minimal real-PostgreSQL fixture shared by integration tests.
// Tests are skipped unless GATEWAY_TEST_DATABASE_URL points at a disposable server.
package pgtest

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// EnvURL names the environment variable holding the test server's connection URL.
const EnvURL = "GATEWAY_TEST_DATABASE_URL"

// URL returns the configured connection URL, skipping the test when it is not set.
func URL(t testing.TB) string {
	t.Helper()
	value := os.Getenv(EnvURL)
	if value == "" {
		t.Skipf("%s is not set; skipping PostgreSQL integration test", EnvURL)
	}
	return value
}

// Open returns a connection pool whose connections all use a fresh, empty schema.
// The schema and the pool are removed when the test finishes.
func Open(t testing.TB) *sql.DB {
	t.Helper()
	base := URL(t)

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("failed to generate schema name: %v", err)
	}
	schema := "gw_test_" + hex.EncodeToString(suffix)

	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("failed to open admin connection: %v", err)
	}
	if _, err := admin.Exec("CREATE SCHEMA " + schema); err != nil {
		admin.Close()
		t.Fatalf("failed to create test schema: %v", err)
	}

	pool, err := sql.Open("postgres", withSearchPath(t, base, schema))
	if err != nil {
		admin.Close()
		t.Fatalf("failed to open test pool: %v", err)
	}
	if err := pool.Ping(); err != nil {
		pool.Close()
		admin.Close()
		t.Fatalf("failed to connect to the test schema: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		if _, err := admin.Exec("DROP SCHEMA " + schema + " CASCADE"); err != nil {
			t.Errorf("failed to drop test schema %s: %v", schema, err)
		}
		admin.Close()
	})
	return pool
}

// withSearchPath adds a search_path runtime parameter to a postgres:// URL or a key=value string.
func withSearchPath(t testing.TB, base, schema string) string {
	t.Helper()
	parsed, err := url.Parse(base)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return base + " search_path=" + schema
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
