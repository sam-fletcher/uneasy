//go:build integration

// handler/plan_clandestinely_liaise_secret_text_integration_test.go — Secrets We
// Keep: each player writes the meeting on the underside of their own asset.
//
// The step used to ask only for an asset, and the server wrote
// "Clandestine meeting with <holder's own name> — <prep note>": the wrong name,
// and a note the table had already read. These tests pin what replaced it:
//
//   - the secret holds the words the player sent, under their own seat, behind a
//     header naming who the holder met ("Met bob's Ser Ott"): the OTHER player and
//     the peer they brought, as the peer is named now;
//   - a tab that predates the text box still works and gets the same header in front
//     of the prep note, and a meeting peer that can't be found falls back to the other
//     player's name alone;
//   - a blank box is refused, and refused before anything is written;
//   - the words reach nobody else: not a watcher's socket, not the plan, not the
//     log, and not the other seat, who can read an asset's underside only through
//     Things We Share.

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
)

// clAtSecretsWeKeep takes a liaison between players[0] (preparer) and players[1]
// (partner) as far as Secrets We Keep, where nobody has submitted. players[2], when
// the table has one, is a bystander. It is the shared prepare → reveal → resolve
// setup plus the one advance that opens the phase.
func clAtSecretsWeKeep(t *testing.T, h *planLifecycle) clMeeting {
	t.Helper()
	m := clPrepareAndResolve(t, h)
	clAdvance(t, h, m.plan.ID) // together_at_last → secrets_we_keep
	return m
}

// clKeepSecretWith submits keep-secret as players[idx] on their main character.
// A nil text leaves the key out, as a tab that predates the text box does.
func clKeepSecretWith(t *testing.T, h *planLifecycle, planID int64, idx int, text *string) (int, map[string]any) {
	t.Helper()
	body := map[string]any{"asset_id": clMC(t, h, idx)}
	if text != nil {
		body["text"] = *text
	}
	return h.post(idx, clPath(planID, "keep-secret"), body)
}

// clHeader is the header a keep-secret by players[idx] must lead with in a liaison set
// up by clPrepareAndResolve: the OTHER player, and the peer that player brought.
func clHeader(h *planLifecycle, idx int) string {
	if idx == 0 {
		return "Met " + h.tg.Players[1].DisplayName + "'s partner's confidant"
	}
	return "Met " + h.tg.Players[0].DisplayName + "'s preparer's confidant"
}

func clSecretsOn(t *testing.T, h *planLifecycle, assetID int64) []dbgen.Secret {
	t.Helper()
	secrets, err := h.q.ListSecretsByAsset(context.Background(), assetID)
	require.NoError(t, err)
	return secrets
}

// Each player's words land on their own asset, authored by their own seat.
func TestLiaise_KeepSecret_StoresEachPlayersOwnText(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clAtSecretsWeKeep(t, h)

	const alice = "We spoke by the orangery window until the lamps went out."
	const bob = "She asked me for the harbourmaster's seal; I said I'd think on it."
	// Padding is not part of what a player wrote.
	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new("  "+alice+"\n"))
	require.Equalf(t, http.StatusOK, code, "preparer keep-secret: %v", body)
	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, new(bob))
	require.Equalf(t, http.StatusOK, code, "partner keep-secret: %v", body)

	for _, c := range []struct {
		idx  int
		want string
	}{{0, alice}, {1, bob}} {
		secrets := clSecretsOn(t, h, clMC(t, h, c.idx))
		require.Len(t, secrets, 1, "player %d: one secret on the asset they picked", c.idx)
		assert.Equal(t, clHeader(h, c.idx)+" — "+c.want, secrets[0].Text,
			"player %d: the secret is what they wrote, under a header naming who they met", c.idx)
		assert.Equal(t, h.tg.Players[c.idx].ID, secrets[0].AuthorID, "player %d: authored by their own seat", c.idx)
		assert.NotContains(t, secrets[0].Text, *m.plan.PreparationNotes, "player %d: the public prep note is not the secret", c.idx)
	}
}

