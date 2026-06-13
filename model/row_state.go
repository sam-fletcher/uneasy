package model

// RowStateKind is the discriminator for RowState — the single field that
// names the row's current "step" in the rulebook sense (steps 1–8) plus the
// pre-step gates (war battle costs, Make War delay reveal, open surrender
// claims) that pause normal step progression.
//
// The eight rulebook steps collapse to fewer kinds because some steps don't
// need their own UI surface:
//
//   - Step 1 (pay battle costs):       AwaitBattleCost
//   - Step 2 (resolve pending plan):   PlanResolving / PlanPending
//     (sub-phase overrides: AwaitDemandCounter, AwaitDemandDraftPick,
//     AwaitFestivityGuestTurn,
//     AwaitFestivityChallengeResponse,
//     AwaitDuelStaking, AwaitDuelBout)
//   - Step 3 (set scene):              SceneSetting
//   - Step 4 (roleplay scene):         SceneActive
//   - Step 5 (prepare plan / refresh): PostSceneAction
//   - Step 6 (pass focus):             handled server-side; never a UI state
//   - Step 7 (repeat if plans remain): folded into PlanPending on the new focus
//   - Step 8 (advance row):            handled server-side; never a UI state
//
// Plus the two cross-cutting gates that interrupt the normal flow:
//
//   - Open delay-reveal plan:          AwaitDelayReveal       (Make War, Clandestinely Liaise)
//   - Open surrender claim:            AwaitSurrenderClaim
//
// And the catch-all for phases other than main_event:
//
//   - Anything outside main_event:     PhaseNotMainEvent
type RowStateKind string

const (
	// RowStatePhaseNotMainEvent is returned for any phase other than
	// main_event. Row-steps don't exist in lobby/prologue/shake_up/ended.
	RowStatePhaseNotMainEvent RowStateKind = "phase_not_main_event"

	// RowStateAwaitSurrenderClaim — a surrender claim from a Make War
	// payment is still open. The claimant must take an asset from the
	// surrendering player before the row can advance. Blocks everything.
	RowStateAwaitSurrenderClaim RowStateKind = "await_surrender_claim"

	// RowStateAwaitBattleCost — at least one war participant owes a battle
	// cost on the current row. Per the rulebook this is step 1 of every
	// row with an active war; the server treats it as a gate on row
	// advance rather than a strict serial step, matching current handler
	// behaviour.
	RowStateAwaitBattleCost RowStateKind = "await_battle_cost"

	// RowStateAwaitDelayReveal — a plan whose landing row is decided by a
	// simultaneous reveal has been prepared, and the reveal is still open.
	// All participants must submit a hidden die before the plan's row is
	// fixed and it enters the normal pending/resolving flow.
	//
	// Today this covers Make War and Clandestinely Liaise. The kind is
	// shared because the row-blocking semantics are identical; the client
	// dispatches to a plan-type-specific panel via RowState.PlanID.
	RowStateAwaitDelayReveal RowStateKind = "await_delay_reveal"

	// RowStatePlanResolving — a plan is currently in 'resolving' status.
	// Step 2, active.
	RowStatePlanResolving RowStateKind = "plan_resolving"

	// RowStateAwaitDemandCounter — a Make Demands plan is resolving, its
	// dice roll came up 'mar', and the target of the demand has not yet
	// placed (or deferred) their free counter-demand. The table is blocked
	// on the target's decision, who is typically *not* the focus player.
	// Narrower than PlanResolving so the WaitingOnBar can name the actual
	// waitee instead of mis-attributing the wait to the focus player.
	RowStateAwaitDemandCounter RowStateKind = "await_demand_counter"

	// RowStateAwaitDemandDraftPick — a made Make Demands plan is in the
	// alternating draft of demand options (control_leverage,
	// keep_or_change_target, keep_assets, perform_steps). Demander and
	// target-plan preparer alternate; ActingPlayerIDs names whoever owes
	// the next pick. Half of every draft blocks on a non-focus player.
	RowStateAwaitDemandDraftPick RowStateKind = "await_demand_draft_pick"

	// RowStateAwaitFestivityGuestTurn — a Host Festivity plan is in the
	// 'socializing' phase and waiting on the next guest (in lowest-esteem-
	// first order, host goes last) to roll or opt out. ActingPlayerIDs names
	// that guest — typically not the focus player.
	RowStateAwaitFestivityGuestTurn RowStateKind = "await_festivity_guest_turn"

	// RowStateAwaitFestivityChallengeResponse — a Host Festivity plan has
	// an open duel challenge; all other festivity actions pause until the
	// challenged guest accepts or declines. ActingPlayerIDs names the
	// challenge target.
	RowStateAwaitFestivityChallengeResponse RowStateKind = "await_festivity_challenge_response"

	// RowStateAwaitDuelStaking — a Propose Duel is in 'setup' or 'staking'
	// phase: both duellists simultaneously submit stake counts (setup), then
	// the specific assets (staking). ActingPlayerIDs holds the duellists who
	// still owe a submission (preparer + target_player_id minus those already
	// in), filtered server-side.
	RowStateAwaitDuelStaking RowStateKind = "await_duel_staking"

	// RowStateAwaitDuelBout — a Propose Duel is in 'bouts' phase. The
	// initiative-holder declares; the responder responds. ActingPlayerIDs
	// names whichever side owes the next action: if a bout has been
	// declared and is unresolved, the responder; otherwise the declarer
	// (= InitiativePlayerID).
	RowStateAwaitDuelBout RowStateKind = "await_duel_bout"

	// RowStateAwaitTakeConsent — a Spread Rumors "take asset" choice is
	// waiting for the victim (the player who would lose the asset) to agree
	// or disagree. The aggressor's picks are not yet committed; nothing else
	// may happen until the victim responds. ActingPlayerIDs names the victim.
	RowStateAwaitTakeConsent RowStateKind = "await_take_consent"

	// RowStateAwaitQuestionAnswer — a Seek Answers "ask a player a question"
	// pick is waiting for the target to answer (or veto). Nothing else may
	// happen until they respond. ActingPlayerIDs names the target.
	RowStateAwaitQuestionAnswer RowStateKind = "await_question_answer"

	// RowStateLiaiseResolving — a Clandestinely Liaise plan is resolving and a
	// collaborative submit phase blocks on the participants, not the focus
	// player (who is often not even a participant). ActingPlayerIDs names the
	// participants who still owe a submission (or the preparer once both are in,
	// who must advance). The preparer-only phases (together_at_last, done) do
	// NOT use this kind — they ride the generic plan_resolving case, which names
	// the resolving plan's preparer.
	RowStateLiaiseResolving RowStateKind = "liaise_resolving"

	// RowStateAwaitCourtierResponse — an Exchange Courtiers plan is resolving
	// inside a target-driven sub-step (fair-trade offer, messy break, mar
	// choices, or peer claims). ActingPlayerIDs names the target player, who is
	// not the preparer (the normal resolver) and not the focus player.
	RowStateAwaitCourtierResponse RowStateKind = "await_courtier_response"

	// RowStateAwaitChronicleChoices — a marred Chronicle Histories plan is
	// resolving and every player present when the mar scene began must each
	// submit one option. ActingPlayerIDs names those who still owe a choice.
	RowStateAwaitChronicleChoices RowStateKind = "await_chronicle_choices"

	// RowStatePlanPending — a plan is in 'pending' status on the current
	// row and ready to be resolved. Step 2, queued.
	RowStatePlanPending RowStateKind = "plan_pending"

	// RowStateSceneActive — the focus player's turn-scene is in progress
	// (started, not yet ended). Step 4.
	RowStateSceneActive RowStateKind = "scene_active"

	// RowStatePostSceneAction — the focus player has ended their
	// turn-scene and must now prepare a plan or refresh assets before
	// focus passes. Step 5.
	RowStatePostSceneAction RowStateKind = "post_scene_action"

	// RowStateSceneSetting — default in-row state. The focus player needs
	// to set their turn-scene. Step 3.
	RowStateSceneSetting RowStateKind = "scene_setting"
)

