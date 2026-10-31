package database_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dvictor357/blockchain-gateway/internal/pgtest"
	"github.com/dvictor357/blockchain-gateway/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const repoMigrationsDir = "../../migrations"

func writeMigrations(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, contents := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600))
	}
	return dir
}

func initialMigration(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repoMigrationsDir, "001_create_coin_markets_table.sql"))
	require.NoError(t, err)
	return string(contents)
}

func applied(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query("SELECT version FROM schema_migrations ORDER BY version")
	require.NoError(t, err)
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		require.NoError(t, rows.Scan(&version))
		versions = append(versions, version)
	}
	require.NoError(t, rows.Err())
	return versions
}

func exists(t *testing.T, db *sql.DB, relation string) bool {
	t.Helper()
	var found bool
	require.NoError(t, db.QueryRow("SELECT to_regclass($1) IS NOT NULL", relation).Scan(&found))
	return found
}

func columnExists(t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()
	var found bool
	require.NoError(t, db.QueryRow(`SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2)`, table, column).Scan(&found))
	return found
}

func TestRunMigrations_AppliesRepositoryMigrationsOnceAndIsIdempotent(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()

	require.NoError(t, database.RunMigrations(ctx, db, repoMigrationsDir))
	require.NoError(t, database.RunMigrations(ctx, db, repoMigrationsDir))

	assert.True(t, exists(t, db, "coin_markets"))
	assert.Equal(t, []string{"001_create_coin_markets_table.sql"}, applied(t, db))
}

func TestRunMigrations_AdoptsDatabaseCreatedBeforeTracking(t *testing.T) {
	db := pgtest.Open(t)
	_, err := db.Exec(initialMigration(t)) // what the old table-existence logic left behind
	require.NoError(t, err)
	_, err = db.Exec("INSERT INTO coin_markets (id, symbol, name) VALUES ('bitcoin', 'btc', 'Bitcoin')")
	require.NoError(t, err)
	require.False(t, exists(t, db, "schema_migrations"))

	require.NoError(t, database.RunMigrations(context.Background(), db, repoMigrationsDir))

	assert.Equal(t, []string{"001_create_coin_markets_table.sql"}, applied(t, db))
	var name string
	require.NoError(t, db.QueryRow("SELECT name FROM coin_markets WHERE id = 'bitcoin'").Scan(&name))
	assert.Equal(t, "Bitcoin", name, "existing data must survive adoption")
}

func TestRunMigrations_LaterMigrationsRunExactlyOnce(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	dir := writeMigrations(t, map[string]string{
		"001_create_coin_markets_table.sql": initialMigration(t),
	})
	require.NoError(t, database.RunMigrations(ctx, db, dir))

	// ADD COLUMN is not idempotent, so a second execution would fail.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "002_add_note.sql"),
		[]byte("ALTER TABLE coin_markets ADD COLUMN note TEXT;"), 0o600))
	require.NoError(t, database.RunMigrations(ctx, db, dir))
	require.NoError(t, database.RunMigrations(ctx, db, dir))

	assert.True(t, columnExists(t, db, "coin_markets", "note"))
	assert.Equal(t, []string{"001_create_coin_markets_table.sql", "002_add_note.sql"}, applied(t, db))
}

func TestRunMigrations_ConcurrentRunnersApplyEachMigrationOnce(t *testing.T) {
	db := pgtest.Open(t)
	dir := writeMigrations(t, map[string]string{
		"001_create_coin_markets_table.sql": initialMigration(t),
		"002_add_note.sql":                  "ALTER TABLE coin_markets ADD COLUMN note TEXT;",
	})

	const runners = 8
	var wg sync.WaitGroup
	errs := make([]error, runners)
	start := make(chan struct{})
	for i := range runners {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = database.RunMigrations(context.Background(), db, dir)
		}()
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		assert.NoError(t, err, "runner %d", i)
	}
	assert.Equal(t, []string{"001_create_coin_markets_table.sql", "002_add_note.sql"}, applied(t, db))
}

