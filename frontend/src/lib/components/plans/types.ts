// Shared per-plan component contract.
//
// Every plan panel takes the same three props: a context object with the
// game-wide state and callbacks, the optional plan being resolved/viewed,
// and a mode tag telling the panel which UI to render.
//
// Panels destructure ctx into local $derived shims so the rest of their
// script body keeps reading bare names (gameID, assets, …) — the contract
// only changes the top of each file, not its internals.

import type {
	Plan, Asset, Player, Ranking, DiceRoll,
} from '$lib/api';

// 'prep' — preparation form (post-scene action).
// 'resolve' — currently resolving plan (also reused for read-only views
//   like the Make War "active wars" drawer, since the resolve UI is
//   already a "show me this war" view).
// 'delayReveal' — Make War / Clandestinely Liaise simultaneous-reveal
//   panel, rendered by MainEventView for every player when the row_state
//   kind is 'await_delay_reveal'.
export type PlanMode = 'prep' | 'resolve' | 'delayReveal';

export interface PlanContext {
	gameID: number;
	currentRow: number;
	plans: Plan[];
	assets: Asset[];
	players: Player[];
	rankings: Ranking[];
	currentPlayerID: number | null;
	isFocusPlayer: boolean;
	rollActive: boolean;
	rollOutcome: 'make' | 'mar' | null;
	activeRoll: DiceRoll | null;
	onRollCreated: (roll: DiceRoll) => void;
	onPlansChanged: () => void;
	onPlanPrepared: () => void;
	/**
	 * True for non-focus viewers in prep mode. Panels should disable inputs
	 * and hide the submit button when this is set, and render values from
	 * `prepDraft` instead of local state.
	 */
	readOnly: boolean;
	/**
	 * The focus player's in-flight prep snapshot, scoped to the currently
	 * highlighted plan_type. Null until the focus player touches the form
	 * (or when readOnly is false). Shape is plan-specific; each panel casts.
	 */
	prepDraft: Record<string, unknown> | null;
	/**
	 * Called by panels (focus player only) when their prep inputs change.
	 * Fans out an EventPreparePlanDraft over the WS, stitched with the
	 * currently-selected plan_type by PlanPanel.
	 */
	emitPrepDraft: (prep: Record<string, unknown>) => void;
}

export interface PlanPanelProps {
	ctx: PlanContext;
	plan?: Plan | null;
	mode: PlanMode;
}