// A tab that predates the text box submits only an asset. It still works: the prep
// note stands in for the words, under the same header, so both paths read alike. The
// header names the player the holder met and the peer they brought, never the holder.
func TestLiaise_KeepSecret_AnOldTabGetsThePrepNoteUnderTheSameHeader(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clAtSecretsWeKeep(t, h)

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, nil)
	require.Equalf(t, http.StatusOK, code, "preparer, old tab: %v", body)
	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, nil)
	require.Equalf(t, http.StatusOK, code, "partner, old tab: %v", body)

	for idx := range 2 {
		secrets := clSecretsOn(t, h, clMC(t, h, idx))
		require.Len(t, secrets, 1)
		assert.Equal(t, clHeader(h, idx)+" — "+*m.plan.PreparationNotes, secrets[0].Text,
			"player %d: the header, then the prep note", idx)
		assert.NotContains(t, secrets[0].Text, h.tg.Players[idx].DisplayName,
			"player %d: the secret never names its own holder", idx)
	}
}

// The header names the peer as it is NOW, at the time of writing, not as it was when
// the liaison was prepared, and a peer destroyed since the meeting was arranged is
// still what the meeting was with.
func TestLiaise_KeepSecret_TheHeaderNamesThePeerAsItIsNow(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m := clAtSecretsWeKeep(t, h)

	_, err := h.pool.Exec(ctx, `UPDATE assets SET name = 'Ser Ott' WHERE id = $1`, m.partnerPeerID)
	require.NoError(t, err)
	_, err = h.pool.Exec(ctx, `UPDATE assets SET is_destroyed = true WHERE id = $1`, m.preparerPeerID)
	require.NoError(t, err)

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new("Words."))
	require.Equalf(t, http.StatusOK, code, "preparer: %v", body)
	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, new("Words."))
	require.Equalf(t, http.StatusOK, code, "partner: %v", body)

	assert.Equal(t, "Met "+h.tg.Players[1].DisplayName+"'s Ser Ott — Words.",
		clSecretsOn(t, h, clMC(t, h, 0))[0].Text, "the partner's peer, renamed since prep")
	assert.Equal(t, "Met "+h.tg.Players[0].DisplayName+"'s preparer's confidant — Words.",
		clSecretsOn(t, h, clMC(t, h, 1))[0].Text, "the preparer's peer, destroyed since prep, is still named")
}

// A seat's meeting peer that can't be found (never recorded, or gone from the game)
// leaves the other player's name alone: the secret is still attributed.
func TestLiaise_KeepSecret_AMissingMeetingPeerFallsBackToThePlayersName(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	m := clAtSecretsWeKeep(t, h)

	// m.plan predates the advance into Secrets We Keep; edit the plan as it stands now.
	current, err := h.q.GetPlanByID(ctx, m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(current.ResolutionData)
	ld := rd.EnsureLiaise()
	ld.PartnerPeerID = nil                  // never recorded
	ld.PreparerPeerID = new(int64(1) << 40) // recorded, but not in this game
	require.NoError(t, saveResolutionData(ctx, h.q, m.plan.ID, rd))

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new("Words."))
	require.Equalf(t, http.StatusOK, code, "preparer: %v", body)
	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, nil)
	require.Equalf(t, http.StatusOK, code, "partner, old tab: %v", body)

	assert.Equal(t, "Met "+h.tg.Players[1].DisplayName+" — Words.",
		clSecretsOn(t, h, clMC(t, h, 0))[0].Text, "no partner peer recorded: the player alone")
	assert.Equal(t, "Met "+h.tg.Players[0].DisplayName+" — "+*m.plan.PreparationNotes,
		clSecretsOn(t, h, clMC(t, h, 1))[0].Text, "a peer that is not found, on the old-tab path too")
}