func TestRunMigrations_FailedMigrationLeavesNoPartialChanges(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()
	dir := writeMigrations(t, map[string]string{
		"001_create_coin_markets_table.sql": initialMigration(t),
		"002_broken.sql":                    "ALTER TABLE coin_markets ADD COLUMN half_done TEXT; SELECT * FROM table_that_does_not_exist;",
	})

	err := database.RunMigrations(ctx, db, dir)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "002_broken.sql")
	assert.False(t, columnExists(t, db, "coin_markets", "half_done"), "DDL from the failed migration must roll back")
	assert.Equal(t, []string{"001_create_coin_markets_table.sql"}, applied(t, db), "earlier migrations stay applied")

	// Fixing the file lets the same database proceed.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "002_broken.sql"),
		[]byte("ALTER TABLE coin_markets ADD COLUMN half_done TEXT;"), 0o600))
	require.NoError(t, database.RunMigrations(ctx, db, dir))
	assert.True(t, columnExists(t, db, "coin_markets", "half_done"))
}

func TestRunMigrations_OrdersByNumberNotByName(t *testing.T) {
	db := pgtest.Open(t)
	// Lexically "10_" sorts before "9_"; 10 depends on 9.
	dir := writeMigrations(t, map[string]string{
		"9_create_things.sql": "CREATE TABLE things (id INT);",
		"10_alter_things.sql": "ALTER TABLE things ADD COLUMN label TEXT;",
	})

	require.NoError(t, database.RunMigrations(context.Background(), db, dir))

	assert.True(t, columnExists(t, db, "things", "label"))
}

func TestRunMigrations_RejectsUnusableMigrationDirectories(t *testing.T) {
	db := pgtest.Open(t)
	ctx := context.Background()

	tests := map[string]string{
		"missing directory":         filepath.Join(t.TempDir(), "absent"),
		"directory without scripts": writeMigrations(t, map[string]string{"README.txt": "not a migration"}),
		"unnumbered script":         writeMigrations(t, map[string]string{"create_tables.sql": "SELECT 1;"}),
		"duplicate version":         writeMigrations(t, map[string]string{"1_a.sql": "SELECT 1;", "01_b.sql": "SELECT 1;"}),
	}
	for name, dir := range tests {
		assert.Error(t, database.RunMigrations(ctx, db, dir), name)
	}
	assert.False(t, exists(t, db, "schema_migrations"), "nothing may be applied from a rejected directory")
}

func TestRunMigrations_CancelledContextAppliesNothing(t *testing.T) {
	db := pgtest.Open(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	assert.Error(t, database.RunMigrations(ctx, db, repoMigrationsDir))
	assert.False(t, exists(t, db, "coin_markets"))
}

func TestConnectDB_HandlesPasswordsWithSpacesQuotesAndBackslashes(t *testing.T) {
	baseURL := pgtest.URL(t)
	parsed, err := url.Parse(baseURL)
	require.NoError(t, err, "this test needs a postgres:// URL")

	admin, err := sql.Open("postgres", baseURL)
	require.NoError(t, err)
	defer admin.Close()

	suffix := make([]byte, 6)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	role := "gw_role_" + hex.EncodeToString(suffix)
	const password = `p'a\ss "w ord`
	if _, err := admin.Exec(`CREATE ROLE ` + role + ` LOGIN PASSWORD 'p''a\ss "w ord'`); err != nil {
		t.Skipf("cannot create a test role (needs CREATEROLE): %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec("DROP ROLE " + role) })

	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	sslMode := parsed.Query().Get("sslmode")
	if sslMode == "" {
		sslMode = "disable"
	}
	cfg := database.DBConfig{
		Host: parsed.Hostname(), Port: port, User: role, Password: password,
		DBName: parsed.Path[1:], SSLMode: sslMode,
	}

	wrong := cfg
	wrong.Password = "wrong"
	if db, err := database.ConnectDB(context.Background(), wrong); err == nil {
		db.Close()
		t.Skip("the server does not enforce password authentication for this client, so credentials cannot be verified")
	}

	db, err := database.ConnectDB(context.Background(), cfg)
	require.NoError(t, err, "a password containing a space, quote and backslash must authenticate")
	defer db.Close()
	var current string
	require.NoError(t, db.QueryRow("SELECT current_user").Scan(&current))
	assert.Equal(t, role, current)
}

func TestConnectDB_FailsFastForUnreachableServer(t *testing.T) {
	// Port 1 on loopback refuses connections immediately; no DNS or network is involved.
	db, err := database.ConnectDB(context.Background(), database.DBConfig{
		Host: "127.0.0.1", Port: "1", User: "u", Password: "p", DBName: "d", SSLMode: "disable",
	})

	assert.Error(t, err)
	assert.Nil(t, db)
}
