package handler

// handler/plan_host_festivity.go — Host Festivity plan handler (Phase 3d).
//
// Host Festivity (esteem, delay 6). The host throws a social event; any
// player may join as a guest, then each guest independently rolls (or opts
// out) against the host's esteem status to pick a make or mar effect. After
// all guests have acted, the host selects one make option for each guest
// who rolled a mar or opted out (including themselves). Guests who rolled
// a make hold an IOU that lets them force the host to take a mar option
// at any time before the event concludes.
//
// Phases (stored in ResolutionData.FestivityPhase):
//
//	"socializing"   — guests joining, rolling, opting out, and choosing.
//	"host_choosing" — all guests acted; host picks makes for mar/opt-out.
//	"done"          — final host choice submitted; event over.
//
// Extra routes:
//
//	POST /api/plans/:planId/join-festivity    — add caller to guest list.
//	POST /api/plans/:planId/guest-roll        — {action:"roll"|"opt_out"}.
//	POST /api/plans/:planId/guest-choice      — {choice, ...option params}.
//	POST /api/plans/:planId/insist-host-mar   — {mar_option}, consumes IOU.
//	POST /api/plans/:planId/host-choice       — {target_player_id, choice}.
//	POST /api/plans/:planId/challenge-duel    — {target_player_id}.
//	POST /api/plans/:planId/respond-challenge — {accept:bool}, target only.
//
// Center-of-table: assets placed in the play area (newly-introduced peers
// from `introduce_peer` make, and peers shoved there by `disagreement` mar)
// keep their current owner — leverage is not applied — but their IDs are
// recorded in state.CenteredAssetIDs so the UI can list them and other
// guests can steal them via `take_center_peer`.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	dbgen "uneasy/db/gen"
	gamepkg "uneasy/game"
	"uneasy/model"
)

func init() {
	RegisterPlan(model.PlanHostFestivity, hfHandler{})
}

type hfHandler struct{}

func (hfHandler) Metadata() PlanMetadata {
	return PlanMetadata{Category: model.CategoryEsteem, Delay: 6}
}

func (hfHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (*int16, string) {
	if v.Notes == "" {
		return nil, "host_festivity requires preparation_notes (event description)"
	}
	return nil, ""
}

func (hfHandler) ComputeDifficulty(
	ctx context.Context,
	q *dbgen.Queries,
	plan *dbgen.Plan,
	_ *ResolutionData,
) (int16, error) {
	rank, err := playerRankInCategory(ctx, q, plan.GameID, plan.PreparerID, model.CategoryEsteem)
	if err != nil {
		return 0, fmt.Errorf("host esteem rank: %w", err)
	}
	return gamepkg.HostFestivityDifficulty(rank), nil
}

// OnResolve sets the phase to socializing. No plan-level dice roll; each
// guest creates their own via guest-roll.
func (hfHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	state := resData.EnsureFestivity()
	state.Phase = gamepkg.FestivityPhaseSocializing
	// Host is always a guest implicitly.
	if !state.IsGuest(plan.PreparerID) {
		state.Guests = append(state.Guests, plan.PreparerID)
	}
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("save festivity setup: %w", err)
	}
	return nil, nil
}

// ApplyChoice is unused; all mechanical effects flow through extra routes.
func (hfHandler) ApplyChoice(
	_ context.Context,
	_ *PlanDeps,
	_ *dbgen.Plan,
	_ *ResolutionData,
	_ []string,
	_ string,
) error {
	return nil
}

func (hfHandler) CanComplete(_ *dbgen.Plan, resData *ResolutionData) error {
	if resData.EnsureFestivity().Phase != gamepkg.FestivityPhaseDone {
		return errors.New("festivity is not done: all guests must act and host must finish their choices")
	}
	return nil
}

