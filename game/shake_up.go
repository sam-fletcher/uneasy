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
	Key             string
	Category        string
	Description     string
	NeedsAsset      bool   // true → spend body must include target_asset_id
	NeedsMarginalia bool   // true → spend body must also include target_marginalia_id (break options: break = tear one marginalia)
	BumpsTrack      string // non-empty for bump_X options; the track being bumped
}

// shakeUpOptions is the canonical option table. Break options tear a single
// marginalia (NeedsMarginalia) — "break = tear off one marginalia; all 4 gone →
// destroyed" — so they carry both the target asset and the chosen marginalia.
var shakeUpOptions = map[string]ShakeUpOptionInfo{
	ShakeUpOptTakePeer:     {ShakeUpOptTakePeer, ShakeUpCategoryEsteem, "Take a peer asset.", true, false, ""},
	ShakeUpOptTakeArtifact: {ShakeUpOptTakeArtifact, ShakeUpCategoryEsteem, "Take an artifact asset.", true, false, ""},
	ShakeUpOptBreakResource: {
		ShakeUpOptBreakResource, ShakeUpCategoryEsteem, "Break a resource asset.", true, true, "",
	},
	ShakeUpOptBumpKnowledge: {
		ShakeUpOptBumpKnowledge,
		ShakeUpCategoryEsteem,
		"Bump up a rank on knowledge.",
		false,
		false,
		"knowledge",
	},
	ShakeUpOptTakeResource: {
		ShakeUpOptTakeResource,
		ShakeUpCategoryKnowledge,
		"Take a resource asset.",
		true,
		false,
		"",
	},
	ShakeUpOptBreakHolding: {
		ShakeUpOptBreakHolding,
		ShakeUpCategoryKnowledge,
		"Break a holding asset.",
		true,
		true,
		"",
	},
	ShakeUpOptBreakPeer: {ShakeUpOptBreakPeer, ShakeUpCategoryKnowledge, "Break a peer asset.", true, true, ""},
	ShakeUpOptBumpPower: {
		ShakeUpOptBumpPower,
		ShakeUpCategoryKnowledge,
		"Bump up a rank on power.",
		false,
		false,
		"power",
	},
	ShakeUpOptTakeHolding: {ShakeUpOptTakeHolding, ShakeUpCategoryPower, "Take a holding asset.", true, false, ""},
	ShakeUpOptBreakArtifact: {
		ShakeUpOptBreakArtifact,
		ShakeUpCategoryPower,
		"Break an artifact asset.",
		true,
		true,
		"",
	},
	ShakeUpOptClaimTitle: {ShakeUpOptClaimTitle, ShakeUpCategoryPower, "Claim a new title.", false, false, ""},
	ShakeUpOptBumpEsteem: {
		ShakeUpOptBumpEsteem,
		ShakeUpCategoryPower,
		"Bump up a rank on esteem.",
		false,
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

// NextShakeUpActor returns the player whose turn it is to announce a spend
// during a category's spending step. order is the reverse-rank turn order
// (lowest status first — see ShakeUpTurnOrder); hasTokens reports who still
// holds spendable tokens; lastActor is the player who committed the previous
// spend this category, or nil if none has yet.
//
// Turn order loops: after lastActor, the turn passes to the next player in
// order who still holds tokens, wrapping around. Players with no tokens are
// skipped ("Loop the turn order until everybody is out of tokens"). Returns 0
// when no one holds tokens.
func NextShakeUpActor(order []int64, hasTokens map[int64]bool, lastActor *int64) int64 {
	n := len(order)
	if n == 0 {
		return 0
	}
	start := 0
	if lastActor != nil {
		for i, pid := range order {
			if pid == *lastActor {
				start = i + 1
				break
			}
		}
	}
	for off := range n {
		pid := order[(start+off)%n]
		if hasTokens[pid] {
			return pid
		}
	}
	return 0
}
