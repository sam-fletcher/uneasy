//go:build integration

// handler/log_completeness_integration_test.go — the cross-cutting log pass
// (adr/audit/FINDINGS.md, "The patterns underneath" #3, and Tier 5).
//
// The rule under test: every change to the board — and every decision a player
// makes that the table would see at a real table — posts a correctly attributed
// line to the action log. It broke three ways, and each test below is named for
// the way it broke:
//
//   - a route mutated and posted nothing (Exchange Courtiers' messy break and
//     fair-trade offer, Spread Rumors' take request and refusal, Host Festivity's
//     opt-out, Make Demands' draft picks);
//   - a route posted "something happened" but not the part that is public record
//     (Propose Decree's amendment / addendum / final law text, Host Festivity's
//     rumor text, the Duel's claimed assets, a vetoed Seek Answers question);
//   - a body named a ROLE where it should mark the player ("the preparer", "the
//     target", "the host"), which is what turns a name into a link.
//
// Plans whose Tier 5 item turned out to be fixed already (Make War's rejected
// peace, delay-reveal and cost-of-battle anchoring) keep their existing tests;
// see the hand-over note's verdict table.

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

// allLogBodies returns every post body in the harness's game, oldest first.
func allLogBodies(t *testing.T, h *planLifecycle) []string {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	out := make([]string, 0, len(posts))
	for _, p := range posts {
		out = append(out, p.Body)
	}
	return out
}

// logWith returns the first log body containing every needle, failing the test
// (with the whole log, for diagnosis) when none does.
func logWith(t *testing.T, h *planLifecycle, needles ...string) string {
	t.Helper()
	bodies := allLogBodies(t, h)
	for _, b := range bodies {
		ok := true
		for _, n := range needles {
			if !strings.Contains(b, n) {
				ok = false
				break
			}
		}
		if ok {
			return b
		}
	}
	t.Fatalf("no log post contains all of %q; log was:\n%s", needles, strings.Join(bodies, "\n---\n"))
	return ""
}

// noLogWith fails when any log body contains the needle.
func noLogWith(t *testing.T, h *planLifecycle, needle string) {
	t.Helper()
	for _, b := range allLogBodies(t, h) {
		assert.NotContainsf(t, b, needle, "log body must not contain %q", needle)
	}
}

// mark renders a player the way playerDisplayName does for the harness's players.
func (h *planLifecycle) mark(idx int) string {
	return playerMark(h.tg.Players[idx].ID, h.tg.Players[idx].DisplayName)
}

func planPath(planID int64, route string) string {
	return "/api/plans/" + strconv.FormatInt(planID, 10) + "/" + route
}

// ── Exchange Courtiers ───────────────────────────────────────────────────────

