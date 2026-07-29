//go:build integration

package handler

// handler/finale_integration_test.go — Explosive Finale mechanics, backend
// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §3–§6, Session 3).
//
// Two things are under test here and they are two halves of one rule:
//
//  1. the mode-aware overflow decision — the prep grid must grey out exactly
//     what a prepare call would reject, for every ending mode × slot state; and
//  2. what happens when a plan falls through — the token comes off the shield,
//     the preparer can't re-pick that type on that row, and the log says so.

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

// setEndingMode settles the game's ending mode directly (the table vote is
// covered by ending_vote_integration_test.go; these tests start from a settled
// mode). Pass "" to clear it back to an unsettled game.
func setEndingMode(t *testing.T, h *planLifecycle, mode string) {
	t.Helper()
	var m *string
	if mode != "" {
		m = &mode
	}
	require.NoError(t, h.q.SetEndingMode(context.Background(), dbgen.SetEndingModeParams{
		ID: h.tg.Game.ID, EndingMode: m,
	}))
}

// spendFinaleSlot plants a finished bonus plan for the player, which is what
// "your slot is spent" means — the accounting is derived from plans, never from
// a flag on the player.
func spendFinaleSlot(t *testing.T, h *planLifecycle, playerIdx int) {
	t.Helper()
	ctx := context.Background()
	row := int16(publicRecordRowCount)
	plan, err := h.q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:        h.tg.Game.ID,
		PlanType:      model.PlanSpreadPropaganda,
		Category:      model.CategoryEsteem,
		PreparerID:    h.tg.Players[playerIdx].ID,
		RowNumber:     &row,
		PreparedAtRow: h.tg.Game.CurrentRow,
	})
	require.NoError(t, err)
	require.NoError(t, h.q.SetPlanFinaleBonus(ctx, plan.ID))
}

// prepareRaw drives prepare-plan as the current focus player WITHOUT asserting
// success — these tests care about the rejections as much as the acceptances.
func prepareRaw(h *planLifecycle, req PreparePlanRequest) (int, map[string]any) {
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/prepare-plan"
	return h.post(h.focusPlayerIdx(), path, req)
}

// gridEntry runs the prep grid's own eligibility pipeline for one plan type as
// players[idx] — the exact call GET /plan-eligibility makes per tile.
func gridEntry(
	t *testing.T,
	h *planLifecycle,
	idx int,
	planType model.PlanType,
) (reason string, targetRow int16, finaleBonus bool) {
	t.Helper()
	ctx := context.Background()
	game, err := h.q.GetGameByID(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	ph, ok := GetHandler(planType)
	require.True(t, ok)
	reason, targetRow, finaleBonus, err = planIneligibilityReason(
		ctx, h.q, &game, &h.tg.Players[idx], planType, ph, false)
	require.NoError(t, err)
	return reason, targetRow, finaleBonus
}

// gamePostBodies returns every system post body for the game, newest last.
func gamePostBodies(t *testing.T, h *planLifecycle) []string {
	t.Helper()
	posts, err := h.q.ListGamePosts(context.Background(), h.tg.Game.ID)
	require.NoError(t, err)
	var out []string
	for _, p := range posts {
		if p.SystemCode != nil {
			out = append(out, *p.SystemCode+" | "+p.Body)
		}
	}
	return out
}

// declareWarOverflowing prepares a Make War from players[preparerIdx] against
// players[enemyIdx] and drives its delay reveal to a face that lands past row
// 13. Returns the plan id.
func declareWarOverflowing(t *testing.T, h *planLifecycle, preparerIdx, enemyIdx int, face int16) int64 {
	t.Helper()
	h.setFocus(preparerIdx)
	notes := "past the end of the record"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeWar,
		EnemyPlayerIDs:   []int64{h.tg.Players[enemyIdx].ID},
		PreparationNotes: &notes,
	})
	require.Nil(t, plan.RowNumber, "a war declaration takes no row until its reveal closes")

	mw := loadResolutionData(plan.ResolutionData).MakeWar
	require.NotNil(t, mw)
	require.NotNil(t, mw.DelayRevealID)
	revPath := "/api/reveals/" + strconv.FormatInt(*mw.DelayRevealID, 10) + "/submit"
	for _, idx := range []int{preparerIdx, enemyIdx} {
		code, body := h.post(idx, revPath, map[string]any{"face": face})
		require.Equalf(t, http.StatusOK, code, "reveal submit: %v", body)
	}
	return plan.ID
}

