package handler

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

// ── Elect Champion ────────────────────────────────────────────────────────────

// pduelElectChampionHandler: POST /api/plans/:planId/elect-champion
// Body: {"asset_id": N | null}. If asset_id is null or omitted, the player is
// signalling "I'll fight myself." If present, the asset must be a peer owned
// by the caller. The initiative-holder must declare first so the other side's
// UI knows when to unlock.
//
//nolint:gocognit // champion election with eligibility + auto-advance
func pduelElectChampionHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may elect a champion")
			return
		}

		var body struct {
			AssetID *int64 `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()

		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "champions can only be elected during setup")
			return
		}
		// Champion election is optional and simultaneous: either duellist may
		// nominate a peer (or pass nil to fight in person) at any time during
		// setup, and may change their mind until staking begins. It gates
		// nothing — the phase advances on the stake-count reveal alone.

		if body.AssetID != nil {
			asset, err := deps.Q.GetAssetByID(ctx, *body.AssetID)
			if err != nil {
				respondErr(w, http.StatusNotFound, "asset not found")
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, "you do not own this asset")
				return
			}
			if asset.AssetType != model.AssetPeer {
				respondErr(w, http.StatusBadRequest, "champion must be a peer asset")
				return
			}
		}

		// No-op guard: if the duellist's choice is unchanged, don't re-save,
		// re-broadcast, or (most importantly) re-log. The client commits a single
		// final choice, but this also defends against any redundant calls.
		var current *int64
		if player.ID == plan.PreparerID {
			current = state.PreparerChampionID
		} else {
			current = state.TargetChampionID
		}
		unchanged := (current == nil && body.AssetID == nil) ||
			(current != nil && body.AssetID != nil && *current == *body.AssetID)
		if unchanged {
			respond(w, http.StatusOK, map[string]any{
				"plan_id": plan.ID, "player_id": player.ID, "asset_id": body.AssetID,
			})
			return
		}

		if player.ID == plan.PreparerID {
			state.PreparerChampionID = body.AssetID
			state.PreparerChampionDeclared = true
		} else {
			state.TargetChampionID = body.AssetID
			state.TargetChampionDeclared = true
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save champion", err)
			return
		}

		if h, ok := deps.Manager.Get(plan.GameID); ok {
			var aid int64
			if body.AssetID != nil {
				aid = *body.AssetID
			}
			h.BroadcastEvent(model.EventDuelChampionElected, model.DuelChampionElectedPayload{
				PlanID: plan.ID, PlayerID: player.ID, AssetID: aid,
			})
		}

		// Log in narrative terms: the duellist is their main character, who either
		// steps up themselves or sends another peer in their stead. Naming the MC
		// (not the out-of-game player) keeps the log in-fiction.
		duellistName := playerDisplayName(ctx, deps.Q, player.ID)
		if mc, mcErr := deps.Q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
			GameID: plan.GameID, OwnerID: player.ID,
		}); mcErr == nil {
			duellistName = mc.Name
		}
		if body.AssetID != nil {
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s elects %s as their champion.",
				duellistName, assetDisplayName(ctx, deps.Q, *body.AssetID)))
		} else {
			pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
				"%s will fight for themselves.", duellistName))
		}

		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "player_id": player.ID, "asset_id": body.AssetID,
		})
	}
}

// ── Select Stakes ─────────────────────────────────────────────────────────────

// pduelSelectStakesHandler: POST /api/plans/:planId/select-stakes
// Body: {"asset_ids": [N, ...]}
//
// A single combined commit: the duellist picks which assets to stake (the count
// is len(asset_ids), which must be 1..1+their esteem status). The server rolls
// and tucks a hidden d6 under each. A duellist's stakes — and even their count —
// stay hidden from the opponent until BOTH have committed (see GetDuelState),
// preserving the rules' simultaneous blind reveal. Once both have committed, the
// duel advances straight to the bouts.
//
//nolint:gocognit,funlen // combined count+selection commit plus the advance path
func pduelSelectStakesHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanProposeDuel)
		if !ok {
			return
		}
		if !pduelIsParticipant(plan, player.ID) {
			respondErr(w, http.StatusForbidden, "only duellists may stake assets")
			return
		}

		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureDuel()
		if state.Phase != duelPhaseSetup {
			respondErr(w, http.StatusConflict, "stakes are chosen during setup")
			return
		}

		var body struct {
			AssetIDs []int64 `json:"asset_ids"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()

		// The count is whatever the duellist selected: at least one, at most
		// 1 + their esteem status.
		rank, err := playerRankInCategory(ctx, deps.Q, plan.GameID, player.ID, model.CategoryEsteem)
		if err != nil {
			respondInternalErr(w, r, "could not load esteem rank", err)
			return
		}
		n := int16(len(body.AssetIDs))
		if n < 1 {
			respondErr(w, http.StatusBadRequest, "stake at least one asset")
			return
		}
		if n > gamepkg.MaxStakes(rank) {
			respondErr(w, http.StatusBadRequest,
				fmt.Sprintf("staking %d exceeds your maximum of %d", n, gamepkg.MaxStakes(rank)))
			return
		}

		// Reject a second commit.
		existing, err := deps.Q.ListDuelStakesByPlanPlayer(ctx, dbgen.ListDuelStakesByPlanPlayerParams{
			PlanID: plan.ID, PlayerID: player.ID,
		})
		if err != nil {
			respondInternalErr(w, r, "could not load existing stakes", err)
			return
		}
		if len(existing) > 0 {
			respondErr(w, http.StatusConflict, "you have already committed your stakes")
			return
		}

		// Validate each asset: owned, non-destroyed (already-leveraged is fine).
		for _, aid := range body.AssetIDs {
			asset, errAsset := deps.Q.GetAssetByID(ctx, aid)
			if errAsset != nil {
				respondErr(w, http.StatusNotFound, fmt.Sprintf("asset %d not found", aid))
				return
			}
			if asset.GameID != plan.GameID || asset.OwnerID != player.ID {
				respondErr(w, http.StatusForbidden, fmt.Sprintf("you do not own asset %d", aid))
				return
			}
			if asset.IsDestroyed {
				respondErr(w, http.StatusBadRequest, fmt.Sprintf("asset %d is destroyed", aid))
				return
			}
		}

		// Create stakes with a hidden d6 per asset. Collect them so the caller
		// can see their own hidden dice in the response without polling.
		createdStakes := make([]dbgen.DuelStakedAsset, 0, len(body.AssetIDs))
		for _, aid := range body.AssetIDs {
			face := int16(rand.IntN(gamepkg.DiceSides) + 1)
			stake, errStake := deps.Q.CreateDuelStake(ctx, dbgen.CreateDuelStakeParams{
				PlanID:    plan.ID,
				PlayerID:  player.ID,
				AssetID:   aid,
				HiddenDie: face,
			})
			if errStake != nil {
				respondInternalErr(w, r, "could not create stake", errStake)
				return
			}
			createdStakes = append(createdStakes, stake)
		}

		// Determine commit state from the stake rows themselves — never from a
		// count written into resolution_data, which is sent to every client and
		// would leak this side's count to the opponent before both have committed.
		allStakes, err := deps.Q.ListDuelStakesByPlan(ctx, plan.ID)
		if err != nil {
			respondInternalErr(w, r, "could not load stakes", err)
			return
		}
		var prepCount, targCount int16
		for i := range allStakes {
			if allStakes[i].PlayerID == plan.PreparerID {
				prepCount++
			} else {
				targCount++
			}
		}
		bothCommitted := prepCount > 0 && targCount > 0
		if bothCommitted {
			// Both sides are in — now it's safe to record the (about-to-be-public)
			// counts and advance straight to the bouts.
			state.PreparerStakeCount = prepCount
			state.TargetStakeCount = targCount
			state.Phase = duelPhaseBouts
			state.CurrentBout = 0
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save stakes", err)
			return
		}

		// Narrate ONLY once both sides have committed. Logging on the first commit
		// would reveal that side's count to the opponent before they choose,
		// handing the second mover a last-mover advantage — so we hold the
		// narration and reveal both counts together. (The assets themselves stay
		// secret until the duel resolves; we name only counts + the duellists.)
		if bothCommitted {
			duellistName := func(pid int64) string {
				if mc, mcErr := deps.Q.GetMainCharacterByOwner(ctx, dbgen.GetMainCharacterByOwnerParams{
					GameID: plan.GameID, OwnerID: pid,
				}); mcErr == nil {
					return mc.Name
				}
				return playerDisplayName(ctx, deps.Q, pid)
			}
			stakeWord := func(c int16) string {
				if c == 1 {
					return "stake"
				}
				return "stakes"
			}
			logCommit := func(pid int64, count int16) {
				pduelLog(ctx, deps, plan, model.SeverityDefault, fmt.Sprintf(
					"%s lays %d %s on the line, each guarding a hidden die.",
					duellistName(pid), count, stakeWord(count)))
			}
			logCommit(plan.PreparerID, prepCount)
			if plan.TargetPlayerID != nil {
				logCommit(*plan.TargetPlayerID, targCount)
			}

			// Reveal and move to the bouts. The waiting duellist needs a duel event
			// to refetch and leave the setup panel.
			broadcastEvent(deps.Manager, plan.GameID, model.EventDuelStakesSelected, model.DuelStakesSelectedPayload{
				PlanID: plan.ID,
			})
		}
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "staked": len(body.AssetIDs), "stakes": createdStakes,
		})
	}
}
