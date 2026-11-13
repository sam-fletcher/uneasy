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

/** Who the game is waiting on, and for what. `stepLabel` finishes the sentence
 *  the open step's heading starts ("Your move — …", "Waiting on otto — …"), so
 *  it is a lower-case phrase that reads after the dash and never repeats the
 *  plan's name (the title bar and the finished steps above already say that). */
export interface WaitingOnState {
	waitees: Waitee[];
	stepLabel?: string;
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
	/** Max assets the focus player may refresh (for the post-scene label). */
	maxRefresh: number;
	/** The viewer. Only a dice roll's wording needs it: the actor's move is the
	 *  roll itself, everyone else's is to aid or interfere with it. */
	currentPlayerID?: number | null;
}

const player = (playerID: number): Waitee => ({ kind: 'player', playerID });

/** The roll step's ask. The actor's move is the roll, and everyone else's is
 *  to aid or interfere; when the actor is the only one left, it is the roll
 *  whoever is looking. */
const rollStepLabel = (
	roll: DiceRoll | null,
	acting: number[],
	viewerID: number | null,
): string => {
	switch (roll?.stage) {
		case 'decide_vote':
			return 'call a difficulty vote?';
		case 'voting':
			return 'difficulty vote';
		case 'leverage': {
			const actor = roll.actor_id;
			if (viewerID != null && viewerID === actor) return 'the roll';
			if (acting.length > 0 && acting.every(id => id === actor)) return 'the roll';
			return 'aid or interfere';
		}
		default:
			return 'the roll';
	}
};

