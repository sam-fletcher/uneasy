package handler

// Pure (no-DB) unit tests for the break helpers' blank-asset contract — the
// argument shapes a blank break introduces, which every caller now has to be
// safe against. The DB-backed behaviour of breakAsset itself is covered by
// asset_effects_integration_test.go.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	dbgen "uneasy/db/gen"
)

// brokenAssetDetail takes *dbgen.Marginalium and used to dereference it
// unconditionally. Breaking a blank asset passes nil (there is nothing torn to
// quote), so a nil deref here would panic mid-resolution on every plan that
// logs a break.
func TestBrokenAssetDetail_NilMarginaliaIsSafe(t *testing.T) {
	// destroyed is always true for a blank break (nothing survives), and the
	// re-describe prompt is suppressed when destroyed — so this never reaches
	// the DB and q may be nil.
	assert.Empty(t, brokenAssetDetail(context.Background(), nil, 42, nil, true),
		"a blank break quotes no torn text and prompts no re-description")
}

// The ordinary path is unchanged: the torn text is quoted, and the owner is
// invited to re-describe an asset that survived.
func TestBrokenAssetDetail_QuotesTornText(t *testing.T) {
	m := &dbgen.Marginalium{ID: 7, Text: "Owes the crown a favour."}
	assert.Equal(t,
		` The torn marginalia read "Owes the crown a favour.".`,
		brokenAssetDetail(context.Background(), nil, 42, m, true),
		"destroyed suppresses only the prompt, not the quote")
}

func TestBreakVerb(t *testing.T) {
	assert.Equal(t, "destroyed", breakVerb(true))
	assert.Equal(t, "broke", breakVerb(false))
}
