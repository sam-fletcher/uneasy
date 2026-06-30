package handler

// handler/plan_host_festivity.go — Host Festivity plan handler (Phase 3d).
//
// Host Festivity (esteem, delay 6). The host throws a social event; every
// other player at the table attends as a guest, then each guest independently
// rolls (or opts out) against the host's esteem status to pick a make or mar
// effect. The host does not roll: they've earned an extra make for hosting
// (recorded up front via OnResolve as FestivityOutcomeHost) rather than risk a
// mar that rolling would expose them to.
//
// There are no phases or turns — the whole event is one open stretch of
// socializing. All of these happen freely, in any order:
//
//   - guests roll/opt-out and pick a make or mar option;
//   - the host takes their extra makes (one for hosting, one per guest who
//     marred or opted out) — these are the host's spoils, not debts owed to
//     those guests, so they're tracked as a count, not per-guest;
//   - successful (make-rolling) guests inflict an extra mar on the host.
//
// The only ordering rule: a single roll-and-choice must conclude before the
// next action starts (see hfBlockOnActiveRoll). The host winds the event down
// via the end-event route — gated on every guest having chosen, every earned
// make taken, and every outstanding mar inflicted (FestivityResolutionData.
// EventEndable). Generic /complete is refused (CanComplete) so the gate can't
// be skipped.
//
// Extra routes:
//
//	POST /api/plans/:planId/guest-roll        — {action:"roll"|"opt_out"}.
//	POST /api/plans/:planId/guest-choice      — {choice, ...option params}.
//	POST /api/plans/:planId/insist-host-mar   — {mar_option}, consumes IOU.
//	POST /api/plans/:planId/resolve-host-mar  — host settles an insisted mar that
//	                                            needs their own choice (break_self,
//	                                            disagreement).
//	POST /api/plans/:planId/host-choice       — {choice, ...option params}.
//	POST /api/plans/:planId/challenge-duel    — {target_player_id}.
//	POST /api/plans/:planId/respond-challenge — {accept:bool}, target only.
//	POST /api/plans/:planId/end-event         — host winds the event down.
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

// OnResolve opens the event. No plan-level dice roll; each guest creates their
// own via guest-roll. The host's outcome is recorded up front as
// FestivityOutcomeHost — they don't roll or opt out, they've earned an extra
// make for hosting. This both removes the footgun (rolling is strictly worse
// than the guaranteed make) and surfaces the earned make to the host from the
// start.
func (hfHandler) OnResolve(ctx context.Context, deps *PlanDeps, plan *dbgen.Plan) (*dbgen.DiceRoll, error) {
	resData := loadResolutionData(plan.ResolutionData)
	state := resData.EnsureFestivity()
	// No guest list is stored — every player at the table attends as a guest
	// (the roster is fixed once a game starts), so it's derived where needed.
	// Pre-record the host's earned make so they're never prompted to roll.
	if state.Outcomes == nil {
		state.Outcomes = map[string]string{}
	}
	state.Outcomes[strconv.FormatInt(plan.PreparerID, 10)] = gamepkg.FestivityOutcomeHost
	if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
		return nil, fmt.Errorf("save festivity setup: %w", err)
	}
	// kickoffPlanResolution broadcasts plan.resolving *before* OnResolve runs,
	// so its payload carries the pre-resolve resolution_data (no host outcome).
	// Clients watching the kickoff live — notably the host — would otherwise
	// keep that stale snapshot, so nudge every client to refetch the plan.
	hfBroadcastUpdated(deps, plan)
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

// CanComplete blocks the generic /complete path. A festivity is wound down by
// the host via the dedicated end-event route, which loads the roster (every
// player) to gate on EventEndable — state the pure CanComplete contract can't
// reach. Routing here would skip that gate, so it is refused.
func (hfHandler) CanComplete(_ *dbgen.Plan, _ *ResolutionData) error {
	return errors.New("a festivity is wound down via the host's end-event action, not generic completion")
}

// ResolvedDescriptor gives the festivity's always-make resolution a flavor line
// instead of the tautological "Host Festivity succeeded." (it always succeeds —
// the event exists to benefit the host).
func (hfHandler) ResolvedDescriptor(_ context.Context, _ *dbgen.Queries, _ dbgen.Plan, result string) (string, bool) {
	if result == makeOutcome {
		return "The festivity drew to a close.", true
	}
	return "", false
}

func (hfHandler) ExtraRoutes(deps *PlanDeps) map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"guest-roll":        hfGuestRollHandler(deps),
		"guest-choice":      hfGuestChoiceHandler(deps),
		"insist-host-mar":   hfInsistHostMarHandler(deps),
		"resolve-host-mar":  hfResolveHostMarHandler(deps),
		"host-choice":       hfHostChoiceHandler(deps),
		"challenge-duel":    hfChallengeDuelHandler(deps),
		"respond-challenge": hfRespondChallengeHandler(deps),
		"end-event":         hfEndEventHandler(deps),
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

