package handler

// plan_propose_duel_view_test.go — the duel's information model, pinned.
//
// A bout row is written at DECLARE time with the declarer's hidden die already
// in it, so serialising the row raw hands the responder — whose entire job is
// to answer blind — the number they are meant not to know. That is invisible
// in the UI (the panel never prints it), which is exactly why it needs a test:
// nothing else will notice if it reopens.
//
// These pin the masking function itself, with no database. The same property is
// asserted end-to-end over HTTP in plan_propose_duel_integration_test.go.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

const (
	declarerSeat  int64 = 10
	responderSeat int64 = 20
	bystanderSeat int64 = 30
)

// declaredBout is a bout as it stands after bout-declare: the declarer's die is
// on the row, the responder has not answered, nothing is resolved.
//
// (Built in steps rather than as one returned literal on purpose: `go fix`
// treats a single-return helper as a trivial wrapper and inlines it at every
// call site, which would scatter this fixture through the file.)
func declaredBout() dbgen.DuelBout {
	b := dbgen.DuelBout{
		ID: 1, PlanID: 7, BoutNumber: 1,
		DeclarerID: declarerSeat, DeclarerStakeID: 100,
		ResponderID: responderSeat,

		Declaration: new("high"),
		DeclarerDie: new(int16(5))}
	return b
}

// resolvedBout is the same bout once the responder has answered: both dice are
// on the row and ResolvedAt is set.
func resolvedBout() dbgen.DuelBout {
	b := declaredBout()
	b.ResponderStakeID = new(int64(200))
	b.ResponderDie = new(int16(2))
	b.WinnerID = new(declarerSeat)
	b.ResolvedAt = pgtype.Timestamptz{Valid: true}
	return b
}

// TestDuelBoutViews_UnresolvedBoutHidesDeclarerDieFromEveryoneButTheDeclarer is
// THE regression test: while a bout is open, only the declarer may see the
// declarer's die. The responder and any bystander must get null.
func TestDuelBoutViews_UnresolvedBoutHidesDeclarerDieFromEveryoneButTheDeclarer(t *testing.T) {
	bouts := []dbgen.DuelBout{declaredBout()}

	t.Run("responder cannot see it", func(t *testing.T) {
		v := duelBoutViews(bouts, responderSeat)
		require.Len(t, v, 1)
		assert.Nil(t, v[0].DeclarerDie, "the responder must answer blind")
	})
	t.Run("a bystander cannot see it", func(t *testing.T) {
		v := duelBoutViews(bouts, bystanderSeat)
		require.Len(t, v, 1)
		assert.Nil(t, v[0].DeclarerDie)
	})
	t.Run("the declarer can", func(t *testing.T) {
		v := duelBoutViews(bouts, declarerSeat)
		require.Len(t, v, 1)
		require.NotNil(t, v[0].DeclarerDie)
		assert.Equal(t, int16(5), *v[0].DeclarerDie)
	})
}

// TestDuelBoutViews_ResolvedBoutRevealsBothDiceToEveryone: the reveal is the
// whole point of a bout, and BoutsPhase.svelte renders resolved bouts' dice off
// declarer_die / responder_die, so they must survive masking once resolved.
func TestDuelBoutViews_ResolvedBoutRevealsBothDiceToEveryone(t *testing.T) {
	bouts := []dbgen.DuelBout{resolvedBout()}
	for name, viewer := range map[string]int64{
		"declarer":  declarerSeat,
		"responder": responderSeat,
		"bystander": bystanderSeat,
	} {
		t.Run(name, func(t *testing.T) {
			v := duelBoutViews(bouts, viewer)
			require.Len(t, v, 1)
			require.NotNil(t, v[0].DeclarerDie)
			require.NotNil(t, v[0].ResponderDie)
			assert.Equal(t, int16(5), *v[0].DeclarerDie)
			assert.Equal(t, int16(2), *v[0].ResponderDie)
		})
	}
}

// TestDuelBoutViews_ResponderDieIsMaskedWhileUnresolved: responder_die is only
// written when the bout resolves, so today it is never set on an open bout. The
// mask is still keyed on it independently, so that stays true if a future change
// ever writes it earlier (e.g. recording the answer before comparing).
func TestDuelBoutViews_ResponderDieIsMaskedWhileUnresolved(t *testing.T) {
	b := declaredBout()
	b.ResponderStakeID = new(int64(200))
	b.ResponderDie = new(int16(2)) // hypothetical: answered but not yet resolved

	hidden := duelBoutViews([]dbgen.DuelBout{b}, declarerSeat)
	require.Len(t, hidden, 1)
	assert.Nil(t, hidden[0].ResponderDie, "the declarer must not read the responder's die early")

	own := duelBoutViews([]dbgen.DuelBout{b}, responderSeat)
	require.Len(t, own, 1)
	require.NotNil(t, own[0].ResponderDie)
	assert.Equal(t, int16(2), *own[0].ResponderDie)
}

// TestDuelBoutViews_DeclarationIsPublic: which stake opened the bout and whether
// it was called high or low is the call the responder is answering. It must
// reach them while the bout is still open — the silence half of the bug.
func TestDuelBoutViews_DeclarationIsPublic(t *testing.T) {
	v := duelBoutViews([]dbgen.DuelBout{declaredBout()}, responderSeat)
	require.Len(t, v, 1)
	require.NotNil(t, v[0].Declaration)
	assert.Equal(t, "high", *v[0].Declaration)
	assert.Equal(t, int64(100), v[0].DeclarerStakeID)
	assert.Equal(t, declarerSeat, v[0].DeclarerID)
}

// TestDuelBoutView_WireShapeMatchesTheFrontendType: the view replaces the raw
// dbgen row on the wire, and frontend/src/lib/api/plans.ts's DuelBout is typed
// against the old shape. Every key it declares must still be present (null, not
// absent) so a masked bout and an unmasked one deserialise the same.
func TestDuelBoutView_WireShapeMatchesTheFrontendType(t *testing.T) {
	raw, err := json.Marshal(duelBoutViews([]dbgen.DuelBout{declaredBout()}, responderSeat)[0])
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	for _, key := range []string{
		"id", "plan_id", "bout_number", "declarer_id", "declarer_stake_id",
		"responder_id", "responder_stake_id", "declaration", "declarer_die",
		"responder_die", "winner_id", "is_match", "created_at", "resolved_at",
	} {
		assert.Contains(t, m, key, "wire field %q is part of the DuelBout contract", key)
	}
	assert.Nil(t, m["declarer_die"], "masked → explicit null, not the die")
}

// TestDuelBoutDeclaredPayload_CarriesTheCallButNeverADie: the WS event fans out
// to every client at the table, the responder's included. It must carry the
// call (the half that was missing) and must not grow a die field (the half
// that must never be there). Keys are checked rather than values so adding
// "declarer_die" later fails here, whatever it is set to.
func TestDuelBoutDeclaredPayload_CarriesTheCallButNeverADie(t *testing.T) {
	raw, err := json.Marshal(model.DuelBoutDeclaredPayload{
		PlanID: 7, BoutNumber: 1, DeclarerID: declarerSeat, DeclarerStakeID: 100,
		Declaration: "low", ResponderID: responderSeat,
	})
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))

	assert.Equal(t, "low", m["declaration"], "the responder is answering this call")
	assert.EqualValues(t, 100, m["declarer_stake_id"], "…made with this stake")
	for key := range m {
		assert.NotContains(t, strings.ToLower(key), "die",
			"payload field %q would broadcast a hidden die to the responder", key)
	}
}
