package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "uneasy/db/gen"
)

// notAPool is a DBTX that is not a pool — standing in for the pgx.Tx that
// InTx hands its callback.
type notAPool struct{}

func (notAPool) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (notAPool) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil }
func (notAPool) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func TestPoolBacked(t *testing.T) {
	// A typed nil pool is enough: PoolBacked looks at the handle's type, never
	// dereferences it.
	if !PoolBacked(dbgen.New((*pgxpool.Pool)(nil))) {
		t.Error("a pool-backed Queries should report true")
	}
	if PoolBacked(dbgen.New(notAPool{})) {
		t.Error("a non-pool handle should report false")
	}
	if PoolBacked(nil) {
		t.Error("nil should report false")
	}
}