func (hfHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"join-festivity":    hfJoinHandler(deps),
		"guest-roll":        hfGuestRollHandler(deps),
		"guest-choice":      hfGuestChoiceHandler(deps),
		"insist-host-mar":   hfInsistHostMarHandler(deps),
		"host-choice":       hfHostChoiceHandler(deps),
		"challenge-duel":    hfChallengeDuelHandler(deps),
		"respond-challenge": hfRespondChallengeHandler(deps),
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

// hfBlockOnPendingChallenge writes a 409 and returns true if a challenge is
// awaiting response; all festivity game actions (but not chat) pause until the
// target accepts or declines.
func hfBlockOnPendingChallenge(w http.ResponseWriter, state *gamepkg.FestivityResolutionData) bool {
	if state.PendingChallenge == nil {
		return false
	}
	respondErr(w, http.StatusConflict, "a duel challenge is awaiting the target's response")
	return true
}

func hfBroadcastPhase(deps *PlanDeps, plan *dbgen.Plan, phase string) {
	broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityPhaseChanged, model.FestivityPhaseChangedPayload{
		PlanID: plan.ID, Phase: phase,
	})
}

// hfMaybeAdvanceToHostChoosing checks if all guests have acted and, if so,
// transitions state.Phase to host_choosing (if not already past it).
// Caller must persist state afterwards.
func hfMaybeAdvanceToHostChoosing(state *gamepkg.FestivityResolutionData) bool {
	if state.Phase != gamepkg.FestivityPhaseSocializing {
		return false
	}
	if !state.AllGuestsResolved() {
		return false
	}
	state.Phase = gamepkg.FestivityPhaseHostChoosing
	return true
}

// hfFinalizeIfDone: after the host submits the final pending host-choice AND
// all IOU insists are consumed (IOUs can only be cashed before done), mark
// the plan done and set result="make".
func hfFinalizeIfDone(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	state *gamepkg.FestivityResolutionData,
) error {
	if state.Phase != gamepkg.FestivityPhaseHostChoosing {
		return nil
	}
	if len(state.PendingHostChoices()) > 0 {
		return nil
	}
	state.Phase = gamepkg.FestivityPhaseDone
	res := makeOutcome
	if err := deps.Q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: &res}); err != nil {
		return err
	}
	hfLog(ctx, deps, plan, model.SeverityImportant, "The festivity drew to a close.")
	return nil
}

// ── join-festivity ───────────────────────────────────────────────────────────

func hfJoinHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if state.Phase != gamepkg.FestivityPhaseSocializing {
			respondErr(w, http.StatusConflict, "festivity is no longer accepting new guests")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		if state.IsGuest(player.ID) {
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "already_joined": true})
			return
		}
		state.Guests = append(state.Guests, player.ID)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save guest list", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityGuestJoined, model.FestivityGuestJoinedPayload{
			PlanID: plan.ID, PlayerID: player.ID,
		})
		hfLog(ctx, deps, plan, model.SeverityMinor, fmt.Sprintf("%s joined the festivity.",
			playerDisplayName(ctx, deps.Q, player.ID)))
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "player_id": player.ID})
	}
}

// ── guest-roll ───────────────────────────────────────────────────────────────

