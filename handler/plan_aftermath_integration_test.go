//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// postsByCodePrefix lists the game's posts whose system code starts with prefix, oldest first.
func postsByCodePrefix(t *testing.T, h *planLifecycle, prefix string) []dbgen.ScenePost {
	t.Helper()
	all, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []dbgen.ScenePost
	for _, p := range all {
		if p.SystemCode != nil && strings.HasPrefix(*p.SystemCode, prefix) {
			out = append(out, p)
		}
	}
	return out
}

func lastPost(t *testing.T, posts []dbgen.ScenePost) dbgen.ScenePost {
	t.Helper()
	require.NotEmpty(t, posts)
	return posts[len(posts)-1]
}

// The post that ends a plan says who acts next, read from the live wait state.
// A declined counter-propaganda whose parent still owes a step is the case the
// obvious answer gets wrong: the next actor is the parent's preparer, not the
// focus player, and the line must not read as the parent plan ending.
func TestPlanResolvedPost_CounterDeclined_NamesParentStepAndIsNotAFailure(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx := spPrepared(t, h)
	h.seedPeer(prepIdx, "peer to give")
	roll := h.resolveAndCast(plan.ID)
	interfererIdx := (prepIdx + 1) % 3
	_, err := h.q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
		RollID: roll.ID, PlayerID: h.tg.Players[interfererIdx].ID, IsInterference: true,
	})
	require.NoError(t, err)
	h.forceRoll(roll.ID, "mar", 0)
	h.makeChoice(plan.ID, "mar", []string{spOptGivePeer, spOptCounterProp})
	parent := mustPlan(t, h, plan.ID)
	childID := *loadResolutionData(parent.ResolutionData).SpreadPropaganda.RecursivePlanID

	code, body := h.post(interfererIdx, spPath(childID, "sp-decline-counter"), nil)
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)

	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.NotContains(t, got.Body, "came to nothing", "a decline is not a failure")
	assert.Contains(t, got.Body, "counter-propaganda is over")
	assert.Contains(t, got.Body, "not ended by this", "the parent plan is said to carry on")
	assert.Contains(t, got.Body, "Next: "+mark(h, prepIdx)+" to carry on with Spread Propaganda.")
	assert.Equal(t, model.SeverityImportant, got.Severity)
}

// Under a Make Demands perform win the stand-in acts, so the closing post names
// the stand-in as next, not the preparer the plan belongs to.
func TestPlanResolvedPost_NamesTheMakeDemandsStandIn(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx := spPrepared(t, h)
	h.seedPeer(prepIdx, "peer to give")
	interfererIdx := (prepIdx + 1) % 3
	winnerIdx := (prepIdx + 2) % 3
	roll := h.resolveAndCast(plan.ID)
	_, err := h.q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
		RollID: roll.ID, PlayerID: h.tg.Players[interfererIdx].ID, IsInterference: true,
	})
	require.NoError(t, err)
	h.seedMadeDemand(winnerIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[winnerIdx].ID,
	})
	h.forceRoll(roll.ID, "mar", 0)
	h.makeChoiceAs(winnerIdx, plan.ID, "mar", []string{spOptGivePeer, spOptCounterProp})
	parent := mustPlan(t, h, plan.ID)
	childID := *loadResolutionData(parent.ResolutionData).SpreadPropaganda.RecursivePlanID

	code, body := h.post(interfererIdx, spPath(childID, "sp-decline-counter"), nil)
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)

	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, "Next: "+mark(h, winnerIdx)+" to carry on with Spread Propaganda.")
	assert.NotContains(t, got.Body, "Next: "+mark(h, prepIdx))
}

