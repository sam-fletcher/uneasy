package game

// DiceSides is the number of sides on a standard die used in the game.
const DiceSides = 6

// ClampDifficulty confines a computed difficulty to the band the book's
// glossary defines (owner ruling, 2026-11-01: a result between 1 and 6). Every
// difficulty formula returns through it, so no plan can produce a roll the
// dice_rolls check (1 to 6) would reject, nor one that cannot be won.
func ClampDifficulty[T ~int | ~int16 | ~int64](d T) T {
	return min(max(d, 1), T(DiceSides))
}

// ── Per-plan pure difficulty functions ───────────────────────────────────────

// ExchangeCourtiersDifficulty returns the difficulty given the target
// player's rank on the power track.
// Difficulty = target's status = 6 - rank (minimum 1).
func ExchangeCourtiersDifficulty(targetRank int16) int16 {
	return ClampDifficulty(int16(DiceSides) - targetRank)
}

// MakeIntroductionsDifficulty returns the difficulty given the ResData.
// Difficulty = 2 + peer_count (peer_count 0 treated as 1; range 1–4 → 3–6).
func MakeIntroductionsDifficulty(resData ResolutionData) int16 {
	const baseDifficulty = int16(2)
	var pc int16
	if resData.MakeIntroductions != nil {
		pc = resData.MakeIntroductions.PeerCount
	}
	return ClampDifficulty(baseDifficulty + max(pc, 1))
}

// SpreadPropagandaDifficulty returns the difficulty for a given preparer
// rank on the esteem track. Difficulty = rank (1–5).
func SpreadPropagandaDifficulty(preparerRank int16) int16 {
	return ClampDifficulty(preparerRank)
}

// SeekAnswersDifficulty returns the difficulty: preparer's knowledge rank.
func SeekAnswersDifficulty(preparerKnowledgeRank int16) int16 {
	return ClampDifficulty(preparerKnowledgeRank)
}

// SpreadRumorsDifficulty returns the difficulty based on whether the target
// is a main character:
//   - targetIsMainChar == true:  6 - relevantRank (target's esteem status)
//   - targetIsMainChar == false: relevantRank (preparer's esteem rank)
func SpreadRumorsDifficulty(relevantRank int16, targetIsMainChar bool) int16 {
	if targetIsMainChar {
		return ClampDifficulty(int16(DiceSides) - relevantRank)
	}
	return ClampDifficulty(relevantRank)
}

// ChronicleHistoriesDifficulty returns the difficulty:
//
//	max(preparerKnowledgeRank, len(InvokedArtifactIDs))
func ChronicleHistoriesDifficulty(preparerKnowledgeRank int16, resData ResolutionData) int16 {
	var artifactCount int16
	if resData.ChronicleHistories != nil {
		artifactCount = int16(len(resData.ChronicleHistories.InvokedArtifactIDs))
	}
	return ClampDifficulty(max(preparerKnowledgeRank, artifactCount))
}

// ProposeDecreeDifficulty returns the difficulty: preparer's power rank.
func ProposeDecreeDifficulty(preparerPowerRank int16) int16 {
	return ClampDifficulty(preparerPowerRank)
}

// ProposeDuelDifficulty returns the difficulty given the target player's
// esteem rank. Difficulty = target's esteem status = 6 - rank (minimum 1).
func ProposeDuelDifficulty(targetEsteemRank int16) int16 {
	return ClampDifficulty(int16(DiceSides) - targetEsteemRank)
}
