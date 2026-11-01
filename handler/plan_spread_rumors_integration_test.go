//go:build integration

// handler/plan_spread_rumors_integration_test.go — the Spread Rumors batch
// (adr/audit/FINDINGS.md Tier 3 #9, #10; rules Q1, Q2).
//
//   - The pre-roll: the plan parks with no roll until the preparer states the
//     rumor and how it spreads; that narration is posted BEFORE the roll exists,
//     is the committed rumor (not the prep draft), and is what makes a secret
//     rumor public — on a make or a mar.
//   - (e) is gloating: a conspicuous log line, no change of attribution.
//   - The mar: the target asset's owner writes the counter-rumor, which becomes
//     the Rumors-sheet row (about the preparer's main character, sourced to its
//     author unless they hide); break and leverage land on that main character.
//   - Hygiene found by the re-aim session: target validation, and a Make Demands
//     winner retargeting a secret rumor.

package handler

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

func srPath(planID int64, route string) string { return "/api/plans/" + itoa(planID) + "/" + route }

// playerPosts returns the bodies of every non-system post (the scene posts and
// chat a player authored) in the game.
func (h *planLifecycle) playerPosts() []string {
	h.t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(h.t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode == nil {
			out = append(out, p.Body)
		}
	}
	return out
}

// srRumorAbout prepares a Spread Rumors by the focus player about a fresh peer of
// the next player and returns the plan, both indices and the subject. The plan is
// left pending; resolve with srToPreRoll.
func srRumorAbout(t *testing.T, h *planLifecycle, secretAsset *int64) (plan dbgen.Plan, prepIdx, ownerIdx int, subject int64) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	ownerIdx = (prepIdx + 1) % len(h.tg.Players)
	subject = h.seedPeer(ownerIdx, "the Duchess")
	notes := "she poisons her own wine"
	plan = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanSpreadRumors,
		TargetAssetID:    &subject,
		PreparationNotes: &notes,
		SecretAssetID:    secretAsset,
	})
	require.NotNil(t, plan.RowNumber)
	return plan, prepIdx, ownerIdx, subject
}

// srCast drives sr-cast-roll as the preparer with the given rumor and returns the
// roll. The "how" is fixed.
func srCast(t *testing.T, h *planLifecycle, planID int64, rumor string) *dbgen.DiceRoll {
	t.Helper()
	code, body := h.post(h.preparerIdxFor(planID), srPath(planID, "sr-cast-roll"),
		map[string]any{"rumor": rumor, "how": "Whispered over every tavern table."})
	require.Equalf(t, http.StatusOK, code, "sr-cast-roll: %v", body)
	roll, err := h.q.GetDiceRollByPlanID(context.Background(), &planID)
	require.NoError(t, err)
	return &roll
}

// srForceMar forces a mar at result 0 with the difficulty raised to 6, so a test
// may pick up to six options regardless of the seeded ranks.
func srForceMar(t *testing.T, h *planLifecycle, roll *dbgen.DiceRoll) {
	t.Helper()
	_, err := h.pool.Exec(context.Background(), `UPDATE dice_rolls SET difficulty = 6 WHERE id = $1`, roll.ID)
	require.NoError(t, err)
	h.forceRoll(roll.ID, marOutcome, 0)
}

// ── Pre-roll ─────────────────────────────────────────────────────────────────

