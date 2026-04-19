package game

import "testing"

func TestDemandDifficulty_NoPowerDiff(t *testing.T) {
	// Same rank → no bonus; difficulty equals target's own.
	got := MakeDemandsDifficulty(4, 3, 3)
	if got != 4 {
		t.Errorf("same rank: got %d, want 4", got)
	}
}

func TestDemandDifficulty_Uphill(t *testing.T) {
	// Target outranks demander (lower rank number). Bonus = demander − target.
	// Demander rank 5, target rank 1 → +4 on top of target diff 2 → 6.
	got := MakeDemandsDifficulty(2, 5, 1)
	if got != 6 {
		t.Errorf("uphill: got %d, want 6", got)
	}
}

func TestDemandDifficulty_Downhill(t *testing.T) {
	// Demander outranks target → no bonus.
	got := MakeDemandsDifficulty(3, 1, 4)
	if got != 3 {
		t.Errorf("downhill: got %d, want 3", got)
	}
}

func TestDemandDifficulty_BaselineZero(t *testing.T) {
	// Target plan with difficulty 0 (e.g., Make War equivalent) — same rank.
	got := MakeDemandsDifficulty(0, 2, 2)
	if got != 0 {
		t.Errorf("baseline zero same rank: got %d, want 0", got)
	}
	// Uphill off a zero baseline still picks up the power delta.
	got = MakeDemandsDifficulty(0, 4, 2)
	if got != 2 {
		t.Errorf("baseline zero uphill: got %d, want 2", got)
	}
}

func TestDemandRowPlacement_BeforeTarget(t *testing.T) {
	// Target at row 7, game on row 3 → demand lands at row 6.
	got := DemandRowPlacement(7, 3)
	if got != 6 {
		t.Errorf("before target: got %d, want 6", got)
	}
}

func TestDemandRowPlacement_Immediate(t *testing.T) {
	// Target on the current row → demand resolves now (not on row −1).
	got := DemandRowPlacement(5, 5)
	if got != 5 {
		t.Errorf("immediate: got %d, want 5", got)
	}
	// Also clamp when target-1 falls behind the current row.
	got = DemandRowPlacement(4, 5)
	if got != 5 {
		t.Errorf("clamp behind current: got %d, want 5", got)
	}
}

// TestDemandChain_TwoLevels exercises a chain of demands (option (a) from
// the Stage 6 design discussion). Plan C is a normal asset-awarding plan;
// demand B targets C; demand A targets B. Winners flow one link at a time
// with no special chain treatment — this just verifies the pure difficulty
// math composes sensibly when the "target's difficulty" is itself a demand.
//
// Ranks: A's preparer = 4, B's preparer = 2, C's preparer = 1. C's own
// difficulty is 2. Then:
//   - B (demand on C): B's preparer rank 2 vs C's preparer rank 1 → uphill
//     by 1 → B's difficulty = 2 + 1 = 3.
//   - A (demand on B): A's preparer rank 4 vs B's preparer rank 2 → uphill
//     by 2 → A's difficulty = 3 + 2 = 5.
func TestDemandChain_TwoLevels(t *testing.T) {
	const cDiff int16 = 2
	const rankA, rankB, rankC int16 = 4, 2, 1

	bDiff := MakeDemandsDifficulty(cDiff, rankB, rankC)
	if bDiff != 3 {
		t.Fatalf("B's difficulty: got %d, want 3", bDiff)
	}
	aDiff := MakeDemandsDifficulty(bDiff, rankA, rankB)
	if aDiff != 5 {
		t.Fatalf("A's difficulty: got %d, want 5", aDiff)
	}

	// Draft order at each link follows the same higher-rank-first rule.
	const aPrepID, bPrepID, cPrepID int64 = 10, 20, 30
	first, _ := DemandDraftPickers(aPrepID, bPrepID, rankA, rankB)
	if first != bPrepID {
		t.Errorf("link A→B: higher rank should pick first, got %d", first)
	}
	first, _ = DemandDraftPickers(bPrepID, cPrepID, rankB, rankC)
	if first != cPrepID {
		t.Errorf("link B→C: higher rank should pick first, got %d", first)
	}
}

func TestDraftOrder_HigherRankFirst(t *testing.T) {
	const demanderID, targetPreparerID = int64(100), int64(200)

	// Demander is higher-ranked (lower rank number) → picks first.
	first, second := DemandDraftPickers(demanderID, targetPreparerID, 1, 4)
	if first != demanderID || second != targetPreparerID {
		t.Errorf("demander higher rank: got (%d,%d), want (%d,%d)",
			first, second, demanderID, targetPreparerID)
	}

	// Target's preparer is higher-ranked → picks first.
	first, second = DemandDraftPickers(demanderID, targetPreparerID, 3, 2)
	if first != targetPreparerID || second != demanderID {
		t.Errorf("target higher rank: got (%d,%d), want (%d,%d)",
			first, second, targetPreparerID, demanderID)
	}
}
