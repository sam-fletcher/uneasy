package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// playerMark is the backend half of the chat feed's player-name markup
// (adr/CHAT_VISUAL_HIERARCHY_PLAN.md S4). Its parser lives in
// frontend/src/lib/logMarkup.ts — logMarkup.test.ts covers the same cases
// from the other side, so the two must be changed together.

func TestPlayerMark_WrapsNameWithID(t *testing.T) {
	assert.Equal(t, "@@7|alice@@", playerMark(7, "alice"))
	// Names are free text: spaces, punctuation, and non-ASCII all pass through
	// untouched, since only the delimiters carry meaning.
	assert.Equal(t, "@@12|Ana Beatriz d'Éon@@", playerMark(12, "Ana Beatriz d'Éon"))
}

func TestPlayerMark_ContainsNoHTMLSpecials(t *testing.T) {
	// The feed escapes a body before parsing marks, so a delimiter containing
	// &, < or > would arrive rewritten as an entity and never match.
	assert.NotContains(t, playerMark(3, "bob"), "&")
	assert.NotContains(t, playerMark(3, "bob"), "<")
	assert.NotContains(t, playerMark(3, "bob"), ">")
}

func TestPlayerMark_FallsBackToPlainOnDelimiterChars(t *testing.T) {
	// '@' and '|' would make the token ambiguous; '*' could pair with an
	// assetMark elsewhere in the same body and split an <em> across the token.
	// Losing the colour beats emitting something unparseable.
	assert.Equal(t, "a@b", playerMark(1, "a@b"))
	assert.Equal(t, "a|b", playerMark(1, "a|b"))
	assert.Equal(t, "a*b", playerMark(1, "a*b"))
}

func TestAssetMark_UnchangedByPlayerMark(t *testing.T) {
	// The two marks share one body and must not interfere: an asset name is
	// still just **…**, and a marked player name inside the same string keeps
	// its own delimiters.
	body := playerMark(4, "carol") + " renamed " + assetMark("Old Keep")
	assert.Equal(t, "@@4|carol@@ renamed **Old Keep**", body)
}
