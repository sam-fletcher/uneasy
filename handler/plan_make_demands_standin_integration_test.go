//go:build integration

// handler/plan_make_demands_standin_integration_test.go — the owner's 2026-11-03
// stand-in ruling: the Make Demands winner of the perform option takes the choices
// and steps the preparer would make after the dice, while outcomes still land on
// the preparer.

package handler

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// The perform-steps Make Demands winner picks which of the PREPARER's assets
// shelters the hidden source's secret (principle 9). Before the ruling the
// secret had to go on the Make Demands winner's own asset, which also made the preparer's
// rumor lead nowhere near them.
func TestMakeDemandsRulings_StandIn_HideSourceGoesOnThePreparersAsset(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	subject, _ := h.seedPeerWithMarginalia(ownerIdx, "the Duchess's steward")
	_, err := h.pool.Exec(ctx, `UPDATE plans SET target_asset_id = $2 WHERE id = $1`, plan.ID, subject)
	require.NoError(t, err)
	winnerIdx := (prepIdx + 2) % 3
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[winnerIdx].ID,
	})
	prepAsset := h.seedPeer(prepIdx, "Cover Story")
	winnerAsset := h.seedPeer(winnerIdx, "Winner's own")

	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 1)
	code, body := h.post(winnerIdx, srPath(plan.ID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{"hide_source"}})
	require.Equalf(t, http.StatusOK, code, "the winner chooses: %v", body)

	// Their own asset is refused: the secret is about the preparer's rumor.
	code, body = h.post(winnerIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": winnerAsset})
	assert.Equalf(t, http.StatusForbidden, code, "the winner's own asset must be refused: %v", body)

	code, body = h.post(winnerIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": prepAsset})
	require.Equalf(t, http.StatusOK, code, "the winner picks the preparer's asset: %v", body)

	secrets, err := h.q.ListSecretsByAsset(ctx, prepAsset)
	require.NoError(t, err)
	require.Len(t, secrets, 1, "the secret sits on the preparer's asset")
	assert.Equal(t, h.tg.Players[winnerIdx].ID, secrets[0].AuthorID, "the winner wrote it")

	found := false
	for _, b := range rulingsPosts(t, h, "plan.spread_rumors") {
		if strings.Contains(b, "the Make Demands winner, hid") {
			found = true
			assert.NotContains(t, b, "northern pass", "the secret's text never reaches the log")
		}
	}
	assert.True(t, found, "the stand-in's hide-source needs its own log line")
}

// rulingsPosts returns the bodies of the game's system posts with one code.
func rulingsPosts(t *testing.T, h *planLifecycle, code string) []string {
	t.Helper()
	posts, err := h.q.ListGamePosts(t.Context(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil && *p.SystemCode == code {
			out = append(out, p.Body)
		}
	}
	return out
}

// The riposte break is a decision, so the perform-steps winner takes it. The
// peer is still the preparer's and still goes to the target; the preparer is
// refused, the bar names the Make Demands winner, and the log says who did it.
func TestMakeDemandsRulings_StandIn_RiposteBreakIsTheWinners(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)
	winnerIdx := 3 - preparerIdx - targetIdx
	winnerID := h.tg.Players[winnerIdx].ID
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{game.DemandOptionPerformSteps: winnerID})

	marg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 1, Text: "note"})
	require.NoError(t, err)
	_, err = h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 2, Text: "second"})
	require.NoError(t, err)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "the target chooses riposte: %v", body)
	h.assertWaitees("the winner owes the riposte break", model.RowStatePlanResolving, winnerID)

	breakPath := planPath(plan.ID, "riposte-break")
	code, _ = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": marg.ID})
	assert.Equal(t, http.StatusForbidden, code, "the preparer is locked out of the riposte break")
	code, body = h.post(winnerIdx, breakPath, map[string]any{"marginalia_id": marg.ID})
	require.Equalf(t, http.StatusOK, code, "the winner breaks it: %v", body)

	moved, err := h.q.GetAssetByID(ctx, requestedPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, moved.OwnerID, "the outcome still lands: the peer goes to the target")
	assert.Equal(t, plan.PreparerID, h.tg.Players[preparerIdx].ID)

	found := false
	for _, b := range rulingsPosts(t, h, "plan.exchange_courtiers") {
		if strings.Contains(b, "the Make Demands winner, damaged") {
			found = true
		}
	}
	assert.True(t, found, "the winner's break needs its own log line")
}