func TestSpreadRumors_PreRoll_ParksWithNoRollUntilTheRumorIsStated(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)

	require.Nil(t, h.resolve(plan.ID), "the plan parks at its pre-roll: no roll yet")
	_, err := h.q.GetDiceRollByPlanID(ctx, &plan.ID)
	require.Error(t, err)
	h.assertWaitees("the preparer owes the narration", model.RowStatePlanResolving, h.tg.Players[prepIdx].ID)

	// Nothing can be chosen past a narration nobody wrote.
	code, body := h.post(prepIdx, srPath(plan.ID, "make-choice"), map[string]any{"result": "make", "choices": []string{}})
	assert.Equalf(t, http.StatusConflict, code, "make-choice before the roll must be refused: %v", body)

	// Only the preparer casts; both fields are required.
	code, _ = h.post(ownerIdx, srPath(plan.ID, "sr-cast-roll"), map[string]any{"rumor": "x", "how": "y"})
	assert.Equal(t, http.StatusForbidden, code)
	code, _ = h.post(prepIdx, srPath(plan.ID, "sr-cast-roll"), map[string]any{"rumor": "x", "how": ""})
	assert.Equal(t, http.StatusBadRequest, code, "the vector is part of the step")
	code, _ = h.post(prepIdx, srPath(plan.ID, "sr-cast-roll"), map[string]any{"rumor": "  ", "how": "y"})
	assert.Equal(t, http.StatusBadRequest, code)
	_, err = h.q.GetDiceRollByPlanID(ctx, &plan.ID)
	require.Error(t, err, "a refused cast opens nothing")

	roll := srCast(t, h, plan.ID, "The Duchess was never born at all.")
	assert.NotZero(t, roll.ID)
	// With the roll open but unsettled there is still no branch to choose from.
	code, body = h.post(prepIdx, srPath(plan.ID, "make-choice"), map[string]any{"result": "make", "choices": []string{}})
	assert.Equalf(t, http.StatusConflict, code, "make-choice while the dice are still rolling must be refused: %v", body)
	code, _ = h.post(prepIdx, srPath(plan.ID, "sr-cast-roll"), map[string]any{"rumor": "again", "how": "again"})
	assert.Equal(t, http.StatusConflict, code, "the dice are cast once")

	got, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	sr := loadResolutionData(got.ResolutionData).SpreadRumors
	require.NotNil(t, sr)
	assert.True(t, sr.PreRollDone)
	assert.Equal(t, "The Duchess was never born at all.", sr.RumorText)
	assert.Equal(t, "Whispered over every tavern table.", sr.HowSpread)
}

// The narration is the thing other players read to decide whether to help or
// interfere (principle 1), so it must be on the record before the roll exists.
func TestSpreadRumors_PreRoll_NarrationIsPostedBeforeTheRollOpens(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, _, _ := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)

	roll := srCast(t, h, plan.ID, "The Duchess was never born at all.")

	posts, err := h.q.ListGamePosts(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	var narration *dbgen.ScenePost
	for i := range posts {
		if posts[i].SystemCode == nil && strings.Contains(posts[i].Body, "The Duchess was never born at all.") {
			narration = &posts[i]
		}
	}
	require.NotNil(t, narration, "the stated rumor is posted for the table")
	assert.Contains(t, narration.Body, "Whispered over every tavern table.", "and so is how it spreads")
	require.NotNil(t, narration.AuthorID)
	assert.Equal(t, h.tg.Players[prepIdx].ID, *narration.AuthorID, "in the preparer's own name")
	assert.False(t, narration.CreatedAt.Time.After(roll.CreatedAt.Time),
		"the narration (%v) must exist no later than the roll (%v)", narration.CreatedAt.Time, roll.CreatedAt.Time)
}

// The book: "optionally write the rumor … if you'd like to keep it secret for
// now". Stating it in the pre-roll is what makes it public — on a make AND on a
// mar — and the stated text, not the draft, is the rumor that is spread.
func TestSpreadRumors_PreRoll_SecretRumorBecomesPublicWhenStated(t *testing.T) {
	for _, outcome := range []string{makeOutcome, marOutcome} {
		t.Run(outcome, func(t *testing.T) {
			h := newPlanLifecycle(t, 3)
			ctx := context.Background()
			prepIdx := h.focusPlayerIdx()
			holder := h.seedPeer(prepIdx, "Brutus")
			plan, _, _, _ := srRumorAbout(t, h, &holder)
			h.jumpToRow(*plan.RowNumber)
			h.resolve(plan.ID)

			// Nothing public yet: the draft is in the Secret and nowhere else.
			assert.False(t, anyContains(h.playerPosts(), "poisons her own wine"))

			// The player types the rumor — different from the draft: it is committed here.
			roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
			assert.True(t, anyContains(h.playerPosts(), "The Duchess sold the northern pass."),
				"stating it makes it public, even though it was kept secret at prep")

			if outcome == makeOutcome {
				h.forceRoll(roll.ID, makeOutcome, 0)
				h.makeChoice(plan.ID, makeOutcome, []string{})
				rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
				require.NoError(t, err)
				require.Len(t, rumors, 1)
				assert.Equal(t, "The Duchess sold the northern pass.", rumors[0].Text,
					"a make spreads the STATED rumor, not the draft")
			} else {
				srForceMar(t, h, roll)
				rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
				require.NoError(t, err)
				assert.Empty(t, rumors, "a failed rumor never reaches the Rumors sheet")
			}

			// The Secret row stays; its text is never copied into any log body.
			secrets, err := h.q.ListSecretsByAsset(ctx, holder)
			require.NoError(t, err)
			require.Len(t, secrets, 1)
			assert.Equal(t, "she poisons her own wine", secrets[0].Text)
			for _, p := range gamePostBodies(t, h) {
				assert.NotContains(t, p, "poisons her own wine", "secret text must never reach a log body")
			}
		})
	}
}

