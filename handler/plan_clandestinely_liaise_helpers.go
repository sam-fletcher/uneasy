package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	dbgen "uneasy/db/gen"
	"uneasy/game"
	"uneasy/model"
)

// ── Redelay Reveal ────────────────────────────────────────────────────────────

// clRedelayRevealHandler handles POST /api/plans/:planId/redelay-reveal.
//
// Phase 4 — When Will I See You Again?: both players submit a die face (1–6)
// or 0 to cancel the future meeting. Handled by the reveals endpoint, but
// this route is a convenience wrapper that submits to the linked redelay reveal.
//
// Request body: {"face": N}  (0 = cancel, 1–6 = schedule new meeting)
func clRedelayRevealHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanClandestinelyLiaise {
			respondErr(w, http.StatusBadRequest, "redelay-reveal is only for Clandestinely Liaise")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		ld := resData.EnsureLiaise()

		if ld.Phase != LiaiseWhenWillISeeYouAgain {
			respondErr(
				w,
				http.StatusConflict,
				fmt.Sprintf("redelay-reveal requires phase %q", LiaiseWhenWillISeeYouAgain),
			)
			return
		}
		seats, ok := clRequireSeats(w, ctx, deps.Q, plan, ld, player.ID, "redelay-reveal")
		if !ok {
			return
		}
		if ld.RedelayRevealID == nil {
			respondErr(w, http.StatusConflict, "redelay reveal not initialised")
			return
		}
		// The reveal's entries are keyed by seat (the preparer's and the partner's
		// own ids), so a stand-in submits under the seat they hold, not their own id.
		seat := clOpenSeat(seats, liaiseRedelaySubmitters(ctx, deps.Q, ld))

		var body struct {
			Face int16 `json:"face"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Face < 0 || body.Face > 6 {
			respondErr(w, http.StatusBadRequest, "face must be 0–6 (0 = cancel)")
			return
		}

		face := body.Face

		// Record the face in the reveal entry.
		if err := deps.Q.SetRevealEntryFace(ctx, dbgen.SetRevealEntryFaceParams{
			RevealID: *ld.RedelayRevealID,
			PlayerID: seat,
			Face:     &face,
		}); err != nil {
			respondInternalErr(w, r, "could not record redelay reveal", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventRevealSubmitted, model.RevealSubmittedPayload{
			RevealID: *ld.RedelayRevealID,
			PlayerID: seat,
		})

		// Check if both players have submitted.
		submitted, err := deps.Q.CountRevealEntriesSubmitted(ctx, *ld.RedelayRevealID)
		if err != nil {
			respondInternalErr(w, r, "could not check reveal status", err)
			return
		}
		total, err := deps.Q.CountRevealEntries(ctx, *ld.RedelayRevealID)
		if err != nil {
			respondInternalErr(w, r, "could not count reveal entries", err)
			return
		}

		if submitted >= total {
			if !clFinalizeRedelayReveal(r, ctx, w, deps, plan, &resData) {
				return
			}
		}

		// A submission narrows the acting set (the submitter no longer owes a
		// face), and the final submission finalizes the liaise — both change
		// ComputeRowState's result. RevealSubmitted/Complete drive a plan
		// refetch on clients, not a row-state recompute, so broadcast it here.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": seat,
			"face":      face,
		})
	}
}

// clEnsureRedelayReveal creates the "When will I see you again?" redelay
// simultaneous reveal and registers both participants, if it does not already
// exist. Mutates ld.RedelayRevealID; the caller is responsible for persisting
// resolution_data. No-op when the reveal already exists or the partner is unset.
func clEnsureRedelayReveal(
	ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, ld *LiaiseResolutionData,
) error {
	if ld.RedelayRevealID != nil || ld.PartnerID == nil {
		return nil
	}
	redelayReveal, err := deps.Q.CreateSimultaneousReveal(ctx, dbgen.CreateSimultaneousRevealParams{
		GameID:     plan.GameID,
		PlanID:     &plan.ID,
		RevealType: revealTypeLiaiseRedelay,
	})
	if err != nil {
		return fmt.Errorf("create redelay reveal: %w", err)
	}
	if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
		RevealID: redelayReveal.ID,
		PlayerID: plan.PreparerID,
	}); err != nil {
		return fmt.Errorf("register preparer in redelay reveal: %w", err)
	}
	if err := deps.Q.CreateRevealEntry(ctx, dbgen.CreateRevealEntryParams{
		RevealID: redelayReveal.ID,
		PlayerID: *ld.PartnerID,
	}); err != nil {
		return fmt.Errorf("register partner in redelay reveal: %w", err)
	}
	ld.RedelayRevealID = &redelayReveal.ID
	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// clFinalizeRedelayReveal runs the "both submitted" branch of redelay-reveal:
// computes the result delay (or detects a cancellation), marks the reveal
// complete, broadcasts the result, schedules a follow-up meeting if needed,
// and marks the liaise done. Writes HTTP errors and returns false on failure.
func clFinalizeRedelayReveal(
	r *http.Request,
	ctx context.Context,
	w http.ResponseWriter,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
) bool {
	ld := resData.EnsureLiaise()
	entries, err := deps.Q.ListRevealEntries(ctx, *ld.RedelayRevealID)
	if err != nil {
		respondInternalErr(w, r, "could not load reveal entries", err)
		return false
	}

	cancelled := false
	var resultDelay int16
	for _, e := range entries {
		if e.Face == nil || *e.Face == 0 {
			cancelled = true
			break
		}
	}
	if !cancelled {
		resultDelay = revealCeilAverage(entries)
	}

	err = deps.Q.SetRevealComplete(ctx, dbgen.SetRevealCompleteParams{
		ID:          *ld.RedelayRevealID,
		ResultDelay: &resultDelay,
	})
	if err != nil {
		respondInternalErr(w, r, "could not complete redelay reveal", err)
		return false
	}

	entryResults := make([]model.RevealEntryResult, 0, len(entries))
	for _, e := range entries {
		var f int16
		if e.Face != nil {
			f = *e.Face
		}
		entryResults = append(entryResults, model.RevealEntryResult{
			PlayerID: e.PlayerID,
			Face:     f,
		})
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventRevealComplete, model.RevealCompletePayload{
		RevealID:    *ld.RedelayRevealID,
		Entries:     entryResults,
		ResultDelay: resultDelay,
	})

	if err = clApplyRedelayOutcome(ctx, deps, plan, resData, resultDelay, cancelled); err != nil {
		respondInternalErr(w, r, "could not complete liaise", err)
		return false
	}
	return true
}

// clApplyRedelayOutcome finalizes the "When will I see you again?" reveal once
// the result delay is known: it schedules the follow-up Clandestinely Liaise
// plan (unless the pair cancelled with a 0, the delay is non-positive, or there
// is no room left on the record), logs the outcome, marks the liaise done, and
// resolves the plan. resData is mutated and persisted.
//
// The liaison auto-resolves here — there is no make/mar roll and no preparer
// decision left to make, so it never goes through the manual CompletePlan
// endpoint (which requires a roll outcome or stored result CL never has). This
// mirrors Exchange Courtiers' fair-trade path, which likewise resolves directly
// with a "make" outcome. The phase-change broadcast still fires because clients
// refetch the plan on liaise.phase_changed (reveal.complete only refreshes the
// reveal widget); the row-state recompute is left to the callers.
func clApplyRedelayOutcome(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	resData *ResolutionData,
	resultDelay int16,
	cancelled bool,
) error {
	ld := resData.EnsureLiaise()

	scheduled := false
	if !cancelled && resultDelay > 0 && ld.PartnerID != nil {
		if g, gerr := deps.Q.GetGameByID(ctx, plan.GameID); gerr == nil {
			newRow := g.CurrentRow + resultDelay
			if newRow <= publicRecordRowCount {
				clScheduleNewMeeting(ctx, deps, plan, newRow, *ld.PartnerID)
				scheduled = true
			}
		}
	}
	if scheduled {
		clLog(ctx, deps, plan, model.SeverityDefault, "The pair arranged to meet again.")
	} else {
		clLog(ctx, deps, plan, model.SeverityDefault, "The pair parted ways with no future meeting planned.")
	}

	ld.Phase = LiaiseDone
	if err := saveResolutionData(ctx, deps.Q, plan.ID, *resData); err != nil {
		return err
	}
	broadcastEvent(deps.Manager, plan.GameID, model.EventLiaisePhaseChanged,
		model.LiaisePhaseChangedPayload{PlanID: plan.ID, Phase: string(LiaiseDone)})

	// Resolve the plan outright (status → resolved). A liaison always "makes":
	// it carries no roll and no failure path.
	return resolvePlan(ctx, deps.Q, deps.Manager, plan, makeOutcome)
}

// ── Seats ─────────────────────────────────────────────────────────────────────
//
// A liaison has exactly two SEATS — the partner's and the preparer's — and every
// submission step (keep-secret, a Things We Share pick, a redelay face) fills
// one. A seat is named by the player it belongs to, and that is what the record
// keys on (kept_secrets, share_submitter_ids, liaise_choices, the reveal's
// entries), never by whoever happened to be typing.
//
// The distinction matters because Make Demands can hand someone else the
// preparer's controls: a resolved, made demand's perform_steps winner stands in
// for the preparer (actsForPreparer) and the preparer is locked out. A guard that
// asks only "is this caller the preparer or the partner?" refuses the winner AND
// — since the preparer is locked out — everyone else, so the step is performed by
// nobody and the row cannot finish. Asking "which seats may this caller fill?"
// admits whoever actually holds each one.
//
// Two shapes follow from that. A caller may hold NO seat (a spectator, or the
// locked-out preparer). And a caller may hold BOTH: the demander can be the
// liaison's own partner (Make Demands only forbids demanding against your own
// plan), so they fill their own seat and the preparer's.

// clSeatsFor returns the seats playerID may fill in this liaison, partner's
// first. The partner always fills their own; the preparer's belongs to the
// resolution actor — the preparer, unless a perform_steps winner stands in.
func clSeatsFor(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	ld *LiaiseResolutionData,
	playerID int64,
) []int64 {
	var seats []int64
	if ld.PartnerID != nil && *ld.PartnerID == playerID {
		seats = append(seats, playerID)
	}
	if actsForPreparer(ctx, q, plan, playerID) {
		seats = append(seats, plan.PreparerID)
	}
	return seats
}

// clRequireSeats is clSeatsFor as a route guard: it writes a 403 and returns
// ok=false when the caller holds no seat, naming step in the message.
func clRequireSeats(
	w http.ResponseWriter,
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	ld *LiaiseResolutionData,
	playerID int64,
	step string,
) ([]int64, bool) {
	seats := clSeatsFor(ctx, q, plan, ld, playerID)
	if len(seats) == 0 {
		respondErr(w, http.StatusForbidden,
			"only the partner, and whoever holds the preparer's slot, may submit "+step)
		return nil, false
	}
	return seats, true
}

// clOpenSeat picks which of a caller's seats a seatless submission (a redelay
// face carries no asset to tell the seats apart by) is for: the first that has
// not submitted yet, else the first. A one-seat caller has no choice to make.
func clOpenSeat(seats []int64, submitted map[int64]bool) int64 {
	for _, seat := range seats {
		if !submitted[seat] {
			return seat
		}
	}
	return seats[0]
}

// clPreparerSeatHolder is whoever fills the preparer's seat right now: the
// perform_steps winner if a demand handed it over, else the preparer.
func clPreparerSeatHolder(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) int64 {
	if winner := performStepsWinner(ctx, q, plan); winner != 0 {
		return winner
	}
	return plan.PreparerID
}

// clRevealSeatsFor is clSeatsFor for a reveal row: the seats viewerID fills in
// the redelay reveal. It is nil both for a viewer who holds none and for any
// other reveal type, so callers must check the type first (reveals.go does) before
// reading nil as "no seat". Only liaise_redelay can have a stand-in — it happens
// mid-resolution, after a demand can have resolved against the plan, whereas the
// delay reveal at prep time precedes any row to demand on. reveals.go uses it so
// the generic submit/get routes agree with this file's.
func clRevealSeatsFor(
	ctx context.Context,
	q *dbgen.Queries,
	reveal *dbgen.SimultaneousReveal,
	viewerID int64,
) []int64 {
	if reveal.RevealType != revealTypeLiaiseRedelay || reveal.PlanID == nil {
		return nil
	}
	plan, err := q.GetPlanByID(ctx, *reveal.PlanID)
	if err != nil {
		return nil
	}
	resData := loadResolutionData(plan.ResolutionData)
	return clSeatsFor(ctx, q, &plan, resData.EnsureLiaise(), viewerID)
}

// clShareSeat picks the seat a Things We Share submission is for. Every option
// targets the OTHER seat's assets, so for a caller holding both seats the target's
// owner settles it: a partner's asset means the preparer's seat is acting, and
// vice versa. ok is false when a two-seat caller named no target, or one owned by
// neither side — there is nothing to tell the seats apart by, and the plain
// target validation would only reject it anyway.
func clShareSeat(
	ctx context.Context,
	q *dbgen.Queries,
	seats []int64,
	plan *dbgen.Plan,
	ld *LiaiseResolutionData,
	targetAssetID *int64,
) (int64, bool) {
	if len(seats) == 1 {
		return seats[0], true
	}
	if targetAssetID == nil || ld.PartnerID == nil {
		return 0, false
	}
	target, err := q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		return 0, false
	}
	switch target.OwnerID {
	case *ld.PartnerID:
		return plan.PreparerID, true
	case plan.PreparerID:
		return *ld.PartnerID, true
	default:
		return 0, false
	}
}

// clOtherParticipantID returns the participant who is NOT playerID (the
// partner from playerID's perspective). Returns 0 if it can't be determined.
func clOtherParticipantID(plan *dbgen.Plan, playerID int64, ld LiaiseResolutionData) int64 {
	if playerID == plan.PreparerID {
		if ld.PartnerID != nil {
			return *ld.PartnerID
		}
		return 0
	}
	return plan.PreparerID
}

// clPartnerMeetingPeerID returns the meeting peer of playerID's PARTNER — the
// peer the other player brought to the liaison. From the preparer's seat that's
// PartnerPeerID; from the partner's seat it's PreparerPeerID. Returns nil if
// the relevant peer was never set.
func clPartnerMeetingPeerID(plan *dbgen.Plan, playerID int64, ld LiaiseResolutionData) *int64 {
	if playerID == plan.PreparerID {
		return ld.PartnerPeerID
	}
	return ld.PreparerPeerID
}

// clValidateShareTarget enforces that a Things We Share choice targets the
// partner's assets per the rules (all five options are second-person — "your
// partner's …"). Returns (0, "") when valid, or an HTTP status + message.
//
//   - look_at_secret / leverage_partner: any partner-owned, non-destroyed asset.
//   - take_gift:                          a partner-owned, non-destroyed NON-peer.
//   - update_peer / break_peer:           the partner's MEETING PEER specifically
//     (meetingPeerID — the peer they brought to this liaison), non-destroyed.
//     Both require a marginalia on that peer: break_peer tears it; update_peer
//     rewrites it (and so also requires non-empty updateText).
func clValidateShareTarget(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	partnerID int64,
	meetingPeerID *int64,
	choice string,
	targetAssetID, targetMargID *int64,
	updateText string,
) (status int, msg string) {
	if partnerID == 0 {
		return http.StatusConflict, "liaison partner is not set"
	}
	if targetAssetID == nil {
		return http.StatusBadRequest, fmt.Sprintf("target_asset_id is required for choice %q", choice)
	}
	asset, err := deps.Q.GetAssetByID(ctx, *targetAssetID)
	if err != nil {
		return http.StatusNotFound, "target asset not found"
	}
	if asset.GameID != plan.GameID {
		return http.StatusBadRequest, "target asset does not belong to this game"
	}
	if asset.OwnerID != partnerID {
		return http.StatusForbidden, "Things We Share options target your partner's assets"
	}
	if asset.IsDestroyed {
		return http.StatusBadRequest, "target asset is destroyed"
	}

	switch choice {
	case liaiseChoiceTakeGift:
		if asset.AssetType == model.AssetPeer {
			return http.StatusBadRequest, "a gift must be a non-peer asset"
		}
	case liaiseChoiceUpdatePeer, liaiseChoiceBreakPeer:
		if asset.AssetType != model.AssetPeer {
			return http.StatusBadRequest, fmt.Sprintf("%q targets a peer asset", choice)
		}
		// These options must target the partner's MEETING PEER — the specific
		// peer they brought to the liaison — not an arbitrary partner peer.
		if meetingPeerID == nil {
			return http.StatusConflict, fmt.Sprintf(
				"%q is unavailable — the meeting peer no longer exists", choice)
		}
		if *targetAssetID != *meetingPeerID {
			return http.StatusBadRequest, fmt.Sprintf(
				"%q must target your partner's meeting peer", choice)
		}
		// Both update_peer and break_peer act on a specific marginalia of the
		// meeting peer. break_peer tears it; update_peer rewrites it.
		if targetMargID == nil {
			verb := "the marginalia to tear"
			if choice == liaiseChoiceUpdatePeer {
				verb = "the marginalia to rewrite"
			}
			return http.StatusBadRequest, fmt.Sprintf("%s requires target_marginalia_id (%s)", choice, verb)
		}
		m, err := deps.Q.GetMarginaliaByID(ctx, *targetMargID)
		if err != nil {
			return http.StatusNotFound, "marginalia not found"
		}
		if m.AssetID != *targetAssetID {
			return http.StatusBadRequest, "marginalia does not belong to the target peer"
		}
		if m.IsTorn {
			return http.StatusConflict, "marginalia is already torn"
		}
		if choice == liaiseChoiceUpdatePeer && updateText == "" {
			return http.StatusBadRequest, "update_peer requires update_text (the rewritten marginalia)"
		}
	}
	return 0, ""
}

// clScheduleNewMeeting creates a new Clandestinely Liaise plan for the re-delay meeting.
func clScheduleNewMeeting(
	ctx context.Context,
	deps *PlanDeps,
	originalPlan *dbgen.Plan,
	targetRow int16,
	partnerID int64,
) {
	count, err := deps.Q.CountPlansOnRow(ctx, dbgen.CountPlansOnRowParams{
		GameID:    originalPlan.GameID,
		RowNumber: new(targetRow),
	})
	if err != nil {
		count = 0
	}

	newPlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
		GameID:           originalPlan.GameID,
		PlanType:         model.PlanClandestinelyLiaise,
		Category:         model.CategoryKnowledge,
		PreparerID:       originalPlan.PreparerID,
		TargetPlayerID:   &partnerID,
		RowNumber:        new(targetRow),
		RowOrder:         int16(count),
		PreparedAtRow:    targetRow, // prepared "at" the row it resolves on
		PreparationNotes: originalPlan.PreparationNotes,
	})
	if err != nil {
		return // Scheduling the new meeting is best-effort.
	}

	// Initialise resolution data for the new plan. Carry forward the original
	// meeting peers — the same two peers reconvene. If either has been destroyed
	// by the time the follow-up resolves, Things We Share handles it gracefully.
	origLD := game.LoadLiaiseData(originalPlan.ResolutionData)
	newResData := ResolutionData{
		Liaise: &LiaiseResolutionData{
			PartnerID:      &partnerID,
			PreparerPeerID: origLD.PreparerPeerID,
			PartnerPeerID:  origLD.PartnerPeerID,
		},
	}
	_ = saveResolutionData(ctx, deps.Q, newPlan.ID, newResData)

	broadcastEvent(deps.Manager, originalPlan.GameID, model.EventPlanPrepared, model.PlanPayload{Plan: newPlan})
	EmitPlanPrepared(ctx, deps.Q, deps.Manager, newPlan)
}
