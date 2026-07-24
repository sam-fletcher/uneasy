package gametest

import (
	"fmt"

	"uneasy/model"
)

// seedConfig accumulates the optional knobs applied by Option functions. The
// zero value (modulo the per-phase default current_row) means "mirror the real
// phase-transition handler with no overrides".
type seedConfig struct {
	currentRow int16

	// boardSetup controls whether seedBase lays down the Public Record board
	// (record rows, current_row, focus player). True for phases that begin at
	// or after the Main Event; false for pre-Main-Event phases (the prologue),
	// which have no board yet — advanceToMainEvent creates it, and a
	// pre-seeded board would make that transition dup-key. Not an Option: it's
	// intrinsic to the phase, set by the Seed* function, not the caller.
	boardSetup bool

	// rankings overrides, keyed by category. Each value is a slice of player
	// indices in status order (highest status first): value[k] is the index
	// (into usernames) of the player awarded the k-th open rank slot for the
	// player count (game.OpenRanks). A nil entry falls back to seat order.
	rankings map[model.RankingCategory][]int

	plans []SeedPlan

	// laws / rumors to write onto the public record. Empty by default (opt in
	// via WithLaw / WithRumor); a seeded law is signed by the first player, a
	// seeded rumor sourced to the last. Neither carries an origin plan.
	laws   []string
	rumors []string

	// startingMarginalia gives each seeded starting asset one marginalia at
	// position 1. Off by default so Go fixtures keep all four slots free (tests
	// add their own notes without colliding); the dev seed turns it on, since a
	// blank asset can't clear the prologue closing gate.
	startingMarginalia bool

	// Shake-up-only knobs (ignored by SeedMainEvent).
	shakeUpTokens int16  // per-player grant; 0 => none (mirrors BeginShakeUp)
	shakeUpStep   *int16 // nil => rolling (step 1)
}

// Option mutates a seedConfig. Pass any number to a Seed* function.
type Option func(*seedConfig)

// SeedPlan describes a plan inserted directly onto the board. PreparerIdx is an
// index into the usernames slice; Row/RowOrder place it on the public record;
// PlanType and Category must be valid enum values.
type SeedPlan struct {
	PreparerIdx int
	PlanType    model.PlanType
	Category    model.RankingCategory
	Row         int16
	RowOrder    int16
}

// WithCurrentRow overrides the game's current_row. Default is 1 for main_event
// and 13 for shake_up.
func WithCurrentRow(row int16) Option {
	return func(c *seedConfig) { c.currentRow = row }
}

// WithRankings overrides one category's ranking order. orderByIdx[k] is the
// player index awarded the k-th open rank slot for the player count (highest
// status first); it must be a permutation of 0..N-1.
func WithRankings(cat model.RankingCategory, orderByIdx []int) Option {
	return func(c *seedConfig) {
		if c.rankings == nil {
			c.rankings = map[model.RankingCategory][]int{}
		}
		c.rankings[cat] = orderByIdx
	}
}

// WithPlan inserts a plan onto the board (via CreatePlan). Repeatable.
func WithPlan(p SeedPlan) Option {
	return func(c *seedConfig) { c.plans = append(c.plans, p) }
}

// WithLaw writes a law onto the public record, signed by the first player
// (seat 1). Repeatable. No origin plan, so the LawsRumors byline shows just
// "Signed by <player>". Handy for populating the laws/rumors UI in a seed.
func WithLaw(text string) Option {
	return func(c *seedConfig) { c.laws = append(c.laws, text) }
}

// WithRumor writes a rumor onto the public record, sourced to the last player.
// Repeatable. No origin plan, so the byline shows "Spread by <player>".
func WithRumor(text string) Option {
	return func(c *seedConfig) { c.rumors = append(c.rumors, text) }
}

// WithStartingMarginalia gives every seeded starting asset one marginalia at
// position 1. Opt in when the fixture must satisfy the prologue closing gate
// (which refuses Ready while any owned asset is blank) or when a break/tear
// flow needs something to target. Leaving it off keeps all four slots free,
// which is what most tests want.
func WithStartingMarginalia() Option {
	return func(c *seedConfig) { c.startingMarginalia = true }
}

// WithShakeUpTokens grants each player n shake-up tokens. Only meaningful for
// SeedShakeUp — a freshly-entered shake-up has zero tokens (players earn them
// by rolling), so set this to reach the spending step in tests.
func WithShakeUpTokens(n int16) Option {
	return func(c *seedConfig) { c.shakeUpTokens = n }
}

// WithShakeUpStep overrides the shake-up step (game.ShakeUpStepRolling /
// game.ShakeUpStepSpending). Only meaningful for SeedShakeUp.
func WithShakeUpStep(step int16) Option {
	return func(c *seedConfig) { c.shakeUpStep = &step }
}

// applyOptions builds a seedConfig from the per-phase default row plus opts.
// boardSetup defaults on (the Main-Event / Shake-Up phases have a board); the
// prologue seed turns it off after this.
func applyOptions(defaultRow int16, opts []Option) seedConfig {
	c := seedConfig{currentRow: defaultRow, boardSetup: true}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// validate checks the config is structurally sound for n players. This is a
// shape check (permutations, row bounds, indices) — NOT a guarantee that the
// resulting board is one the game engine could actually reach. Faithful
// reachability is the job of the play-forward layer / engine decoupling, not
// the seed.
func (c *seedConfig) validate(n int) error {
	if c.currentRow < 1 || c.currentRow > 13 {
		return fmt.Errorf("current_row %d out of range 1..13", c.currentRow)
	}
	for cat, order := range c.rankings {
		if err := validatePermutation(order, n); err != nil {
			return fmt.Errorf("rankings[%s]: %w", cat, err)
		}
	}
	for i, p := range c.plans {
		if p.PreparerIdx < 0 || p.PreparerIdx >= n {
			return fmt.Errorf("plans[%d]: preparer index %d out of range 0..%d", i, p.PreparerIdx, n-1)
		}
		if p.Row < 1 || p.Row > 13 {
			return fmt.Errorf("plans[%d]: row %d out of range 1..13", i, p.Row)
		}
	}
	return nil
}

// validatePermutation checks order is a permutation of 0..n-1.
func validatePermutation(order []int, n int) error {
	if len(order) != n {
		return fmt.Errorf("expected %d entries, got %d", n, len(order))
	}
	seen := make([]bool, n)
	for _, idx := range order {
		if idx < 0 || idx >= n {
			return fmt.Errorf("index %d out of range 0..%d", idx, n-1)
		}
		if seen[idx] {
			return fmt.Errorf("index %d repeated", idx)
		}
		seen[idx] = true
	}
	return nil
}