// ── (e) is gloating ──────────────────────────────────────────────────────────

func TestSpreadRumors_Make_RevealSourceIsGloatingNotAttribution(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, _, subject := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 1)
	h.makeChoice(plan.ID, makeOutcome, []string{"reveal_source"})

	prep := h.tg.Players[prepIdx]
	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	require.NotNil(t, rumors[0].SourcePlayerID, "without (d) the characters know who started it")
	assert.Equal(t, prep.ID, *rumors[0].SourcePlayerID)

	logs := h.logBodies("plan.spread_rumors")
	require.Truef(t, anyContains(logs, "basks in the glory"), "(e) must say something happened: %v", logs)
	gloat := ""
	for _, l := range logs {
		if strings.Contains(l, "basks in the glory") {
			gloat = l
		}
	}
	assert.Contains(t, gloat, "**the Duchess**", "it names the subject")
	_ = subject
	data := loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData).SpreadRumors
	assert.False(t, data.SourceHidden, "gloating touches no attribution flag")
}

// Hidden AND gloating: (d) hides the character, (e) is the player's private
// satisfaction. Reveal must not re-attribute the rumor.
func TestSpreadRumors_Make_RevealDoesNotUndoHide(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, _, _, _ := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 2)
	h.makeChoice(plan.ID, makeOutcome, []string{"hide_source", "reveal_source"})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	assert.Nil(t, rumors[0].SourcePlayerID, "(e) does not reattribute a rumor whose source was hidden")
	logs := h.logBodies("plan.spread_rumors")
	assert.True(t, anyContains(logs, "in private"), "the gloating happens out of the court's sight: %v", logs)
	assert.False(t, anyContains(logs, "basks in the glory"))
	assert.True(t, loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData).SpreadRumors.SourceHidden,
		"(e) leaves the hidden-source flag alone")
}

// ── The mar: the target writes the counter-rumor ─────────────────────────────

// srMar drives a Spread Rumors to a forced mar with the preparer's rumor stated,
// returning what the mar tests need. The rumor is about ownerIdx's peer, so
// ownerIdx is the mar actor.
func srMar(t *testing.T, h *planLifecycle) (plan dbgen.Plan, prepIdx, actorIdx int, subject int64, roll *dbgen.DiceRoll) {
	t.Helper()
	plan, prepIdx, actorIdx, subject = srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll = srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	srForceMar(t, h, roll)
	return plan, prepIdx, actorIdx, subject, roll
}

