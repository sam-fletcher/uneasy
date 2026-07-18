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
	"uneasy/game"
	"uneasy/model"
)

// SeededGame is what every seed function returns: the game row plus the
// players in input order. Player[0] is always the facilitator and focus
// player; rankings default to the open rank slots for the player count, in
// input order (see seedRankings).
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
//   - power/knowledge/esteem: real players on the open rank slots for the
//     player count (e.g. ranks 2,4 for 2 players), dummy tokens on the rest
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

// SeedPrologueClosing creates a game parked at the prologue's closing step
// ("The Stage is Set"; phase = prologue, prologue_ranking_step = closing) with
// the given usernames seated as players.
//
// Invariants of the returned game (with no options):
//   - phase = prologue, prologue_ranking_step = closing
//   - power/knowledge/esteem rankings seeded (so the recap's final-standings
//     board renders), same spread as SeedMainEvent
//   - each player holds their four starting assets (main character + one of
//     each type), so the retinue tallies render
//   - NO Public Record board, focus player, or closing-ready rows — this is a
//     pre-Main-Event game, so an actual all-ready advance can run
//     advanceToMainEvent cleanly (no dup-key board insert)
//
// Unlike a game that reached closing by playing the prologue, this skips the
// choosing flow: there are no prologue claims/cards, main characters carry
// their seeded names (not the "[Main Character]" placeholder), and no laws or
// rumors exist. It's a fixture for eyeballing / driving the closing UI, not a
// faithful mid-prologue board. The WithRankings option still applies.
func SeedPrologueClosing(
	ctx context.Context,
	q *dbgen.Queries,
	usernames []string,
	opts ...Option,
) (SeededGame, error) {
	cfg := applyOptions(1, opts)
	cfg.boardSetup = false // the prologue has no Public Record board yet
	seeded, err := seedBase(ctx, q, usernames, cfg)
	if err != nil {
		return SeededGame{}, err
	}
	if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: seeded.Game.ID, Phase: model.PhasePrologue,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set phase: %w", err)
	}
	step := game.PrologueStepClosing
	if err := q.SetPrologueRankingStep(ctx, dbgen.SetPrologueRankingStepParams{
		ID: seeded.Game.ID, PrologueRankingStep: &step,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set prologue step: %w", err)
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

	// The Public Record board (rows, current_row, focus player) only exists
	// from the Main Event onward. Pre-Main-Event phases (the prologue) skip it:
	// advanceToMainEvent lays it down, and a pre-seeded board would dup-key.
	if cfg.boardSetup {
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
	}

	if err := seedStartingAssets(ctx, q, game.ID, players); err != nil {
		return SeededGame{}, err
	}

	if err := seedPlans(ctx, q, game.ID, players, cfg); err != nil {
		return SeededGame{}, err
	}

	if err := seedLawsRumors(ctx, q, game.ID, players, cfg); err != nil {
		return SeededGame{}, err
	}

	return SeededGame{Game: game, Players: players}, nil
}

// seedStartingAssets gives every player one asset of each type: a peer (flagged
// as their main character), a holding, an artifact, and a resource.
//
// The main character preserves the invariant production guarantees via the
// prologue — each player creates exactly one main character — which
// ComputeRowState now relies on (a player with no main character is treated as
// owing a replacement choice). The three non-MC assets aren't part of that
// invariant; they exist so seeded games are realistic enough to exercise plans
// whose preparation or resolution needs a holding/artifact/resource to point at
// (stakes, targets, transfers). A single peer per player is not enough for most
// plan testing.
//
// All four start with no marginalia: the prologue explicitly allows assets to
// start blank ("you don't need to fill these all the way out yet"), and leaving
// positions 1–4 free lets tests add their own marginalia without colliding.
func seedStartingAssets(ctx context.Context, q *dbgen.Queries, gameID int64, players []dbgen.Player) error {
	for i := range players {
		p := &players[i]
		if _, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
			GameID:          gameID,
			OwnerID:         p.ID,
			CreatorID:       p.ID,
			AssetType:       model.AssetPeer,
			Name:            fmt.Sprintf("%s's main character", p.DisplayName),
			IsMainCharacter: true,
		}); err != nil {
			return fmt.Errorf("seed main character for %q: %w", p.DisplayName, err)
		}
		for _, t := range []model.AssetType{
			model.AssetHolding, model.AssetArtifact, model.AssetResource,
		} {
			if _, err := q.CreateAsset(ctx, dbgen.CreateAssetParams{
				GameID:    gameID,
				OwnerID:   p.ID,
				CreatorID: p.ID,
				AssetType: t,
				Name:      fmt.Sprintf("%s's %s", p.DisplayName, t),
			}); err != nil {
				return fmt.Errorf("seed %s for %q: %w", t, p.DisplayName, err)
			}
		}
	}
	return nil
}