// ecMessyBreak drives EC to the messy break (preparer = player 0, target =
// player 1) with the preparer's asset carrying `margs` marginalia, and returns
// the plan, the asset and its first marginalia. The fair-trade offer and decline
// are made on the way, so callers can assert on those posts too.
func ecMessyBreak(t *testing.T, h *planLifecycle, margs int) (plan dbgen.Plan, asset, marg int64) {
	t.Helper()
	ctx := context.Background()
	asset = h.seedPeer(0, "Silver Courtier")
	targetPeer := h.seedPeer(1, "P2 target peer")
	for i := range margs {
		m, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{
			AssetID: asset, Position: int16(i + 1), Text: "marginalia " + strconv.Itoa(i+1),
		})
		require.NoError(t, err)
		if i == 0 {
			marg = m.ID
		}
	}

	notes := "poach"
	plan = h.prepare(PreparePlanRequest{
		PlanType:         model.PlanExchangeCourtiers,
		TargetPlayerID:   &h.tg.Players[1].ID,
		TargetAssetID:    &targetPeer,
		PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	require.Nil(t, h.resolveAndCast(plan.ID))

	code, body := h.post(1, planPath(plan.ID, "fair-trade"), map[string]any{"action": "offer", "offered_asset_id": asset})
	require.Equalf(t, http.StatusOK, code, "offer: %v", body)
	code, body = h.post(0, planPath(plan.ID, "fair-trade"), map[string]any{"action": "decline"})
	require.Equalf(t, http.StatusOK, code, "decline: %v", body)
	rollID := int64(body["roll"].(map[string]any)["id"].(float64))

	h.forceRoll(rollID, "make", 0)
	h.makeChoice(plan.ID, "make", []string{"messy"})
	return plan, asset, marg
}

// The reported case: a messy break that does not destroy posted NOTHING, and one
// that does left a lone, actorless "X destroyed.". The breaker, the victim, the
// asset and the torn text all belong in the line.
func TestLog_ExchangeCourtiers_MessyBreak_NamesWhoBrokeWhat(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, _, marg := ecMessyBreak(t, h, 2)

	code, body := h.post(1, planPath(plan.ID, "messy-break"), map[string]any{"marginalia_id": marg})
	require.Equalf(t, http.StatusOK, code, "messy-break: %v", body)

	line := logWith(t, h, h.mark(1), "broke", h.mark(0), "**Silver Courtier**")
	assert.Contains(t, line, `"marginalia 1"`, "the torn text is public record")
	assert.Contains(t, line, "how has the asset changed?", "the owner is invited to narrate the change")
}

func TestLog_ExchangeCourtiers_MessyBreak_DestroyingBreakIsAttributed(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, _, marg := ecMessyBreak(t, h, 1)

	code, body := h.post(1, planPath(plan.ID, "messy-break"), map[string]any{"marginalia_id": marg})
	require.Equalf(t, http.StatusOK, code, "messy-break: %v", body)

	logWith(t, h, h.mark(1), "destroyed", h.mark(0), "**Silver Courtier**")
}

// The pre-roll trade is spoken aloud, and the offered peer is the one the target
// claims if the roll mars — everyone should see which one it is. The "got messy"
// line named "the target" and "the preparer" instead of marking them.
func TestLog_ExchangeCourtiers_FairTradeOfferDeclineAndMessyLine(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ecMessyBreak(t, h, 2)

	logWith(t, h, h.mark(1), "proposed a fair trade", h.mark(0), "**Silver Courtier**")
	logWith(t, h, h.mark(0), "declined the fair trade")
	logWith(t, h, "got messy", h.mark(1), h.mark(0))
	noLogWith(t, h, "the target may")
}

func TestLog_ExchangeCourtiers_RiposteBreakMarksThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	ctx := context.Background()
	plan, preparerIdx, targetIdx, _, requestedPeer := ecToMarWithRequest(t, h)
	first, err := h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 1, Text: "note"})
	require.NoError(t, err)
	_, err = h.q.CreateMarginalia(ctx, dbgen.CreateMarginaliaParams{AssetID: requestedPeer, Position: 2, Text: "second"})
	require.NoError(t, err)

	code, body := ecMakeChoice(t, h, targetIdx, plan.ID, []string{"riposte"})
	require.Equalf(t, http.StatusOK, code, "riposte: %v", body)
	code, body = h.post(preparerIdx, planPath(plan.ID, "riposte-break"), map[string]any{"marginalia_id": first.ID})
	require.Equalf(t, http.StatusOK, code, "riposte-break: %v", body)

	logWith(t, h, h.mark(preparerIdx), "damaged their own", "before surrendering it")
	noLogWith(t, h, "The preparer damaged")
}

// ── Seek Answers / Chronicle Histories ───────────────────────────────────────

func TestLog_SeekAnswers_SuccessAndMarMarkThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 1)
	h.makeChoice(plan.ID, "make", []string{"reveal_secret"})
	logWith(t, h, "Seek Answers succeeded", h.mark(preparerIdx), "presses their inquiries")
	noLogWith(t, h, "the preparer presses")
}

