package handler

// dice_roll_guard.go — the "one open interactive roll per game" invariant.
//
// The app assumes a single in-flight dice roll at a time (the table page tracks
// one activeRoll; GetActiveRollForGame returns the latest still-open roll). Two
// players racing to start a roll must not both succeed. Enforcement is layered:
//
//   - uq_one_open_roll_per_game (migration 035) makes a second open roll
//     impossible at the DB level, even under a true simultaneous race.
//   - gameHasOpenRoll is the friendly pre-check that returns a clean 409 in the
//     common (non-racing) case so callers don't rely on catching a constraint
//     violation for ordinary control flow.
//
// Map a lost race to openRollBusyMsg via isUniqueViolation(err, openRollConstraint).

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	dbgen "uneasy/db/gen"
)

const (
	openRollConstraint = "uq_one_open_roll_per_game"
	openRollBusyMsg    = "a dice roll is already in progress — wait for it to resolve, then try again"
)

// isUniqueViolation reports whether err is a Postgres unique-constraint
// violation (SQLSTATE 23505) for the named constraint/index.
func isUniqueViolation(err error, constraint string) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == "23505" && pgErr.ConstraintName == constraint
	}
	return false
}

// gameHasOpenRoll reports whether the game already has an in-flight interactive
// roll (shake-up rolls excluded — see GetOpenRollByGame).
func gameHasOpenRoll(ctx context.Context, q *dbgen.Queries, gameID int64) (bool, error) {
	if _, err := q.GetOpenRollByGame(ctx, gameID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// blockIfOpenRoll writes a 409 and returns true if the game already has an
// in-flight interactive roll. On a DB error it writes a 500 and returns true so
// the caller stops. Use as the friendly pre-check before creating a roll.
func blockIfOpenRoll(ctx context.Context, w http.ResponseWriter, r *http.Request, q *dbgen.Queries, gameID int64) bool {
	open, err := gameHasOpenRoll(ctx, q, gameID)
	if err != nil {
		respondInternalErr(w, r, "could not check for an in-progress roll", err)
		return true
	}
	if open {
		respondErr(w, http.StatusConflict, openRollBusyMsg)
		return true
	}
	return false
}