// ── §3/§4: the mode-aware overflow decision ──────────────────────────────────

// TestFinaleRow8_HostFestivityMirrorsAcrossModes walks the row-8 case the
// deadline was chosen for: at row 8 Host Festivity (delay 6) is the ONLY plan
// that overflows, so it is the whole of the mode's effect on the grid that row.
//
// Each case asserts the grid and the prepare call agree — that mirror is the
// point of routing both through planOverflowOutcome, and it is what the audit
// found broken (the grid was mode-blind, so choosing Explosive Finale un-greyed
// nothing).
func TestFinaleRow8_HostFestivityMirrorsAcrossModes(t *testing.T) {
	cases := []struct {
		name      string
		mode      string
		slotSpent bool
		// wantReason "" means the tile stays live and the prepare succeeds.
		wantReason string
		// wantPrepareErr overrides the expected prepare rejection where it
		// legitimately differs from the tile's text; empty means "the same
		// sentence", which is the rule everywhere else.
		wantPrepareErr string
	}{
		{
			// The one case where the two texts differ on purpose: the tile has
			// nowhere to put the structured endgame_choice_required body, so it
			// falls back to the plain row-room wording. Unreachable in normal
			// play — the vote settles the mode before row 8 begins — and only
			// reachable at all through the dev fast-forward.
			name:           "unsettled mode greys out and 409s",
			mode:           "",
			wantReason:     "no room on the public record (would exceed row 13)",
			wantPrepareErr: "plan would land past row 13, and the table has not settled how the game ends",
		},
		{
			name: "smooth landing names the mode",
			mode: EndingModeSmoothLanding,
			wantReason: "under a Smooth Landing no plan may land past row 13 — " +
				"choose a different plan, or don't prepare anything",
		},
		{
			name: "explosive finale with the slot free clamps to row 13",
			mode: EndingModeExplosiveFinale,
		},
		{
			name:       "explosive finale with the slot spent refuses",
			mode:       EndingModeExplosiveFinale,
			slotSpent:  true,
			wantReason: "you have already used your one Explosive Finale plan",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newPlanLifecycle(t, 3)
			h.jumpToRow(8)
			setEndingMode(t, h, tc.mode)
			focus := h.focusPlayerIdx()
			if tc.slotSpent {
				spendFinaleSlot(t, h, focus)
			}

			reason, targetRow, bonus := gridEntry(t, h, focus, model.PlanHostFestivity)
			require.Equal(t, tc.wantReason, reason, "prep grid reason")

			notes := "a grand ball at the end of the world"
			code, body := prepareRaw(h, PreparePlanRequest{
				PlanType:         model.PlanHostFestivity,
				PreparationNotes: &notes,
			})

			if tc.wantReason != "" {
				wantErr := tc.wantPrepareErr
				if wantErr == "" {
					wantErr = tc.wantReason
				}
				require.Equal(t, http.StatusConflict, code, "prepare should mirror the grid: %v", body)
				require.Equal(t, wantErr, body["error"],
					"the grid's reason and the prepare rejection must be the same sentence")
				require.False(t, bonus)
				return
			}

			// Allowed: clamped onto row 13, and marked as the one bonus plan.
			require.Equalf(t, http.StatusCreated, code, "prepare: %v", body)
			require.Equal(t, int16(publicRecordRowCount), targetRow, "grid target row")
			require.True(t, bonus, "the grid must mark a tile that would spend the slot")

			plans, err := h.q.ListPlansByGame(context.Background(), h.tg.Game.ID)
			require.NoError(t, err)
			require.Len(t, plans, 1)
			require.NotNil(t, plans[0].RowNumber)
			require.Equal(t, int16(publicRecordRowCount), *plans[0].RowNumber,
				"an Explosive Finale plan lands on row 13, not its own row")
			require.True(t, plans[0].IsFinaleBonus, "the plan is the slot accounting")
		})
	}
}