func TestLog_SeekAnswers_MarMarksThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := saPrepareToRoll(t, h, "mar", 2)
	saSeedResource(t, h, preparerIdx, "preparer ledger", 1)
	h.makeChoice(plan.ID, "mar", []string{"break_resource"})
	logWith(t, h, "Seek Answers marred", h.mark(preparerIdx), "must break")
	noLogWith(t, h, "the preparer must")
}

// A question is only logged once answered, so a vetoed one left no trace of what
// was asked — the veto line has to carry it.
func TestLog_SeekAnswers_VetoQuotesTheQuestion(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := saPrepareToRoll(t, h, "make", 1)
	otherIdx := (preparerIdx + 1) % len(h.tg.Players)
	saPinKnowledgeRank(t, h, h.tg.Players[otherIdx].ID, 2)
	h.makeChoice(plan.ID, "make", []string{"ask_question"})

	code, body := h.post(preparerIdx, planPath(plan.ID, "ask-question"),
		map[string]any{"target_id": h.tg.Players[otherIdx].ID, "question": "where is the seal?"})
	require.Equalf(t, http.StatusOK, code, "ask: %v", body)
	code, body = h.post(otherIdx, planPath(plan.ID, "veto-question"), nil)
	require.Equalf(t, http.StatusOK, code, "veto: %v", body)

	logWith(t, h, h.mark(otherIdx), "vetoed", h.mark(preparerIdx), `"where is the seal?"`)
}

func TestLog_ChronicleHistories_SuccessMarksThePreparer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, preparerIdx, _ := chPrepareToRoll(t, h, "make", 1)
	art, margs := chSeedArtifact(t, h, preparerIdx, "The Charter", 2)
	// A narrative option needs no target, so the first step is enough to open the
	// make path (and to announce the success).
	code, body := chMakeStep(h, plan.ID, "echo_present", 0, 0)
	require.Equalf(t, http.StatusOK, code, "make-step: %v", body)
	_, _ = art, margs

	logWith(t, h, "Chronicle Histories succeeded", h.mark(preparerIdx), "shapes the scene from history")
	noLogWith(t, h, "the preparer shapes")
}

// ── Propose Decree ───────────────────────────────────────────────────────────

// The law is public record, yet only "a decree passed" reached the log: not the
// council's amendments, not the signatory's rider, not the final wording.
func TestLog_ProposeDecree_AmendmentAddendumAndFinalLawAreQuoted(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)
	sigIdx := pdPlayerIdx(t, h, *pd.SignatoryID)
	roll := pdCallRoll(t, h, plan.ID, sigIdx)
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)

	pd = pdData(t, h, plan.ID)
	for i, amenderID := range pd.AmendmentOrder {
		code, body := h.post(pdPlayerIdx(t, h, amenderID), planPath(plan.ID, "amend-decree"),
			map[string]any{"text": "Amended body v" + strconv.Itoa(i+1)})
		require.Equalf(t, http.StatusOK, code, "amend %d: %v", i+1, body)
	}
	logWith(t, h, "amended the decree", `"Amended body v1"`)
	logWith(t, h, "amended the decree", `"Amended body v2"`)

	code, body := h.post(sigIdx, planPath(plan.ID, "set-addendum"),
		map[string]any{"connector": "but", "addendum": "the nobles are exempt"})
	require.Equalf(t, http.StatusOK, code, "addendum: %v", body)
	logWith(t, h, "addendum", `"but the nobles are exempt"`)

	pdEnact(t, h, plan.ID, "")
	final := logWith(t, h, "amended decree was enacted", h.mark(sigIdx))
	assert.Contains(t, final, "Amended body v2 but the nobles are exempt",
		"the enactment post carries the assembled law, rider included")
}

