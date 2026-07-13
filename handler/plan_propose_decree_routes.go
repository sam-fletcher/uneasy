package handler

// handler/plan_propose_decree_routes.go — HTTP route handlers for Propose
// Decree's council/debate/amendment/enactment sub-flow (the ExtraRoutes
// registered in plan_propose_decree.go). See that file for the plan's
// contract implementation and full lifecycle doc.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── Start Debate ────────────────────────────────────────────────────────────

// pdStartDebateHandler handles POST /api/plans/:planId/start-debate.
//
// The preparer finalizes the decree's text (pre-populated from their preparation
// notes, editable here) and opens the council debate. This is a required pre-roll
// step: the finalized text becomes the law body at enactment, and the signatory
// cannot call the roll until the debate has been opened (and every eligible
// player has decided). Opening the debate posts the proposed law to the chat to
// seed discussion. Only the preparer may do this, once.
//
// Request body: {"text": "the finalized decree body"}
func pdStartDebateHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "start-debate is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the preparer may open the debate")
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
			respondErr(w, http.StatusBadRequest, "text (the finalized decree body) is required")
			return
		}
		text, ok := textField(w, "text", body.Text, maxLongTextLen)
		if !ok {
			return
		}
		body.Text = text

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if pd.DebateStarted {
			respondErr(w, http.StatusConflict, "the debate has already been opened")
			return
		}

		pd.LawText = strings.TrimSpace(body.Text)
		pd.DebateStarted = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not open the debate", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventDecreeDebateStarted, model.DecreeCouncilJoinedPayload{
			PlanID:   plan.ID,
			PlayerID: player.ID,
		})
		pdLog(ctx, deps, plan, model.SeverityImportant,
			fmt.Sprintf("%s opened the council debate on the proposed decree:\n\n%q",
				playerDisplayName(ctx, deps.Q, player.ID), pd.LawText))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"law_text":  pd.LawText,
			"debate_on": true,
		})
	}
}

// ── Join Council ──────────────────────────────────────────────────────────────

