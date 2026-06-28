package game

// Monarch line of succession (ADR-007).
//
// Each throne-line title has a stable, canonical id stamped onto its
// marginalia at claim time. The id convention is the title's display name with
// "The " dropped, lowercased, spaces → underscores: "The True Heir" →
// "true_heir". These ids are durable game state (immutable after claim), so the
// strings below are part of the on-disk contract — do not rename them.
const (
	TitleMonarch     = "monarch"
	TitleTrueHeir    = "true_heir"
	TitleFavoredHeir = "favored_heir"
	TitleClaimant    = "claimant"
	TitleConsort     = "consort"
	TitleGeneral     = "general"
)

// SuccessionOrder is the predefined throne line (ADR-007's "SUCCESSION_ORDER"),
// earliest = highest claim. The current monarch is the controller of the asset
// bearing the title that appears earliest in this list among all live (untorn,
// non-destroyed) claims; tearing or destroying that claim lets the next title
// in the list ascend.
//
// Order ruled by the table (supersedes ADR §4's proposal): the legal monarch,
// then the two heirs, then the outside claimant, the consort, and finally the
// general as the last line of defence for the crown.
//
// Deliberately excluded (no claim to THIS realm's throne): paramour, absolver,
// heretic, visiting_royal, lawyer, spymaster. A title not in this list never
// confers a place in the succession, no matter how it is stamped.
var SuccessionOrder = []string{
	TitleMonarch,
	TitleTrueHeir,
	TitleFavoredHeir,
	TitleClaimant,
	TitleConsort,
	TitleGeneral,
}

// successionRank maps a title id to its index in SuccessionOrder (0 = highest
// claim). Titles outside the line are absent. Built once at init so the lookup
// in currentMonarch is O(1) per candidate rather than a linear scan.
var successionRank = func() map[string]int {
	m := make(map[string]int, len(SuccessionOrder))
	for i, id := range SuccessionOrder {
		m[id] = i
	}
	return m
}()

// SuccessionRank returns the claim strength of a title id: a 0-based index into
// SuccessionOrder (lower = stronger claim) and ok=false for any id not in the
// line of succession.
func SuccessionRank(titleID string) (rank int, ok bool) {
	rank, ok = successionRank[titleID]
	return rank, ok
}