// hfRoster returns the festivity guest list: every player at the table. The
// guest set is not stored — the roster is fixed once a game starts, and every
// player attends.
func hfRoster(ctx context.Context, q *dbgen.Queries, gameID int64) ([]int64, error) {
	players, err := q.GetPlayersByGame(ctx, gameID)
	if err != nil {
		return nil, fmt.Errorf("load festivity roster: %w", err)
	}
	ids := make([]int64, len(players))
	for i := range players {
		ids[i] = players[i].ID
	}
	return ids, nil
}

// hfBroadcastUpdated nudges every client to refetch the festivity plan. Used
// where state changed without a more specific event (notably OnResolve, before
// any guest action).
func hfBroadcastUpdated(deps *PlanDeps, plan *dbgen.Plan) {
	broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityUpdated, model.FestivityUpdatedPayload{
		PlanID: plan.ID,
	})
}

// hfBlockOnActiveRoll writes a 409 and returns true if another guest's
// roll-and-choice is in progress. A single roll-and-choice must conclude
// (dice resolved *and* a make/mar option chosen) before any other festivity
// action starts, so the table reads clearly. The active roller themselves is
// exempt — they are the one completing the sequence. roster is the full guest
// list (every player).
func hfBlockOnActiveRoll(
	w http.ResponseWriter,
	state *gamepkg.FestivityResolutionData,
	roster []int64,
	actorID int64,
) bool {
	if roller := state.ActiveRoller(roster); roller != 0 && roller != actorID {
		respondErr(w, http.StatusConflict,
			"a dice roll is in progress — wait for it to conclude, then try again")
		return true
	}
	return false
}

// ── guest-roll ───────────────────────────────────────────────────────────────

//nolint:funlen,gocognit // guest roll lifecycle (opt-out vs roll-creation paths)
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
		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusForbidden,
				"the host doesn't roll — you hold a free make, earned for hosting, to take as the event winds down")
			return
		}

		ctx := r.Context()
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the festivity is over")
			return
		}
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		// Every player at the table is a guest, so no membership check is needed.
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		// A roll-and-choice already in flight blocks any new action.
		if hfBlockOnActiveRoll(w, state, roster, player.ID) {
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

		// action == "roll": create a dice roll for the guest. One in-flight
		// interactive roll per game (friendly pre-check; uq_one_open_roll_per_game
		// is the atomic backstop on the CreateDiceRoll below).
		if blockIfOpenRoll(ctx, w, r, deps.Q, plan.GameID) {
			return
		}
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
		// Festivity rolls skip the difficulty-vote step entirely to keep the
		// event moving — the guest goes straight to leverage, exactly as if
		// "Skip vote" had been pressed (advanceToLeverage below runs the same
		// short-circuit). Duels likewise start at leverage; only generic
		// scene/plan rolls offer the vote.
		roll, err := deps.Q.CreateDiceRoll(ctx, dbgen.CreateDiceRollParams{
			GameID:     plan.GameID,
			PlanID:     &plan.ID,
			RowNumber:  &game.CurrentRow,
			ActorID:    player.ID,
			Difficulty: difficulty,
			Stage:      stageLeverage,
		})
		if err != nil {
			if isUniqueViolation(err, openRollConstraint) {
				respondErr(w, http.StatusConflict, openRollBusyMsg)
				return
			}
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
		if err := seedRollParticipants(ctx, deps.Q, plan.GameID, roll.ID, player.ID, new(plan.ID)); err != nil {
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
		// Run the leverage-entry short-circuit (force-ready anyone with no dice,
		// emit the skip-leverage log, auto-resolve if nobody can commit). The
		// resolution data above is already persisted, so an immediate
		// auto-resolve sees GuestRollIDs. Mirrors the SkipVote path.
		if err := advanceToLeverage(ctx, w, r, deps.Q, deps.Manager, &roll); err != nil {
			respondInternalErr(w, r, "could not advance to leverage", err)
			return
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
		// Every player at the table is a guest, so no membership check is needed.
		if player.ID == plan.PreparerID {
			respondErr(w, http.StatusForbidden,
				"the host takes their earned make via host-choice, not guest-choice")
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

		// The choosing guest is the active roller concluding their own
		// roll-and-choice; no active-roll guard is needed for them.
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save guest choice", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventFestivityGuestChose, model.FestivityGuestChosePayload{
			PlanID: plan.ID, PlayerID: player.ID,
			Outcome: state.Outcomes[key], Choice: body.Choice,
		})
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{
			"plan_id": plan.ID, "outcome": state.Outcomes[key], "choice": body.Choice,
		})
	}
}