// pdJoinCouncilHandler handles POST /api/plans/:planId/join-council.
//
// A player joins the council by leveraging exactly ONE of their assets at this
// stage (it becomes a die for the roll). More assets can be leveraged normally
// once the roll is open — the council step is just the cost of a seat. Eligible
// players: anyone ranked BELOW the preparer on the power track (the "other
// players" of pre-roll rule 2). Everyone ranked above the preparer — including
// whoever sits highest on the track, and the monarchOwner at any rank — is
// already auto-seated for free.
//
// Joining and declining (decline-council) are the two ways an eligible player
// records their pre-roll decision; an eligible player who has already joined or
// declined cannot join again.
//
// Joining does NOT change the signatory: it was fixed when the council was
// seated (OnResolve), and an eligible joiner is always ranked below the
// preparer, so they can never out-rank the sitting signatory. Joining just
// adds a member and the one die they leverage in.
//
// Request body: {"asset_ids": [N]}  (exactly one)
//
//nolint:funlen,gocognit // verify-and-leverage loop with eligibility branches
func pdJoinCouncilHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "join-council is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.AssetIDs) != 1 {
			respondErr(w, http.StatusBadRequest,
				"exactly one asset_id is required to join the council (more can be leveraged once the roll is open)")
			return
		}

		ctx := r.Context()

		// Determine preparer's power rank.
		preparerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, plan.PreparerID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine preparer power rank", err)
			return
		}

		// Check if the joining player is the preparer themselves — they're already in.
		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusConflict, "the preparer is already in the council")
			return
		}

		// Determine the joiner's power rank.
		joinerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine your power rank", err)
			return
		}

		// Eligibility: the leverage-to-join path is for the "other players" — those
		// ranked BELOW the preparer on power (higher rank number). The Monarch and
		// everyone ranked above the preparer are already auto-seated for free at
		// OnResolve, so they have no reason (and no route) to leverage in.
		if joinerRank <= preparerRank {
			respondErr(w, http.StatusForbidden,
				"only players ranked below the preparer on power may leverage to join the council")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()

		// An eligible player decides once. If they have already joined or declined,
		// they cannot join again (this also guards the auto-seated monarchOwner, who
		// is below the preparer on rank but already sits on the council).
		if slices.Contains(pd.SignatoryPlayerIDs, player.ID) {
			respondErr(w, http.StatusConflict, "you are already on the council")
			return
		}
		if slices.Contains(pd.DeclinedPlayerIDs, player.ID) {
			respondErr(w, http.StatusConflict, "you have already declined to join the council")
			return
		}

		// Verify and leverage each specified asset. Each leveraged asset provides
		// one die "to help or interfere when the roll comes" (pre-roll rule 2): we
		// mark the asset leveraged (the join cost) and mint an ephemeral 'decree'
		// banked die the joiner spends during the roll's leverage stage. Any decree
		// die left unspent is discarded when the law is enacted (ApplyChoice).
		for _, assetID := range body.AssetIDs {
			asset, err := deps.Q.GetAssetByID(ctx, assetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", assetID))
				return
			}
			if asset.GameID != plan.GameID {
				respondErr(w, http.StatusBadRequest, "asset does not belong to this game")
				return
			}
			if asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.IsLeveraged {
				respondErr(w, http.StatusConflict, fmt.Sprintf("asset %d is already leveraged", assetID))
				return
			}
			if err := deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{
				ID:          assetID,
				IsLeveraged: true,
			}); err != nil {
				respondInternalErr(w, r, "could not leverage asset", err)
				return
			}
			if _, err := deps.Q.CreateBankedDie(ctx, dbgen.CreateBankedDieParams{
				GameID:   plan.GameID,
				PlayerID: player.ID,
				Source:   decreeBankedDieSource,
			}); err != nil {
				respondInternalErr(w, r, "could not create council die", err)
				return
			}
			broadcastEvent(deps.Manager, plan.GameID, model.EventAssetLeveraged, model.AssetIDPayload{
				AssetID:  assetID,
				PlayerID: player.ID,
			})
		}

		// Add to council. The signatory does NOT move: it was fixed when the
		// council was seated (OnResolve), and joining cannot change it. A joiner is
		// by eligibility ranked below the preparer, so they can never out-rank the
		// sitting signatory; and the game's linear async flow offers no chance to
		// depose the monarch between seating and the roll. So joining only adds a
		// member and their one die.
		pd.SignatoryPlayerIDs = append(pd.SignatoryPlayerIDs, player.ID)

		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save council data", err)
			return
		}

		// Add the joiner's main character as a plan-scene peer (adr/CHAT_OVERHAUL_PLAN.md
		// Phase 5) so they can speak in character for the rest of the council
		// meeting — a no-op if this plan never opened one.
		if scene, sErr := loadActiveScene(ctx, deps.Q, plan.GameID); sErr == nil && scene != nil {
			AddPlanSceneParticipant(ctx, deps.Q, deps.Manager, scene, player.ID)
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventDecreeCouncilJoined, model.DecreeCouncilJoinedPayload{
			PlanID:      plan.ID,
			PlayerID:    player.ID,
			SignatoryID: *pd.SignatoryID,
		})

		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf("%s leveraged into the council, bringing a die to the roll.",
				playerDisplayName(ctx, deps.Q, player.ID)),
		)

		// The waiting-on bar names the eligible players who still owe a join/decline
		// decision; recompute and rebroadcast the row state now that one decided.
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":      plan.ID,
			"player_id":    player.ID,
			"signatory_id": *pd.SignatoryID,
			"council":      pd.SignatoryPlayerIDs,
		})
	}
}

// ── Decline Council ────────────────────────────────────────────────────────────

// pdDeclineCouncilHandler handles POST /api/plans/:planId/decline-council.
//
// An eligible player (ranked below the preparer on power, not auto-seated)
// records that they will NOT join the council. Declining is the counterpart to
// join-council: the signatory cannot call the roll until every eligible player
// has either joined or declined, and the waiting-on bar names whoever still
// owes that decision. No assets are leveraged.
func pdDeclineCouncilHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "decline-council is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusConflict, "the preparer cannot decline their own council")
			return
		}

		preparerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, plan.PreparerID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine preparer power rank", err)
			return
		}
		declinerRank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryPower)
		if err != nil {
			respondInternalErr(w, r, "could not determine your power rank", err)
			return
		}
		if declinerRank <= preparerRank {
			respondErr(w, http.StatusForbidden,
				"only players ranked below the preparer on power decide whether to join the council")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if slices.Contains(pd.SignatoryPlayerIDs, player.ID) {
			respondErr(w, http.StatusConflict, "you are already on the council")
			return
		}
		if slices.Contains(pd.DeclinedPlayerIDs, player.ID) {
			respondErr(w, http.StatusConflict, "you have already declined to join the council")
			return
		}

		pd.DeclinedPlayerIDs = append(pd.DeclinedPlayerIDs, player.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save your decision", err)
			return
		}

		broadcastEvent(deps.Manager, plan.GameID, model.EventDecreeCouncilDeclined, model.DecreeCouncilJoinedPayload{
			PlanID:   plan.ID,
			PlayerID: player.ID,
		})
		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s declined to join the council.", playerDisplayName(ctx, deps.Q, player.ID)))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"player_id": player.ID,
			"declined":  pd.DeclinedPlayerIDs,
		})
	}
}