// TestFinaleRow8_SlotIsOnePerPlayer: spending the slot closes it for that
// player only. The grid and prepare agree on the refusal, and a different
// player's slot is untouched — the accounting is per preparer.
func TestFinaleRow8_SlotIsOnePerPlayer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(8)
	setEndingMode(t, h, EndingModeExplosiveFinale)

	first := h.focusPlayerIdx()

	// Before spending: Exchange Courtiers (delay 5) still fits at row 8, so it
	// is neither refused nor marked.
	reason, targetRow, bonus := gridEntry(t, h, first, model.PlanExchangeCourtiers)
	require.Empty(t, reason)
	require.Equal(t, int16(publicRecordRowCount), targetRow, "8 + 5 = 13 fits exactly")
	require.False(t, bonus, "a plan that fits is never a bonus plan")

	notes := "a grand ball"
	code, body := prepareRaw(h, PreparePlanRequest{
		PlanType:         model.PlanHostFestivity,
		PreparationNotes: &notes,
	})
	require.Equalf(t, http.StatusCreated, code, "first bonus plan: %v", body)

	// Preparing is the focus player's step-5 action, so it auto-passes focus and
	// (with no plans left on row 8) advances the row. Pin the row so the
	// assertions below don't ride on that side effect.
	h.jumpToRow(9) // now Exchange Courtiers (delay 5) overflows too

	// The spender is done: every overflowing tile is now refused with the slot
	// message rather than a row-room one.
	reason, _, bonus = gridEntry(t, h, first, model.PlanExchangeCourtiers)
	require.Equal(t, "you have already used your one Explosive Finale plan", reason)
	require.False(t, bonus)

	// A different player still holds theirs.
	other := (first + 1) % len(h.tg.Players)
	reason, targetRow, bonus = gridEntry(t, h, other, model.PlanExchangeCourtiers)
	require.Empty(t, reason)
	require.Equal(t, int16(publicRecordRowCount), targetRow)
	require.True(t, bonus)
}

// TestFinale_DiceDelayTilesAreNeverMarked: Make War and Clandestinely Liaise
// report target_row -1 and are never marked as bonus plans at prep time —
// whether they overflow is the reveal's business (§4). With the slot free they
// stay live even where every possible delay overflows; with it spent they are
// refused, because then every outcome is a guaranteed fall-through.
func TestFinale_DiceDelayTilesAreNeverMarked(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	setEndingMode(t, h, EndingModeExplosiveFinale)
	h.jumpToRow(publicRecordRowCount) // row 13: even MinDelay 1 overflows
	focus := h.focusPlayerIdx()

	reason, targetRow, bonus := gridEntry(t, h, focus, model.PlanMakeWar)
	require.Empty(t, reason, "with the slot free the declaration is no longer futile — it collapses")
	require.Equal(t, int16(-1), targetRow, "a dice-delay plan reports no row until its reveal")
	require.False(t, bonus, "a dice-delay tile can't be marked: the reveal decides")

	spendFinaleSlot(t, h, focus)
	reason, _, bonus = gridEntry(t, h, focus, model.PlanMakeWar)
	require.Equal(t, "you have already used your one Explosive Finale plan", reason,
		"with the slot spent every delay is a guaranteed fall-through, so refuse it up front")
	require.False(t, bonus)
}

// ── §5/§6: the dice-delay collapse and the fall-through ──────────────────────

// TestFinale_DiceOverflowCollapsesAndSpendsSlot: the one place a slot is spent
// without an explicit choice. It must land on row 13, mark the plan, and say so
// in the log.
func TestFinale_DiceOverflowCollapsesAndSpendsSlot(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(9)
	setEndingMode(t, h, EndingModeExplosiveFinale)

	planID := declareWarOverflowing(t, h, 0, 2, 5) // 9 + 5 = 14

	plan, err := h.q.GetPlanByID(context.Background(), planID)
	require.NoError(t, err)
	require.Equal(t, model.PlanPending, plan.Status, "the declaration stands")
	require.NotNil(t, plan.RowNumber)
	require.Equal(t, int16(publicRecordRowCount), *plan.RowNumber, "collapsed onto row 13")
	require.True(t, plan.IsFinaleBonus, "the collapse spends the slot")

	bodies := gamePostBodies(t, h)
	require.True(t, anyContains(bodies, "plan.finale_collapse"),
		"a slot spent without an explicit choice must be stated in the log: %v", bodies)
	require.True(t, anyContains(bodies, "spends"), "the post must name the spend: %v", bodies)

	// And the slot really is spent: an overflowing prepare is now refused.
	reason, _, _ := gridEntry(t, h, 0, model.PlanExchangeCourtiers) // delay 5 from row 9 → 14
	require.Equal(t, "you have already used your one Explosive Finale plan", reason)
}

// TestFinale_DiceOverflowFallsThroughWhenSlotSpent: the status quo, kept. With
// no slot to collapse onto, the plan falls through — and everything §6 asks for
// happens: the token comes off the shield, the preparer is blocked from
// re-picking that type on this row (but not the next), and the log post names
// the cause, the token and what the preparer may do.
func TestFinale_DiceOverflowFallsThroughWhenSlotSpent(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(9)
	setEndingMode(t, h, EndingModeExplosiveFinale)
	spendFinaleSlot(t, h, 0)

	planID := declareWarOverflowing(t, h, 0, 2, 5) // 9 + 5 = 14, no slot left
	ctx := context.Background()

	plan, err := h.q.GetPlanByID(ctx, planID)
	require.NoError(t, err)
	require.Equal(t, model.PlanCancelled, plan.Status, "the declaration fell through")
	require.Nil(t, plan.RowNumber, "a fallen-through plan holds no row — it never reaches the record")
	require.False(t, plan.IsFinaleBonus)

	// The shield is clear: no token for a plan that wasn't actually prepared.
	tokens, err := h.q.ListPlanTokensByType(ctx, dbgen.ListPlanTokensByTypeParams{
		GameID: h.tg.Game.ID, PlanType: model.PlanMakeWar,
	})
	require.NoError(t, err)
	require.Empty(t, tokens, "the plan token must come off the shield on a fall-through")

	// The log is the whole account the table gets, so it carries all three parts.
	bodies := gamePostBodies(t, h)
	require.True(t, anyContains(bodies, "plan.cancelled"), "%v", bodies)
	require.True(t, anyContains(bodies, "fell through"), "%v", bodies)
	require.True(t, anyContains(bodies, "past row 13"), "the cause: %v", bodies)
	require.True(t, anyContains(bodies, "No token stays on the shield"), "the token: %v", bodies)
	require.True(t, anyContains(bodies, "before the next row"), "the re-pick block: %v", bodies)
	for _, b := range bodies {
		if strings.HasPrefix(b, "plan.cancelled") {
			require.NotContains(t, b, "cancelled.",
				`player-facing copy says "fell through", never "cancelled"`)
		}
	}

	// The preparer may not re-declare this row — the faces are chosen, not
	// rolled, so a free retry would make the reveal meaningless.
	reason, _, _ := gridEntry(t, h, 0, model.PlanMakeWar)
	require.Equal(t,
		"this plan fell through on this row — prepare a different one, or try this again on a later row",
		reason)

	// A DIFFERENT plan type is fine on the same row.
	reason, _, _ = gridEntry(t, h, 0, model.PlanExchangeCourtiers)
	require.NotContains(t, reason, "fell through")

	// And the same type is fine again next row.
	h.jumpToRow(10)
	reason, _, _ = gridEntry(t, h, 0, model.PlanMakeWar)
	require.Empty(t, reason, "the block is scoped to the row the plan fell through on")
}