// A liaison already in Secrets We Keep when this deploys has one seat written by
// the old code and one still to write: the first by a tab without the box, the
// second by a tab with it. Nothing about the stored state is new, so both go in
// and the phase moves on.
func TestLiaise_KeepSecret_AnInFlightLiaisonMixesOldAndNewTabs(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clAtSecretsWeKeep(t, h)

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, nil)
	require.Equalf(t, http.StatusOK, code, "old tab: %v", body)
	const bob = "I told her nothing, and she believed me."
	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, new(bob))
	require.Equalf(t, http.StatusOK, code, "new tab: %v", body)

	refreshed, err := h.q.GetPlanByID(context.Background(), m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	assert.Equal(t, string(LiaiseThingsWeShare), string(rd.EnsureLiaise().Phase), "both seats in: the liaison moves on")
	assert.Equal(t, clHeader(h, 0)+" — "+*m.plan.PreparationNotes, clSecretsOn(t, h, clMC(t, h, 0))[0].Text,
		"the seat the old tab wrote")
	assert.Equal(t, clHeader(h, 1)+" — "+bob, clSecretsOn(t, h, clMC(t, h, 1))[0].Text, "the seat the new tab wrote")
}

// A box left blank is refused, and refused before anything is written: the seat is
// not recorded, no secret is stored, and the player can write again.
func TestLiaise_KeepSecret_ABlankOrOverlongTextIsRefusedAndWritesNothing(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clAtSecretsWeKeep(t, h)
	mc := clMC(t, h, 0)

	for name, text := range map[string]string{
		"empty":      "",
		"whitespace": " \n\t ",
		"too long":   strings.Repeat("x", maxNarrativeLen+1),
	} {
		code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new(text))
		assert.Equalf(t, http.StatusBadRequest, code, "%s text must be refused: %v", name, body)
	}
	assert.Empty(t, clSecretsOn(t, h, mc), "no refused submission stored a secret")
	refreshed, err := h.q.GetPlanByID(context.Background(), m.plan.ID)
	require.NoError(t, err)
	rd := loadResolutionData(refreshed.ResolutionData)
	assert.Empty(t, rd.EnsureLiaise().KeptSecrets, "no refused submission took the seat")

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new("Now I have something to say."))
	assert.Equalf(t, http.StatusOK, code, "the same player can write again: %v", body)
}

// The words reach nobody else: not the bystander's socket, the plan they read, the
// log, nor the other seat. Existence stays public (secret.created, the log line).
func TestLiaise_KeepSecret_TheTextReachesNobodyElse(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	m := clAtSecretsWeKeep(t, h)
	ctx := context.Background()

	const alice = "The ledger is in the second drawer, under the loaf."
	const bob = "Marguerite wept, and I let her think I did not see."
	bystanderFrames := captureFrames(t, h.manager, h.tg.Game.ID, h.tg.Players[2])
	partnerFrames := captureFrames(t, h.manager, h.tg.Game.ID, h.tg.Players[1])

	code, body := clKeepSecretWith(t, h, m.plan.ID, 0, new(alice))
	require.Equalf(t, http.StatusOK, code, "preparer keep-secret: %v", body)

	// Positive first: the existence of the secret did reach them, so the checks
	// below are looking at a socket that was live.
	assertSecretCreatedFrame(t, bystanderFrames, clMC(t, h, 0))
	assertSecretCreatedFrame(t, partnerFrames, clMC(t, h, 0))

	// The partner, who has yet to write, cannot read it: it is on a different
	// player's asset, and only Things We Share lets anyone look.
	visible, err := h.q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
		AssetID: clMC(t, h, 0), PlayerID: h.tg.Players[1].ID,
	})
	require.NoError(t, err)
	assert.Empty(t, visible, "the partner cannot read the preparer's secret")

	code, body = clKeepSecretWith(t, h, m.plan.ID, 1, new(bob))
	require.Equalf(t, http.StatusOK, code, "partner keep-secret: %v", body)

	for _, who := range []struct {
		name   string
		frames func() []string
	}{{"bystander", bystanderFrames}, {"partner", partnerFrames}} {
		all := strings.Join(who.frames(), "\n")
		assert.NotContains(t, all, "ledger", "%s's socket carries the preparer's words", who.name)
		assert.NotContains(t, all, "Marguerite", "%s's socket carries the partner's words", who.name)
	}

	// The plan every player reads, and the log, carry which asset holds a secret
	// and who wrote it, never what it says.
	for idx := range h.tg.Players {
		code, planBody := h.get(idx, "/api/plans/"+itoa(m.plan.ID))
		require.Equalf(t, http.StatusOK, code, "player %d GET plan", idx)
		raw, err := json.Marshal(planBody)
		require.NoError(t, err)
		assert.NotContains(t, string(raw), "ledger", "player %d reads the plan", idx)
		assert.NotContains(t, string(raw), "Marguerite", "player %d reads the plan", idx)
	}
	posts, err := h.q.ListGamePosts(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	announced := 0
	for _, p := range posts {
		assert.NotContains(t, p.Body, "ledger")
		assert.NotContains(t, p.Body, "Marguerite")
		if p.SystemCode != nil && *p.SystemCode == "secret.written" {
			announced++
		}
	}
	assert.Equal(t, 2, announced, "two secrets, two lines naming the asset")
	assert.Len(t, clSecretsOn(t, h, clMC(t, h, 0)), 1, "existence is public: each asset bears one")
	assert.Len(t, clSecretsOn(t, h, clMC(t, h, 1)), 1)
}

