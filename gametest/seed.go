// Package gametest holds shared fixtures for creating games in specific
// phases. Imported by:
//
//   - handler integration tests (gated by the integration build tag),
//     which wrap each call in require.NoError; and
//   - the dev-only POST /api/dev/seed handler, which exposes the same
//     fixtures over HTTP so the Playwright E2E suite can fast-forward
//     past phases it isn't currently testing.
//
// Keeping the implementation in one place means E2E and Go tests
// disagree about "what does a valid game in phase X look like" exactly
// once: when this file is wrong.
package gametest

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// SeededGame is what every seed function returns: the game row plus the
// players in input order. Player[0] is always the facilitator and focus
// player; rankings are 1..N in input order.
type SeededGame struct {
	Game    dbgen.Game
	Players []dbgen.Player
}

// SeedMainEvent creates a game in phase main_event with the given account
// usernames seated as players. Accounts are looked up by username and
// created if missing (mirroring DevLogin).
//
// Invariants of the returned game:
//   - phase = main_event, current_row = 1
//   - public_record_rows seeded
//   - power/knowledge/esteem rankings 1..N in input order
//   - player[0] is facilitator and focus_player_id
//
// Returns an error rather than panicking so callers can choose between
// require.NoError (tests) and HTTP error responses (dev seed handler).
func SeedMainEvent(ctx context.Context, q *dbgen.Queries, usernames []string) (SeededGame, error) {
	if n := len(usernames); n < 2 || n > 5 {
		return SeededGame{}, fmt.Errorf("need 2..5 usernames, got %d", n)
	}

	code, err := db.GenerateJoinCode()
	if err != nil {
		return SeededGame{}, fmt.Errorf("generate join code: %w", err)
	}
	game, err := q.CreateGame(ctx, code)
	if err != nil {
		return SeededGame{}, fmt.Errorf("create game: %w", err)
	}

	players := make([]dbgen.Player, len(usernames))
	for i, username := range usernames {
		acct, err := findOrCreateAccount(ctx, q, username)
		if err != nil {
			return SeededGame{}, fmt.Errorf("account %q: %w", username, err)
		}
		p, err := q.CreatePlayer(ctx, dbgen.CreatePlayerParams{
			GameID:        game.ID,
			DisplayName:   username,
			AccountID:     acct.ID,
			IsFacilitator: i == 0,
		})
		if err != nil {
			return SeededGame{}, fmt.Errorf("create player %q: %w", username, err)
		}
		seat := int16(i + 1)
		if err := q.SetPlayerSeatOrder(ctx, dbgen.SetPlayerSeatOrderParams{
			ID: p.ID, SeatOrder: &seat,
		}); err != nil {
			return SeededGame{}, fmt.Errorf("set seat order: %w", err)
		}
		p.SeatOrder = &seat
		players[i] = p
	}

	if err := q.SetFacilitator(ctx, dbgen.SetFacilitatorParams{
		FacilitatorID: &players[0].ID, ID: game.ID,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set facilitator: %w", err)
	}

	for _, cat := range []model.RankingCategory{
		model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem,
	} {
		for i, p := range players {
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID:   game.ID,
				PlayerID: &p.ID,
				Category: cat,
				Rank:     int16(i + 1),
			}); err != nil {
				return SeededGame{}, fmt.Errorf("upsert ranking %s p%d: %w", cat, i+1, err)
			}
		}
	}

	if err := q.CreatePublicRecordRows(ctx, game.ID); err != nil {
		return SeededGame{}, fmt.Errorf("create record rows: %w", err)
	}
	if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: game.ID, Phase: model.PhaseMainEvent,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set phase: %w", err)
	}
	if err := q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: game.ID, CurrentRow: 1,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set current row: %w", err)
	}
	if err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: game.ID, FocusPlayerID: &players[0].ID,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set focus player: %w", err)
	}

	refreshed, err := q.GetGameByID(ctx, game.ID)
	if err != nil {
		return SeededGame{}, fmt.Errorf("reload game: %w", err)
	}
	return SeededGame{Game: refreshed, Players: players}, nil
}

func findOrCreateAccount(ctx context.Context, q *dbgen.Queries, username string) (dbgen.Account, error) {
	acct, err := q.GetAccountByUsername(ctx, username)
	if err == nil {
		return acct, nil
	}
	// Any error (including pgx.ErrNoRows) — try to create. If the
	// underlying issue was something other than missing-row, the create
	// will surface a real error.
	hash, _ := bcrypt.GenerateFromPassword([]byte("dev"), bcrypt.MinCost)
	return q.CreateAccount(ctx, dbgen.CreateAccountParams{
		Username: username,
		CodeHash: string(hash),
	})
}
