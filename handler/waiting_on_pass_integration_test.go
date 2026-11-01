//go:build integration

// handler/waiting_on_pass_integration_test.go — the cross-cutting waiting-on
// pass (adr/audit/FINDINGS.md, "The patterns underneath" #2).
//
// The rule under test is one sentence: the Waiting On bar names EXACTLY the
// players the server will let act right now — every one of them, and nobody
// else. It fails two ways, and each test below is named for the way it breaks:
//
//   - a plan's override REPLACES the acting set with "the new actor" when the
//     real question is "who is owed, in total?" (Make Introductions dropped the
//     preparer who still owed a peer; Seek Answers named only a question's target
//     while the actor still had picks to make);
//   - a plan's steps are driven by a Make Demands perform_steps WINNER, and the
//     bar names the preparer — the one player the server has locked out
//     (Make Introductions, Propose Decree, Propose Duel, Seek Answers,
//     Chronicle Histories).
//
// The other half of the coin is who MAY act: the bar can only be right if the
// routes agree with it, so the tail of the file pins the two make-choice holes
// (the target-driven mar admitting the preparer, and any choice accepted while
// the dice are still in the air).

package handler

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// mustAsset reloads an asset by id.
func mustAsset(t *testing.T, h *planLifecycle, id int64) dbgen.Asset {
	t.Helper()
	a, err := h.q.GetAssetByID(context.Background(), id)
	require.NoError(t, err)
	return a
}

// ── Make Introductions ───────────────────────────────────────────────────────

// TestWaitingOn_MakeIntroductions_Mar_NamesPreparerAndAuthor: with two peers and
// only one resolved, the recipient owes a marginalia AND the preparer still owes
// the second peer. The bar names both; once the preparer has resolved the last
// peer it narrows to the recipient alone, and once the recipient writes it is
// the preparer's to complete.
func TestWaitingOn_MakeIntroductions_Mar_NamesPreparerAndAuthor(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	planID, preparerIdx, drafts := miToMar(t, h, 2)
	preparer := h.tg.Players[preparerIdx].ID
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	other := h.tg.Players[otherIdx].ID

	// Before anything is resolved the preparer owes both peers.
	h.assertWaitees("mar, nothing resolved", model.RowStatePlanResolving, preparer)

	marPath := "/api/plans/" + strconv.FormatInt(planID, 10) + "/introductions-mar"
	code, body := h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[0].ID, "outcome": "other_retinue", "target_player_id": other,
	})
	require.Equalf(t, http.StatusOK, code, "other_retinue: %v", body)
	h.assertWaitees("first peer sent away, second unresolved",
		model.RowStateAwaitIntroductionsMarginalia, preparer, other)

	code, body = h.post(preparerIdx, marPath, map[string]any{
		"draft_id": drafts[1].ID, "outcome": "delayed",
	})
	require.Equalf(t, http.StatusOK, code, "delayed: %v", body)
	h.assertWaitees("every peer resolved, recipient still owes",
		model.RowStateAwaitIntroductionsMarginalia, other)

	code, body = h.post(otherIdx, "/api/plans/"+strconv.FormatInt(planID, 10)+"/introductions-marginalia",
		map[string]any{"draft_id": drafts[0].ID, "text": "a useful ally"})
	require.Equalf(t, http.StatusOK, code, "recipient writes: %v", body)
	// The recipient's marginalia was the last thing owed: the plan ends itself, so
	// the bar has nobody to name and there is no Complete click for the preparer.
	h.requireResolved(planID, "mar")
	_ = preparer
}