// A Make Demands winner who stands in for the preparer writes the PREPARER's secret:
// the words are the winner's, but they sit on the preparer's asset and are authored by
// the preparer's seat, so the preparer (whose character it is) reads them and the
// winner who typed them does not. A stand-in's words are also as private as anyone's:
// the partner and the bystander are sent nothing.
func TestLiaise_KeepSecret_AStandInsWordsAreThePreparersSecret(t *testing.T) {
	h := newPlanLifecycle(t, 4)
	ctx := context.Background()
	const winnerIdx = 2
	preparerID, winnerID := h.tg.Players[0].ID, h.tg.Players[winnerIdx].ID

	m := clPrepareAndResolve(t, h)
	h.seedMadeDemand(winnerIdx, m.plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: winnerID,
	})
	clAdvanceAs(t, h, m.plan.ID, winnerIdx)

	const written = "She never did say where the ledger went; I wrote down the hour she left."
	bystanderFrames := captureFrames(t, h.manager, h.tg.Game.ID, h.tg.Players[3])
	preparerMC := clMC(t, h, 0)
	code, body := h.post(winnerIdx, clPath(m.plan.ID, "keep-secret"),
		map[string]any{"asset_id": preparerMC, "text": written})
	require.Equalf(t, http.StatusOK, code, "the winner fills the preparer's seat: %v", body)
	assert.EqualValues(t, preparerID, body["seat"])

	secrets := clSecretsOn(t, h, preparerMC)
	require.Len(t, secrets, 1)
	assert.Equal(t, clHeader(h, 0)+" — "+written, secrets[0].Text,
		"the winner's words are what is stored, headed with the preparer's meeting, not the winner")
	assert.Equal(t, preparerID, secrets[0].AuthorID, "authored by the preparer's seat, not the winner")

	readable := func(idx int) []dbgen.Secret {
		rows, err := h.q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
			AssetID: preparerMC, PlayerID: h.tg.Players[idx].ID,
		})
		require.NoError(t, err)
		return rows
	}
	assert.Len(t, readable(0), 1, "the preparer reads it: it is their character's secret")
	assert.Empty(t, readable(winnerIdx), "the winner who typed it does not")
	assert.Empty(t, readable(1), "nor does the partner")
	assert.NotContains(t, strings.Join(bystanderFrames(), "\n"), "ledger", "nor the bystander's socket")
}
