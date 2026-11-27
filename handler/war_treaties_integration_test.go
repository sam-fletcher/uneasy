//go:build integration

package handler

// war_treaties_integration_test.go — agreed peace terms are kept as treaties
// (owner's ruling, 2026-11-26; migration 061). A treaty is an accepted peace
// proposal read back with the players who agreed, sent with the laws payload to
// every player, and counted by the Laws tab's "new" dot through a rev drawn from
// the laws sequence.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// treatiesFor reads the treaties as seat idx sees them in GET /state, and the
// same list through GET /laws, which must agree.
func treatiesFor(t *testing.T, h *planLifecycle, idx int) []map[string]any {
	t.Helper()
	gid := strconv.FormatInt(h.tg.Game.ID, 10)
	code, state := h.get(idx, "/api/tables/"+gid+"/state")
	require.Equal(t, http.StatusOK, code)
	code, laws := h.get(idx, "/api/tables/"+gid+"/laws")
	require.Equal(t, http.StatusOK, code)

	asList := func(v any) []map[string]any {
		out := []map[string]any{}
		raw, _ := v.([]any)
		for _, e := range raw {
			out = append(out, e.(map[string]any))
		}
		return out
	}
	fromState, fromLaws := asList(state["treaties"]), asList(laws["treaties"])
	require.Equal(t, fromState, fromLaws, "GET /state and GET /laws carry the same treaties")
	return fromState
}

// put drives a PUT as seat idx and returns the status and decoded body.
func (h *planLifecycle) put(idx int, path string, body any) (int, map[string]any) {
	h.t.Helper()
	buf, err := json.Marshal(body)
	require.NoError(h.t, err)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(buf))
	req.AddCookie(&http.Cookie{Name: "player_token", Value: h.tokens[idx]})
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	serveGuarded(h.t, h.router, rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func lawsMarker(t *testing.T, h *planLifecycle, idx int) int64 {
	t.Helper()
	m, err := h.q.GetLawsRumorsSeen(context.Background(), h.tg.Players[idx].ID)
	require.NoError(t, err)
	return m.LawsSeenRev
}

// proposePeace has seat prepIdx propose terms and returns the proposal id.
func proposePeace(t *testing.T, h *planLifecycle, planID int64, prepIdx int, terms string) int64 {
	t.Helper()
	code, body := h.post(prepIdx, warRoute(planID, "propose-peace"), map[string]any{"terms": terms})
	require.Equalf(t, http.StatusOK, code, "propose-peace: %v", body)
	return int64(body["proposal_id"].(float64))
}

func votePeace(t *testing.T, h *planLifecycle, planID int64, idx int, proposalID int64, accepted bool) {
	t.Helper()
	code, body := h.post(idx, warRoute(planID, "vote-peace"), map[string]any{
		"proposal_id": proposalID, "accepted": accepted,
	})
	require.Equalf(t, http.StatusOK, code, "vote-peace: %v", body)
}

// Accepting a proposal turns it into a treaty every player can read, with its
// terms, its war and everyone who agreed, and it moves the laws revision so the
// "new" dot lights for everyone — the proposer and the last accepter included.
func TestMakeWarHTTP_PeaceAcceptKeepsATreaty(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)

	// A law that exists first, so "the revision moved" means past something.
	law, err := h.q.CreateLaw(ctx, dbgen.CreateLawParams{GameID: h.tg.Game.ID, Text: "No dogs."})
	require.NoError(t, err)

	terms := "White peace; both sides withdraw."
	proposalID := proposePeace(t, h, planID, prepIdx, terms)
	for i := range h.tg.Players {
		require.Empty(t, treatiesFor(t, h, i), "an open proposal is not a treaty yet")
	}
	votePeace(t, h, planID, enemyIdx, proposalID, true)

	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, "ended", war.Status)

	// Every seat reads the same treaty: peace terms are public.
	for i := range h.tg.Players {
		got := treatiesFor(t, h, i)
		require.Lenf(t, got, 1, "seat %d must receive the treaty", i)
		tr := got[0]
		assert.Equal(t, terms, tr["terms"])
		assert.EqualValues(t, proposalID, tr["id"])
		assert.EqualValues(t, war.ID, tr["war_id"])
		assert.EqualValues(t, planID, tr["origin_plan_id"], "the war's name comes from its declaring plan")
		assert.ElementsMatch(t,
			[]any{float64(h.tg.Players[prepIdx].ID), float64(h.tg.Players[enemyIdx].ID)},
			tr["agreed_by"], "the proposer and the accepter agreed")

		// The laws revision moved: the treaty's rev is above the law's, so it is
		// new against any marker that was caught up on the laws alone.
		rev := int64(tr["rev"].(float64))
		assert.Greater(t, rev, law.Rev, "accepting draws a rev past the newest law")
		assert.Greater(t, rev, lawsMarker(t, h, i), "so the treaty is new to seat %d", i)
	}

	// And the closing line quotes the terms.
	posts := warSystemPosts(t, h.q, h.tg.Game.ID)
	require.True(t, anyContains(posts, `agreed to peace terms: "`+terms+`" — the war is over.`),
		"closing line quotes the terms, got %v", posts)
}

