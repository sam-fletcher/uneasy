package gametest

import (
	"context"
	"fmt"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// SeedShakeUp creates a game in phase shake_up, mirroring the real BeginShakeUp
// transition (handler/shake_up.go): assets refreshed, tokens zeroed, the
// rolling step opened on the esteem category. current_row defaults to 13 (the
// shake-up follows the final public-record row).
//
// A freshly-entered shake-up has ZERO tokens — players earn them by rolling in
// step 1. To exercise the spending step (step 2) in tests, pass
// WithShakeUpTokens(n) and WithShakeUpStep(game.ShakeUpStepSpending).
//
// The skeleton (players, seat order, rankings, public record) and the shared
// options (WithCurrentRow, WithRankings, WithPlan) come from seedBase.
func SeedShakeUp(ctx context.Context, q *dbgen.Queries, usernames []string, opts ...Option) (SeededGame, error) {
	cfg := applyOptions(13, opts)
	seeded, err := seedBase(ctx, q, usernames, cfg)
	if err != nil {
		return SeededGame{}, err
	}
	gameID := seeded.Game.ID

	// Mirror BeginShakeUp's tail.
	if err := q.RefreshAllAssets(ctx, gameID); err != nil {
		return SeededGame{}, fmt.Errorf("refresh assets: %w", err)
	}
	if err := q.ZeroShakeUpTokens(ctx, gameID); err != nil {
		return SeededGame{}, fmt.Errorf("zero tokens: %w", err)
	}

	cat := gamepkg.ShakeUpCategoryEsteem
	step := gamepkg.ShakeUpStepRolling
	if cfg.shakeUpStep != nil {
		step = *cfg.shakeUpStep
	}
	if err := q.SetShakeUpStep(ctx, dbgen.SetShakeUpStepParams{
		ID: gameID, ShakeUpCategory: &cat, ShakeUpStep: &step,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set shake-up step: %w", err)
	}

	if cfg.shakeUpTokens > 0 {
		for _, p := range seeded.Players {
			if _, err := q.AddShakeUpTokens(ctx, dbgen.AddShakeUpTokensParams{
				ID: p.ID, ShakeUpTokens: cfg.shakeUpTokens,
			}); err != nil {
				return SeededGame{}, fmt.Errorf("grant tokens to player %d: %w", p.ID, err)
			}
		}
	}

	if err := q.SetGamePhase(ctx, dbgen.SetGamePhaseParams{
		ID: gameID, Phase: model.PhaseShakeUp,
	}); err != nil {
		return SeededGame{}, fmt.Errorf("set phase: %w", err)
	}
	return reload(ctx, q, seeded)
}