func TestLog_ProposeDecree_BlankAddendumIsStillAnnounced(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan := pdPrepareAndResolve(t, h, 2)
	pd := pdData(t, h, plan.ID)
	sigIdx := pdPlayerIdx(t, h, *pd.SignatoryID)
	roll := pdCallRoll(t, h, plan.ID, sigIdx)
	h.forceRoll(roll.ID, "mar", roll.Difficulty-1)
	pd = pdData(t, h, plan.ID)
	for _, amenderID := range pd.AmendmentOrder {
		code, body := h.post(pdPlayerIdx(t, h, amenderID), planPath(plan.ID, "skip-amend"), nil)
		require.Equalf(t, http.StatusOK, code, "skip: %v", body)
	}
	code, body := h.post(sigIdx, planPath(plan.ID, "set-addendum"), map[string]any{"connector": "", "addendum": ""})
	require.Equalf(t, http.StatusOK, code, "addendum: %v", body)
	logWith(t, h, h.mark(sigIdx), "without an addendum")
}

// ── Spread Rumors ────────────────────────────────────────────────────────────

// srTakeRequest drives Spread Rumors (preparer = focus, victim = next player)
// to a pending take-asset consent request and returns the pieces.
func srTakeRequest(t *testing.T, h *planLifecycle) (plan dbgen.Plan, prepIdx, victimIdx int, loot int64) {
	t.Helper()
	prepIdx = h.focusPlayerIdx()
	victimIdx = (prepIdx + 1) % len(h.tg.Players)
	target := h.seedPeer(victimIdx, "rumor target")
	loot = h.seedPeer(victimIdx, "Crown Jewels")
	notes := "the king is a fraud"
	plan = h.prepare(PreparePlanRequest{
		PlanType: model.PlanSpreadRumors, TargetAssetID: &target, PreparationNotes: &notes,
	})
	h.jumpToRow(*plan.RowNumber)
	roll := h.resolveAndCast(plan.ID)
	require.NotNil(t, roll)
	h.forceRoll(roll.ID, makeOutcome, 1)
	code, body := h.post(prepIdx, planPath(plan.ID, "request-take-consent"), map[string]any{
		"result": makeOutcome, "choices": []string{"take_asset"}, "take_asset_ids": []int64{loot},
	})
	require.Equalf(t, http.StatusOK, code, "request-take-consent: %v", body)
	return plan, prepIdx, victimIdx, loot
}

// A take request and its refusal were both unlogged, so a refused grab left no
// trace at either level.
func TestLog_SpreadRumors_TakeRequestAndRefusalAreLogged(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, victimIdx, _ := srTakeRequest(t, h)

	logWith(t, h, h.mark(prepIdx), "asked", h.mark(victimIdx), "consent to take", "**Crown Jewels**")

	code, body := h.post(victimIdx, planPath(plan.ID, "respond-take-consent"), map[string]any{"agree": false})
	require.Equalf(t, http.StatusOK, code, "refuse: %v", body)
	logWith(t, h, h.mark(victimIdx), "refused", h.mark(prepIdx), "**Crown Jewels**")
}

func TestLog_SpreadRumors_TakeConsentIsLoggedBeforeTheTransfer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, prepIdx, victimIdx, _ := srTakeRequest(t, h)

	code, body := h.post(victimIdx, planPath(plan.ID, "respond-take-consent"), map[string]any{"agree": true})
	require.Equalf(t, http.StatusOK, code, "agree: %v", body)

	bodies := allLogBodies(t, h)
	consent, taken := -1, -1
	for i, b := range bodies {
		if strings.Contains(b, "consented to") && strings.Contains(b, h.mark(victimIdx)) {
			consent = i
		}
		if strings.Contains(b, "took **Crown Jewels**") {
			taken = i
		}
	}
	require.NotEqual(t, -1, consent, "the victim's consent is logged")
	require.NotEqual(t, -1, taken, "the transfer is logged")
	assert.Less(t, consent, taken, "consent reads before the take it licenses")
	_ = prepIdx
}

// ── Host Festivity ───────────────────────────────────────────────────────────

// Opting out is announced at the table and is what earns the host a make.
func TestLog_HostFestivity_OptOutIsLoggedAndEarnsTheHostAMake(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)

	code, body := h.post(guestIdx, planPath(plan.ID, "guest-roll"), map[string]any{"action": "opt_out"})
	require.Equalf(t, http.StatusOK, code, "opt-out: %v", body)

	logWith(t, h, h.mark(guestIdx), "opted out", h.mark(hostIdx), "earns a make")
}