// The one marker covers laws and treaties: reporting the treaty's rev as seen
// is accepted (the clamp counts treaties), clears the dot, and is not clamped
// down to the newest law.
func TestMakeWarHTTP_PeaceTreatyCountsTowardTheLawsMarker(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)
	_, err := h.q.CreateLaw(context.Background(), dbgen.CreateLawParams{GameID: h.tg.Game.ID, Text: "No dogs."})
	require.NoError(t, err)

	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")
	votePeace(t, h, planID, enemyIdx, proposalID, true)
	rev := int64(treatiesFor(t, h, 2)[0]["rev"].(float64))

	gid := strconv.FormatInt(h.tg.Game.ID, 10)
	req := func(idx int, seen int64) int64 {
		code, body := h.put(idx, "/api/tables/"+gid+"/seen/laws", map[string]any{"seen_rev": seen})
		require.Equal(t, http.StatusOK, code)
		return int64(body["seen_rev"].(float64))
	}
	assert.Equal(t, rev, req(2, rev), "a bystander who read the Laws tab is caught up past the treaty")
	assert.Equal(t, rev, lawsMarker(t, h, 2))
	assert.Equal(t, rev, req(3, rev+1000), "a client can't mark a rev that doesn't exist yet seen")
	assert.Zero(t, lawsMarker(t, h, 4), "someone who hasn't looked still has it new")
}

// A law written while a treaty is unseen must not carry its writer's marker past
// the treaty: the marker is one number on one scale, so jumping over it would
// mark the treaty seen.
func TestMakeWarHTTP_PeaceTreatyIsNotSkippedByALaterLaw(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)
	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")
	votePeace(t, h, planID, enemyIdx, proposalID, true)

	writer := h.tg.Players[3]
	_, err := h.q.CreateLaw(ctx, dbgen.CreateLawParams{
		GameID: h.tg.Game.ID, Text: "No dogs.", WriterID: &writer.ID,
	})
	require.NoError(t, err)
	assert.Zero(t, lawsMarker(t, h, 3), "their own law is not a reason to skip a treaty they haven't read")
}

// A rejection leaves nothing behind: no treaty, and no revision drawn.
func TestMakeWarHTTP_PeaceRejectedOrOpenIsNotATreaty(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)

	proposalID := proposePeace(t, h, planID, prepIdx, "Surrender everything.")
	votePeace(t, h, planID, enemyIdx, proposalID, false)
	for i := range h.tg.Players {
		require.Empty(t, treatiesFor(t, h, i))
	}
	war, err := h.q.GetWarByOriginPlan(context.Background(), planID)
	require.NoError(t, err)
	require.Equal(t, "active", war.Status)
}

// The database refuses an accepted proposal with no rev, so a future path that
// accepts one without AcceptPeaceProposal fails loudly instead of yielding a
// treaty that never shows as new.
func TestMakeWarHTTP_PeaceAcceptedWithoutARevIsRefused(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, _, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)
	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")

	err := h.q.SetPeaceProposalStatus(context.Background(), dbgen.SetPeaceProposalStatusParams{
		ID: proposalID, Status: "accepted",
	})
	require.Error(t, err)

	accepted, err := h.q.AcceptPeaceProposal(context.Background(), proposalID)
	require.NoError(t, err)
	require.NotNil(t, accepted.Rev)
	_, err = h.q.AcceptPeaceProposal(context.Background(), proposalID)
	require.Error(t, err, "a resolved proposal can't be accepted again")
}

// An edit to a law must not carry its editor's marker over a treaty they haven't
// read, the same rule as writing one (CreateLaw): the marker is one number.
func TestMakeWarHTTP_PeaceTreatyIsNotSkippedByALawEdit(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)

	// Seat 3 has read the one law, so only an unread treaty can stop their marker.
	editor := h.tg.Players[3]
	law, err := h.q.CreateLaw(ctx, dbgen.CreateLawParams{GameID: h.tg.Game.ID, Text: "No dogs."})
	require.NoError(t, err)
	_, err = h.q.MarkLawsSeen(ctx, dbgen.MarkLawsSeenParams{
		PlayerID: editor.ID, RequestedRev: law.Rev, GameID: h.tg.Game.ID,
	})
	require.NoError(t, err)

	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")
	votePeace(t, h, planID, enemyIdx, proposalID, true)

	_, err = h.q.UpdateLawText(ctx, dbgen.UpdateLawTextParams{
		ID: law.ID, Text: "No hounds.", EditorID: &editor.ID,
	})
	require.NoError(t, err)
	assert.Equal(t, law.Rev, lawsMarker(t, h, 3),
		"their own edit is not a reason to skip a treaty they haven't read")
}

