// waitingOn.ts
// Pure derivation of "whose action is the game waiting on?" for the main-event
// phase. Extracted from MainEventView.svelte so it can be unit-tested in
// isolation — this is the exact layer the June 2026 Clandestinely Liaise bug
// lived in (the bar named a bystander instead of the actor).
//
// The function is intentionally free of Svelte runes and component state: every
// input is passed explicitly. The backend's RowState is authoritative for who
// must act — `acting_player_ids` is read directly, with no client-side
// preparer/focus proxy (the proxy is what produced the original bug).

import type { DiceRoll, RowState, PlanType } from '$lib/api';

/** A single party the game is waiting on. */
export type Waitee =
	| { kind: 'player'; playerID: number }
	| { kind: 'everyone' }
	| { kind: 'label'; text: string };

/** The rendered state of the WaitingOnBar. */
export interface WaitingOnState {
	waitees: Waitee[];
	stepLabel?: string;
	stepSubtitle?: string;
}

/** Everything the main-event derivation reads. All server-authoritative or
 *  already-derived upstream — this function adds no fetches and no guesses. */
export interface MainEventWaitingOnInput {
	/** Authoritative row-state from the server; null briefly during first fetch. */
	rowState: RowState | null;
	/** Active (unresolved) dice roll, or null. Only its `stage` is read here —
	 *  purely for the AwaitDiceRoll step label; the waitees themselves come
	 *  from `rowState.acting_player_ids` like every other kind. */
	activeRoll: DiceRoll | null;
	/** plan_type of the open delay-reveal plan, or null when not in that kind. */
	delayRevealPlanType: PlanType | null;
	/** Max assets the focus player may refresh (for the post-scene subtitle). */
	maxRefresh: number;
}

const player = (playerID: number): Waitee => ({ kind: 'player', playerID });

const rollStepLabel = (stage: DiceRoll['stage'] | undefined): string => {
	switch (stage) {
		case 'decide_vote':
			return 'Dice roll — call a vote?';
		case 'voting':
			return 'Dice roll — difficulty vote';
		case 'leverage':
			return 'Dice roll — leverage & ready';
		default:
			return 'Dice roll';
	}
};

/**
 * Compute the WaitingOnBar state for the main-event phase.
 *
 * The server's RowState kind selects the waitees for every kind, including
 * AwaitDiceRoll (an open dice roll — the backend checks this ahead of every
 * other gate, so it wins the same way the client's roll override used to) and
 * the focus-player kinds (scenes, post-scene). Every kind reads
 * `acting_player_ids` verbatim — the backend has already named the exact
 * decision-maker(s) from persisted state, so there is no client-side
 * preparer/focus/roll-participant proxy left to mis-attribute the wait.
 */