// ResolvingWaitees names every player who must still act for a resolving Host
// Festivity to be windable-down. Precedence:
//
//  1. an open duel challenge blocks on the target alone;
//  2. a roll-and-choice in progress blocks on that guest alone (it must conclude
//     before anything else happens);
//  3. otherwise the table waits on everyone who still owes an action — guests
//     who haven't chosen, successful guests who haven't inflicted their mar, and
//     the host while they still have earned makes to take or an insisted mar to
//     resolve themselves (break_self / disagreement);
//  4. when nothing is outstanding, it waits on the host to wind the event down.
func (hfHandler) ResolvingWaitees(ctx context.Context, q *dbgen.Queries, plan *dbgen.Plan) (model.RowState, bool) {
	if plan.Status != model.PlanResolving {
		return model.RowState{}, false
	}
	state := gamepkg.LoadFestivityData(plan.ResolutionData)
	if state.PendingChallenge != nil {
		return model.RowState{
			Kind:            model.RowStateAwaitFestivityChallengeResponse,
			ActingPlayerIDs: []int64{state.PendingChallenge.TargetID},
		}, true
	}
	roster, err := hfRoster(ctx, q, plan.GameID)
	if err != nil {
		return model.RowState{}, false
	}
	if roller := state.ActiveRoller(roster); roller != 0 {
		return model.RowState{
			Kind:            model.RowStateAwaitFestivityGuestTurn,
			ActingPlayerIDs: []int64{roller},
		}, true
	}
	// Everyone with an outstanding obligation: unchosen guests, then successful
	// guests still holding a mar to inflict, then the host if makes remain.
	waiters := state.PendingGuests(roster)
	for _, id := range state.GuestIOUs {
		if !slices.Contains(waiters, id) {
			waiters = append(waiters, id)
		}
	}
	if (state.RemainingHostMakes(roster) > 0 || len(state.PendingHostMars) > 0) &&
		!slices.Contains(waiters, plan.PreparerID) {
		waiters = append(waiters, plan.PreparerID)
	}
	// Nothing outstanding → the host may now wind the event down.
	if len(waiters) == 0 {
		waiters = []int64{plan.PreparerID}
	}
	return model.RowState{Kind: model.RowStateAwaitFestivityGuestTurn, ActingPlayerIDs: waiters}, true
}

// ── end-event ────────────────────────────────────────────────────────────────

// hfEndEventHandler resolves the festivity. Host-only, and gated on
// EventEndable: every guest has chosen, the host has taken all their earned
// makes, and every outstanding mar has been inflicted. Mirrors the tail of
// CompletePlan (festivity can't use the generic path — see CanComplete).
func hfEndEventHandler(deps *PlanDeps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plan, player, ok := requirePlanForExtraRoute(w, r, deps.Q, model.PlanHostFestivity)
		if !ok {
			return
		}
		if player.ID != plan.PreparerID {
			respondErr(w, http.StatusForbidden, "only the host may end the event")
			return
		}
		if plan.Status != model.PlanResolving {
			respondErr(w, http.StatusConflict, "the festivity is already over")
			return
		}

		ctx := r.Context()
		resData := loadResolutionData(plan.ResolutionData)
		state := resData.EnsureFestivity()
		if hfBlockOnPendingChallenge(w, state) {
			return
		}
		roster, err := hfRoster(ctx, deps.Q, plan.GameID)
		if err != nil {
			respondInternalErr(w, r, "could not load roster", err)
			return
		}
		if !state.EventEndable(roster) {
			respondErr(w, http.StatusConflict,
				"the festivity is still in full swing: every guest must choose, "+
					"the host must take all their makes, and all mars must be inflicted")
			return
		}

		// Settle every disagreement whose peer no one took: it rejoins its owner
		// broken. Do this before resolving so the breaks land while the plan (and
		// its action-log row) is still open.
		if err := hfBreakAbandonedDisagreementPeers(ctx, deps, plan, state); err != nil {
			respondInternalErr(w, r, "could not settle abandoned peers", err)
			return
		}
		if err := saveResolutionData(ctx, deps.Q, plan.ID, resData); err != nil {
			respondInternalErr(w, r, "could not save festivity wind-down", err)
			return
		}

		// The festivity always benefits the host, so its result is a make.
		res := makeOutcome
		if err := deps.Q.SetPlanResult(ctx, dbgen.SetPlanResultParams{ID: plan.ID, Result: &res}); err != nil {
			respondInternalErr(w, r, "could not end the festivity", err)
			return
		}
		broadcastEvent(deps.Manager, plan.GameID, model.EventPlanResolved, model.PlanResolvedPayload{
			PlanID: plan.ID, Result: res,
		})
		// EmitPlanResolved renders the festivity's custom "drew to a close" line
		// (via hfHandler.ResolvedDescriptor) in place of "Host Festivity succeeded."
		EmitPlanResolved(ctx, deps.Q, deps.Manager, *plan, res)
		broadcastRowState(ctx, deps.Q, deps.Manager, plan.GameID)
		respond(w, http.StatusOK, map[string]any{"plan_id": plan.ID, "result": res})
	}
}
