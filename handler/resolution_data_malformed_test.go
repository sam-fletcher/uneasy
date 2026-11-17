package handler

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"uneasy/game"
)

// A plan whose saved state doesn't decode reads as whatever of it did. Saving
// that value back would replace the stored blob — the only evidence of what
// went wrong — with a mostly-empty copy, so saveResolutionData must refuse. The
// refusal comes before the query is touched, so this needs no database (a nil
// *Queries would panic if the guard were missing).
func TestSaveResolutionData_RefusesAValueDecodedFromAMalformedBlob(t *testing.T) {
	raw := `{"choice_auto_applied":true,"make_mar_choices":[`
	rd := loadResolutionData(&raw)
	require.ErrorIs(t, rd.LoadErr(), game.ErrResolutionDataMalformed)

	err := saveResolutionData(context.Background(), nil, 7, rd)
	require.Error(t, err)
	assert.ErrorIs(t, err, game.ErrResolutionDataMalformed)
}

// A plan saved before a field changed type has that one field in an older shape
// and is otherwise fine. It must not be treated as unreadable: it would make
// every old finished plan refuse to load.
func TestLoadResolutionData_AnOlderFieldShapeIsNotUnreadable(t *testing.T) {
	raw := `{"choice_auto_applied":true,"make_mar_choices":["peers_arrive"]}`
	rd := loadResolutionData(&raw)
	require.NoError(t, rd.LoadErr())
	assert.True(t, rd.ChoiceAutoApplied, "the rest of the blob still reads")
}
