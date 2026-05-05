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
// Center-of-table (disagreement mar) is modeled pragmatically: the asset is
// leveraged and its ID recorded in CenteredAssetIDs. A true unowned-asset
// state is deferred per PHASE3_SPEC "Not in Scope".

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
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

func (hfHandler) ValidatePreparation(_ context.Context, v *ValidationContext) (int16, string) {
	if v.Notes == "" {
		return 0, "host_festivity requires preparation_notes (event description)"
	}
	return 0, ""
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
	state := resData.FestivityState()
	state.Phase = gamepkg.FestivityPhaseSocializing
	// Host is always a guest implicitly.
	if !state.IsGuest(plan.PreparerID) {
		state.Guests = append(state.Guests, plan.PreparerID)
	}
	resData.SetFestivityState(state)
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
	if resData.FestivityState().Phase != gamepkg.FestivityPhaseDone {
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
func hfBlockOnPendingChallenge(w http.ResponseWriter, state gamepkg.FestivityState) bool {
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
func hfMaybeAdvanceToHostChoosing(state *gamepkg.FestivityState) bool {
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
func hfFinalizeIfDone(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan, state *gamepkg.FestivityState) error {
	if state.Phase != gamepkg.FestivityPhaseHostChoosing {
		return nil
	}
	if len(state.PendingHostChoices()) > 0 {
		return nil
	}
	state.Phase = gamepkg.FestivityPhaseDone
	res := makeOutcome
	return deps.Q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: &res})
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
		state := resData.FestivityState()
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
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save guest list")
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityGuestJoined, model.FestivityGuestJoinedPayload{
			PlanID: plan.ID, PlayerID: player.ID,
		})
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
		state := resData.FestivityState()
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
			if hfMaybeAdvanceToHostChoosing(&state) {
				defer hfBroadcastPhase(deps, plan, state.Phase)
			}
			resData.SetFestivityState(state)
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save opt-out")
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
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "action": "opt_out"})
			return
		}

		// action == "roll": create a dice roll for the guest.
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}
		difficulty, err := hfHandler{}.ComputeDifficulty(ctx, deps.Q, plan, &resData)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not compute difficulty: "+err.Error())
			return
		}
		roll, err := deps.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
			GameID:     plan.GameID,
			PlanID:     &plan.ID,
			RowNumber:  &game.CurrentRow,
			ActorID:    player.ID,
			Difficulty: difficulty,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create roll")
			return
		}
		for range 2 {
			if _, err := deps.Q.CreateDiceRollDie(ctx, dbgen.CreateDiceRollDieParams{
				RollID:           roll.ID,
				PlayerID:         player.ID,
				IsInterference:   false,
				LeveragedAssetID: nil,
			}); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not create base dice")
				return
			}
		}
		if state.GuestRollIDs == nil {
			state.GuestRollIDs = map[string]int64{}
		}
		state.GuestRollIDs[key] = roll.ID
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save guest roll id")
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
			Choice    string `json:"choice"`
			RumorText string `json:"rumor_text"`
			PeerName  string `json:"peer_name"`
			AssetID   int64  `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.FestivityState()
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
			respondErr(w, http.StatusInternalServerError, "could not load roll")
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
			&state,
			player.ID,
			body.Choice,
			body.RumorText,
			body.PeerName,
			body.AssetID,
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

		phaseChanged := hfMaybeAdvanceToHostChoosing(&state)
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save guest choice")
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityGuestChose, model.FestivityGuestChosePayload{
			PlanID: plan.ID, PlayerID: player.ID,
			Outcome: state.Outcomes[key], Choice: body.Choice,
		})
		if phaseChanged {
			hfBroadcastPhase(deps, plan, state.Phase)
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "outcome": state.Outcomes[key], "choice": body.Choice,
		})
	}
}

// festivityOptionContext bundles the parameters threaded through every
// festivity option applier.
type festivityOptionContext struct {
	deps           *PlanDeps
	plan           *dbgen.Plan
	state          *gamepkg.FestivityState
	actingPlayerID int64
	rumorText      string
	peerName       string
	assetID        int64
	isMake         bool
}

type festivityOptionApplier func(ctx context.Context, fc *festivityOptionContext) error

// festivityOptionAppliers dispatches a make/mar choice to its mechanical
// effect. SpreadRumor and RumorAboutYou share an applier (it branches on
// fc.isMake) because the underlying rumor-creation flow is the same.
var festivityOptionAppliers = map[string]festivityOptionApplier{
	gamepkg.FestivityMakeSpreadRumor:    applyFestivityRumor,
	gamepkg.FestivityMarRumorAboutYou:   applyFestivityRumor,
	gamepkg.FestivityMakeIntroducePeer:  applyFestivityIntroducePeer,
	gamepkg.FestivityMakeTakeCenterPeer: applyFestivityTakeCenterPeer,
	gamepkg.FestivityMarDisagreement:    applyFestivityDisagreement,
	gamepkg.FestivityMarAcceptDuels:     applyFestivityAcceptDuels,
	gamepkg.FestivityMarBreakSelf:       applyFestivityBreakSelf,
}

// hfApplyOption performs the mechanical effect for a chosen make/mar option.
// It mutates state as needed (e.g. recording centered assets, accept_duels).
func hfApplyOption(
	ctx context.Context,
	deps *PlanDeps,
	plan *dbgen.Plan,
	state *gamepkg.FestivityState,
	actingPlayerID int64,
	choice, rumorText, peerName string,
	assetID int64,
	isMake bool,
) error {
	applier, ok := festivityOptionAppliers[choice]
	if !ok {
		return nil
	}
	return applier(ctx, &festivityOptionContext{
		deps:           deps,
		plan:           plan,
		state:          state,
		actingPlayerID: actingPlayerID,
		rumorText:      rumorText,
		peerName:       peerName,
		assetID:        assetID,
		isMake:         isMake,
	})
}

func applyFestivityRumor(ctx context.Context, fc *festivityOptionContext) error {
	txt := fc.rumorText
	if txt == "" {
		txt = "(untold rumor)"
	}
	var targetAssetID *int64
	if !fc.isMake {
		if mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.actingPlayerID); err == nil {
			targetAssetID = &mcID
		}
	}
	existing, _ := fc.deps.Q.ListRumors(ctx, fc.plan.GameID)
	var src *int64
	if fc.isMake {
		src = &fc.actingPlayerID
	}
	rumor, err := fc.deps.Q.CreateRumor(ctx, dbgen.CreateRumorParams{
		GameID:         fc.plan.GameID,
		Text:           txt,
		TargetAssetID:  targetAssetID,
		OriginPlanID:   &fc.plan.ID,
		SourcePlayerID: src,
		DisplayOrder:   int16(len(existing)),
	})
	if err != nil {
		return fmt.Errorf("create rumor: %w", err)
	}
	broadcastEvent(fc.deps.Manager, fc.plan.GameID, model.EventRumorCreated, model.RumorCreatedPayload{Rumor: rumor})
	return nil
}

func applyFestivityIntroducePeer(ctx context.Context, fc *festivityOptionContext) error {
	name := fc.peerName
	if name == "" {
		name = "New peer"
	}
	ownerID := fc.actingPlayerID
	if fc.actingPlayerID == fc.plan.PreparerID {
		recipient, err := gamepkg.AssetRecipientForPlan(ctx, fc.deps.Q, fc.plan)
		if err != nil {
			return fmt.Errorf("resolve asset recipient: %w", err)
		}
		ownerID = recipient
	}
	asset, err := fc.deps.Q.CreateAsset(ctx, dbgen.CreateAssetParams{
		GameID:    fc.plan.GameID,
		OwnerID:   ownerID,
		CreatorID: fc.actingPlayerID,
		AssetType: model.AssetPeer,
		Name:      name,
	})
	if err != nil {
		return fmt.Errorf("create peer: %w", err)
	}
	broadcastEvent(fc.deps.Manager, fc.plan.GameID, model.EventAssetCreated, model.AssetPayload{Asset: asset})
	return nil
}

func applyFestivityTakeCenterPeer(ctx context.Context, fc *festivityOptionContext) error {
	if fc.assetID == 0 {
		return errors.New("asset_id required")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.assetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if !slices.Contains(fc.state.CenteredAssetIDs, fc.assetID) {
		return errors.New("asset is not in the center of the table")
	}
	oldOwner := asset.OwnerID
	newOwner := fc.actingPlayerID
	if fc.actingPlayerID == fc.plan.PreparerID {
		recipient, rerr := gamepkg.AssetRecipientForPlan(ctx, fc.deps.Q, fc.plan)
		if rerr != nil {
			return fmt.Errorf("resolve asset recipient: %w", rerr)
		}
		newOwner = recipient
	}
	err = fc.deps.Q.TransferAsset(
		ctx,
		dbgen.TransferAssetParams{ID: fc.assetID, OwnerID: newOwner},
	)
	if err != nil {
		return fmt.Errorf("transfer asset: %w", err)
	}
	remaining := fc.state.CenteredAssetIDs[:0]
	for _, id := range fc.state.CenteredAssetIDs {
		if id != fc.assetID {
			remaining = append(remaining, id)
		}
	}
	fc.state.CenteredAssetIDs = append([]int64(nil), remaining...)
	updated, _ := fc.deps.Q.GetAssetByID(ctx, fc.assetID)
	broadcastEvent(fc.deps.Manager, fc.plan.GameID, model.EventAssetTaken, model.AssetTakenPayload{
		Asset: updated, OldOwnerID: oldOwner, NewOwnerID: newOwner,
	})
	return nil
}

func applyFestivityDisagreement(ctx context.Context, fc *festivityOptionContext) error {
	if fc.assetID == 0 {
		return errors.New("asset_id required for disagreement")
	}
	asset, err := fc.deps.Q.GetAssetByID(ctx, fc.assetID)
	if err != nil {
		return errors.New("asset not found")
	}
	if asset.AssetType != model.AssetPeer {
		return errors.New("disagreement target must be a peer")
	}
	_ = fc.deps.Q.SetAssetLeveraged(ctx, dbgen.SetAssetLeveragedParams{ID: fc.assetID, IsLeveraged: true})
	fc.state.CenteredAssetIDs = append(fc.state.CenteredAssetIDs, fc.assetID)
	broadcastEvent(
		fc.deps.Manager,
		fc.plan.GameID,
		model.EventAssetLeveraged,
		model.AssetIDPayload{AssetID: fc.assetID, PlayerID: asset.OwnerID},
	)
	return nil
}

func applyFestivityAcceptDuels(_ context.Context, fc *festivityOptionContext) error {
	if !slices.Contains(fc.state.AcceptDuels, fc.actingPlayerID) {
		fc.state.AcceptDuels = append(fc.state.AcceptDuels, fc.actingPlayerID)
	}
	return nil
}

func applyFestivityBreakSelf(ctx context.Context, fc *festivityOptionContext) error {
	mcID, err := hfFindMainCharacter(ctx, fc.deps, fc.plan.GameID, fc.actingPlayerID)
	if err != nil {
		return fmt.Errorf("find main character: %w", err)
	}
	marg, err := fc.deps.Q.ListIntactMarginalia(ctx, mcID)
	if err != nil || len(marg) == 0 {
		return errors.New("no intact marginalia to tear")
	}
	m := marg[0]
	err = fc.deps.Q.TearMarginalia(ctx, dbgen.TearMarginaliaParams{
		ID: m.ID, TornByID: &fc.actingPlayerID,
	})
	if err != nil {
		return fmt.Errorf("tear marginalia: %w", err)
	}
	broadcastEvent(fc.deps.Manager, fc.plan.GameID, model.EventMarginaliaTorn, model.MarginaliaTornPayload{
		AssetID: mcID, Position: m.Position, TornByID: fc.actingPlayerID,
	})
	return nil
}

func hfFindMainCharacter(ctx context.Context, deps *PlanDeps, gameID, playerID int64) (int64, error) {
	assets, err := deps.Q.ListAssetsByOwner(ctx, playerID)
	if err != nil {
		return 0, err
	}
	for _, a := range assets {
		if a.GameID == gameID && a.IsMainCharacter {
			return a.ID, nil
		}
	}
	return 0, errors.New("no main character found")
}

// ── insist-host-mar ──────────────────────────────────────────────────────────

func hfInsistHostMarHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			MarOption string `json:"mar_option"`
			AssetID   int64  `json:"asset_id"`
			RumorText string `json:"rumor_text"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMarOption(body.MarOption) {
			respondErr(w, http.StatusBadRequest, "invalid mar option")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.FestivityState()
		if state.Phase == gamepkg.FestivityPhaseDone {
			respondErr(w, http.StatusConflict, "event is over; IOUs can no longer be cashed")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		if !state.ConsumeIOU(player.ID) {
			respondErr(w, http.StatusForbidden, "you have no IOU to cash (must have rolled a make)")
			return
		}

		// Apply the mar effect to the host.
		if err := hfApplyOption(ctx, deps, plan, &state, plan.PreparerID,
			body.MarOption, body.RumorText, "", body.AssetID, false); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		state.HostMarInsists = append(state.HostMarInsists, body.MarOption)

		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save insist")
			return
		}
		broadcastEvent(
			deps.Manager,
			plan.GameID,
			model.EventFestivityInsistHostMar,
			model.FestivityInsistHostMarPayload{
				PlanID: plan.ID, InsisterID: player.ID, MarOption: body.MarOption,
			},
		)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "mar_option": body.MarOption})
	}
}

