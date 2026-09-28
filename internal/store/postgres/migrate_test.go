package postgres_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/store/postgres"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("ORCHESTRATOR_TEST_DSN")
	if dsn == "" {
		t.Skip("ORCHESTRATOR_TEST_DSN not set; skipping PostgreSQL tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool
}

func resetDB(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		DROP TABLE IF EXISTS approvals, gates, events, sessions, work_items, schema_migrations CASCADE;
	`)
	require.NoError(t, err)
}

func tableExists(t *testing.T, pool *pgxpool.Pool, name string) bool {
	t.Helper()
	var exists bool
	err := pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema='public' AND table_name=$1)`,
		name).Scan(&exists)
	require.NoError(t, err)
	return exists
}

func TestMigrate_CreatesTablesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testPool(t)
	resetDB(t, pool)

	require.NoError(t, postgres.Migrate(ctx, pool))
	// Running again must not error.
	require.NoError(t, postgres.Migrate(ctx, pool))

	for _, table := range []string{"work_items", "sessions", "gates", "approvals", "events", "schema_migrations"} {
		assert.True(t, tableExists(t, pool, table), "table %s should exist", table)
	}
}