// TestWaitingOn_MakeIntroductions_PerformStepsWinnerIsNamed: every Make
// Introductions step after the roll is the preparer's slot, so after a
// perform_steps win it is the winner's — bringing the peers in and (on a mar)
// resolving each one; the preparer is refused all of it, so the bar must not
// name them. Naming the peers and rolling are pre-roll (perform_steps is
// post-roll only), so they stay the preparer's.
func TestWaitingOn_MakeIntroductions_PerformStepsWinnerIsNamed(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	preparerIdx := h.focusPlayerIdx()
	demanderIdx := (preparerIdx + 1) % 3
	preparer := h.tg.Players[preparerIdx].ID
	demander := h.tg.Players[demanderIdx].ID

	notes := "introductions"
	plan := h.prepare(PreparePlanRequest{
		PlanType: model.PlanMakeIntroductions, PeerCount: 1, PreparationNotes: &notes,
	})
	require.NotNil(t, plan.RowNumber)
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolve(plan.ID), "MI defers its roll until peers are named")
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: demander,
	})

	base := "/api/plans/" + strconv.FormatInt(plan.ID, 10)
	// Naming the peers is create-peer: still the preparer's, with no roll open yet.
	h.assertWaitees("naming the peers", model.RowStatePlanResolving, preparer)
	code, body := h.post(preparerIdx, base+"/create-peer", map[string]any{"name": "Demander's pick"})
	require.Equalf(t, http.StatusCreated, code, "create-peer: %v", body)
	code, body = h.post(preparerIdx, base+"/finalize-peers", nil)
	require.Equalf(t, http.StatusCreated, code, "finalize-peers: %v", body)
	rollMap, _ := body["roll"].(map[string]any)
	require.NotNil(t, rollMap)
	h.forceRoll(int64(rollMap["id"].(float64)), "make", 0)

	h.requireAutoAppliedAs(demanderIdx, plan.ID, "make", "peers_arrive")
	h.assertWaitees("the arrival form is owed", model.RowStatePlanResolving, demander)

	drafts := miDraftsOf(t, h, plan.ID)
	require.Len(t, drafts, 1)
	code, body = h.post(demanderIdx, base+"/introductions-arrival", map[string]any{
		"draft_id": drafts[0].ID, "name": drafts[0].Name, "marginalia": "owes a favour",
	})
	require.Equalf(t, http.StatusCreated, code, "arrival: %v", body)
	// The last arrival ended the plan; nobody — least of all the locked-out
	// preparer — is left to click Complete.
	h.requireResolved(plan.ID, "make")
	_ = preparer
}

// ── Propose Decree ───────────────────────────────────────────────────────────

// TestWaitingOn_ProposeDecree_PerformStepsWinnerIsNamed: a Monarch preparer is
// their own signatory, so the addendum and the enactment are both the preparer's
// slot — the winner's after a perform_steps win. The bar named the signatory (the
// locked-out preparer) for the addendum.
func TestWaitingOn_ProposeDecree_PerformStepsWinnerIsNamed(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan := pdPrepareAndResolve(t, h, 0)
	preparerIdx := h.preparerIdxFor(plan.ID)
	demanderIdx := (preparerIdx + 1) % 3
	demander := h.tg.Players[demanderIdx].ID

	pdDeclineRemaining(t, h, plan.ID)
	roll := pdCallRoll(t, h, plan.ID, preparerIdx)
	h.forceRoll(roll.ID, "make", roll.Difficulty)
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: demander,
	})

	h.assertWaitees("the signatory's addendum", model.RowStatePlanResolving, demander)
	base := "/api/plans/" + strconv.FormatInt(plan.ID, 10)
	code, body := h.post(demanderIdx, base+"/set-addendum", map[string]any{"connector": "and", "addendum": "exempting grain"})
	require.Equalf(t, http.StatusOK, code, "set-addendum: %v", body)
	h.assertWaitees("enacting the law", model.RowStatePlanResolving, demander)
}

// ── Propose Duel ─────────────────────────────────────────────────────────────

// TestWaitingOn_ProposeDuel_PerformStepsWinnerHoldsThePreparersSeat: the winner
// declares the preparer's bouts, so while the preparer's seat is up the bar names
// the winner; the target's seat is never transferred, so it names the target.
func TestWaitingOn_ProposeDuel_PerformStepsWinnerHoldsThePreparersSeat(t *testing.T) {
	h := newPlanLifecycle(t, 5)

	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5
	demIdx := (prepIdx0 + 2) % 5
	prepA := h.seedPeer(prepIdx0, "Preparer stake A")
	prepB := h.seedPeer(prepIdx0, "Preparer stake B")
	targA := h.seedPeer(targIdx, "Target stake A")
	targB := h.seedPeer(targIdx, "Target stake B")

	planID, prepIdx := seedDuelToBouts(t, h, targIdx, []int64{prepA, prepB}, []int64{targA, targB})
	prepID := h.tg.Players[prepIdx].ID
	targID := h.tg.Players[targIdx].ID
	demID := h.tg.Players[demIdx].ID
	require.Equal(t, prepID, duelInitiative(t, h, planID), "the preparer holds initiative")

	h.assertWaitees("before the demand: the preparer declares", model.RowStateAwaitDuelBout, prepID)

	h.seedMadeDemand(demIdx, planID, game.DemandOptionWinners{game.DemandOptionPerformSteps: demID})
	h.assertWaitees("the winner declares for the preparer", model.RowStateAwaitDuelBout, demID)

	stake := firstUnresolvedStakeID(t, h, planID, prepID)
	code, body := h.post(demIdx, duelRoute(planID, "bout-declare"),
		map[string]any{"stake_id": stake, "declaration": "high"})
	require.Equalf(t, http.StatusOK, code, "winner declares: %v", body)
	h.assertWaitees("the target responds as themselves", model.RowStateAwaitDuelBout, targID)
}

