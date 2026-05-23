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
	// AwaitDelayReveal. Nil otherwise.
	PlanID *int64 `json:"plan_id,omitempty"`

	// SceneID is the focus player's turn-scene id for: SceneActive. Nil
	// otherwise (including for plan-resolution scenes, whose presence
	// already implies PlanResolving).
	SceneID *int64 `json:"scene_id,omitempty"`

	// WarID is the war that owes battle costs for: AwaitBattleCost. If
	// multiple wars owe costs on the same row, this is one of them
	// (clients fetch full war state separately to render specifics).
	WarID *int64 `json:"war_id,omitempty"`

	// ClaimID is the open surrender claim for: AwaitSurrenderClaim.
	ClaimID *int64 `json:"claim_id,omitempty"`
}

// RowStateChangedPayload is the payload for EventRowStateChanged.
type RowStateChangedPayload struct {
	RowState RowState `json:"row_state"`
}