func TestSpreadRumors_Mar_ActorWritesTheCounterRumorFirst(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	actor, prep := h.tg.Players[actorIdx], h.tg.Players[prepIdx]

	// The bar names the mar actor — the preparer's rumor just failed.
	h.assertWaitees("the target writes the counter-rumor", model.RowStatePlanResolving, actor.ID)

	// Options wait for the text; the preparer may never choose; only the actor writes.
	code, body := h.post(actorIdx, srPath(plan.ID, "make-choice"), map[string]any{"result": "mar", "choices": []string{"leverage_target"}})
	assert.Equalf(t, http.StatusConflict, code, "no options before the counter-rumor: %v", body)
	code, _ = h.post(prepIdx, srPath(plan.ID, "make-choice"), map[string]any{"result": "mar", "choices": []string{"leverage_target"}})
	assert.Equal(t, http.StatusForbidden, code, "the preparer does not choose on their own failed rumor")
	code, _ = h.post(prepIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "I wrote it myself"})
	assert.Equal(t, http.StatusForbidden, code)
	code, _ = h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "   "})
	assert.Equal(t, http.StatusBadRequest, code, "no empty counter-rumor, and no placeholder")

	code, body = h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	require.Equalf(t, http.StatusOK, code, "counter-rumor: %v", body)
	code, _ = h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Changed my mind."})
	assert.Equal(t, http.StatusConflict, code, "written once")

	assert.True(t, anyContains(h.playerPosts(), "Her accuser keeps a second set of books."), "the table reads it")
	assert.True(t, anyContains(h.logBodies("plan.spread_rumors"), "describes the rumor that spreads about"))
	// Still the actor's move (the options), not the preparer's.
	h.assertWaitees("then the options", model.RowStatePlanResolving, actor.ID)
	_ = prep
	_ = ctx
}

// A mar committed with no picks leaves make_mar_choices empty, so the commit has
// to be read from the durable marker: with it unread the bar kept naming the mar
// actor after they had finished, instead of handing the wait to the preparer.
// The commit is stored directly because a real zero-pick mar auto-completes the
// plan in the same request, which would hide the transient wait this guards.
func TestSpreadRumors_Mar_ZeroPickCommitHandsTheWaitToThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	actor, prep := h.tg.Players[actorIdx], h.tg.Players[prepIdx]

	code, body := h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	require.Equalf(t, http.StatusOK, code, "counter-rumor: %v", body)
	h.assertWaitees("the actor still owes the options", model.RowStatePlanResolving, actor.ID)

	rd := loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData)
	require.Empty(t, rd.MakeMarChoices)
	rd.ChoiceSubmitted = true
	require.NoError(t, saveResolutionData(ctx, h.q, plan.ID, rd))
	h.assertWaitees("the actor has finished; the preparer completes", model.RowStatePlanResolving, prep.ID)
}

// Once the mar's choice is committed the counter-rumor is closed, even when the
// commit carried no picks (make_mar_choices empty, so only the marker says so).
func TestSpreadRumors_Mar_CounterRumorClosedByAZeroPickCommit(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, _, actorIdx, _, _ := srMar(t, h)

	rd := loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData)
	require.Empty(t, rd.MakeMarChoices)
	rd.ChoiceSubmitted = true
	require.NoError(t, saveResolutionData(ctx, h.q, plan.ID, rd))

	code, _ := h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Too late."})
	assert.Equal(t, http.StatusConflict, code, "no counter-rumor after the choice is committed")
}

// Q2: the counter-rumor is about the preparer's main character AS IT STANDS WHEN
// THE OPTIONS ARE APPLIED; its text and source are the actor's; the preparer's
// failed rumor never reaches the sheet.
func TestSpreadRumors_Mar_CounterRumorRowIsAboutTheMainCharacterAndSourcedToItsAuthor(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	actor := h.tg.Players[actorIdx]

	// The preparer's main-character flag moves between preparation and now.
	oldMC := h.mainCharacterID(prepIdx)
	newMC := h.seedPeer(prepIdx, "the heir")
	_, err := h.pool.Exec(ctx, `UPDATE assets SET is_main_character = (id = $2) WHERE owner_id = $1`,
		h.tg.Players[prepIdx].ID, newMC)
	require.NoError(t, err)
	require.NotEqual(t, oldMC, newMC)

	code, body := h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	require.Equalf(t, http.StatusOK, code, "%v", body)
	h.makeChoiceAs(actorIdx, plan.ID, marOutcome, []string{"reveal_source"})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1, "exactly one row: the counter-rumor")
	assert.Equal(t, "Her accuser keeps a second set of books.", rumors[0].Text)
	assert.NotContains(t, rumors[0].Text, "northern pass", "the failed rumor never reaches the sheet")
	assert.NotEqual(t, "(no rumor text)", rumors[0].Text)
	require.NotNil(t, rumors[0].TargetAssetID)
	assert.Equal(t, newMC, *rumors[0].TargetAssetID, "the MC at apply time, not at preparation")
	require.NotNil(t, rumors[0].SourcePlayerID)
	assert.Equal(t, actor.ID, *rumors[0].SourcePlayerID, "sourced to its author unless they hide")
	for _, p := range h.logBodies("rumor.created") {
		assert.NotContains(t, p, "northern pass")
	}
}

