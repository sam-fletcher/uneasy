package game

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

// ProposeDuelDifficulty returns the difficulty given the target player's
// esteem rank. Difficulty = target's esteem status = 6 - rank (minimum 1).
func ProposeDuelDifficulty(targetEsteemRank int16) int16 {
	return max(int16(DiceSides)-targetEsteemRank, 1)
}
