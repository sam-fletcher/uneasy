package handler

// handler/plan_make_war_peace.go — Peace negotiation routes for Make War:
// /propose-peace and /vote-peace. Splitting these off of plan_make_war.go
// isolates the unanimous-vote state machine from the payment routes.

import (
	"encoding/json"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// mwProposePeaceHandler handles POST /api/plans/:planId/propose-peace.
//
// Only the active participant whose turn it is to pay cost of battle (first
// in reverse power order among outstanding payers) may propose peace, because
// the proposal is itself the caller's cost-of-battle choice for that pair of
// (payer, opponent). The proposer does not record a battle_cost row until the
// vote concludes: if accepted the war ends (clearing all costs); if rejected
// the proposer must still pay using break_asset or leverage_two.
//
// Body: {"terms": "..."}
func mwProposePeaceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			Terms string `json:"terms"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Terms == "" {
			respondErr(w, http.StatusBadRequest, "terms required")
			return
		}
		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		if war.Status != "active" {
			respondErr(w, http.StatusConflict, "war is no longer active")
			return
		}
		if _, err := deps.Q.GetOpenPeaceProposal(ctx, war.ID); err == nil {
			respondErr(w, http.StatusConflict, "an open peace proposal already exists for this war")
			return
		}

		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}
		snap, err := mwSnapshotWar(ctx, deps.Q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load war participants")
			return
		}
		ranks, err := mwPowerRanks(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load rankings")
			return
		}
		missing, err := mwOutstandingCostsForWar(ctx, deps.Q, snap, ranks, game.CurrentRow)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute outstanding costs")
			return
		}
		if len(missing) == 0 || missing[0].PayerID != player.ID {
			respondErr(w, http.StatusConflict,
				"only the active participant whose turn it is to pay cost of battle may propose peace")
			return
		}

		prop, err := deps.Q.CreatePeaceProposal(ctx, dbgen.CreatePeaceProposalParams{
			WarID:      war.ID,
			ProposerID: player.ID,
			Terms:      body.Terms,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create peace proposal")
			return
		}

		_ = deps.Q.UpsertPeaceVote(ctx, dbgen.UpsertPeaceVoteParams{
			ProposalID: prop.ID,
			PlayerID:   player.ID,
			Accepted:   true,
		})

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPeaceProposed, model.WarPeaceProposedPayload{
				WarID: war.ID, ProposalID: prop.ID,
				ProposerID: player.ID, Terms: body.Terms,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"proposal_id": prop.ID,
			"war_id":      war.ID,
		})
	}
}

// mwVotePeaceHandler handles POST /api/plans/:planId/vote-peace.
//
// Body: {"proposal_id": int64, "accepted": bool}
//
// Any active participant may vote. A single "reject" closes the proposal; the
// proposer must then pay their cost with break_asset or leverage_two. Once
// every active participant has voted accept, the war ends with reason=peace.
func mwVotePeaceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanAccess(w, r, deps.Q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		var body struct {
			ProposalID int64 `json:"proposal_id"`
			Accepted   bool  `json:"accepted"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		ctx := r.Context()
		war, ok := mwLoadWar(ctx, w, deps.Q, plan)
		if !ok {
			return
		}
		prop, err := deps.Q.GetPeaceProposal(ctx, body.ProposalID)
		if err != nil || prop.WarID != war.ID {
			respondErr(w, http.StatusNotFound, "peace proposal not found")
			return
		}
		if prop.Status != gamepkg.PeaceOpen {
			respondErr(w, http.StatusConflict, "proposal is already resolved")
			return
		}

		part, err := deps.Q.GetWarParticipant(ctx, dbgen.GetWarParticipantParams{
			WarID: war.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondErr(w, http.StatusForbidden, "you are not a participant in this war")
			return
		}
		if part.SurrenderedAtRow != nil {
			respondErr(w, http.StatusForbidden, "surrendered players cannot vote on peace")
			return
		}

		err = deps.Q.UpsertPeaceVote(ctx, dbgen.UpsertPeaceVoteParams{
			ProposalID: prop.ID,
			PlayerID:   player.ID,
			Accepted:   body.Accepted,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not record vote")
			return
		}
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarPeaceVote, model.WarPeaceVotePayload{
				WarID: war.ID, ProposalID: prop.ID,
				PlayerID: player.ID, Accepted: body.Accepted,
			})
		}

		if !body.Accepted {
			_ = deps.Q.SetPeaceProposalStatus(ctx, dbgen.SetPeaceProposalStatusParams{
				ID:     prop.ID,
				Status: gamepkg.PeaceRejected,
			})
			respond(w, http.StatusOK, map[string]any{
				"proposal_id": prop.ID,
				"status":      gamepkg.PeaceRejected,
			})
			return
		}

		active, err := deps.Q.ListActiveWarParticipants(ctx, war.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load active participants")
			return
		}
		votes, err := deps.Q.ListPeaceVotes(ctx, prop.ID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load votes")
			return
		}
		accepted := map[int64]bool{}
		for _, v := range votes {
			if v.Accepted {
				accepted[v.PlayerID] = true
			}
		}
		activeIDs := make([]int64, len(active))
		for i, a := range active {
			activeIDs[i] = a.PlayerID
		}
		if unanimous, awaiting := gamepkg.PeaceTally(activeIDs, accepted); !unanimous {
			respond(w, http.StatusOK, map[string]any{
				"proposal_id": prop.ID,
				"status":      gamepkg.PeaceOpen,
				"awaiting":    awaiting,
			})
			return
		}

		game, _ := deps.Q.GetGameByID(ctx, plan.GameID)
		_ = deps.Q.SetPeaceProposalStatus(ctx, dbgen.SetPeaceProposalStatusParams{
			ID:     prop.ID,
			Status: gamepkg.PeaceAccepted,
		})
		_ = deps.Q.EndWar(ctx, dbgen.EndWarParams{
			ID:         war.ID,
			EndReason:  new(gamepkg.WarEndPeace),
			EndedAtRow: &game.CurrentRow,
		})
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventWarEnded, model.WarEndedPayload{
				WarID: war.ID, Reason: gamepkg.WarEndPeace, RowNumber: game.CurrentRow,
			})
		}
		respond(w, http.StatusOK, map[string]any{
			"proposal_id": prop.ID,
			"status":      gamepkg.PeaceAccepted,
			"war_ended":   true,
		})
	}
}