func TestSpreadRumors_Mar_HideSourceHidesTheActorsCharacter(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	actor := h.tg.Players[actorIdx]
	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	h.makeChoiceAs(actorIdx, plan.ID, marOutcome, []string{"hide_source"})

	rumors, err := h.q.ListRumors(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.Len(t, rumors, 1)
	assert.Nil(t, rumors[0].SourcePlayerID, "(d) hides the actor's character as the source")

	// The preparer may not perform the actor's step; the actor shelters the secret
	// under THEIR OWN asset.
	mine := h.seedPeer(actorIdx, "Cassius")
	code, _ := h.post(prepIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": h.mainCharacterID(prepIdx)})
	assert.Equal(t, http.StatusForbidden, code)
	code, body := h.post(actorIdx, srPath(plan.ID, "hide-source"), map[string]any{"secret_asset_id": mine})
	require.Equalf(t, http.StatusOK, code, "%v", body)
	secrets, err := h.q.ListSecretsByAsset(ctx, mine)
	require.NoError(t, err)
	require.Len(t, secrets, 1)
	assert.Equal(t, actor.ID, secrets[0].AuthorID)
	assert.Contains(t, secrets[0].Text, "Her accuser keeps a second set of books.", "the secret records the counter-rumor, not the failed rumor")
	assert.True(t, anyContains(h.logBodies("plan.spread_rumors"), "hid themselves as the source"))
}

func TestSpreadRumors_Mar_BreakTearsAMarginaliaOnThePreparersMainCharacter(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, subject, _ := srMar(t, h)
	mc := h.mainCharacterID(prepIdx)
	_, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: mc, Position: 1, Text: "a spotless name"})
	require.NoError(t, err)
	_, cookMarg := h.seedPeerWithMarginalia(prepIdx, "the cook")
	_, subjMarg := h.seedPeerWithMarginalia((prepIdx+1)%3, "another of the Duchess's")
	_ = subject
	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	h.makeChoiceAs(actorIdx, plan.ID, marOutcome, []string{"break_target"})

	margs, err := h.q.ListMarginaliaByAsset(ctx, mc)
	require.NoError(t, err)
	require.NotEmpty(t, margs)

	// The actor picks WHICH marginalia, not which asset: another of the preparer's
	// assets, or any other asset at all, is not a thing a mar can break.
	code, _ := h.post(actorIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": cookMarg})
	assert.Equal(t, http.StatusBadRequest, code, "only the main character's marginalia can be torn")
	code, _ = h.post(actorIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": subjMarg})
	assert.Equal(t, http.StatusBadRequest, code)
	code, body := h.post(h.preparerIdxFor(plan.ID), srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": margs[0].ID})
	assert.Equalf(t, http.StatusForbidden, code, "the preparer cannot tear their own marginalia for the actor: %v", body)

	code, body = h.post(actorIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": margs[0].ID})
	require.Equalf(t, http.StatusOK, code, "%v", body)
	torn, err := h.q.GetMarginaliaByID(ctx, margs[0].ID)
	require.NoError(t, err)
	assert.True(t, torn.IsTorn)
}

// A mar's break picks are counted against the main character — the only thing
// they can land on — so the forfeit hatch opens exactly when IT has nothing left
// to tear, whatever state the rumor's original subject is in.
func TestSpreadRumors_Mar_ForfeitFollowsTheMainCharacter(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	mc := h.mainCharacterID(prepIdx)
	m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: mc, Position: 1, Text: "a spotless name"})
	require.NoError(t, err)
	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	h.makeChoiceAs(actorIdx, plan.ID, marOutcome, []string{"break_target", "break_target"})

	forfeit := func() int {
		code, _ := h.post(actorIdx, srPath(plan.ID, "sr-forfeit-step"), map[string]any{"step": "break_target"})
		return code
	}
	assert.Equal(t, http.StatusConflict, forfeit(), "the main character still has a marginalia to tear")
	code, body := h.post(actorIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": m.ID})
	require.Equalf(t, http.StatusOK, code, "%v", body)
	// Its last marginalia is gone, so the main character is destroyed and the second
	// pick has no target. (The rumor's original subject is untouched and breakable —
	// it must not count.)
	assert.Equal(t, http.StatusOK, forfeit(), "no valid target remains: the pick may be forfeited")
	h.requireEnded(plan.ID)
}

func TestSpreadRumors_Mar_LeverageActuallyLeveragesThePreparersMainCharacter(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	mc := h.mainCharacterID(prepIdx)
	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	h.makeChoiceAs(actorIdx, plan.ID, marOutcome, []string{"leverage_target"})

	a, err := h.q.GetAssetByID(ctx, mc)
	require.NoError(t, err)
	assert.True(t, a.IsLeveraged, "leverage_target on a mar leverages the preparer's main character")
	assert.True(t, anyContains(h.logBodies("asset.leveraged"), a.Name), "and says so")

	// A second pick on an already-leveraged asset is reported, not silently burnt.
	h2 := newPlanLifecycle(t, 3)
	plan2, _, actor2, _, _ := srMar(t, h2)
	h2.post(actor2, srPath(plan2.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	h2.makeChoiceAs(actor2, plan2.ID, marOutcome, []string{"leverage_target", "leverage_target"})
	assert.True(t, anyContains(h2.logBodies("plan.spread_rumors"), "already leveraged"), "%v", h2.logBodies("plan.spread_rumors"))
}

// Take stays "any of the preparer's assets", consent-gated — and the counter-rumor
// precondition holds on this route too, since it commits the pick list.
func TestSpreadRumors_Mar_TakeNeedsTheCounterRumorToo(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	prepAsset := h.seedPeer(prepIdx, "the cook")
	req := map[string]any{"choices": []string{"take_asset"}, "result": marOutcome, "take_asset_ids": []int64{prepAsset}}

	code, body := h.post(actorIdx, srPath(plan.ID, "request-take-consent"), req)
	assert.Equalf(t, http.StatusConflict, code, "no take before the counter-rumor: %v", body)

	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	code, body = h.post(actorIdx, srPath(plan.ID, "request-take-consent"), req)
	require.Equalf(t, http.StatusOK, code, "%v", body)
	h.assertWaitees("the preparer is asked to consent", model.RowStateAwaitTakeConsent, h.tg.Players[prepIdx].ID)
	code, _ = h.post(prepIdx, srPath(plan.ID, "respond-take-consent"), map[string]any{"agree": true})
	require.Equal(t, http.StatusOK, code)

	a, err := h.q.GetAssetByID(context.Background(), prepAsset)
	require.NoError(t, err)
	assert.Equal(t, h.tg.Players[actorIdx].ID, a.OwnerID)
}

// ── Take consent: guards on the request route ────────────────────────────────

// Before the dice settle there is nothing to choose from; a hand-built request
// must be turned away rather than stash a pending consent.
func TestSpreadRumors_TakeConsent_RefusedUntilTheRollIsSettled(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	srCast(t, h, plan.ID, "The Duchess sold the northern pass.") // roll opened, not settled
	victimAsset := h.seedPeer(ownerIdx, "the Duchess's cook")

	code, body := h.post(prepIdx, srPath(plan.ID, "request-take-consent"),
		map[string]any{"choices": []string{"take_asset"}, "result": makeOutcome, "take_asset_ids": []int64{victimAsset}})
	assert.Equalf(t, http.StatusConflict, code, "an unsettled roll cannot be taken against: %v", body)
	assert.Nil(t, loadResolutionData(mustPlan(t, h, plan.ID).ResolutionData).SpreadRumors.PendingTakeConsent)
}

// Once the take has gone through, the options are chosen: a second request would
// commit the choices again and put a second rumor on the sheet.
func TestSpreadRumors_TakeConsent_SecondRequestAfterCommitMakesNoDuplicateRumor(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 2)
	first := h.seedPeer(ownerIdx, "the Duchess's cook")
	second := h.seedPeer(ownerIdx, "the Duchess's valet")
	req := func(id int64) map[string]any {
		return map[string]any{"choices": []string{"take_asset"}, "result": makeOutcome, "take_asset_ids": []int64{id}}
	}

	code, body := h.post(prepIdx, srPath(plan.ID, "request-take-consent"), req(first))
	require.Equalf(t, http.StatusOK, code, "%v", body)
	code, body = h.post(prepIdx, srPath(plan.ID, "request-take-consent"), req(second))
	assert.Equalf(t, http.StatusConflict, code, "one request at a time: %v", body)
	code, _ = h.post(ownerIdx, srPath(plan.ID, "respond-take-consent"), map[string]any{"agree": true})
	require.Equal(t, http.StatusOK, code)

	code, body = h.post(prepIdx, srPath(plan.ID, "request-take-consent"), req(second))
	assert.Equalf(t, http.StatusConflict, code, "the options are already chosen: %v", body)

	rumors, err := h.q.ListRumors(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	assert.Len(t, rumors, 1, "exactly one rumor on the sheet")
}

// The same guard on the mar path, where the aggressor is the target asset's owner.
func TestSpreadRumors_TakeConsent_SecondRequestOnAMarIsRefused(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, actorIdx, _, _ := srMar(t, h)
	first := h.seedPeer(prepIdx, "the cook")
	second := h.seedPeer(prepIdx, "the groom")
	h.post(actorIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Her accuser keeps a second set of books."})
	req := func(id int64) map[string]any {
		return map[string]any{"choices": []string{"take_asset"}, "result": marOutcome, "take_asset_ids": []int64{id}}
	}

	code, body := h.post(actorIdx, srPath(plan.ID, "request-take-consent"), req(first))
	require.Equalf(t, http.StatusOK, code, "%v", body)
	code, _ = h.post(prepIdx, srPath(plan.ID, "respond-take-consent"), map[string]any{"agree": true})
	require.Equal(t, http.StatusOK, code)

	code, body = h.post(actorIdx, srPath(plan.ID, "request-take-consent"), req(second))
	assert.Equalf(t, http.StatusConflict, code, "the options are already chosen: %v", body)
	rumors, err := h.q.ListRumors(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	assert.Len(t, rumors, 1, "only the counter-rumor is on the sheet")
}

// A rumor about one's own asset: the preparer IS the target player, so the mar
// is theirs to drive.
func TestSpreadRumors_Mar_RumorAboutOwnAssetIsDrivenByThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	prepIdx := h.focusPlayerIdx()
	own := h.seedPeer(prepIdx, "my own cousin")
	notes := "a rumor about my own cousin"
	plan := h.prepare(PreparePlanRequest{PlanType: model.PlanSpreadRumors, TargetAssetID: &own, PreparationNotes: &notes})
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "My cousin is a scoundrel.")
	srForceMar(t, h, roll)

	code, body := h.post(prepIdx, srPath(plan.ID, "sr-counter-rumor"), map[string]any{"text": "Rumor has it I am the scoundrel."})
	require.Equalf(t, http.StatusOK, code, "the target player is the preparer: %v", body)
	h.makeChoiceAs(prepIdx, plan.ID, marOutcome, []string{"leverage_target"})
}

// ── Hygiene ──────────────────────────────────────────────────────────────────

func TestSpreadRumors_ValidatePreparation_ChecksTheTargetAsset(t *testing.T) {
	h := newPlanLifecycle(t, 2)
	ctx := context.Background()
	other := newTestGame(t, h.q, 2) // a second game, with its own assets
	foreign, err := h.q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID: other.Game.ID, OwnerID: other.Players[0].ID, CreatorID: other.Players[0].ID,
		AssetType: model.AssetPeer, Name: "Elsewhere",
	})
	require.NoError(t, err)
	destroyed := h.seedPeer(1, "Ashes")
	require.NoError(t, h.q.DestroyAsset(ctx, destroyed))
	living := h.seedPeer(1, "Julius")
	missing := int64(999999999)

	validate := func(id int64, keptSecret bool, notes string) string {
		vc := &ValidationContext{
			Q: h.q, Game: &h.tg.Game, Player: &h.tg.Players[0], TargetAssetID: &id,
			Notes: notes, KeptSecret: keptSecret,
		}
		_, msg := srHandler{}.ValidatePreparation(ctx, vc)
		return msg
	}
	assert.Empty(t, validate(living, false, "a rumor"))
	assert.NotEmpty(t, validate(foreign.ID, false, "a rumor"), "another game's asset")
	assert.NotEmpty(t, validate(destroyed, false, "a rumor"), "a destroyed asset")
	assert.NotEmpty(t, validate(missing, false, "a rumor"), "an asset that does not exist")
	assert.NotEmpty(t, validate(living, false, ""), "a fresh prepare still needs the text")
	assert.Empty(t, validate(living, true, ""), "a settled secret rumor has no note by design")

	// And through the real route.
	code, body := h.post(h.focusPlayerIdx(), "/api/tables/"+itoa(h.tg.Game.ID)+"/prepare-plan", PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &foreign.ID, PreparationNotes: new("a rumor"),
	})
	assert.Equalf(t, http.StatusBadRequest, code, "prepare-plan must refuse another game's asset: %v", body)
}

// A Make Demands keep_or_change_target winner re-aims a plan through the plan's
// own ValidatePreparation. A SECRET rumor has no note to replay, so that used to
// 400 — the winner could not retarget it at all.
func TestSpreadRumors_DemandRetargetOfASecretRumorWorks(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	prepIdx := h.focusPlayerIdx()
	holder := h.seedPeer(prepIdx, "Brutus")
	plan, _, ownerIdx, _ := srRumorAbout(t, h, &holder)
	require.Nil(t, plan.PreparationNotes, "the secret rumor blanks the public note")
	demanderIdx := (prepIdx + 2) % 3
	newSubject := h.seedPeer(ownerIdx, "the Duke")
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionKeepOrChangeTarget: h.tg.Players[demanderIdx].ID,
	})

	code, body := h.post(demanderIdx, srPath(plan.ID, "demand-retarget"), map[string]any{"target_asset_id": newSubject})
	require.Equalf(t, http.StatusOK, code, "retargeting a secret rumor: %v", body)
	got, err := h.q.GetPlanByID(ctx, plan.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TargetAssetID)
	assert.Equal(t, newSubject, *got.TargetAssetID)
}

