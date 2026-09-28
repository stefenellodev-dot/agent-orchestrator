// Package postgres implements the persistence boundary against PostgreSQL.
package postgres

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/stefenello/agent-orchestrator/internal/store"
)

// Store is the PostgreSQL-backed persistence boundary.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

var _ store.Store = (*Store)(nil)

// mapReadErr converts pgx.ErrNoRows into store.ErrNotFound.
func mapReadErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return store.ErrNotFound
	}
	return err
}

// mapWriteErr converts a unique-constraint violation into store.ErrProjectBusy.
func mapWriteErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return store.ErrProjectBusy
	}
	return err
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "null"
	}
	return string(b)
}

func unmarshal(raw []byte, v any) error {
	if len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, v)
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }
