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

export type PlanMode = 'prep' | 'resolve' | 'alwaysOn';

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