// The same winner holds the make's steps — break-target was preparer-only, so a
// winner the panel shows the controls to was refused by the server.
func TestSpreadRumors_PerformStepsWinnerDrivesTheMakeSteps(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, ownerIdx, _ := srRumorAbout(t, h, nil)
	subject, subjMarg := h.seedPeerWithMarginalia(ownerIdx, "the Duchess's steward")
	_, err0 := h.pool.Exec(context.Background(), `UPDATE plans SET target_asset_id = $2 WHERE id = $1`, plan.ID, subject)
	require.NoError(t, err0)
	demanderIdx := (prepIdx + 2) % 3
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: h.tg.Players[demanderIdx].ID,
	})
	h.jumpToRow(*plan.RowNumber)
	h.resolve(plan.ID)
	roll := srCast(t, h, plan.ID, "The Duchess sold the northern pass.")
	h.forceRoll(roll.ID, makeOutcome, 1)

	code, body := h.post(demanderIdx, srPath(plan.ID, "make-choice"), map[string]any{"result": "make", "choices": []string{"break_target"}})
	require.Equalf(t, http.StatusOK, code, "the winner chooses: %v", body)
	h.assertWaitees("the winner owes the break", model.RowStatePlanResolving, h.tg.Players[demanderIdx].ID)

	code, _ = h.post(prepIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": subjMarg})
	assert.Equal(t, http.StatusForbidden, code, "the preparer is locked out once the winner holds the steps")
	code, body = h.post(demanderIdx, srPath(plan.ID, "break-target"), map[string]any{"marginalia_id": subjMarg})
	assert.Equalf(t, http.StatusOK, code, "the winner performs it: %v", body)
}

func mustPlan(t *testing.T, h *planLifecycle, id int64) dbgen.Plan {
	t.Helper()
	p, err := h.q.GetPlanByID(context.Background(), id)
	require.NoError(t, err)
	return p
}