/**
 * Compute who the main-event phase is waiting on, and the phrase that finishes the open step's heading.
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
	const viewerID = input.currentPlayerID ?? null;

	// The server-authoritative acting set. The backend names the exact
	// decision-maker(s) from persisted state for every actor-naming kind —
	// including the generic plan_resolving case (the preparer) — so the bar
	// never guesses or mis-attributes.
	const actingWaitees = (): Waitee[] => (rowState?.acting_player_ids ?? []).map(player);

	switch (rowState?.kind) {
		case 'await_dice_roll':
			return {
					waitees: actingWaitees(),
					stepLabel: rollStepLabel(activeRoll, rowState?.acting_player_ids ?? [], viewerID),
				};
		case 'plan_resolving':
		case 'plan_pending':
			// Resolved by its preparer (named server-side), never the focus
			// player — a delayed plan routinely resolves on a row whose focus is
			// someone else. plan_pending is the brief pre-kickoff/recovery state.
			return { waitees: actingWaitees(), stepLabel: 'the next step' };
		case 'await_demand_counter':
			return { waitees: actingWaitees(), stepLabel: 'the counter-demand' };
		case 'await_demand_draft_pick':
			return { waitees: actingWaitees(), stepLabel: 'draft pick' };
		case 'await_demand_leverage':
			// Control_leverage winner owes the leverage decision on the target
			// plan's open roll. Usually surfaced via the await_dice_roll case
			// above (they're seeded unready on that roll); this is the row-state
			// fallback for the gap before/after the roll itself is open.
			return { waitees: actingWaitees(), stepLabel: 'leverage on the roll' };
		case 'await_demand_retarget':
			// Keep_or_change_target Make Demands winner owes their call on where the
			// target plan is aimed — keeping it is as much a decision as re-aiming it,
			// so the table waits on them either way. On a plan with a roll of its own
			// (Exchange Courtiers, Spread Rumors) it usually surfaces via await_dice_roll
			// above (they're seeded unready on that roll) and this is the fallback. On a
			// Propose Duel or Clandestinely Liaise, which have no such roll, this IS the
			// wait: the plan holds before its first step.
			return { waitees: actingWaitees(), stepLabel: 'keep or change the target' };
		case 'await_plan_reaim':
			// A plan found its prepare-time target gone (destroyed, moved, no longer
			// pending) and holds until the preparer re-aims it — the book's "change
			// the target if the original intent no longer makes sense", forced. The
			// server names the preparer alone: every other route on the plan is
			// refused while it is paused, so nobody else has a move to be waited on.
			return { waitees: actingWaitees(), stepLabel: 'choose a new target' };
		case 'await_festivity_guest_turn':
			return { waitees: actingWaitees(), stepLabel: 'the festivity' };
		case 'await_festivity_challenge_response':
			return { waitees: actingWaitees(), stepLabel: 'answer the challenge' };
		case 'await_duel_staking':
			// Both duellists stake simultaneously; the backend filters to whoever
			// still owes a stake (acting_player_ids).
			return { waitees: actingWaitees(), stepLabel: 'stake for the duel' };
		case 'await_duel_bout':
			return { waitees: actingWaitees(), stepLabel: 'the bout' };
		case 'await_take_consent':
			return { waitees: actingWaitees(), stepLabel: 'consent to the take' };
		case 'await_question_answer':
			return { waitees: actingWaitees(), stepLabel: 'answer the question' };
		case 'liaise_resolving':
			// Collaborative submit phase (secrets / things-we-share /
			// when-will-I-see-you-again). The backend names who still owes a
			// submission, or the preparer once both are in (must advance).
			return { waitees: actingWaitees(), stepLabel: 'the liaison' };
		case 'await_courtier_response':
			// Exchange Courtiers target-driven sub-step (offer / messy break / mar
			// choices / peer claims). Blocks on the target, not the preparer.
			return { waitees: actingWaitees(), stepLabel: 'respond to the exchange' };
		case 'await_introductions_marginalia':
			// Marred Make Introductions — a newcomer's marginalia is somebody
			// else's to write, and until they do the peer is still a draft with no
			// asset behind them.
			return { waitees: actingWaitees(), stepLabel: 'describe a newcomer' };
		case 'await_counter_propaganda':
			// The top interferer was handed a counter-propaganda plan by the
			// preparer's mar: state their message and roll, or decline. Names the
			// interferer, plus whoever still owes a step of the parent plan (the
			// preparer, or a Make Demands perform_steps winner) — the server
			// builds that set (spCounterWaitees); this just renders it.
			return { waitees: actingWaitees(), stepLabel: 'counter-propaganda' };
		case 'await_chronicle_choices':
			// Marred Chronicle Histories — every present player owes one choice.
			return { waitees: actingWaitees(), stepLabel: 'everyone chooses' };
		case 'await_delay_reveal': {
			// Both plans reveal the same way, and the title bar names the plan, so
			// one phrase does. delayRevealPlanType stays in the input for callers
			// that already pass it.
			void delayRevealPlanType;
			return { waitees: actingWaitees(), stepLabel: 'the delay reveal' };
		}
		case 'await_battle_cost':
			return { waitees: actingWaitees(), stepLabel: 'pay the cost of battle' };
		case 'await_surrender_claim':
			return { waitees: actingWaitees(), stepLabel: 'surrender-asset claims' };
		case 'await_endgame_vote':
			// Row 7's advance is paused while the whole table votes on how the
			// game ends; the backend names every seated player who still owes a
			// vote. Nothing shortens this wait — no facilitator override, no
			// timeout, no skip (adr/FACILITATOR_POWERS_AUDIT.md) — so naming the
			// stragglers here is the entire pressure mechanism, alongside the
			// notification system.
			return { waitees: actingWaitees(), stepLabel: 'how will the game end?' };
		case 'await_main_character_choice':
			// One or more players lost their main character; each must choose a
			// replacement before play resumes. The backend names them all.
			return { waitees: actingWaitees(), stepLabel: 'choose a new main character' };
		case 'finale_row_complete':
			// Row 13's plans are all resolved and an Explosive Finale gives that
			// row no turns, so the row is over and nobody is being waited on. The
			// server ends the row itself off this kind, which makes it a beat the
			// bar should stay silent through rather than name anyone.
			return { waitees: [] };
		case 'scene_active':
			return { waitees: actingWaitees(), stepLabel: 'the scene' };
		case 'scene_setting':
			return { waitees: actingWaitees(), stepLabel: 'set the scene' };
		case 'post_scene_action': {
			return {
				waitees: actingWaitees(),
				stepLabel: `prepare a plan, or refresh ${maxRefresh} asset${maxRefresh === 1 ? '' : 's'}`,
			};
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

/** Compute who the shake-up phase is waiting on, and the phrase that finishes the open step's heading. */
export function shakeUpWaitingOn(input: ShakeUpWaitingOnInput): WaitingOnState {
	const { step, currentRollerID, openSpend, currentActor } = input;
	if (step === 1) {
		if (currentRollerID == null) return { waitees: [] };
		return { waitees: [player(currentRollerID)], stepLabel: 'roll for tokens' };
	}
	if (step === 2) {
		if (openSpend) {
			if (!openSpend.commitReady) {
				return { waitees: openSpend.pendingReactorIDs.map(player), stepLabel: 'react to the spend' };
			}
			return { waitees: [player(openSpend.spend.player_id)], stepLabel: 'commit the spend' };
		}
		if (currentActor != null) {
			return { waitees: [player(currentActor)], stepLabel: 'spend tokens' };
		}
		return { waitees: [] };
	}
	return { waitees: [] };
}