// A vote on a war that has already ended is refused. A proposal left open when a
// surrender emptied a side would otherwise be accepted later: it would write a
// treaty for a war that was already over, end it a second time (overwriting the
// surrender as its end reason) and log "the war is over" twice.
func TestMakeWarHTTP_PeaceVoteRefusedOnceTheWarHasEnded(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	_, margA := h.seedPeerWithMarginalia(prepIdx, "Sacrificed asset")
	h.jumpToRow(warRow + 1)

	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")

	// The proposer surrenders while their proposal is open; that empties their
	// side and ends the war.
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id":   h.tg.Players[enemyIdx].ID,
		"choice":        "break_asset",
		"marginalia_id": margA,
		"surrender":     true,
	})
	require.Equalf(t, http.StatusOK, code, "surrender pay: %v", body)
	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, "ended", war.Status)

	code, body = h.post(enemyIdx, warRoute(planID, "vote-peace"), map[string]any{
		"proposal_id": proposalID, "accepted": true,
	})
	require.Equalf(t, http.StatusConflict, code, "vote on an ended war: %v", body)

	for i := range h.tg.Players {
		require.Empty(t, treatiesFor(t, h, i), "a war that ended by surrender has no treaty")
	}
	war, err = h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	require.NotNil(t, war.EndReason)
	assert.Equal(t, game.WarEndSurrender, *war.EndReason, "the end reason stays surrender")
	posts := warSystemPosts(t, h.q, h.tg.Game.ID)
	assert.False(t, anyContains(posts, "agreed to peace terms"))
}

// Two final accepts can race: the second finds the proposal already accepted.
// mwAgreePeace then reports it before it ends the war or says anything, so the
// war is ended and announced once, and the handler turns it into a 409.
func TestMakeWarHTTP_PeaceSecondFinalAcceptChangesNothing(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)
	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")
	votePeace(t, h, planID, enemyIdx, proposalID, true)

	before := len(warSystemPosts(t, h.q, h.tg.Game.ID))
	plan, err := h.q.GetPlanByID(ctx, planID)
	require.NoError(t, err)
	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)
	prop, err := h.q.GetPeaceProposal(ctx, proposalID)
	require.NoError(t, err)
	g, err := h.q.GetGameByID(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	deps := &PlanDeps{Store: h.store, Manager: h.manager}

	err = mwAgreePeace(ctx, deps, &plan, g, war, prop)
	require.ErrorIs(t, err, pgx.ErrNoRows)
	assert.Len(t, warSystemPosts(t, h.q, h.tg.Game.ID), before, "nothing more is logged")
	assert.Len(t, treatiesFor(t, h, 0), 1)
}

// agreed_by is the players whose accepts closed the vote: the accepting votes of
// the war's active participants. Someone who accepted and then surrendered
// before the vote closed was no longer a party to the peace.
func TestMakeWarHTTP_PeaceTreatyOmitsAnAccepterWhoSurrendered(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	war, err := h.q.GetWarByOriginPlan(ctx, planID)
	require.NoError(t, err)

	// An ally of the enemy joins, the war having started.
	allyIdx := -1
	for i := range h.tg.Players {
		if i != prepIdx && i != enemyIdx {
			allyIdx = i
			break
		}
	}
	parts, err := h.q.ListWarParticipants(ctx, war.ID)
	require.NoError(t, err)
	var enemySide int16
	for _, p := range parts {
		if p.PlayerID == h.tg.Players[enemyIdx].ID {
			enemySide = p.Side
		}
	}
	allyID := h.tg.Players[allyIdx].ID
	require.NoError(t, h.q.AddWarParticipant(ctx, dbgen.AddWarParticipantParams{
		WarID: war.ID, PlayerID: allyID, Side: enemySide, JoinedAtRow: warRow,
	}))
	h.jumpToRow(warRow + 1)

	proposalID := proposePeace(t, h, planID, prepIdx, "Withdraw to the old border.")
	votePeace(t, h, planID, allyIdx, proposalID, true)
	row := warRow
	require.NoError(t, h.q.SetWarParticipantSurrendered(ctx, dbgen.SetWarParticipantSurrenderedParams{
		WarID: war.ID, PlayerID: allyID, SurrenderedAtRow: &row,
	}))
	votePeace(t, h, planID, enemyIdx, proposalID, true)

	got := treatiesFor(t, h, 0)
	require.Len(t, got, 1)
	assert.ElementsMatch(t,
		[]any{float64(h.tg.Players[prepIdx].ID), float64(h.tg.Players[enemyIdx].ID)},
		got[0]["agreed_by"], "the ally accepted, then surrendered: not a party to the peace")
}