func TestMakeDemandsRulings_StandIn_WrapTheSceneIsTheWinners(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := chPrepareToRoll(t, h, "make", 2)
	winnerIdx := (preparerIdx + 1) % 3
	winnerID := h.tg.Players[winnerIdx].ID
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{game.DemandOptionPerformSteps: winnerID})

	path := planPath(plan.ID, "make-step")
	for i := 0; i < 5; i++ {
		code, body := h.post(winnerIdx, path, map[string]any{"option": "echo_present"})
		require.Equalf(t, http.StatusOK, code, "make-step %d: %v", i+1, body)
	}
	h.assertWaitees("the winner wraps the scene", model.RowStatePlanResolving, winnerID)

	code, _ := h.post(preparerIdx, planPath(plan.ID, "complete"), nil)
	assert.Equal(t, http.StatusForbidden, code, "the preparer no longer wraps it")
	code, body := h.post(winnerIdx, planPath(plan.ID, "complete"), nil)
	require.Equalf(t, http.StatusOK, code, "the winner wraps the scene: %v", body)
	h.requireEnded(plan.ID)

	found := false
	for _, b := range rulingsPosts(t, h, "plan.chronicle_histories") {
		if strings.Contains(b, "the Make Demands winner, wrapped the scene") {
			found = true
		}
	}
	assert.True(t, found, "the winner's wrap needs its own log line")
}

// They pick which of the preparer's assets breaks, and learn its secrets,
// because whoever breaks an asset may look under it. Already built; pinned here.
func TestMakeDemandsRulings_StandIn_BreakYourselfBreaksThePreparersAsset(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, preparerIdx := spPrepareToMar(t, h)
	winnerIdx := (preparerIdx + 1) % 3
	winnerID := h.tg.Players[winnerIdx].ID
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{game.DemandOptionPerformSteps: winnerID})

	peer := h.seedPeer(preparerIdx, "thin-skinned peer")
	marg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: peer, Position: 1, Text: "a cherished note"})
	require.NoError(t, err)
	secret, err := h.q.CreateSecret(ctx, dbgen.CreateSecretParams{AssetID: peer, AuthorID: h.tg.Players[preparerIdx].ID, Text: "hidden"})
	require.NoError(t, err)
	ownAsset := h.seedPeer(winnerIdx, "the winner's own")
	ownMarg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: ownAsset, Position: 1, Text: "mine"})
	require.NoError(t, err)

	code, body := h.post(winnerIdx, planPath(plan.ID, "make-choice"),
		map[string]any{"result": "mar", "choices": []string{"break_self"}})
	require.Equalf(t, http.StatusOK, code, "the winner chooses: %v", body)

	breakPath := planPath(plan.ID, "break-self")
	code, _ = h.post(winnerIdx, breakPath, map[string]any{"marginalia_id": ownMarg.ID})
	assert.Equal(t, http.StatusForbidden, code, "the winner cannot break their own asset instead")
	code, _ = h.post(preparerIdx, breakPath, map[string]any{"marginalia_id": marg.ID})
	assert.Equal(t, http.StatusForbidden, code, "the preparer is locked out")
	code, body = h.post(winnerIdx, breakPath, map[string]any{"marginalia_id": marg.ID})
	require.Equalf(t, http.StatusOK, code, "the winner breaks the preparer's asset: %v", body)

	torn, err := h.q.GetMarginaliaByID(ctx, marg.ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn)
	visible, err := h.q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{AssetID: peer, PlayerID: winnerID})
	require.NoError(t, err)
	require.Len(t, visible, 1, "the winner learns the broken asset's secrets")
	assert.Equal(t, secret.ID, visible[0].ID)
}

// Perform-steps is post-roll only, so the kickoff of a pending Chronicle stays the
// preparer's, even with a perform-steps Make Demands winner against it.
func TestMakeDemandsRulings_ChronicleKickoffStaysThePreparers(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanChronicleHistories, PreparationNotes: &notes})
	preparerIdx := h.preparerIdxFor(plan.ID)
	winnerIdx := (preparerIdx + 1) % 3
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[winnerIdx].ID,
	})
	h.jumpToRow(*plan.RowNumber)
	code, _ := h.post(winnerIdx, planPath(plan.ID, "resolve"), nil)
	assert.Equal(t, http.StatusForbidden, code, "the Make Demands winner does not start the plan")
	code, body := h.post(preparerIdx, planPath(plan.ID, "resolve"), nil)
	assert.Equalf(t, http.StatusOK, code, "the preparer does: %v", body)
}