// ── Seek Answers ─────────────────────────────────────────────────────────────

// TestWaitingOn_SeekAnswers_PerformStepsWinnerAndPendingQuestion: after the
// winner commits the make-list the winner is who is owed. While a question is out
// the target is owed too — but so is the winner, who may still reveal a secret
// (only a second ask is held back) — and once the winner has nothing left the bar
// narrows to the target alone, then back to the preparer, who completes.
func TestWaitingOn_SeekAnswers_PerformStepsWinnerAndPendingQuestion(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 2)
	demanderIdx := (preparerIdx + 1) % 3
	targetIdx := (preparerIdx + 2) % 3
	preparer := h.tg.Players[preparerIdx].ID
	demander := h.tg.Players[demanderIdx].ID
	target := h.tg.Players[targetIdx].ID

	holder := h.seedPeer(targetIdx, "secret holder")
	h.seedSecret(holder, targetIdx, "the vault code is 1789")
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: demander,
	})

	base := "/api/plans/" + strconv.FormatInt(plan.ID, 10)
	h.assertWaitees("the winner owes the make-list", model.RowStatePlanResolving, demander)
	h.makeChoiceAs(demanderIdx, plan.ID, "make", []string{"reveal_secret", "ask_question"})
	h.assertWaitees("picks committed, the winner performs them", model.RowStatePlanResolving, demander)

	code, body := h.post(demanderIdx, base+"/ask-question",
		map[string]any{"target_id": target, "question": "where were you?"})
	require.Equalf(t, http.StatusOK, code, "ask-question: %v", body)
	h.assertWaitees("question out, reveal still owed",
		model.RowStateAwaitQuestionAnswer, target, demander)

	code, body = h.post(demanderIdx, base+"/reveal-secret", map[string]any{"asset_id": holder})
	require.Equalf(t, http.StatusOK, code, "reveal-secret: %v", body)
	h.assertWaitees("only the answer is owed", model.RowStateAwaitQuestionAnswer, target)

	code, body = h.post(targetIdx, base+"/answer-question", map[string]any{"answer": "at home"})
	require.Equalf(t, http.StatusOK, code, "answer-question: %v", body)
	// The answer was the last thing owed: the plan resolved itself.
	_ = preparer
	h.requireEnded(plan.ID)
}

// ── Chronicle Histories ──────────────────────────────────────────────────────

// TestWaitingOn_ChronicleHistories_PerformStepsWinnerOwesTheSteps: the make path
// is a run of make-steps the winner drives; the bar named the winner only until
// the first step and then fell back to the (locked-out) preparer.
func TestWaitingOn_ChronicleHistories_PerformStepsWinnerOwesTheSteps(t *testing.T) {
	h := newPlanLifecycle(t, 3)

	plan, preparerIdx, _ := chPrepareToRoll(t, h, "make", 2) // budget 5
	demanderIdx := (preparerIdx + 1) % 3
	preparer := h.tg.Players[preparerIdx].ID
	demander := h.tg.Players[demanderIdx].ID
	h.seedMadeDemand(demanderIdx, plan.ID, game.DemandOptionWinners{
		game.DemandOptionPerformSteps: demander,
	})

	path := "/api/plans/" + strconv.FormatInt(plan.ID, 10) + "/make-step"
	h.assertWaitees("no step taken yet", model.RowStatePlanResolving, demander)
	code, body := h.post(demanderIdx, path, map[string]any{"option": "echo_present"})
	require.Equalf(t, http.StatusOK, code, "first make-step: %v", body)
	h.assertWaitees("one step down, the rest still owed", model.RowStatePlanResolving, demander)

	for i := 1; i < 5; i++ {
		code, body = h.post(demanderIdx, path, map[string]any{"option": "echo_present"})
		require.Equalf(t, http.StatusOK, code, "make-step %d: %v", i+1, body)
	}
	h.assertWaitees("budget spent, the preparer completes", model.RowStatePlanResolving, preparer)
}