// seedRankings writes all three tracks as a *valid* spread: real players are
// placed on the open rank slots for the player count (game.OpenRanks), and the
// dummy slots (game.DummyRanks) are filled with nil-player rows. This mirrors
// the real prologue, where the fixed 1–5 track is padded with dummy tokens for
// <5-player games — so e.g. a 2-player game has real players at ranks 2 and 4,
// not 1 and 2.
//
// Within each category the open ranks are filled in player order[k] (default:
// seat order) from highest status (lowest open rank) down. UpsertRanking keys
// the (game, category, rank) slot, so iterating distinct ranks with distinct
// players fills each track exactly once.
func seedRankings(ctx context.Context, q *dbgen.Queries, gameID int64, players []dbgen.Player, cfg seedConfig) error {
	open := game.OpenRanks(len(players))
	dummies := game.DummyRanks(len(players))
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
			rank := open[pos]
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: gameID, PlayerID: &pid, Category: cat, Rank: rank,
			}); err != nil {
				return fmt.Errorf("upsert ranking %s rank %d: %w", cat, rank, err)
			}
		}
		for _, rank := range dummies {
			if err := q.UpsertRanking(ctx, dbgen.UpsertRankingParams{
				GameID: gameID, PlayerID: nil, Category: cat, Rank: rank,
			}); err != nil {
				return fmt.Errorf("upsert dummy ranking %s rank %d: %w", cat, rank, err)
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

// seedLawsRumors writes the configured laws and rumors onto the public record.
// Empty unless the caller opted in via WithLaw / WithRumor (the dev seed
// handler adds one of each so dev-seeded games aren't blank in the laws/rumors
// UI). A seeded law is signed by players[0]; a seeded rumor is sourced to the
// last player. Neither has an origin plan, so the LawsRumors byline falls back
// to the signatory / source (see lawAccent in LawsRumors.svelte). display_order
// follows insertion order, so seeded entries sort before anything later plans
// create.
func seedLawsRumors(ctx context.Context, q *dbgen.Queries, gameID int64, players []dbgen.Player, cfg seedConfig) error {
	signatory := players[0].ID
	for i, text := range cfg.laws {
		if _, err := q.CreateLaw(ctx, dbgen.CreateLawParams{
			GameID:       gameID,
			Text:         text,
			SignatoryID:  &signatory,
			DisplayOrder: int16(i),
		}); err != nil {
			return fmt.Errorf("seed law %d: %w", i, err)
		}
	}
	source := players[len(players)-1].ID
	for i, text := range cfg.rumors {
		if _, err := q.CreateRumor(ctx, dbgen.CreateRumorParams{
			GameID:         gameID,
			Text:           text,
			SourcePlayerID: &source,
			DisplayOrder:   int16(i),
		}); err != nil {
			return fmt.Errorf("seed rumor %d: %w", i, err)
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
		Username:     username,
		PasswordHash: string(hash),
	})
}