// A plan that costs a player their main character says so, and says who
// replaces it, in the one post that ends the plan.
func TestPlanResolvedPost_NamesALostMainCharacterAndTheReplacementStep(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx := spPrepared(t, h)
	roll := h.resolveAndCast(plan.ID)
	h.forceRoll(roll.ID, "make", 6) // the open roll would be the wait state otherwise
	victim := (prepIdx + 1) % 3
	_, err := h.pool.Exec(ctx, `UPDATE assets SET is_main_character = false WHERE id = $1`, h.mainCharacterID(victim))
	require.NoError(t, err)

	p := mustPlan(t, h, plan.ID)
	require.NoError(t, resolvePlan(ctx, h.q, h.manager, &p, "make"))

	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, mark(h, victim)+" lost their main character. Next: "+
		mark(h, victim)+" to choose a new main character")
}

// Where another wait takes precedence, the loss is still named, as a thing
// that will happen, and the next step is whatever the wait state says.
func TestPlanResolvedPost_LostMainCharacterNamedEvenWhileAnotherStepIsFirst(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx := spPrepared(t, h)
	h.resolveAndCast(plan.ID) // roll left open: it is the wait state
	victim := (prepIdx + 1) % 3
	_, err := h.pool.Exec(ctx, `UPDATE assets SET is_main_character = false WHERE id = $1`, h.mainCharacterID(victim))
	require.NoError(t, err)
	p := mustPlan(t, h, plan.ID)
	require.NoError(t, resolvePlan(ctx, h.q, h.manager, &p, "make"))

	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, mark(h, victim)+" has no main character and will choose a replacement.")
	assert.Contains(t, got.Body, "to take part in the dice roll.")
}

// With nothing lost and nothing owed, the post still says whose move it is.
func TestPlanResolvedPost_NamesWhoActsNext(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, _ := spPrepared(t, h)
	h.resolveAndCast(plan.ID)
	p := mustPlan(t, h, plan.ID)
	require.NoError(t, resolvePlan(context.Background(), h.q, h.manager, &p, "make"))

	rs := h.rowState()
	require.NotEmpty(t, rs.ActingPlayerIDs)
	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, "Next: ")
	for _, pl := range h.tg.Players {
		if pl.ID == rs.ActingPlayerIDs[0] {
			assert.Contains(t, got.Body, "Next: "+playerMark(pl.ID, pl.DisplayName)+" to ")
		}
	}
}

// A break of another player's asset logs at Important so its owner gets the
// important-unread marker: the plan's own line, and the asset.destroyed line that
// follows when the break destroys it (a blank asset has no tear post, so that
// line is the whole story).
func TestBreakOfAnotherPlayersAsset_LogsImportant(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	resourceID, margIDs := saSeedResource(t, h, otherIdx, "ledger", 2)
	blankID, _ := saSeedResource(t, h, otherIdx, "blank ledger", 0)
	h.makeChoice(plan.ID, "make", []string{"break_resource", "break_resource"})

	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/break-resource"
	code, body := h.post(preparerIdx, path, map[string]any{"asset_id": resourceID, "marginalia_id": margIDs[0]})
	require.Equalf(t, http.StatusOK, code, "break: %v", body)
	broke := lastPost(t, postsByCodePrefix(t, h, "plan.seek_answers"))
	assert.Contains(t, broke.Body, "broke")
	assert.Equal(t, model.SeverityImportant, broke.Severity)

	code, body = h.post(preparerIdx, path, map[string]any{"asset_id": blankID})
	require.Equalf(t, http.StatusOK, code, "blank break: %v", body)
	gone := lastPost(t, postsByCodePrefix(t, h, "asset.destroyed"))
	assert.Contains(t, gone.Body, "blank ledger")
	assert.Equal(t, model.SeverityImportant, gone.Severity, "a blank asset's destruction reaches its owner")
}

