package db

// fanout.go — can this Queries handle run statements concurrently?

import (
	"reflect"

	"github.com/jackc/pgx/v5/pgxpool"

	dbgen "uneasy/db/gen"
)

// PoolBacked reports whether q issues its statements through a connection
// pool, which is what makes concurrent calls on it safe: each statement
// checks out its own connection. A transaction-bound q (the one InTx hands
// its callback) runs everything on one connection, which pgx refuses to
// share across goroutines ("conn busy"), so callers that fan reads out must
// fall back to running them one after another on such a handle.
//
// sqlc generates Queries with no accessor for its handle, so this reads the
// unexported field reflectively. Any surprise — a renamed field, a nil
// handle, a type that isn't the pool — answers false: the sequential path is
// always correct, only slower. TestPoolBacked pins both answers so a change
// in the generated shape shows up as a failing test, not as silently serial
// reads.
func PoolBacked(q *dbgen.Queries) bool {
	if q == nil {
		return false
	}
	f := reflect.ValueOf(q).Elem().FieldByName("db")
	if !f.IsValid() || f.Kind() != reflect.Interface || f.IsNil() {
		return false
	}
	return f.Elem().Type() == reflect.TypeFor[*pgxpool.Pool]()
}
