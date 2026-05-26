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
}

export interface PlanPanelProps {
	ctx: PlanContext;
	plan?: Plan | null;
	mode: PlanMode;
}