export function mainEventWaitingOn(input: MainEventWaitingOnInput): WaitingOnState {
	const { rowState, activeRoll, delayRevealPlanType, maxRefresh } = input;

	// The server-authoritative acting set. The backend names the exact
	// decision-maker(s) from persisted state for every actor-naming kind —
	// including the generic plan_resolving case (the preparer) — so the bar
	// never guesses or mis-attributes.
	const actingWaitees = (): Waitee[] => (rowState?.acting_player_ids ?? []).map(player);

	switch (rowState?.kind) {
		case 'await_dice_roll':
			return { waitees: actingWaitees(), stepLabel: rollStepLabel(activeRoll?.stage) };
		case 'plan_resolving':
		case 'plan_pending':
			// Resolved by its preparer (named server-side), never the focus
			// player — a delayed plan routinely resolves on a row whose focus is
			// someone else. plan_pending is the brief pre-kickoff/recovery state.
			return { waitees: actingWaitees(), stepLabel: 'Resolving plan' };
		case 'await_demand_counter':
			return { waitees: actingWaitees(), stepLabel: 'Make Demands — awaiting counter' };
		case 'await_demand_draft_pick':
			return { waitees: actingWaitees(), stepLabel: 'Make Demands — draft pick' };
		case 'await_demand_leverage':
			// Control_leverage winner owes the leverage decision on the target
			// plan's open roll. Usually surfaced via the await_dice_roll case
			// above (they're seeded unready on that roll); this is the row-state
			// fallback for the gap before/after the roll itself is open.
			return { waitees: actingWaitees(), stepLabel: 'Make Demands — control leverage' };
		case 'await_festivity_guest_turn':
			return { waitees: actingWaitees(), stepLabel: 'Host Festivity — in progress' };
		case 'await_festivity_challenge_response':
			return { waitees: actingWaitees(), stepLabel: 'Host Festivity — challenge response' };
		case 'await_duel_staking':
			// Both duellists stake simultaneously; the backend filters to whoever
			// still owes a stake (acting_player_ids).
			return { waitees: actingWaitees(), stepLabel: 'Propose Duel — staking' };
		case 'await_duel_bout':
			return { waitees: actingWaitees(), stepLabel: 'Propose Duel — bout' };
		case 'await_take_consent':
			return { waitees: actingWaitees(), stepLabel: 'Spread Rumors — consent to take asset' };
		case 'await_question_answer':
			return { waitees: actingWaitees(), stepLabel: 'Seek Answers — answer a question' };
		case 'liaise_resolving':
			// Collaborative submit phase (secrets / things-we-share /
			// when-will-I-see-you-again). The backend names who still owes a
			// submission, or the preparer once both are in (must advance).
			return { waitees: actingWaitees(), stepLabel: 'Clandestinely Liaise' };
		case 'await_courtier_response':
			// Exchange Courtiers target-driven sub-step (offer / messy break / mar
			// choices / peer claims). Blocks on the target, not the preparer.
			return { waitees: actingWaitees(), stepLabel: 'Exchange Courtiers — target responds' };
		case 'await_chronicle_choices':
			// Marred Chronicle Histories — every present player owes one choice.
			return { waitees: actingWaitees(), stepLabel: 'Chronicle Histories — all choose' };
		case 'await_delay_reveal': {
			const label =
				delayRevealPlanType === 'make_war'
					? 'Make War — delay reveal'
					: delayRevealPlanType === 'clandestinely_liaise'
						? 'Clandestinely Liaise — delay reveal'
						: 'Delay reveal';
			return { waitees: actingWaitees(), stepLabel: label };
		}
		case 'await_battle_cost':
			return { waitees: actingWaitees(), stepLabel: 'Row advance blocked', stepSubtitle: 'cost of battle' };
		case 'await_surrender_claim':
			return {
				waitees: actingWaitees(),
				stepLabel: 'Row advance blocked',
				stepSubtitle: 'surrender-asset claims',
			};
		case 'await_main_character_choice':
			// One or more players lost their main character; each must choose a
			// replacement before play resumes. The backend names them all.
			return { waitees: actingWaitees(), stepLabel: 'Choose a new main character' };
		case 'scene_active':
			return { waitees: actingWaitees(), stepLabel: 'Scene' };
		case 'scene_setting':
			return { waitees: actingWaitees(), stepLabel: 'Set the scene' };
		case 'post_scene_action': {
			const subtitle = `or refresh ${maxRefresh} asset${maxRefresh === 1 ? '' : 's'}`;
			return { waitees: actingWaitees(), stepLabel: 'Prepare a plan', stepSubtitle: subtitle };
		}
	}
	return { waitees: [] };
}

// ── Shake-Up ─────────────────────────────────────────────────────────────────
// Both steps are strictly sequential turn order (reverse rank — lowest status
// first), so exactly one party is ever named: never "everyone", unlike the
// old self-reported-roll skeleton this replaced.

/** Everything the shake-up derivation reads, all server-authoritative. */
export interface ShakeUpWaitingOnInput {
	/** game.shake_up_step: 1 (rolling), 2 (spending), or null before it's set. */
	step: number | null;
	/** Step 1: the player whose turn it is to roll this category
	 *  (GetShakeUp's current_roller_id), or null once everyone has rolled. */
	currentRollerID: number | null;
	/** Step 2: the open spend, if any. While reactors are still pending
	 *  (ruling 5), the table waits on THEM, not the spender; once every
	 *  reactor has adjusted or passed, the wait shifts to the spender's
	 *  commit. */
	openSpend: {
		spend: { player_id: number };
		/** Other token-holding players who haven't yet adjusted or passed
		 *  (GetShakeUp's open_spend.pending_reactor_ids). */
		pendingReactorIDs: number[];
		/** GetShakeUp's open_spend.commit_ready. */
		commitReady: boolean;
	} | null;
	/** Step 2: whose turn it is to announce a spend (reverse-rank order,
	 *  GetShakeUp's current_actor), or null when no one holds tokens. */
	currentActor: number | null;
}

/** Compute the WaitingOnBar state for the shake-up phase. */
export function shakeUpWaitingOn(input: ShakeUpWaitingOnInput): WaitingOnState {
	const { step, currentRollerID, openSpend, currentActor } = input;
	if (step === 1) {
		if (currentRollerID == null) return { waitees: [] };
		return { waitees: [player(currentRollerID)], stepLabel: 'Roll for tokens' };
	}
	if (step === 2) {
		if (openSpend) {
			if (!openSpend.commitReady) {
				return { waitees: openSpend.pendingReactorIDs.map(player), stepLabel: 'React to the spend' };
			}
			return { waitees: [player(openSpend.spend.player_id)], stepLabel: 'Commit the spend' };
		}
		if (currentActor != null) {
			return { waitees: [player(currentActor)], stepLabel: 'Spend tokens' };
		}
		return { waitees: [] };
	}
	return { waitees: [] };
}