// pdPendingDeciders returns the eligible players (ranked below the preparer on
// power) who have neither joined the council nor declined — the players the
// table is waiting on during the council meeting. Auto-seated members (the
// monarchOwner, anyone above the preparer) are already in SignatoryPlayerIDs and
// so never appear here.
func pdPendingDeciders(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	pd *gamepkg.ProposeDecreeResolutionData,
) ([]int64, error) {
	preparerRank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryPower)
	if err != nil {
		return nil, fmt.Errorf("preparer power rank: %w", err)
	}
	ranks, err := q.ListRankingsByGame(ctx, plan.GameID)
	if err != nil {
		return nil, fmt.Errorf("list rankings: %w", err)
	}
	var pending []int64
	for _, rk := range ranks {
		if rk.Category != model.CategoryPower || rk.PlayerID == nil {
			continue
		}
		id := *rk.PlayerID
		if rk.Rank <= preparerRank {
			continue // preparer or higher power: auto-seated, no decision owed
		}
		if slices.Contains(pd.SignatoryPlayerIDs, id) || slices.Contains(pd.DeclinedPlayerIDs, id) {
			continue // already joined or declined
		}
		pending = append(pending, id)
	}
	return pending, nil
}

// ── Call Roll ─────────────────────────────────────────────────────────────────

// pdCallRollHandler handles POST /api/plans/:planId/call-roll.
//
// The signatory closes the council meeting and triggers the dice roll.
// Only the current signatory may call the roll.
func pdCallRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "call-roll is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.ProposeDecree
		if pd == nil || pd.SignatoryID == nil || *pd.SignatoryID != player.ID {
			respondErr(w, http.StatusForbidden, "only the current signatory may call the roll")
			return
		}

		// The debate must be open before it can be closed: the preparer finalizes
		// the decree's text and opens discussion (start-debate) first.
		if !pd.DebateStarted {
			respondErr(w, http.StatusConflict, "the preparer must open the debate before the roll can be called")
			return
		}

		// The council meeting must conclude first: every eligible player has to
		// join or decline before the signatory may close discussion and roll.
		if pending, perr := pdPendingDeciders(ctx, deps.Q, plan, pd); perr != nil {
			respondInternalErr(w, r, "could not check council decisions", perr)
			return
		} else if len(pending) > 0 {
			respondErr(w, http.StatusConflict,
				"every eligible player must join or decline before the roll can be called")
			return
		}

		// Verify there's no existing roll for this plan.
		existingRoll, rollErr := deps.Q.GetDiceRollByPlanID(ctx, &plan.ID)
		if rollErr == nil && existingRoll.ID != 0 {
			respondErr(w, http.StatusConflict, "a roll has already been created for this plan")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}

		difficulty, err := pdHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}

		// The preparer is the actor; the roll uses preparer's dice.
		roll, err := createPlanRoll(ctx, deps.Q, deps.Manager, &game, plan, difficulty, plan.PreparerID)
		if err != nil {
			respondInternalErr(w, r, "could not create dice roll", err)
			return
		}

		pdLog(
			ctx,
			deps,
			plan,
			model.SeverityDefault,
			fmt.Sprintf("%s declared the debate over and calls for the dice roll.",
				playerDisplayName(ctx, deps.Q, player.ID)),
		)

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID,
			"roll":    roll,
		})
	}
}

// ── Amend Decree (mar) ────────────────────────────────────────────────────────