//nolint:funlen,gocognit // guest roll lifecycle (join + roll + advance + finalize)
func hfGuestRollHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			Action string `json:"action"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if body.Action != "roll" && body.Action != "opt_out" {
			respondErr(w, http.StatusBadRequest, "action must be 'roll' or 'opt_out'")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if state.Phase != gamepkg.FestivityPhaseSocializing {
			respondErr(w, http.StatusConflict, "socializing phase has ended")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		if !state.IsGuest(player.ID) {
			respondErr(w, http.StatusForbidden, "you must join the festivity first")
			return
		}
		if state.PendingDuelPlanID != nil {
			// Check if that duel is still unresolved.
			if dp, err := deps.Q.GetPlanByID(
				ctx,
				*state.PendingDuelPlanID,
			); err == nil &&
				dp.Status != model.PlanResolved {
				respondErr(w, http.StatusConflict, "a challenged duel is in progress; resolve it first")
				return
			}
			state.PendingDuelPlanID = nil
		}
		key := strconv.FormatInt(player.ID, 10)
		if _, acted := state.Outcomes[key]; acted {
			respondErr(w, http.StatusConflict, "you have already rolled or opted out")
			return
		}

		if body.Action == "opt_out" {
			if state.Outcomes == nil {
				state.Outcomes = map[string]string{}
			}
			state.Outcomes[key] = gamepkg.FestivityOutcomeOptOut
			if hfMaybeAdvanceToHostChoosing(state) {
				defer hfBroadcastPhase(deps, plan, string(state.Phase))
			}
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondInternalErr(w, r, "could not save opt-out", err)
				return
			}
			broadcastEvent(
				deps.Manager,
				plan.GameID,
				model.EventFestivityGuestRolled,
				model.FestivityGuestRolledPayload{
					PlanID: plan.ID, PlayerID: player.ID, Action: "opt_out",
				},
			)
			broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "action": "opt_out"})
			return
		}

		// action == "roll": create a dice roll for the guest.
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load game", err)
			return
		}
		difficulty, err := hfHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondInternalErr(w, r, "could not compute difficulty", err)
			return
		}
		roll, err := deps.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
			GameID:     plan.GameID,
			PlanID:     &plan.ID,
			RowNumber:  &game.CurrentRow,
			ActorID:    player.ID,
			Difficulty: difficulty,
			Stage:      "decide_vote",
		})
		if err != nil {
			respondInternalErr(w, r, "could not create roll", err)
			return
		}
		for range 2 {
			if _, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
				RollID:           roll.ID,
				PlayerID:         player.ID,
				IsInterference:   false,
				LeveragedAssetID: nil,
			}); err != nil {
				respondInternalErr(w, r, "could not create base dice", err)
				return
			}
		}
		if err := seedRollParticipants(ctx, deps.Q, plan.GameID, roll.ID, player.ID); err != nil {
			respondInternalErr(w, r, "could not seed participants", err)
			return
		}
		if state.GuestRollIDs == nil {
			state.GuestRollIDs = map[string]int64{}
		}
		state.GuestRollIDs[key] = roll.ID
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save guest roll id", err)
			return
		}
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventRollCreated, model.RollCreatedPayload{Roll: roll})
			h.BroadcastEvent(model.EventFestivityGuestRolled, model.FestivityGuestRolledPayload{
				PlanID: plan.ID, PlayerID: player.ID, Action: "roll", RollID: roll.ID,
			})
		}
		respond(w, http.StatusCreated, map[string]any{"plan_id": plan.ID, "roll": roll})
	}
}

// ── guest-choice ─────────────────────────────────────────────────────────────

// Body:
//
//	{"choice": "<key>",
//	 "rumor_text": "...",      // spread_rumor, rumor_about_you
//	 "peer_name": "...",        // introduce_peer
//	 "asset_id": N}             // take_center_peer, disagreement
//
// challenge_duel goes through /challenge-duel, not here.
//
//nolint:gocognit // guest make/mar dispatch with phase-advance side effects
func hfGuestChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			Choice       string `json:"choice"`
			RumorText    string `json:"rumor_text"`
			PeerName     string `json:"peer_name"`
			AssetID      int64  `json:"asset_id"`
			MarginaliaID int64  `json:"marginalia_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		key := strconv.FormatInt(player.ID, 10)
		if !state.IsGuest(player.ID) {
			respondErr(w, http.StatusForbidden, "you are not a guest")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		rollID, ok := state.GuestRollIDs[key]
		if !ok {
			respondErr(w, http.StatusConflict, "call guest-roll with action=roll first")
			return
		}
		if _, already := state.Outcomes[key]; already {
			respondErr(w, http.StatusConflict, "you have already submitted your choice")
			return
		}
		roll, err := deps.Q.GetDiceRollByID(ctx, rollID)
		if err != nil {
			respondInternalErr(w, r, "could not load roll", err)
			return
		}
		if roll.Outcome == nil {
			respondErr(w, http.StatusConflict, "your roll has not resolved yet")
			return
		}
		outcome := *roll.Outcome

		isMake := outcome == makeOutcome
		if isMake {
			if !gamepkg.IsValidFestivityMakeOption(body.Choice) {
				respondErr(w, http.StatusBadRequest, "invalid make option for festivity")
				return
			}
			if body.Choice == gamepkg.FestivityMakeChallengeDuel {
				respondErr(w, http.StatusBadRequest, "use /challenge-duel to trigger a duel")
				return
			}
		} else if !gamepkg.IsValidFestivityMarOption(body.Choice) {
			respondErr(w, http.StatusBadRequest, "invalid mar option for festivity")
			return
		}

		// Apply effect.
		if err := hfApplyOption(
			ctx,
			deps,
			plan,
			state,
			player.ID,
			body.Choice,
			body.RumorText,
			body.PeerName,
			body.AssetID,
			body.MarginaliaID,
			isMake,
		); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}

		if state.Outcomes == nil {
			state.Outcomes = map[string]string{}
		}
		if isMake {
			state.Outcomes[key] = gamepkg.FestivityOutcomeMake
			if state.GuestMakes == nil {
				state.GuestMakes = map[string]string{}
			}
			state.GuestMakes[key] = body.Choice
			state.GuestIOUs = append(state.GuestIOUs, player.ID)
		} else {
			state.Outcomes[key] = gamepkg.FestivityOutcomeMar
			if state.GuestMars == nil {
				state.GuestMars = map[string]string{}
			}
			state.GuestMars[key] = body.Choice
		}

		phaseChanged := hfMaybeAdvanceToHostChoosing(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save guest choice", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityGuestChose, model.FestivityGuestChosePayload{
			PlanID: plan.ID, PlayerID: player.ID,
			Outcome: state.Outcomes[key], Choice: body.Choice,
		})
		if phaseChanged {
			hfBroadcastPhase(deps, plan, string(state.Phase))
		}
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "outcome": state.Outcomes[key], "choice": body.Choice,
		})
	}
}

