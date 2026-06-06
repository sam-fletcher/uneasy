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
//
// Each Seed* function builds a common skeleton via seedBase, then applies a
// phase-specific tail that mirrors the real transition handler (SeedMainEvent
// ↔ advanceToMainEvent; SeedShakeUp ↔ BeginShakeUp). Optional knobs are passed
// as Option values (see options.go) so callers that don't need them — and the
// existing call sites — stay unchanged.
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
// player; rankings default to 1..N in input order.
type SeededGame struct {
	Game    dbgen.Game
	Players []dbgen.Player
}

// SeedMainEvent creates a game in phase main_event with the given account
// usernames seated as players. Accounts are looked up by username and
// created if missing (mirroring DevLogin).
//
// Invariants of the returned game (with no options):
//   - phase = main_event, current_row = 1
//   - public_record_rows seeded
//   - power/knowledge/esteem rankings 1..N in input order
//   - player[0] is facilitator and focus_player_id
//
// Options (WithCurrentRow, WithRankings, WithPlan) override the defaults.
//
// Returns an error rather than panicking so callers can choose between
// require.NoError (tests) and HTTP error responses (dev seed handler).
func SeedMainEvent(ctx context.Context, q *dbgen.Queries, usernames []string, opts ...Option) (SeededGame, error) {
	cfg := applyOptions(1, opts)
	seeded, err := seedBase(ctx, q, usernames, cfg)
	if err != nil {
		return SeededGame{}, err
	}
	if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: seeded.Game.ID, Phase: model.PhaseMainEvent,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set phase: %w", err)
	}
	return reload(ctx, q, seeded)
}

// seedBase builds the phase-agnostic skeleton shared by every Seed* function:
// game, players (seat order 1..N), facilitator, rankings, public record rows,
// current_row, focus player. It does NOT set the game phase — that is the
// caller's phase-specific tail. The returned SeededGame holds the freshly
// created (pre-phase) game row; callers reload after setting the phase.
func seedBase(ctx context.Context, q *dbgen.Queries, usernames []string, cfg seedConfig) (SeededGame, error) {
	n := len(usernames)
	if n < 2 || n > 5 {
		return SeededGame{}, fmt.Errorf("need 2..5 usernames, got %d", n)
	}
	if err := cfg.validate(n); err != nil {
		return SeededGame{}, err
	}

	code, err := db.GenerateJoinCode()
	if err != nil {
		return SeededGame{}, fmt.Errorf("generate join code: %w", err)
	}
	game, err := q.CreateGame(ctx, code)
	if err != nil {
		return SeededGame{}, fmt.Errorf("create game: %w", err)
	}

	players := make([]dbgen.Player, n)
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

	if err := seedRankings(ctx, q, game.ID, players, cfg); err != nil {
		return SeededGame{}, err
	}

	if err := q.CreatePublicRecordRows(ctx, game.ID); err != nil {
		return SeededGame{}, fmt.Errorf("create record rows: %w", err)
	}
	if err := q.SetCurrentRow(ctx, dbgen.SetCurrentRowParams{
		ID: game.ID, CurrentRow: cfg.currentRow,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set current row: %w", err)
	}
	if err := q.SetFocusPlayer(ctx, dbgen.SetFocusPlayerParams{
		ID: game.ID, FocusPlayerID: &players[0].ID,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set focus player: %w", err)
	}

	if err := seedPlans(ctx, q, game.ID, players, cfg); err != nil {
		return SeededGame{}, err
	}

	return SeededGame{Game: game, Players: players}, nil
}

// seedRankings writes all three tracks. For each category the rank slot k+1 is
// assigned to player order[k] (default: seat order). UpsertRanking keys the
// (game, category, rank) slot, so iterating distinct ranks with distinct
// players fills each track exactly once.
func seedRankings(ctx context.Context, q *dbgen.Queries, gameID int64, players []dbgen.Player, cfg seedConfig) error {
	for _, cat := range []model.RankingCategory{
		model.CategoryPower, model.CategoryKnowledge, model.CategoryEsteem,
	} {
		order := cfg.rankings[cat] // nil unless overridden
		for pos := range players {
			idx := pos
			if order != nil {
				idx = order[pos]
			}
			pid := players[idx].ID
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: gameID, PlayerID: &pid, Category: cat, Rank: int16(pos + 1),
			}); err != nil {
				return fmt.Errorf("upsert ranking %s rank %d: %w", cat, pos+1, err)
			}
		}
	}
	return nil
}

// seedPlans inserts each configured plan directly on the board.
func seedPlans(ctx context.Context, q *dbgen.Queries, gameID int64, players []dbgen.Player, cfg seedConfig) error {
	for i, sp := range cfg.plans {
		row := sp.Row
		if _, err := q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:        gameID,
			PlanType:      sp.PlanType,
			Category:      sp.Category,
			PreparerID:    players[sp.PreparerIdx].ID,
			RowNumber:     &row,
			RowOrder:      sp.RowOrder,
			PreparedAtRow: cfg.currentRow,
		}); err != nil {
			return fmt.Errorf("create plan %d: %w", i, err)
		}
	}
	return nil
}

// reload refreshes the game row (e.g. after the phase was set) and returns the
// updated SeededGame.
func reload(ctx context.Context, q *dbgen.Queries, seeded SeededGame) (SeededGame, error) {
	refreshed, err := q.GetGameByID(ctx, seeded.Game.ID)
	if err != nil {
		return SeededGame{}, fmt.Errorf("reload game: %w", err)
	}
	seeded.Game = refreshed
	return seeded, nil
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