// The wrap line is anchored to the row, comes after the resolution it describes,
// and is posted once. On a mar the Make Demands winner wraps too.
func TestMakeDemandsRulings_WrapOnAMarIsTheWinnersAndLoggedOnce(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanChronicleHistories, PreparationNotes: &notes})
	preparerIdx := h.preparerIdxFor(plan.ID)
	winnerIdx := (preparerIdx + 1) % 3
	winnerID := h.tg.Players[winnerIdx].ID
	saPinKnowledgeRank(t, h, plan.PreparerID, 3)
	artifactID, _ := chSeedArtifact(t, h, winnerIdx, "ancient codex", 2)
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{game.DemandOptionPerformSteps: winnerID})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))
	roll := chCastRoll(t, h, plan.ID, artifactID)
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	for i := range 3 {
		code, body := h.post(i, planPath(plan.ID, "mar-choice"), map[string]any{"choice": "echo_present"})
		require.Equalf(t, http.StatusOK, code, "player %d: %v", i, body)
	}
	h.assertWaitees("the winner wraps the marred scene", model.RowStatePlanResolving, winnerID)
	code, _ := h.post(preparerIdx, planPath(plan.ID, "complete"), nil)
	assert.Equal(t, http.StatusForbidden, code, "the preparer is locked out on a mar too")
	code, body := h.post(winnerIdx, planPath(plan.ID, "complete"), nil)
	require.Equalf(t, http.StatusOK, code, "the winner wraps it: %v", body)

	posts, err := h.q.ListGamePosts(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	var wrapID, resolvedID int64
	wraps := 0
	for _, p := range posts {
		if strings.Contains(p.Body, "wrapped the scene") {
			wraps++
			wrapID = p.ID
			require.NotNil(t, p.RowNumber, "the wrap line is anchored to a row")
			assert.Equal(t, h.tg.Game.CurrentRow, *p.RowNumber)
		}
		if p.SystemCode != nil && strings.HasPrefix(*p.SystemCode, "plan.resolved") {
			resolvedID = p.ID
		}
	}
	assert.Equal(t, 1, wraps)
	require.NotZero(t, resolvedID, "the plan's own resolution is logged")
	assert.Greater(t, wrapID, resolvedID, "the wrap is posted after the plan resolves, not before")
}

func srStandInHideSource(t *testing.T) (h *planLifecycle, plan dbgen.Plan, prepIdx, winnerIdx int) {
	t.Helper()
	h = newPlanLifecycle(t, 3)
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	subject, _ := h.seedPeerWithMarginalia(ownerIdx, "the Duchess's steward")
	_, err := h.pool.Exec(t.Context(), `UPDATE plans SET target_asset_id = $2 WHERE id = $1`, plan.ID, subject)
	require.NoError(t, err)
	winnerIdx = (prepIdx + 2) % 3
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[winnerIdx].ID,
	})
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 1)
	code, body := h.post(winnerIdx, srPath(plan.ID, "make-choice"),
		map[string]any{"result": "make", "choices": []string{"hide_source"}})
	require.Equalf(t, http.StatusOK, code, "the winner chooses: %v", body)
	return h, plan, prepIdx, winnerIdx
}