// ── Who may act ──────────────────────────────────────────────────────────────

// TestMakeChoice_TargetDrivenMar_RefusesThePreparer: on Exchange Courtiers and
// Propose Duel a mar hands the options to the TARGET. The preparer — whose plan
// just failed — was still admitted by makeChoiceAuthorized, so the player the bar
// does not name could pick the option for them. Spread Rumors already refuses
// them through its ChoiceGate.
func TestMakeChoice_TargetDrivenMar_RefusesThePreparer(t *testing.T) {
	t.Run("exchange_courtiers", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)

		code, body := ecMakeChoice(t, h, preparerIdx, plan.ID, []string{"forfeit"})
		require.Equalf(t, http.StatusForbidden, code, "the preparer must not choose the mar: %v", body)
		assert.Equal(t, h.tg.Players[preparerIdx].ID, mustAsset(t, h, requestedPeer).OwnerID,
			"the refused forfeit moved nothing")

		code, body = ecMakeChoice(t, h, targetIdx, plan.ID, []string{"forfeit"})
		require.Equalf(t, http.StatusOK, code, "the target still chooses: %v", body)
	})

	t.Run("propose_duel", func(t *testing.T) {
		h := newPlanLifecycle(t, 5)
		prepIdx0 := h.focusPlayerIdx()
		targIdx := (prepIdx0 + 1) % 5
		// Two stakes against one, so the mar claim is a genuine pick (a forced
		// claim would settle the duel by itself and leave nothing to refuse).
		prepPeer := h.seedPeer(prepIdx0, "Preparer stake A")
		prepPeerB := h.seedPeer(prepIdx0, "Preparer stake B")
		targPeer := h.seedPeer(targIdx, "Target stake")
		planID, prepIdx := seedDuelToRollPinned(t, h, targIdx,
			[]int64{prepPeer, prepPeerB}, []int64{targPeer}, targetSweepsPrep, targetSweepsTarg)

		claim := map[string]any{"result": "mar", "choices": []string{strconv.FormatInt(prepPeer, 10)}}
		code, body := h.post(prepIdx, duelRoute(planID, "make-choice"), claim)
		require.Equalf(t, http.StatusForbidden, code, "the preparer must not claim their own stake: %v", body)
		assert.Equal(t, h.tg.Players[prepIdx].ID, mustAsset(t, h, prepPeer).OwnerID,
			"the refused claim moved nothing")
	})
}

// TestMakeChoice_RefusedWhileTheDiceAreOpen: a make-choice submitted while the
// roll is still being voted on or leveraged names an outcome nobody has rolled.
// With no outcome to compare it against, the result check was skipped, so a
// stale or hand-built client could commit a mar (and its effects) ahead of the
// dice — on the plans whose preparer still holds the choice.
func TestMakeChoice_RefusedWhileTheDiceAreOpen(t *testing.T) {
	t.Run("exchange_courtiers_mar", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		plan, preparerIdx, _, _, requestedPeer, _, _ := ecToRoll(t, h) // roll created, not yet resolved

		code, body := ecMakeChoice(t, h, preparerIdx, plan.ID, []string{"forfeit"})
		assert.Truef(t, code == http.StatusConflict || code == http.StatusForbidden,
			"a mar before the dice settle must be refused, got %d: %v", code, body)
		assert.Equal(t, h.tg.Players[preparerIdx].ID, mustAsset(t, h, requestedPeer).OwnerID,
			"nothing may have moved")
	})

	t.Run("seek_answers_make", func(t *testing.T) {
		h := newPlanLifecycle(t, 3)
		notes := "researching the archives"
		plan := h.prepare(PreparePlanRequest{PlanType: model.PlanSeekAnswers, PreparationNotes: &notes})
		require.NotNil(t, plan.RowNumber)
		h.jumpToRow(*plan.RowNumber)
		require.Nil(t, h.resolve(plan.ID))
		saCastRoll(t, h, plan.ID) // the roll is open: nothing has settled it

		code, body := h.post(h.preparerIdxFor(plan.ID), "/api/plans/"+strconv.FormatInt(plan.ID, 10)+"/make-choice",
			map[string]any{"result": "make", "choices": []string{"declare_truth"}})
		require.Equalf(t, http.StatusConflict, code, "a choice before the dice settle: %v", body)
	})
}
