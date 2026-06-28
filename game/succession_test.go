package game

import "testing"

func TestSuccessionRank_OrderAndMembership(t *testing.T) {
	// In-line titles rank in strictly increasing order, monarch strongest.
	inLine := []string{
		TitleMonarch, TitleTrueHeir, TitleFavoredHeir,
		TitleClaimant, TitleConsort, TitleGeneral,
	}
	prev := -1
	for _, id := range inLine {
		rank, ok := SuccessionRank(id)
		if !ok {
			t.Fatalf("title %q should be in the line of succession", id)
		}
		if rank <= prev {
			t.Fatalf("title %q rank %d not strictly after previous %d", id, rank, prev)
		}
		prev = rank
	}

	// SuccessionOrder and the rank lookup agree on length and content.
	if len(SuccessionOrder) != len(inLine) {
		t.Fatalf("SuccessionOrder has %d entries, expected %d", len(SuccessionOrder), len(inLine))
	}
	if SuccessionOrder[0] != TitleMonarch {
		t.Fatalf("monarch must be the highest claim, got %q", SuccessionOrder[0])
	}
}

func TestSuccessionRank_ExcludedTitles(t *testing.T) {
	// Titles deliberately outside the line (per the table's ruling) and any
	// unknown id confer no claim.
	for _, id := range []string{
		"paramour", "absolver", "heretic", "visiting_royal",
		"lawyer", "spymaster", "", "not_a_title",
	} {
		if rank, ok := SuccessionRank(id); ok {
			t.Fatalf("title %q must not be in the line of succession (got rank %d)", id, rank)
		}
	}
}
