//go:build integration

// handler/seen_markers_integration_test.go — GET /tables/{id}/seen, the one
// small read a tab makes when the player comes back to it, to learn what they
// read on another device. It must agree with the endpoints that carry the same
// markers, and it must hand a player nothing but their own.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"uneasy/db"
	dbgen "uneasy/db/gen"
	"uneasy/model"
)

type seenReply struct {
	Laws           int64 `json:"laws"`
	Rumors         int64 `json:"rumors"`
	LastReadPostID int64 `json:"last_read_post_id"`
}

func (h *lawsRumorsHarness) markers(idx int) seenReply {
	h.t.Helper()
	code, body := h.do(http.MethodGet, idx, h.table("/seen"), nil)
	require.Equal(h.t, http.StatusOK, code, string(body))
	var out seenReply
	require.NoError(h.t, json.Unmarshal(body, &out))
	return out
}

// post makes a system post, so chat has something a read marker can point at.
func (h *lawsRumorsHarness) post() dbgen.ScenePost {
	h.t.Helper()
	code := "test.post"
	p, err := h.q.CreateSystemPost(context.Background(), dbgen.CreateSystemPostParams{
		GameID: h.tg.Game.ID, Body: "a post", Severity: model.SeverityDefault, SystemCode: &code,
	})
	require.NoError(h.t, err)
	return p
}

func TestGetSeenMarkers_AnUnmarkedSeatReadsZeros(t *testing.T) {
	h := newLawsRumorsHarness(t)
	h.law("a", 0)
	h.rumor("b", nil, nil)
	h.post()

	require.Equal(t, seenReply{}, h.markers(1))
}

func TestGetSeenMarkers_FollowsAMarkerMovedElsewhere(t *testing.T) {
	// The scenario the endpoint is for: the same seat marks things seen on
	// "another device" (here, just another request), and the first device asks.
	h := newLawsRumorsHarness(t)
	l := h.law("a", 0)
	r := h.rumor("b", nil, nil)
	p := h.post()

	before := h.markers(1)
	require.Equal(t, seenReply{}, before)

	_, _ = h.markLaws(1, l.Rev)
	require.Equal(t, seenReply{Laws: l.Rev}, h.markers(1), "reading Laws moves only the Laws marker")

	_, _ = h.markRumors(1, r.Rev)
	require.Equal(t, seenReply{Laws: l.Rev, Rumors: r.Rev}, h.markers(1))

	_, err := h.q.UpdateReadMarker(context.Background(), dbgen.UpdateReadMarkerParams{
		RequestedID: p.ID, GameID: h.tg.Game.ID, PlayerID: h.tg.Players[1].ID,
	})
	require.NoError(t, err)
	require.Equal(t, seenReply{Laws: l.Rev, Rumors: r.Rev, LastReadPostID: p.ID}, h.markers(1),
		"reading chat moves the chat marker")
}

func TestGetSeenMarkers_AgreesWithTheEndpointsThatCarryTheSameMarkers(t *testing.T) {
	h := newLawsRumorsHarness(t)
	l := h.law("a", 0)
	r := h.rumor("b", nil, nil)
	_, _ = h.markLaws(1, l.Rev)
	_, _ = h.markRumors(1, r.Rev)

	got := h.markers(1)
	require.Equal(t, got.Laws, h.seen(1, "laws"), "GET /laws seen_rev")
	require.Equal(t, got.Rumors, h.seen(1, "rumors"), "GET /rumors seen_rev")

	code, body := h.do(http.MethodGet, 1, h.table("/state"), nil)
	require.Equal(t, http.StatusOK, code, string(body))
	var state struct {
		Seen struct {
			Laws   int64 `json:"laws"`
			Rumors int64 `json:"rumors"`
		} `json:"laws_rumors_seen"`
	}
	require.NoError(t, json.Unmarshal(body, &state))
	require.Equal(t, got.Laws, state.Seen.Laws, "GET /state laws_rumors_seen.laws")
	require.Equal(t, got.Rumors, state.Seen.Rumors, "GET /state laws_rumors_seen.rumors")
}

func TestGetSeenMarkers_OnlyEverTheCallersOwn(t *testing.T) {
	h := newLawsRumorsHarness(t)
	l := h.law("a", 0)
	r := h.rumor("b", nil, nil)
	p := h.post()
	_, _ = h.markLaws(1, l.Rev)
	_, _ = h.markRumors(1, r.Rev)
	_, err := h.q.UpdateReadMarker(context.Background(), dbgen.UpdateReadMarkerParams{
		RequestedID: p.ID, GameID: h.tg.Game.ID, PlayerID: h.tg.Players[1].ID,
	})
	require.NoError(t, err)

	// Seat 1 has read everything; seats 0 and 2 have read nothing, and neither
	// may be told anything about seat 1.
	require.Equal(t, seenReply{Laws: l.Rev, Rumors: r.Rev, LastReadPostID: p.ID}, h.markers(1))
	require.Equal(t, seenReply{}, h.markers(0))
	require.Equal(t, seenReply{}, h.markers(2))

	// And the body is exactly the three markers: no player id, no roster, no
	// other seat's row riding along.
	_, raw := h.do(http.MethodGet, 2, h.table("/seen"), nil)
	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &keys))
	require.Len(t, keys, 3, string(raw))
	for _, k := range []string{"laws", "rumors", "last_read_post_id"} {
		require.Contains(t, keys, k)
	}
}

func TestGetSeenMarkers_RefusesANonMemberAndASignedOutCaller(t *testing.T) {
	h := newLawsRumorsHarness(t)
	h.law("a", 0)

	outsider := newTestGame(t, h.q, 2)
	tok, err := db.NewCookieToken()
	require.NoError(t, err)
	_, err = h.q.CreateSession(context.Background(), dbgen.CreateSessionParams{
		Token: tok, AccountID: outsider.Players[0].AccountID,
	})
	require.NoError(t, err)

	h.tokens = append(h.tokens, tok)
	code, _ := h.do(http.MethodGet, len(h.tokens)-1, h.table("/seen"), nil)
	require.Equal(t, http.StatusForbidden, code, "someone with no seat at this table")

	h.tokens = append(h.tokens, "not-a-session")
	code, _ = h.do(http.MethodGet, len(h.tokens)-1, h.table("/seen"), nil)
	require.Equal(t, http.StatusUnauthorized, code, "no session")
}