// festivityOptionContext bundles the parameters threaded through every
// festivity option applier.

// ResolvingWaitees returns the narrower RowState for a resolving Host Festivity
// plan. During 'socializing': an open challenge takes precedence (block on the
// target), otherwise block on the next guest in esteem order. 'host_choosing'
// and 'done' return false and ride the generic PlanResolving case — the host is
// the preparer (= focus player at resolve time) so the existing label is already
// accurate.
func (hfHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	state := gamepkg.LoadFestivityData(plan.ResolutionData)
	if state.Phase != gamepkg.FestivityPhaseSocializing {
		return model.RowState{}, false
	}
	if state.PendingChallenge != nil {
		actor := state.PendingChallenge.TargetID
		return model.RowState{
			Kind:            model.RowStateAwaitFestivityChallengeResponse,
			ActingPlayerIDs: []int64{actor},
		}, true
	}
	rankings, err := q.ListRankingsByGame(ctx, plan.GameID)
	if err != nil {
		return model.RowState{}, false
	}
	rankFor := func(playerID int64) int16 {
		for i := range rankings {
			r := &rankings[i]
			if r.Category != model.CategoryEsteem || r.PlayerID == nil {
				continue
			}
			if *r.PlayerID == playerID {
				return r.Rank
			}
		}
		// Low sentinel: an unranked guest sorts LAST in NextSocializingTurn's
		// descending-by-rank order (a high value would wrongly put them first).
		return 0
	}
	next := state.NextSocializingTurn(plan.PreparerID, rankFor)
	if next == 0 {
		return model.RowState{}, false
	}
	return model.RowState{Kind: model.RowStateAwaitFestivityGuestTurn, ActingPlayerIDs: []int64{next}}, true
}
