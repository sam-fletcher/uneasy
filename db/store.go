package db

// store.go — DB handle bundling the pgx pool and the sqlc query set,
// plus the InTx helper handlers use for atomic multi-write sequences.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "uneasy/db/gen"
)

// Store bundles the connection pool and the sqlc query handle. Pool is
// exposed for callers that need a transaction; Q covers the read-only
// or single-write common case.
type Store struct {
	Pool *pgxpool.Pool
	Q    *dbgen.Queries
}

// NewStore builds a Store from a live pool.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Pool: pool, Q: dbgen.New(pool)}
}

// WithQ returns a copy of the Store with a different Queries handle (typically
// the transactional one provided by InTx). The pool is unchanged, so callers
// inside a transaction can still open nested transactions via InTx — though
// in pgx that would create a savepoint rather than a true nested tx.
func (s *Store) WithQ(q *dbgen.Queries) *Store {
	return &Store{Pool: s.Pool, Q: q}
}

// InTx runs fn inside a database transaction. The callback receives a
// transactional *dbgen.Queries; the q.WithTx ceremony is hidden. The
// transaction commits iff fn returns nil; otherwise it rolls back. The
// deferred rollback after a successful commit is a no-op in pgx.
//
// WebSocket broadcasts performed inside fn fire before commit returns;
// on the rare path where commit fails, clients may receive events that
// don't reflect committed state. This is acceptable given the alternative
// (partial state on every validation failure) is far more common.
func (s *Store) InTx(ctx context.Context, fn func(*dbgen.Queries) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := fn(s.Q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