// ── host-choice ──────────────────────────────────────────────────────────────

func hfHostChoiceHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the host may submit host-choice")
			return
		}
		var body struct {
			TargetPlayerID int64  `json:"target_player_id"`
			Choice         string `json:"choice"`
			RumorText      string `json:"rumor_text"`
			PeerName       string `json:"peer_name"`
			AssetID        int64  `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if !gamepkg.IsValidFestivityMakeOption(body.Choice) || body.Choice == gamepkg.FestivityMakeChallengeDuel {
			respondErr(w, http.StatusBadRequest, "host-choice must be a non-duel make option")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.FestivityState()
		if state.Phase != gamepkg.FestivityPhaseHostChoosing {
			respondErr(w, http.StatusConflict, "host-choice is only allowed during the host_choosing phase")
			return
		}
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		tk := strconv.FormatInt(body.TargetPlayerID, 10)
		oc, ok := state.Outcomes[tk]
		if !ok || (oc != gamepkg.FestivityOutcomeMar && oc != gamepkg.FestivityOutcomeOptOut) {
			respondErr(w, http.StatusBadRequest, "target guest did not roll mar or opt out")
			return
		}
		if _, done := state.HostChoices[tk]; done {
			respondErr(w, http.StatusConflict, "host has already chosen for this guest")
			return
		}

		// Apply the make effect to the target guest (host's choice acts FOR
		// the guest, so actingPlayerID is the guest).
		if err := hfApplyOption(ctx, deps, plan, &state, body.TargetPlayerID,
			body.Choice, body.RumorText, body.PeerName, body.AssetID, true); err != nil {
			respondErr(w, http.StatusBadRequest, err.Error())
			return
		}
		if state.HostChoices == nil {
			state.HostChoices = map[string]string{}
		}
		state.HostChoices[tk] = body.Choice

		if err := hfFinalizeIfDone(ctx, deps, plan, &state); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not finalize: "+err.Error())
			return
		}
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save host choice")
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityHostChose, model.FestivityHostChosePayload{
			PlanID: plan.ID, GuestPlayerID: body.TargetPlayerID, Choice: body.Choice,
		})
		if state.Phase == gamepkg.FestivityPhaseDone {
			hfBroadcastPhase(deps, plan, state.Phase)
		}
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "target_player_id": body.TargetPlayerID, "choice": body.Choice, "phase": state.Phase,
		})
	}
}

// ── challenge-duel ───────────────────────────────────────────────────────────

// A guest who rolled make uses this route (instead of guest-choice) to invoke
// the challenge_duel option. The target is anyone in the guest list — they may
// accept or decline via /respond-challenge. Targets who have taken the
// accept_duels mar cannot decline (the UI should disable that option). The
// challenger's make is spent on challenge_duel regardless of the target's
// response. All festivity game actions pause while PendingChallenge is set.
func hfChallengeDuelHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			TargetPlayerID int64  `json:"target_player_id"`
			Notes          string `json:"preparation_notes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TargetPlayerID == 0 {
			respondErr(w, http.StatusBadRequest, "target_player_id required")
			return
		}
		if body.TargetPlayerID == player.ID {
			respondErr(w, http.StatusBadRequest, "cannot duel yourself")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.FestivityState()
		if !state.IsGuest(player.ID) || !state.IsGuest(body.TargetPlayerID) {
			respondErr(w, http.StatusBadRequest, "challenger and target must both be guests")
			return
		}
		// Caller must have rolled make.
		ck := strconv.FormatInt(player.ID, 10)
		if state.Outcomes[ck] != gamepkg.FestivityOutcomeMake {
			respondErr(w, http.StatusForbidden, "challenge_duel requires rolling a make")
			return
		}
		if state.PendingDuelPlanID != nil {
			respondErr(w, http.StatusConflict, "a nested duel is already in progress")
			return
		}
		if state.PendingChallenge != nil {
			respondErr(w, http.StatusConflict, "a challenge is already awaiting a response")
			return
		}

		// Record the challenger's make pick up front: challenge_duel is spent
		// whether or not the target accepts (per house rule).
		if state.Outcomes == nil {
			state.Outcomes = map[string]string{}
		}
		if state.GuestMakes == nil {
			state.GuestMakes = map[string]string{}
		}
		if _, already := state.GuestMakes[ck]; !already {
			state.Outcomes[ck] = gamepkg.FestivityOutcomeMake
			state.GuestMakes[ck] = gamepkg.FestivityMakeChallengeDuel
			state.GuestIOUs = append(state.GuestIOUs, player.ID)
		}
		state.PendingChallenge = &gamepkg.PendingChallenge{
			ChallengerID: player.ID,
			TargetID:     body.TargetPlayerID,
			Notes:        body.Notes,
		}
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save challenge state")
			return
		}
		broadcastEvent(
			deps.Manager,
			plan.GameID,
			model.EventFestivityChallengeIssued,
			model.FestivityChallengeIssuedPayload{
				PlanID:       plan.ID,
				ChallengerID: player.ID,
				TargetID:     body.TargetPlayerID,
				MustAccept:   state.HasAcceptDuels(body.TargetPlayerID),
			},
		)
		respond(w, http.StatusCreated, map[string]any{
			"plan_id":       plan.ID,
			"challenger_id": player.ID,
			"target_id":     body.TargetPlayerID,
			"must_accept":   state.HasAcceptDuels(body.TargetPlayerID),
		})
	}
}