// Chronicle Histories raised the ruling: breaking another player's invoked artifact.
func TestChronicleBreakOfAnotherPlayersArtifact_LogsImportant(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	notes := "the lost charter"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanChronicleHistories, PreparationNotes: &notes})
	require.NotNil(t, plan.RowNumber)
	preparerIdx := h.preparerIdxFor(plan.ID)
	saPinKnowledgeRank(t, h, plan.PreparerID, 1)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	artifactID, margIDs := chSeedArtifact(t, h, otherIdx, "brittle scroll", 2)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID))
	roll := chCastRoll(t, h, plan.ID, artifactID)
	h.forceRoll(roll.ID, "make", roll.Difficulty)

	code, body := chMakeStep(h, plan.ID, "break_artifact", artifactID, margIDs[0])
	require.Equalf(t, http.StatusOK, code, "break_artifact: %v", body)
	got := lastPost(t, postsByCodePrefix(t, h, "plan.chronicle_histories"))
	assert.Contains(t, got.Body, "broke")
	assert.Equal(t, model.SeverityImportant, got.Severity)
}

// The other side of the rule: a player breaking their own asset stays routine.
func TestExchangeCourtiersRiposteSelfBreak_StaysDefault(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)
	marg, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 1, Text: "note"})
	require.NoError(t, err)
	_, err = h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 2, Text: "second"})
	require.NoError(t, err)
	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "riposte: %v", body)

	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/riposte-break"
	code, body = h.post(preparerIdx, path, map[string]any{"marginalia_id": marg.ID})
	require.Equalf(t, http.StatusOK, code, "riposte-break: %v", body)
	var got *dbgen.ScenePost
	for _, p := range postsByCodePrefix(t, h, "plan.exchange_courtiers") {
		if strings.Contains(p.Body, "The torn marginalia read") {
			got = &p
		}
	}
	require.NotNil(t, got, "the riposte break is logged")
	assert.Equal(t, model.SeverityDefault, got.Severity, "the preparer broke their own peer")
}

// A take or destruction says "That was X's main character" only when the asset in
// hand was one: not for an ordinary asset of a player who is already short, and not
// for a second take in the same plan.
func TestMainCharacterNote_FollowsTheAssetNotTheOwner(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	victim := 1
	mc := h.mainCharacterID(victim)
	spare := h.seedPeer(victim, "spare")
	gameID := h.tg.Game.ID

	// The main character is destroyed: the post says so.
	mcAsset, err := h.q.GetAssetByID(ctx, mc)
	require.NoError(t, err)
	require.NoError(t, h.q.DestroyAsset(ctx, mc))
	EmitAssetDestroyed(ctx, h.q, h.manager, gameID, mcAsset, nil)
	assert.Contains(t, lastPost(t, postsByCodePrefix(t, h, "asset.destroyed")).Body,
		"That was "+mark(h, victim)+"'s main character.")

	// The owner now lacks a main character; an ordinary asset destroyed or taken next must not claim to be one.
	spareAsset, err := h.q.GetAssetByID(ctx, spare)
	require.NoError(t, err)
	third := h.seedPeer(victim, "third")
	thirdAsset, err := h.q.GetAssetByID(ctx, third)
	require.NoError(t, err)
	EmitAssetDestroyed(ctx, h.q, h.manager, gameID, spareAsset, nil)
	assert.NotContains(t, lastPost(t, postsByCodePrefix(t, h, "asset.destroyed")).Body, "main character")
	EmitAssetTaken(ctx, h.q, h.manager, gameID, thirdAsset, h.tg.Players[victim].ID, h.tg.Players[0].ID, false, nil)
	assert.NotContains(t, lastPost(t, postsByCodePrefix(t, h, "asset.taken")).Body, "main character")
	// and a take that really was the main character says so.
	EmitAssetTaken(ctx, h.q, h.manager, gameID, thirdAsset, h.tg.Players[victim].ID, h.tg.Players[0].ID, true, nil)
	assert.Contains(t, lastPost(t, postsByCodePrefix(t, h, "asset.taken")).Body, "main character.")
}