// The default secret text says whose rumor it hides; it must not say "you" when a
// stand-in writes it about the preparer.
func TestMakeDemandsRulings_StandInHideSourceSecretNamesThePreparer(t *testing.T) {
	h, plan, prepIdx, winnerIdx := srStandInHideSource(t)
	prepAsset := h.seedPeer(prepIdx, "Cover Story")
	code, body := h.post(winnerIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": prepAsset})
	require.Equalf(t, http.StatusOK, code, "hide-source: %v", body)
	secrets, err := h.q.ListSecretsByAsset(t.Context(), prepAsset)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	assert.NotContains(t, secrets[0].Text, "You were the source")
	assert.NotContains(t, secrets[0].Text, "@@", "the secret is read as plain text, so it carries no log markup")
	assert.Contains(t, secrets[0].Text, h.tg.Players[prepIdx].DisplayName)
}

// The secret is about the preparer's own rumor on the preparer's own asset, so
// the preparer can read it. Nobody else gains access, and the preparer gains no
// other secret already tucked under that asset.
func TestMakeDemandsRulings_StandInHideSourceSecretIsReadableByThePreparerOnly(t *testing.T) {
	h, plan, prepIdx, winnerIdx := srStandInHideSource(t)
	ctx := t.Context()
	prepAsset := h.seedPeer(prepIdx, "Cover Story")
	otherIdx := 3 - prepIdx - winnerIdx
	// An older secret on the same asset, written by the bystander and shown to nobody else.
	older, err := createSecret(ctx, h.q, prepAsset, h.tg.Players[otherIdx].ID, "an older secret")
	require.NoError(t, err)

	code, body := h.post(winnerIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": prepAsset})
	require.Equalf(t, http.StatusOK, code, "hide-source: %v", body)

	visibleTo := func(idx int) []string {
		rows, err := h.q.ListVisibleSecrets(ctx, dbgen.ListVisibleSecretsParams{
			AssetID: prepAsset, PlayerID: h.tg.Players[idx].ID,
		})
		require.NoError(t, err)
		texts := make([]string, 0, len(rows))
		for _, r := range rows {
			texts = append(texts, r.Text)
		}
		return texts
	}
	prepSees := visibleTo(prepIdx)
	require.Len(t, prepSees, 1, "the preparer reads the new secret and nothing else on the asset")
	assert.Contains(t, prepSees[0], h.tg.Players[prepIdx].DisplayName)
	assert.NotContains(t, prepSees, older.Text)
	assert.Len(t, visibleTo(winnerIdx), 1, "the Make Demands winner wrote it, so reads it")
	assert.Equal(t, []string{older.Text}, visibleTo(otherIdx), "a bystander sees only their own older secret")
}

// Forfeiting a hide-source pick counts the preparer's assets, not the stand-in's.
func TestMakeDemandsRulings_StandInHideSourceForfeitCountsThePreparersAssets(t *testing.T) {
	h, plan, prepIdx, winnerIdx := srStandInHideSource(t)
	ctx := t.Context()
	_, err := h.pool.Exec(ctx, `UPDATE assets SET is_destroyed = true WHERE owner_id = $1`, h.tg.Players[winnerIdx].ID)
	require.NoError(t, err)
	code, body := h.post(winnerIdx, srPath(plan.ID, "sr-forfeit-step"), map[string]any{"step": "hide_source"})
	assert.Equalf(t, http.StatusConflict, code, "the preparer still has assets to hide under: %v", body)

	_, err = h.pool.Exec(ctx, `UPDATE assets SET is_destroyed = true WHERE owner_id = $1`, h.tg.Players[prepIdx].ID)
	require.NoError(t, err)
	code, body = h.post(winnerIdx, srPath(plan.ID, "sr-forfeit-step"), map[string]any{"step": "hide_source"})
	assert.Equalf(t, http.StatusOK, code, "no preparer asset left, so the pick is forfeited: %v", body)
}

// Surrendering the peer intact is a decision too: the Make Demands winner may take
// it, and the log says who decided.
func TestMakeDemandsRulings_StandInRiposteSkipIsLogged(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := t.Context()
	plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)
	winnerIdx := 3 - preparerIdx - targetIdx
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[winnerIdx].ID,
	})
	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "riposte: %v", body)
	skip := map[string]any{"action": "skip"}
	code, _ = h.post(preparerIdx, planPath(plan.ID, "riposte-break"), skip)
	assert.Equal(t, http.StatusForbidden, code, "the preparer is locked out of the surrender")
	code, body = h.post(winnerIdx, planPath(plan.ID, "riposte-break"), skip)
	require.Equalf(t, http.StatusOK, code, "the winner surrenders it: %v", body)
	moved, err := h.q.GetAssetByID(ctx, requestedPeer)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[targetIdx].ID, moved.OwnerID)
	found := false
	for _, b := range rulingsPosts(t, h, "plan.exchange_courtiers") {
		if strings.Contains(b, "the Make Demands winner, surrendered") {
			found = true
		}
	}
	assert.True(t, found, "the surrender names the Make Demands winner who decided")
}
