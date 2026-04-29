package game

// shake_up.go — Pure rules for the Shake-Up (Phase 4c).
//
// Per SHAKEUP_RULES.md and PHASE4_SPEC.md:
//
//   - Categories run in fixed order: esteem → knowledge → power.
//   - Each category has two steps: (1) every player rolls and gains tokens;
//     (2) players spend tokens in reverse rank order until everyone is out.
//   - Tokens reset between categories — only step-1 results of the *current*
//     category are spendable.
//   - Each option costs 1 base token. Other players may spend their own
//     tokens (1 per nudge) to adjust the cost ±1.
//   - Each category exposes 4 options.
//
// Option semantics (effects belong in handler/, dispatched by Apply funcs):
//   esteem:    take_peer, take_artifact, break_resource, bump_knowledge
//   knowledge: take_resource, break_holding, break_peer, bump_power
//   power:     take_holding, break_artifact, claim_title, bump_esteem

import "errors"

// Category constants. Stored in games.shake_up_category and
// shake_up_spends.category.
const (
	ShakeUpCategoryEsteem    = "esteem"
	ShakeUpCategoryKnowledge = "knowledge"
	ShakeUpCategoryPower     = "power"
)

// Step constants. Stored in games.shake_up_step (1 = rolling, 2 = spending).
const (
	ShakeUpStepRolling  int16 = 1
	ShakeUpStepSpending int16 = 2
)

// Option key constants. Stored in shake_up_spends.option_key.
const (
	// Esteem options
	ShakeUpOptTakePeer      = "take_peer"
	ShakeUpOptTakeArtifact  = "take_artifact"
	ShakeUpOptBreakResource = "break_resource"
	ShakeUpOptBumpKnowledge = "bump_knowledge"
	// Knowledge options
	ShakeUpOptTakeResource = "take_resource"
	ShakeUpOptBreakHolding = "break_holding"
	ShakeUpOptBreakPeer    = "break_peer"
	ShakeUpOptBumpPower    = "bump_power"
	// Power options
	ShakeUpOptTakeHolding   = "take_holding"
	ShakeUpOptBreakArtifact = "break_artifact"
	ShakeUpOptClaimTitle    = "claim_title"
	ShakeUpOptBumpEsteem    = "bump_esteem"
)

// ShakeUpOptionInfo is the static metadata for a Shake-Up option.
type ShakeUpOptionInfo struct {
	Key         string
	Category    string
	Description string
	NeedsAsset  bool   // true → spend body must include target_asset_id
	BumpsTrack  string // non-empty for bump_X options; the track being bumped
}

// shakeUpOptions is the canonical option table.
var shakeUpOptions = map[string]ShakeUpOptionInfo{
	ShakeUpOptTakePeer:      {ShakeUpOptTakePeer, ShakeUpCategoryEsteem, "Take a peer asset.", true, ""},
	ShakeUpOptTakeArtifact:  {ShakeUpOptTakeArtifact, ShakeUpCategoryEsteem, "Take an artifact asset.", true, ""},
	ShakeUpOptBreakResource: {ShakeUpOptBreakResource, ShakeUpCategoryEsteem, "Break a resource asset.", true, ""},
	ShakeUpOptBumpKnowledge: {
		ShakeUpOptBumpKnowledge,
		ShakeUpCategoryEsteem,
		"Bump up one rank on knowledge.",
		false,
		"knowledge",
	},
	ShakeUpOptTakeResource: {ShakeUpOptTakeResource, ShakeUpCategoryKnowledge, "Take a resource asset.", true, ""},
	ShakeUpOptBreakHolding: {ShakeUpOptBreakHolding, ShakeUpCategoryKnowledge, "Break a holding asset.", true, ""},
	ShakeUpOptBreakPeer:    {ShakeUpOptBreakPeer, ShakeUpCategoryKnowledge, "Break a peer asset.", true, ""},
	ShakeUpOptBumpPower: {
		ShakeUpOptBumpPower,
		ShakeUpCategoryKnowledge,
		"Bump up one rank on power.",
		false,
		"power",
	},
	ShakeUpOptTakeHolding:   {ShakeUpOptTakeHolding, ShakeUpCategoryPower, "Take a holding asset.", true, ""},
	ShakeUpOptBreakArtifact: {ShakeUpOptBreakArtifact, ShakeUpCategoryPower, "Break an artifact asset.", true, ""},
	ShakeUpOptClaimTitle:    {ShakeUpOptClaimTitle, ShakeUpCategoryPower, "Claim a new title.", false, ""},
	ShakeUpOptBumpEsteem: {
		ShakeUpOptBumpEsteem,
		ShakeUpCategoryPower,
		"Bump up one rank on esteem.",
		false,
		"esteem",
	},
}

// ShakeUpOption returns the static info for an option key, or an error if
// unknown.
func ShakeUpOption(key string) (ShakeUpOptionInfo, error) {
	info, ok := shakeUpOptions[key]
	if !ok {
		return ShakeUpOptionInfo{}, errors.New("unknown shake-up option")
	}
	return info, nil
}

// ShakeUpOptionsForCategory returns the four options available in a given
// category, in the rulebook's listed order (presentation only — no rule
// enforces ordering).
func ShakeUpOptionsForCategory(category string) []ShakeUpOptionInfo {
	switch category {
	case ShakeUpCategoryEsteem:
		return []ShakeUpOptionInfo{
			shakeUpOptions[ShakeUpOptTakePeer],
			shakeUpOptions[ShakeUpOptTakeArtifact],
			shakeUpOptions[ShakeUpOptBreakResource],
			shakeUpOptions[ShakeUpOptBumpKnowledge],
		}
	case ShakeUpCategoryKnowledge:
		return []ShakeUpOptionInfo{
			shakeUpOptions[ShakeUpOptTakeResource],
			shakeUpOptions[ShakeUpOptBreakHolding],
			shakeUpOptions[ShakeUpOptBreakPeer],
			shakeUpOptions[ShakeUpOptBumpPower],
		}
	case ShakeUpCategoryPower:
		return []ShakeUpOptionInfo{
			shakeUpOptions[ShakeUpOptTakeHolding],
			shakeUpOptions[ShakeUpOptBreakArtifact],
			shakeUpOptions[ShakeUpOptClaimTitle],
			shakeUpOptions[ShakeUpOptBumpEsteem],
		}
	}
	return nil
}

// NextShakeUpCategory returns the category that follows current, or "" if
// current is the last (power).
func NextShakeUpCategory(current string) string {
	switch current {
	case ShakeUpCategoryEsteem:
		return ShakeUpCategoryKnowledge
	case ShakeUpCategoryKnowledge:
		return ShakeUpCategoryPower
	}
	return ""
}

// ShakeUpTurnOrder returns the player IDs ordered by their rank on the
// current category, in reverse: lowest-status (rank 5) first, rank-1 last.
// dummies are skipped — they don't take turns.
func ShakeUpTurnOrder(category string, rankings []RankingRow) []int64 {
	out := make([]int64, 0, len(rankings))
	// Walk ranks 5 → 1.
	for r := int16(5); r >= 1; r-- {
		for _, rr := range rankings {
			if rr.Category != category {
				continue
			}
			if rr.Rank != r {
				continue
			}
			if rr.PlayerID == nil {
				continue // dummy
			}
			out = append(out, *rr.PlayerID)
		}
	}
	return out
}

// RankingRow mirrors a row in the rankings table without importing dbgen
// (game/ doesn't depend on db/gen).
type RankingRow struct {
	PlayerID *int64
	Category string
	Rank     int16
}
