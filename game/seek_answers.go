package game

// seek_answers.go — Pure resolution rules for Seek Answers.
//
// This is the Option E pilot (see adr/TESTABILITY_AND_ENGINE_DECOUPLING_PLAN.md):
// the rule decisions live here as pure functions over a domain snapshot, with no
// DB or HTTP access. The handler (handler/plan_seek_answers.go) is the imperative
// shell — it loads the snapshot from Postgres, calls these functions, and
// persists the result. It extends the pattern already used by
// SeekAnswersDifficulty (game/difficulty.go).

import "uneasy/model"

// SeekAnswersMarPenalty returns how many of the preparer's own resources they
// must flaw when Seek Answers is marred: the shortfall (difficulty − result,
// floored at 0), capped by how many of their resources are still eligible to
// flaw.
func SeekAnswersMarPenalty(difficulty, result, eligibleCount int16) int16 {
	shortfall := max(difficulty-result, 0)
	return min(shortfall, eligibleCount)
}

// ResourceFlawView is the domain snapshot of one asset for Seek Answers
// self-flaw eligibility — only the fields the rule needs, decoupled from
// dbgen.Asset. The handler builds these from the DB (owner's assets + their
// intact-marginalia counts).
type ResourceFlawView struct {
	AssetID               int64
	AssetType             model.AssetType
	IsDestroyed           bool
	IntactMarginaliaCount int
}

// EligibleSelfFlawResourceIDs filters a snapshot to the resource assets that can
// still be flawed: a non-destroyed resource carrying at least one intact
// marginalium, not already flawed this resolution. Order is preserved.
func EligibleSelfFlawResourceIDs(views []ResourceFlawView, flawed []int64) []int64 {
	flawedSet := make(map[int64]struct{}, len(flawed))
	for _, id := range flawed {
		flawedSet[id] = struct{}{}
	}
	var out []int64
	for _, v := range views {
		if v.AssetType != model.AssetResource || v.IsDestroyed {
			continue
		}
		if v.IntactMarginaliaCount < 1 {
			continue
		}
		if _, dup := flawedSet[v.AssetID]; dup {
			continue
		}
		out = append(out, v.AssetID)
	}
	return out
}
