package handler

// handler/wars.go — read-side endpoints exposing war state to the UI.
//
// The Make War flow stores its state across several tables (wars,
// war_participants, war_battle_costs, war_peace_proposals, war_peace_votes,
// war_surrender_claims) plus computed derivations (outstanding cost-of-battle
// for the current row, ordered by reverse power). Mutations broadcast WS
// events but the panel needs an authoritative snapshot, hence these GETs.
//
//	GET /api/plans/:planId/war-state   — one war's state (for MakeWarPanel)
//	GET /api/tables/:id/wars           — every active war in the game (for the
//	                                     turn-indicator gate)

import (
	"context"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	dbgen "uneasy/db/gen"
	"uneasy/model"
)

type warParticipantInfo struct {
	PlayerID             int64  `json:"player_id"`
	Side                 int16  `json:"side"`
	JoinedAtRow          int16  `json:"joined_at_row"`
	SurrenderedAtRow     *int16 `json:"surrendered_at_row"`
	EntryPaymentComplete bool   `json:"entry_payment_complete"`
}

type warBattleCostInfo struct {
	ID          int64  `json:"id"`
	RowNumber   int16  `json:"row_number"`
	PayerID     int64  `json:"payer_id"`
	OpponentID  int64  `json:"opponent_id"`
	Choice      string `json:"choice"`
	AssetID1    *int64 `json:"asset_id_1"`
	AssetID2    *int64 `json:"asset_id_2"`
	Surrendered bool   `json:"surrendered"`
	IsEntry     bool   `json:"is_entry"`
}

type warOutstandingCost struct {
	PayerID    int64 `json:"payer_id"`
	OpponentID int64 `json:"opponent_id"`
}

type peaceVoteInfo struct {
	PlayerID int64 `json:"player_id"`
	Accepted bool  `json:"accepted"`
}

type peaceProposalInfo struct {
	ID         int64           `json:"id"`
	ProposerID int64           `json:"proposer_id"`
	Terms      string          `json:"terms"`
	Status     string          `json:"status"`
	Votes      []peaceVoteInfo `json:"votes"`
	Awaiting   []int64         `json:"awaiting"`
}

type surrenderClaimInfo struct {
	ID            int64 `json:"id"`
	SurrenderedID int64 `json:"surrendered_id"`
	ClaimantID    int64 `json:"claimant_id"`
}

// WarStateResponse is the JSON shape returned by GET /plans/:id/war-state.
// All sub-fields are derived from the war's row in `wars` plus the related
// tables; nothing here lives in plan.resolution_data.
type WarStateResponse struct {
	WarID            int64                `json:"war_id"`
	OriginPlanID     int64                `json:"origin_plan_id"`
	Status           string               `json:"status"`
	StartedAtRow     int16                `json:"started_at_row"`
	EndedAtRow       *int16               `json:"ended_at_row"`
	EndReason        *string              `json:"end_reason"`
	CurrentRow       int16                `json:"current_row"`
	Participants     []warParticipantInfo `json:"participants"`
	BattleCosts      []warBattleCostInfo  `json:"battle_costs"`
	OutstandingCosts []warOutstandingCost `json:"outstanding_costs"`
	OpenProposal     *peaceProposalInfo   `json:"open_proposal"`
	OpenClaims       []surrenderClaimInfo `json:"open_claims"`
}

// GetWarState handles GET /api/plans/:planId/war-state. Any player at the
// table may read it (war state is public to the table, like the plan itself).
func GetWarState(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, _, ok := requirePlanAccess(w, r, q)
		if !ok {
			return
		}
		if !requirePlanType(w, plan, model.PlanMakeWar) {
			return
		}
		ctx := r.Context()
		war, err := q.GetWarByOriginPlan(ctx, plan.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				respondErr(w, http.StatusNotFound, "no war exists yet for this plan")
			} else {
				respondErr(w, http.StatusInternalServerError, "could not load war")
			}
			return
		}
		resp, err := buildWarState(ctx, q, war)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not build war state")
			return
		}
		respond(w, http.StatusOK, resp)
	}
}

// ListWars handles GET /api/tables/:id/wars. Returns every active war in the
// game (so the turn indicator can flag rows blocked on outstanding costs and
// surface a multi-war side panel later if desired).
func ListWars(q *dbgen.Queries) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID, _, ok := parseGamePlayer(w, r)
		if !ok {
			return
		}
		ctx := r.Context()
		wars, err := q.ListActiveWarsByGame(ctx, gameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load wars")
			return
		}
		out := make([]WarStateResponse, 0, len(wars))
		for _, war := range wars {
			ws, err := buildWarState(ctx, q, war)
			if err != nil {
				respondErr(w, http.StatusInternalServerError, "could not build war state")
				return
			}
			out = append(out, ws)
		}
		respond(w, http.StatusOK, map[string]any{"wars": out})
	}
}

