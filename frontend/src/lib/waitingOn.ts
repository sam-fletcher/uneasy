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

import type { DiceRoll, VoteView, RollParticipant, Player, RowState, PlanType } from '$lib/api';

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
	/** The current focus player, or null if none is set yet. */
	focusPlayerID: number | null;
	/** All players (used to name who still owes a difficulty vote). */
	players: Player[];
	/** Active (unresolved) dice roll, or null. An open roll overrides the
	 *  row-state waitees: the table is blocked on the roll's own stage. */
	activeRoll: DiceRoll | null;
	/** Votes cast so far on the active roll. */
	activeRollVotes: VoteView[];
	/** Participants of the active roll (for the leverage/ready stage). */
	activeRollParticipants: RollParticipant[];
	/** plan_type of the open delay-reveal plan, or null when not in that kind. */
	delayRevealPlanType: PlanType | null;
	/** Participants who still owe a hidden die in the delay reveal. */
	delayRevealPendingSubmitterIDs: number[];
	/** War participants who still owe a battle cost on the current row. */
	blockingCostPayers: number[];
	/** Claimants holding an open surrender-asset claim. */
	blockingClaimants: number[];
	/** Max assets the focus player may refresh (for the post-scene subtitle). */
	maxRefresh: number;
}

const player = (playerID: number): Waitee => ({ kind: 'player', playerID });

/** An unresolved dice roll blocks the table on its own stage — whoever still
 *  owes a vote-decision, a difficulty vote, or a leverage/ready submission —
 *  not on the plan's preparer or the focus player. */
function rollWaitingOn(
	roll: DiceRoll,
	players: Player[],
	votes: VoteView[],
	participants: RollParticipant[],
): WaitingOnState {
	switch (roll.stage) {
		case 'decide_vote':
			return { waitees: [player(roll.actor_id)], stepLabel: 'Dice roll — call a vote?' };
		case 'voting': {
			const voted = new Set(votes.map((v) => v.player_id));
			return {
				waitees: players.filter((p) => !voted.has(p.id)).map((p) => player(p.id)),
				stepLabel: 'Dice roll — difficulty vote',
			};
		}
		case 'leverage':
			return {
				waitees: participants.filter((p) => !p.is_ready).map((p) => player(p.player_id)),
				stepLabel: 'Dice roll — leverage & ready',
			};
		default:
			return { waitees: [] };
	}
}

/**
 * Compute the WaitingOnBar state for the main-event phase.
 *
 * Precedence: an open dice roll wins outright; otherwise the server's RowState
 * kind selects the waitees. Actor-naming kinds (plan_resolving and every
 * sub-phase gate) read `acting_player_ids` verbatim — the backend has already
 * named the exact decision-maker(s), so there is no preparer/focus fallback to
 * mis-attribute the wait. Focus-player kinds (scenes, post-scene) name the
 * focus player; the row-advance gates name the cost-payers / claimants.
 */
export function mainEventWaitingOn(input: MainEventWaitingOnInput): WaitingOnState {
	const {
		rowState,
		focusPlayerID,
		players,
		activeRoll,
		activeRollVotes,
		activeRollParticipants,
		delayRevealPlanType,
		delayRevealPendingSubmitterIDs,
		blockingCostPayers,
		blockingClaimants,
		maxRefresh,
	} = input;

	if (activeRoll != null && activeRoll.outcome == null) {
		return rollWaitingOn(activeRoll, players, activeRollVotes, activeRollParticipants);
	}

	const focusWaitee: Waitee[] = focusPlayerID != null ? [player(focusPlayerID)] : [];

	// The server-authoritative acting set. The backend names the exact
	// decision-maker(s) from persisted state for every actor-naming kind —
	// including the generic plan_resolving case (the preparer) — so the bar
	// never guesses or mis-attributes.
	const actingWaitees = (): Waitee[] => (rowState?.acting_player_ids ?? []).map(player);

	switch (rowState?.kind) {
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
			return { waitees: delayRevealPendingSubmitterIDs.map(player), stepLabel: label };
		}
		case 'await_battle_cost':
		case 'await_surrender_claim': {
			const ids = new Set<number>([...blockingCostPayers, ...blockingClaimants]);
			const parts: string[] = [];
			if (blockingCostPayers.length > 0) parts.push('cost of battle');
			if (blockingClaimants.length > 0) parts.push('surrender-asset claims');
			return {
				waitees: [...ids].map(player),
				stepLabel: 'Row advance blocked',
				stepSubtitle: parts.join(' · '),
			};
		}
		case 'scene_active':
			return { waitees: focusWaitee, stepLabel: 'Scene' };
		case 'scene_setting':
			return { waitees: focusWaitee, stepLabel: 'Set the scene' };
		case 'post_scene_action': {
			const subtitle = `or refresh ${maxRefresh} asset${maxRefresh === 1 ? '' : 's'}`;
			return { waitees: focusWaitee, stepLabel: 'Prepare a plan', stepSubtitle: subtitle };
		}
	}
	return { waitees: [] };
}