// RowState is the single source of truth for "which step of the row are we
// in?" — computed server-side from the persisted state of games, plans,
// scenes, wars, and reveals. Carried in every main-event game-state snapshot
// and broadcast as EventRowStateChanged when it transitions.
//
// Kind is always set. The other fields are populated only for the kinds
// that need them, as documented per-field.
type RowState struct {
	Kind RowStateKind `json:"kind"`

	// PlanID is the relevant plan for: PlanResolving, PlanPending,
	// AwaitDelayReveal, AwaitDemandCounter, AwaitDemandDraftPick,
	// AwaitFestivityGuestTurn, AwaitFestivityChallengeResponse,
	// AwaitDuelStaking, AwaitDuelBout. Nil otherwise.
	PlanID *int64 `json:"plan_id,omitempty"`

	// SceneID is the focus player's active scene id for: SceneActive. This
	// covers both a blank-row turn-scene and the follow-scene set after a
	// plan resolves (when two plans share a row, the focus player roleplays
	// the resolved plan's follow-scene before the next plan resolves). Nil
	// for all other kinds.
	SceneID *int64 `json:"scene_id,omitempty"`

	// WarID is the war that owes battle costs for: AwaitBattleCost. If
	// multiple wars owe costs on the same row, this is one of them
	// (clients fetch full war state separately to render specifics).
	WarID *int64 `json:"war_id,omitempty"`

	// ClaimID is the open surrender claim for: AwaitSurrenderClaim.
	ClaimID *int64 `json:"claim_id,omitempty"`

	// ActingPlayerIDs names the full set of players whose action the table is
	// blocked on. The single, authoritative home for "who must act" — set
	// server-side for every actor-naming kind, including the generic
	// PlanResolving / PlanPending case (where it holds the resolving plan's
	// preparer, who owns the resolution) and the single-actor sub-phase gates
	// (AwaitDemandCounter, AwaitDemandDraftPick, AwaitFestivityGuestTurn,
	// AwaitFestivityChallengeResponse, AwaitDuelBout, AwaitTakeConsent,
	// AwaitQuestionAnswer, AwaitCourtierResponse — each a one-element slice) as
	// well as the multi-actor gates (LiaiseResolving submit phases,
	// AwaitChronicleChoices, AwaitDuelStaking).
	//
	// Computed from persisted state (resolution_data, rankings, reveals), so the
	// WaitingOnBar reflects exactly who still owes an action — never the focus
	// player (who, for a delayed plan, is often not even a participant) and never
	// a stale client guess. The frontend reads it directly; there is no
	// client-side preparer/focus proxy. Nil only for kinds with no actor to name.
	ActingPlayerIDs []int64 `json:"acting_player_ids,omitempty"`
}

// RowStateChangedPayload is the payload for EventRowStateChanged.
type RowStateChangedPayload struct {
	RowState RowState `json:"row_state"`
}