// buildWarState computes the per-war snapshot used by both endpoints. Includes
// outstanding cost-of-battle pairs for the *current* row (in reverse-power
// order — first entry is whose turn it is to pay), plus any open peace
// proposal with a tally of accept-votes and the list of participants the
// proposal is still waiting on.
func buildWarState(
	ctx context.Context,
	q *dbgen.Queries,
	war dbgen.War,
) (WarStateResponse, error) {
	game, err := q.GetGameByID(ctx, war.GameID)
	if err != nil {
		return WarStateResponse{}, err
	}
	parts, err := q.ListWarParticipants(ctx, war.ID)
	if err != nil {
		return WarStateResponse{}, err
	}
	pinfo := make([]warParticipantInfo, 0, len(parts))
	for _, p := range parts {
		pinfo = append(pinfo, warParticipantInfo{
			PlayerID:             p.PlayerID,
			Side:                 p.Side,
			JoinedAtRow:          p.JoinedAtRow,
			SurrenderedAtRow:     p.SurrenderedAtRow,
			EntryPaymentComplete: p.EntryPaymentComplete,
		})
	}

	costs, err := q.ListBattleCostsForRow(ctx, dbgen.ListBattleCostsForRowParams{
		WarID: war.ID, RowNumber: game.CurrentRow,
	})
	if err != nil {
		return WarStateResponse{}, err
	}
	binfo := make([]warBattleCostInfo, 0, len(costs))
	for _, c := range costs {
		binfo = append(binfo, warBattleCostInfo{
			ID:          c.ID,
			RowNumber:   c.RowNumber,
			PayerID:     c.PayerID,
			OpponentID:  c.OpponentID,
			Choice:      c.Choice,
			AssetID1:    c.AssetID1,
			AssetID2:    c.AssetID2,
			Surrendered: c.Surrendered,
			IsEntry:     c.IsEntry,
		})
	}

	snap, err := mwSnapshotWar(ctx, q, war)
	if err != nil {
		return WarStateResponse{}, err
	}
	ranks, err := mwPowerRanks(ctx, q, war.GameID)
	if err != nil {
		return WarStateResponse{}, err
	}
	missing, err := mwOutstandingCostsForWar(ctx, q, snap, ranks, game.CurrentRow)
	if err != nil {
		return WarStateResponse{}, err
	}
	outCosts := make([]warOutstandingCost, 0, len(missing))
	for _, k := range missing {
		outCosts = append(outCosts, warOutstandingCost{PayerID: k.PayerID, OpponentID: k.OpponentID})
	}

	var openProposal *peaceProposalInfo
	prop, err := q.GetOpenPeaceProposal(ctx, war.ID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// No open proposal, which is a common state. Not an error.
		openProposal = nil
	case err != nil:
		return WarStateResponse{}, err
	default:
		votes, errVotes := q.ListPeaceVotes(ctx, prop.ID)
		if errVotes != nil {
			return WarStateResponse{}, errVotes
		}
		voteInfos := make([]peaceVoteInfo, 0, len(votes))
		acceptedSet := map[int64]bool{}
		for _, v := range votes {
			voteInfos = append(voteInfos, peaceVoteInfo{PlayerID: v.PlayerID, Accepted: v.Accepted})
			if v.Accepted {
				acceptedSet[v.PlayerID] = true
			}
		}
		// "Awaiting" = active full participants who haven't voted accept yet.
		// Surrendered participants don't vote.
		awaiting := []int64{}
		for _, p := range parts {
			if !p.EntryPaymentComplete || p.SurrenderedAtRow != nil {
				continue
			}
			if !acceptedSet[p.PlayerID] {
				awaiting = append(awaiting, p.PlayerID)
			}
		}
		openProposal = &peaceProposalInfo{
			ID:         prop.ID,
			ProposerID: prop.ProposerID,
			Terms:      prop.Terms,
			Status:     prop.Status,
			Votes:      voteInfos,
			Awaiting:   awaiting,
		}
	}

	openClaims, err := q.ListOpenSurrenderClaimsByWar(ctx, war.ID)
	if err != nil {
		return WarStateResponse{}, err
	}
	claimInfos := make([]surrenderClaimInfo, 0, len(openClaims))
	for _, c := range openClaims {
		claimInfos = append(claimInfos, surrenderClaimInfo{
			ID: c.ID, SurrenderedID: c.SurrenderedID, ClaimantID: c.ClaimantID,
		})
	}

	return WarStateResponse{
		WarID:            war.ID,
		OriginPlanID:     war.OriginPlanID,
		Status:           war.Status,
		StartedAtRow:     war.StartedAtRow,
		EndedAtRow:       war.EndedAtRow,
		EndReason:        war.EndReason,
		CurrentRow:       game.CurrentRow,
		Participants:     pinfo,
		BattleCosts:      binfo,
		OutstandingCosts: outCosts,
		OpenProposal:     openProposal,
		OpenClaims:       claimInfos,
	}, nil
}