// pdAmendDecreeHandler handles POST /api/plans/:planId/amend-decree.
//
// On a marred decree, the non-preparer council members rewrite the law body in
// turn, lowest power first (the order computed at enact). Each amender submits
// the full revised body, which replaces the law's text; the next amender works
// from that output. Only the current NextAmender() may submit.
//
// Request body: {"text": "the revised full law body"}
func pdAmendDecreeHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "amend-decree is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if !pd.OutcomeApplied() {
			respondErr(w, http.StatusConflict, "the decree has not been resolved yet")
			return
		}
		next := pd.NextAmender()
		if next == 0 {
			respondErr(w, http.StatusConflict, "the council has finished amending the law")
			return
		}
		if player.ID != next {
			respondErr(w, http.StatusConflict, "it is not your turn to amend the law")
			return
		}

		var body struct {
			Text string `json:"text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			respondErr(w, http.StatusBadRequest, "text (the revised law body) is required")
			return
		}
		text, ok := textField(w, "text", body.Text, maxLongTextLen)
		if !ok {
			return
		}
		body.Text = text

		// The law isn't enacted yet — amend the working body in resolution_data.
		// It becomes the law row's text at enactment (set-addendum).
		pd.LawText = body.Text
		pd.AmendedBy = append(pd.AmendedBy, player.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save amendment", err)
			return
		}

		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s amended the decree's text.", playerDisplayName(ctx, deps.Q, player.ID)))
		// No law row yet; non-actors refetch the plan to see the revised body.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
			PlanID: plan.ID,
		})

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"amended":   player.ID,
			"next":      pd.NextAmender(),
			"remaining": len(pd.AmendmentOrder) - len(pd.AmendedBy),
		})
	}
}

// ── Skip Amend (mar) ──────────────────────────────────────────────────────────

// pdSkipAmendHandler handles POST /api/plans/:planId/skip-amend.
//
// The rules let each council member amend the marred law "at will" — i.e. they
// may decline. This advances the amendment chain past the current amender
// WITHOUT changing the law's text, so a member content with the current wording
// can pass. The table still pauses on each member in turn (their explicit pass
// is required); only the current NextAmender() may skip.
func pdSkipAmendHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "skip-amend is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if !pd.OutcomeApplied() {
			respondErr(w, http.StatusConflict, "the decree has not been resolved yet")
			return
		}
		next := pd.NextAmender()
		if next == 0 {
			respondErr(w, http.StatusConflict, "the council has finished amending the law")
			return
		}
		if player.ID != next {
			respondErr(w, http.StatusConflict, "it is not your turn to amend the law")
			return
		}

		pd.AmendedBy = append(pd.AmendedBy, player.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save skip", err)
			return
		}

		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s left the decree's text unchanged.", playerDisplayName(ctx, deps.Q, player.ID)))

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"skipped":   player.ID,
			"next":      pd.NextAmender(),
			"remaining": len(pd.AmendmentOrder) - len(pd.AmendedBy),
		})
	}
}

// ── Set Addendum ──────────────────────────────────────────────────────────────

// pdSetAddendumHandler handles POST /api/plans/:planId/set-addendum.
//
// The signatory records their rider — an "and"/"but" connector plus optional
// free text. This is a required step (AddendumPlaced), confirmed even with blank
// text, and it comes immediately BEFORE enactment: the preparer then enacts the
// law (enact-law), which writes the law row carrying this addendum. Keeping the
// addendum a distinct, signatory-only step means the preparer sees the final law
// text (body + amendments + addendum) when authoring the resource. Only valid
// once the decree is resolved (make-choice) and, on a mar, the council has
// finished amending.
//
// Request body: {"connector": "and"|"but", "addendum": "free text"}
func pdSetAddendumHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "set-addendum is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()

		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		// The signatory records the addendum. When the signatory IS the preparer, a
		// Make Demands perform_steps winner stands in for them (and the preparer is
		// locked out) — actsForPreparer is the single source of truth. When the
		// signatory is a higher-power third party (monarch / highest-power member),
		// only they may sign: perform_steps replaces the preparer's role, never a
		// third party's.
		sigIsPreparer := pd.SignatoryID != nil && *pd.SignatoryID == plan.PreparerID
		authorized := pd.SignatoryID != nil && *pd.SignatoryID == player.ID
		if sigIsPreparer {
			authorized = actsForPreparer(ctx, deps.Q, plan, player.ID)
		}
		if !authorized {
			respondErr(
				w,
				http.StatusForbidden,
				"only the current signatory (or, when they are the preparer, a demand's perform-steps winner) may set the addendum",
			)
			return
		}
		if !pd.OutcomeApplied() {
			respondErr(w, http.StatusConflict, "the decree has not been resolved yet")
			return
		}
		if pd.AddendumPlaced {
			respondErr(w, http.StatusConflict, "the addendum has already been placed")
			return
		}
		if next := pd.NextAmender(); next != 0 {
			respondErr(w, http.StatusConflict, "the council must finish amending before the addendum is placed")
			return
		}

		var body struct {
			Connector string `json:"connector"`
			Addendum  string `json:"addendum"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		addendum, ok := textField(w, "addendum", body.Addendum, maxLongTextLen)
		if !ok {
			return
		}
		body.Addendum = addendum
		// A connector is only required when there's addendum text to attach.
		if strings.TrimSpace(body.Addendum) != "" && body.Connector != "and" && body.Connector != "but" {
			respondErr(w, http.StatusBadRequest, "connector must be 'and' or 'but' when an addendum is provided")
			return
		}

		pd.Addendum = strings.TrimSpace(body.Addendum)
		pd.AddendumConnector = body.Connector
		pd.AddendumPlaced = true
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save addendum", err)
			return
		}

		signatoryName := playerDisplayName(ctx, deps.Q, player.ID)
		pdLog(ctx, deps, plan, model.SeverityDefault,
			fmt.Sprintf("%s placed the signatory's addendum to the end of the law.", signatoryName))

		// The law is not enacted here — the preparer enacts it (enact-law). Nudge
		// non-acting clients to refetch the updated sub-phase.
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
			PlanID: plan.ID,
		})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)

		respond(w, http.StatusOK, map[string]any{
			"plan_id":   plan.ID,
			"addendum":  pd.Addendum,
			"connector": pd.AddendumConnector,
		})
	}
}