func TestLog_HostFestivity_MarEarnsTheHostAMake(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)

	hfGuestRoll(t, h, plan.ID, guestIdx, "mar", "accept_duels", nil)

	logWith(t, h, h.mark(guestIdx), "mar earns", h.mark(hostIdx), "a make")
}

// The festivity's rumor is public record; the log said only that one was spread.
func TestLog_HostFestivity_RumorTextIsQuoted(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)

	hfGuestRoll(t, h, plan.ID, guestIdx, "make", "spread_rumor",
		map[string]any{"rumor_text": "the duke dances badly"})
	logWith(t, h, h.mark(guestIdx), "spread a new rumor", `"the duke dances badly"`)
}

func TestLog_HostFestivity_InsistNamesTheHostNotTheRole(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	plan, hostIdx := hfPrepareToSocializing(t, h)
	guestIdx := (hostIdx + 1) % len(h.tg.Players)

	hfGuestRoll(t, h, plan.ID, guestIdx, "make", "spread_rumor", map[string]any{"rumor_text": "a scandal"})
	code, body := h.post(guestIdx, planPath(plan.ID, "insist-host-mar"), nil)
	require.Equalf(t, http.StatusOK, code, "insist: %v", body)

	logWith(t, h, h.mark(guestIdx), "insisted that", h.mark(hostIdx), "take a mar")
	noLogWith(t, h, "insisted the host")
}

// ── Make Demands ─────────────────────────────────────────────────────────────

// The draft summary only appears once the fourth option is taken; a draft left
// half-finished was silent about who had picked what.
func TestLog_MakeDemands_EachDraftPickIsLogged(t *testing.T) {
	h := newMDHTTPHarness(t, 3)
	// P0 (power rank 1) demands and picks first; P1 is the target's preparer.
	_, demand := seedResolvingDemand(t, h.q, &h.tg, 0, 1, "make")
	path := "/api/plans/" + itoa(demand.ID) + "/draft-choice"

	status, body := h.post(t, 0, path, map[string]any{"option": game.DemandOptionControlLeverage})
	require.Equalf(t, http.StatusOK, status, "first pick: %v", body)
	status, body = h.post(t, 1, path, map[string]any{"option": game.DemandOptionPerformSteps})
	require.Equalf(t, http.StatusOK, status, "second pick: %v", body)

	posts := mdSystemPosts(t, h.q, h.tg.Game.ID)
	joined := strings.Join(posts, "\n")
	p0 := playerMark(h.tg.Players[0].ID, h.tg.Players[0].DisplayName)
	p1 := playerMark(h.tg.Players[1].ID, h.tg.Players[1].DisplayName)
	assert.Contains(t, joined, p0+" drafted leverage control", "a half-finished draft still shows who took what")
	assert.Contains(t, joined, p1+" drafted perform make/mar steps")
	assert.NotContains(t, joined, "draft complete", "the summary still waits for the fourth pick")
}

// ── Propose Duel ─────────────────────────────────────────────────────────────

// The stakes are masked until the duel is settled; the settlement is where the
// table learns WHICH assets changed hands, so the post names them.
func TestLog_ProposeDuel_SettlementNamesTheClaimedAssets(t *testing.T) {
	h := newPlanLifecycle(t, 5)
	prepIdx0 := h.focusPlayerIdx()
	targIdx := (prepIdx0 + 1) % 5
	prepPeer := h.seedPeer(prepIdx0, "Preparer stake")
	targPeer := h.seedPeer(targIdx, "Target stake")

	seedDuelToRollPinned(t, h, targIdx, []int64{prepPeer}, []int64{targPeer}, preparerSweepsPrep, preparerSweepsTarg)

	logWith(t, h, "won the duel and took 1 staked asset", "**Target stake**")
}