// TestFinale_FallThroughFreesTheShieldForLowerRanks: the token removal is not
// only for the preparer. While it lingered, every LOWER-ranked player was
// locked out of that plan type until the next engrailed line — for a plan that
// never happened.
func TestFinale_FallThroughFreesTheShieldForLowerRanks(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(9)
	setEndingMode(t, h, EndingModeSmoothLanding)
	ctx := context.Background()

	// players[0] outranks players[1] in power, which is Make War's category.
	pinPowerRank(t, h, h.tg.Players[0].ID, 1)
	pinPowerRank(t, h, h.tg.Players[1].ID, 2)

	declareWarOverflowing(t, h, 0, 2, 5) // 9 + 5 = 14, Smooth Landing → falls through

	ok, reason, err := checkPlanEligible(ctx, h.q, h.tg.Game.ID, h.tg.Players[1].ID,
		h.tg.Game.CurrentRow, model.PlanMakeWar, model.CategoryPower)
	require.NoError(t, err)
	require.Truef(t, ok, "a lower-ranked player must be free to take the shield: %s", reason)
}

// TestFinale_FallThroughLeavesTheTurnWithTheNextPlayer pins what actually
// happens to the turn, which is NOT what §6's audit recorded.
//
// §6 says "they do get their turn back — already, and unintentionally… the
// preparer still holds focus (nothing passed it)". That is wrong on the live
// path: preparing a plan is the focus player's step-5 action and auto-passes
// focus (PreparePlan → autoPassFocus), so by the time the delay reveal resolves
// the preparer's turn is over and the next player is up. The row was held
// throughout by the open-delay-reveal gate, so nothing was skipped — but the
// preparer gets no re-pick, and the log post is worded accordingly.
//
// This test exists so the discrepancy is visible rather than assumed. If the
// owner decides the turn should come back, this is the assertion to flip — and
// note that simply not auto-passing would let the preparer prepare twice in one
// turn, so it needs more than a one-line change.
func TestFinale_FallThroughLeavesTheTurnWithTheNextPlayer(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(9)
	setEndingMode(t, h, EndingModeSmoothLanding)
	ctx := context.Background()

	declareWarOverflowing(t, h, 0, 2, 5) // players[0] declares; falls through

	game, err := h.q.GetGameByID(ctx, h.tg.Game.ID)
	require.NoError(t, err)
	require.NotNil(t, game.FocusPlayerID)
	require.Equal(t, h.tg.Players[1].ID, *game.FocusPlayerID,
		"preparing auto-passed focus at step 6; the fall-through does not hand it back")
	require.Equal(t, int16(9), game.CurrentRow,
		"the row was held for the reveal and does not advance on a fall-through")

	rs := h.rowState()
	require.Equal(t, model.RowStateSceneSetting, rs.Kind, "the next player's turn begins")
	require.Equal(t, []int64{h.tg.Players[1].ID}, rs.ActingPlayerIDs)
}

// TestPassFocus_BlockedWhileOwnDelayRevealOpen: a preparer must not be able to
// spend their turn while the reveal that decides their plan's fate is still
// open — if it falls through afterwards, the re-pick §6 promises them is gone.
func TestPassFocus_BlockedWhileOwnDelayRevealOpen(t *testing.T) {
	h := newPlanLifecycle(t, 3)
	h.jumpToRow(9)
	setEndingMode(t, h, EndingModeSmoothLanding)

	h.setFocus(0)
	notes := "war, then a decision"
	plan := h.prepare(PreparePlanRequest{
		PlanType:         model.PlanMakeWar,
		EnemyPlayerIDs:   []int64{h.tg.Players[2].ID},
		PreparationNotes: &notes,
	})
	require.Nil(t, plan.RowNumber)

	// Hand focus back to the declarer (preparing auto-passed it) and try to pass.
	h.setFocus(0)
	path := "/api/tables/" + strconv.FormatInt(h.tg.Game.ID, 10) + "/pass-focus"
	code, body := h.post(0, path, nil)
	require.Equalf(t, http.StatusConflict, code, "pass-focus should be refused: %v", body)
	require.Contains(t, body["error"], "delay reveal")

	// A player with no open reveal of their own is unaffected.
	h.setFocus(1)
	code, body = h.post(1, path, nil)
	require.Equalf(t, http.StatusOK, code, "an uninvolved focus player may still pass: %v", body)
}