// ── respond-challenge ────────────────────────────────────────────────────────

// Body: {"accept": true|false}. Only the challenge's target may call.
// Decline is refused if the target has the accept_duels mar.
func hfRespondChallengeHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		var body struct {
			Accept bool `json:"accept"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			respondErr(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.FestivityState()
		pc := state.PendingChallenge
		if pc == nil {
			respondErr(w, http.StatusConflict, "no challenge is awaiting a response")
			return
		}
		if player.ID != pc.TargetID {
			respondErr(w, http.StatusForbidden, "only the challenged player may respond")
			return
		}
		if !body.Accept && state.HasAcceptDuels(player.ID) {
			respondErr(w, http.StatusConflict,
				"you have taken the accept_duels mar and cannot decline")
			return
		}

		if !body.Accept {
			// Challenger's make is already spent; just clear the challenge.
			challengerID := pc.ChallengerID
			state.PendingChallenge = nil
			resData.SetFestivityState(state)
			if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
				respondErr(w, http.StatusInternalServerError, "could not save decline")
				return
			}
			broadcastEvent(
				deps.Manager,
				plan.GameID,
				model.EventFestivityChallengeDeclined,
				model.FestivityChallengeDeclinedPayload{
					PlanID: plan.ID, ChallengerID: challengerID, TargetID: player.ID,
				},
			)
			respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "accepted": false})
			return
		}

		// Accept: spawn the nested duel plan and advance it to resolving.
		game, err := deps.Q.GetGameByID(ctx, plan.GameID)
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not load game")
			return
		}
		notes := pc.Notes
		if notes == "" {
			notes = "Duel triggered by festivity"
		}
		challengerID := pc.ChallengerID
		duelPlan, err := deps.Q.CreatePlan(ctx, dbgen.CreatePlanParams{
			GameID:           plan.GameID,
			PlanType:         model.PlanProposeDuel,
			Category:         model.CategoryEsteem,
			PreparerID:       challengerID,
			TargetPlayerID:   &player.ID,
			RowNumber:        game.CurrentRow,
			RowOrder:         0,
			PreparedAtRow:    game.CurrentRow,
			PreparationNotes: &notes,
		})
		if err != nil {
			respondErr(w, http.StatusInternalServerError, "could not create nested duel: "+err.Error())
			return
		}
		if err := deps.Q.SetPlanStatus(ctx, dbgen.SetPlanStatusParams{
			ID: duelPlan.ID, Status: model.PlanResolving,
		}); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not resolve nested duel: "+err.Error())
			return
		}
		if h, ok := GetHandler(model.PlanProposeDuel); ok {
			refreshed, _ := deps.Q.GetPlanByID(ctx, duelPlan.ID)
			if _, err := h.OnResolve(ctx, deps, &refreshed); err != nil {
				respondErr(w, http.StatusInternalServerError, "duel setup failed: "+err.Error())
				return
			}
		}

		state.PendingChallenge = nil
		state.PendingDuelPlanID = &duelPlan.ID
		resData.SetFestivityState(state)
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondErr(w, http.StatusInternalServerError, "could not save accept")
			return
		}
		if h, ok := deps.Manager.Get(plan.GameID); ok {
			h.BroadcastEvent(model.EventFestivityDuelTriggered, model.FestivityDuelTriggeredPayload{
				PlanID: plan.ID, DuelPlanID: duelPlan.ID,
			})
			h.BroadcastEvent(model.EventPlanResolving, model.PlanPayload{Plan: duelPlan})
		}
		respond(w, http.StatusCreated, map[string]any{
			"plan_id": plan.ID, "duel_plan_id": duelPlan.ID, "accepted": true,
		})
	}
}
