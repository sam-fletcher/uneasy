package game

import (
	"fmt"

	"uneasy/model"
)

// DiceSides is the number of sides on a standard die used in the game.
const DiceSides = 6

// ── Per-plan pure difficulty functions ───────────────────────────────────────

// ExchangeCourtiersDifficulty returns the difficulty given the target
// player's rank on the power track.
// Difficulty = target's status = 6 - rank (minimum 1).
func ExchangeCourtiersDifficulty(targetRank int16) int16 {
	return max(int16(DiceSides)-targetRank, 1)
}

// MakeIntroductionsDifficulty returns the difficulty given the ResData.
// Difficulty = 2 + peer_count (peer_count 0 treated as 1; range 1–4 → 3–6).
func MakeIntroductionsDifficulty(resData ResolutionData) int16 {
	const baseDifficulty = int16(2)
	pc := max(resData.PeerCount, 1)
	return baseDifficulty + pc
}

// SpreadPropagandaDifficulty returns the difficulty for a given preparer
// rank on the esteem track. Difficulty = rank (1–5).
func SpreadPropagandaDifficulty(preparerRank int16) int16 {
	return preparerRank
}

// SeekAnswersDifficulty returns the difficulty: preparer's knowledge rank.
func SeekAnswersDifficulty(preparerKnowledgeRank int16) int16 {
	return preparerKnowledgeRank
}

// SpreadRumorsDifficulty returns the difficulty based on whether the target
// is a main character:
//   - targetIsMainChar == true:  6 - relevantRank (target's esteem status)
//   - targetIsMainChar == false: relevantRank (preparer's esteem rank)
func SpreadRumorsDifficulty(relevantRank int16, targetIsMainChar bool) int16 {
	if targetIsMainChar {
		return max(int16(DiceSides)-relevantRank, 1)
	}
	return relevantRank
}

// ChronicleHistoriesDifficulty returns the difficulty:
//
//	max(preparerKnowledgeRank, len(InvokedArtifactIDs))
func ChronicleHistoriesDifficulty(preparerKnowledgeRank int16, resData ResolutionData) int16 {
	artifactCount := int16(len(resData.InvokedArtifactIDs))
	return max(preparerKnowledgeRank, artifactCount)
}

// ProposeDecreeDifficulty returns the difficulty: preparer's power rank.
func ProposeDecreeDifficulty(preparerPowerRank int16) int16 {
	return preparerPowerRank
}

// ── Aggregate dispatcher (for tests) ────────────────────────────────────────

// ComputeDifficultyPure returns the base difficulty without hitting the database.
// Used directly by unit tests. relevantRank meaning per plan type:
//
//   - Exchange Courtiers:   target player's power rank
//   - Make Introductions:   ignored (PeerCount in resData drives difficulty)
//   - Spread Propaganda:    preparer's esteem rank
//   - Seek Answers:         preparer's knowledge rank
//   - Spread Rumors:        if target is main char → target's esteem rank;
//     otherwise → preparer's esteem rank. Pass targetIsMainChar=true for former.
//   - Chronicle Histories:  preparer's knowledge rank (artifact count in resData)
//   - Propose Decree:       preparer's power rank
//   - Clandestinely Liaise: not applicable (no dice roll)
func ComputeDifficultyPure(
	planType model.PlanType,
	resData ResolutionData,
	relevantRank int16,
	targetIsMainChar ...bool,
) (int16, error) {
	switch planType {
	case model.PlanExchangeCourtiers:
		return ExchangeCourtiersDifficulty(relevantRank), nil
	case model.PlanMakeIntroductions:
		return MakeIntroductionsDifficulty(resData), nil
	case model.PlanSpreadPropaganda:
		return SpreadPropagandaDifficulty(relevantRank), nil
	case model.PlanSeekAnswers:
		return SeekAnswersDifficulty(relevantRank), nil
	case model.PlanSpreadRumors:
		isMain := len(targetIsMainChar) > 0 && targetIsMainChar[0]
		return SpreadRumorsDifficulty(relevantRank, isMain), nil
	case model.PlanChronicleHistories:
		return ChronicleHistoriesDifficulty(relevantRank, resData), nil
	case model.PlanProposeDecree:
		return ProposeDecreeDifficulty(relevantRank), nil
	default:
		return 0, fmt.Errorf("unsupported plan type: %s", planType)
	}
}