// A surrender-claim take is logged by Make War itself, not by EmitAssetTaken, so it
// carries the note on its own. Claiming the surrendered player's main character says so.
func TestSurrenderClaimTakeOfAMainCharacterIsNamed(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	_, margA := h.seedPeerWithMarginalia(prepIdx, "Sacrificed asset")
	ordinary := h.seedPeer(prepIdx, "Ordinary spoils")
	h.jumpToRow(warRow + 1)
	prepID := h.tg.Players[prepIdx].ID
	enemyID := h.tg.Players[enemyIdx].ID
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id": enemyID, "choice": "break_asset", "marginalia_id": margA, "surrender": true,
	})
	require.Equalf(t, http.StatusOK, code, "surrender pay: %v", body)

	mc := h.mainCharacterID(prepIdx)
	code, body = h.post(enemyIdx, warRoute(planID, "take-surrender-asset"), map[string]any{
		"surrendered_id": prepID, "asset_id": mc,
	})
	require.Equalf(t, http.StatusOK, code, "claim the main character: %v", body)
	posts := warSystemPosts(t, h.q, h.tg.Game.ID)
	assert.True(t, anyContains(posts, "That was "+mark(h, prepIdx)+"'s main character."), "%v", posts)
	_ = ordinary
}

// A battle cost that destroys a main character says so in the destruction line.
func TestBattleCostDestroyingAMainCharacterIsNamed(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	ctx := context.Background()
	planID, prepIdx, enemyIdx, warRow := seedActiveWar(t, h)
	h.jumpToRow(warRow + 1)
	mc := h.mainCharacterID(prepIdx)
	// A single marginalia, so the break destroys the main character.
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: mc, Position: 1, Text: "the crown's favourite"})
	require.NoError(t, err)
	code, body := h.post(prepIdx, warRoute(planID, "pay-battle-cost"), map[string]any{
		"opponent_id": h.tg.Players[enemyIdx].ID, "choice": "break_asset", "marginalia_id": m.ID,
	})
	require.Equalf(t, http.StatusOK, code, "pay: %v", body)
	got := lastPost(t, postsByCodePrefix(t, h, "asset.destroyed"))
	assert.Contains(t, got.Body, "That was "+mark(h, prepIdx)+"'s main character.")
}

// Two players who lost theirs read as a plural, not "X and Y has no".
func TestPlanResolvedPost_PluralLostMainCharacters(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx := spPrepared(t, h)
	roll := h.resolveAndCast(plan.ID)
	h.forceRoll(roll.ID, "make", 6)
	a, b := (prepIdx+1)%3, (prepIdx+2)%3
	for _, v := range []int{a, b} {
		_, err := h.pool.Exec(ctx, `UPDATE assets SET is_main_character = false WHERE id = $1`, h.mainCharacterID(v))
		require.NoError(t, err)
	}
	p := mustPlan(t, h, plan.ID)
	require.NoError(t, resolvePlan(ctx, h.q, h.manager, &p, "make"))
	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, "lost their main characters")
	assert.NotContains(t, got.Body, " has no main character")
}

// A declined counter-propaganda says what became of the plan it answered, from that
// plan's real state: a mar whose only pick was the counter owes nothing, so the
// parent has already resolved.
func TestPlanResolvedPost_CounterDeclined_ParentAlreadyEnded(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	_, child, interfererIdx := spCounterPropToPreRoll(t, h)
	code, body := h.post(interfererIdx, spPath(child.ID, "sp-decline-counter"), nil)
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	got := lastPost(t, postsByCodePrefix(t, h, "plan.resolved"))
	assert.Contains(t, got.Body, "has already ended")
	assert.NotContains(t, got.Body, "not ended by this")
}

func TestBreakSeverity(t *testing.T) {
	assert.Equal(t, model.SeverityImportant, breakSeverity(1, 2), "another player's asset")
	assert.Equal(t, model.SeverityDefault, breakSeverity(2, 2), "a player's own cost")
}

// mark is how the log writes a player's name.
func mark(h *planLifecycle, idx int) string {
	return playerMark(h.tg.Players[idx].ID, h.tg.Players[idx].DisplayName)
}