// ── Enact Law ───────────────────────────────────────────────────────────────

// pdEnactLawHandler handles POST /api/plans/:planId/enact-law.
//
// The preparer enacts the passed decree — the plan's terminal action. It writes
// the law row (carrying the body, any amendments, and the signatory's addendum)
// and, on a make, creates the resource asset NAMED IN THIS SAME CALL (the rules
// grant the proposer "what you gain"; authoring it here, with the final law in
// view, keeps creation and naming a single transaction — no placeholder). The
// plan then auto-resolves (AutoCompleteAfterChoice), so there is no separate
// Complete step.
//
// Request body (make only): {"resource_name": "the resource's name"}
func pdEnactLawHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if plan.PlanType != model.PlanProposeDecree {
			respondErr(w, http.StatusBadRequest, "enact-law is only for Propose Decree")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "plan is not in resolving status")
			return
		}

		ctx := r.Context()
		// Writing the law + resource is the preparer's terminal make/mar resolution
		// step, so a Make Demands perform_steps winner may drive it in their stead
		// (locking the preparer out). The resource recipient is governed separately
		// by keep_assets (pdCreateLawAsset → AssetRecipientForPlan), so relaxing the
		// gate does not reroute the spoils.
		if !requireResolutionActor(w, ctx, deps.Q, plan, player.ID) {
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		pd := resData.EnsureProposeDecree()
		if !pd.OutcomeApplied() {
			respondErr(w, http.StatusConflict, "the decree has not been resolved yet")
			return
		}
		if next := pd.NextAmender(); next != 0 {
			respondErr(w, http.StatusConflict, "the council must finish amending before the law is enacted")
			return
		}
		if !pd.AddendumPlaced {
			respondErr(w, http.StatusConflict, "the signatory must place the addendum before the law is enacted")
			return
		}
		if pd.LawID != nil {
			respondErr(w, http.StatusConflict, "the law has already been enacted")
			return
		}

		var body struct {
			ResourceName       string   `json:"resource_name"`
			ResourceMarginalia []string `json:"resource_marginalia"`
		}
		// A body is optional on a mar (no asset); required on a make.
		_ = json.NewDecoder(r.Body).Decode(&body)
		resourceName, ok := textField(w, "resource_name", body.ResourceName, maxAssetNameLen)
		if !ok {
			return
		}
		var resourceMarg string
		if pd.Outcome == makeOutcome {
			if resourceName == "" {
				respondErr(w, http.StatusBadRequest, "resource_name is required to enact a made decree")
				return
			}
			var margErr error
			resourceMarg, margErr = requireOneMarginalia(body.ResourceMarginalia)
			if margErr != nil {
				respondErr(w, http.StatusBadRequest, margErr.Error())
				return
			}
		}

		if err := pdEnactLaw(ctx, deps, plan, &resData, resourceName, resourceMarg); err != nil {
			respondInternalErr(w, r, "could not enact the law", err)
			return
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save enactment", err)
			return
		}

		// The decree is fully written: auto-resolve (no separate Complete click).
		resolved, err := maybeAutoComplete(ctx, deps.Q, deps.Manager, pdHandler{}, plan, &resData,
			planResultString(ctx, deps.Q, plan))
		if err != nil {
			respondInternalErr(w, r, "could not complete plan", err)
			return
		}
		if !resolved {
			broadcastEvent(deps.Manager, plan.GameID, model.EventPlanChoiceApplied, model.PlanChoiceAppliedPayload{
				PlanID: plan.ID,
			})
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id":  plan.ID,
			"law_id":   pd.LawID,
			"resolved": resolved,
		})
	}
}
