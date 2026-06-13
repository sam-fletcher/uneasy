import { describe, it, expect } from 'vitest';
import type { DiceRoll, Player, RollParticipant, RowState, RowStateKind, VoteView } from '$lib/api';
import { mainEventWaitingOn, type MainEventWaitingOnInput, type Waitee } from './waitingOn';

// ── Fixtures ───────────────────────────────────────────────────────────────
// The derivation reads only a handful of fields off each input; minimal casts
// keep the fixtures focused on what actually drives the logic.

const player = (id: number) => ({ id }) as unknown as Player;
const vote = (player_id: number) => ({ player_id }) as unknown as VoteView;
const participant = (player_id: number, is_ready: boolean) =>
	({ player_id, is_ready }) as unknown as RollParticipant;
const roll = (over: Partial<DiceRoll>) => ({ stage: 'voting', outcome: null, ...over }) as DiceRoll;

function rowState(kind: RowStateKind, over: Partial<RowState> = {}): RowState {
	return { kind, ...over };
}

/** Build a full input with empty/neutral defaults; override per test. */
function input(over: Partial<MainEventWaitingOnInput> = {}): MainEventWaitingOnInput {
	return {
		rowState: null,
		focusPlayerID: null,
		players: [],
		activeRoll: null,
		activeRollVotes: [],
		activeRollParticipants: [],
		delayRevealPlanType: null,
		delayRevealPendingSubmitterIDs: [],
		blockingCostPayers: [],
		blockingClaimants: [],
		maxRefresh: 0,
		...over,
	};
}

/** IDs of the player-kind waitees, in order. */
const playerIDs = (waitees: Waitee[]): number[] =>
	waitees.filter((w): w is { kind: 'player'; playerID: number } => w.kind === 'player').map((w) => w.playerID);

// ── The founding regression ──────────────────────────────────────────────────
// The June 2026 bug: while a plan resolved, the bar named the focus player (a
// bystander) instead of the actor. The fix made the backend name the preparer
// in acting_player_ids; this layer must read it verbatim and NEVER fall back to
// the focus player.

describe('plan_resolving — the founding regression', () => {
	it('names the preparer from acting_player_ids, not the focus player', () => {
		// Focus is a third party (7); the resolving plan belongs to preparer 3.
		const got = mainEventWaitingOn(
			input({
				focusPlayerID: 7,
				rowState: rowState('plan_resolving', { plan_id: 99, acting_player_ids: [3] }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(playerIDs(got.waitees)).not.toContain(7); // never the focus bystander
		expect(got.stepLabel).toBe('Resolving plan');
	});

	it('does NOT fall back to the focus player when the acting set is empty', () => {
		// The old client-side proxy used focus as a fallback — that was the bug.
		// With no acting set, the honest answer is "no one named", not "focus".
		const got = mainEventWaitingOn(
			input({ focusPlayerID: 7, rowState: rowState('plan_resolving', { acting_player_ids: [] }) }),
		);
		expect(got.waitees).toEqual([]);
	});

	it('plan_pending names the preparer the same way', () => {
		const got = mainEventWaitingOn(
			input({ focusPlayerID: 7, rowState: rowState('plan_pending', { acting_player_ids: [3] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('Resolving plan');
	});
});

// ── Sub-phase gates read acting_player_ids verbatim ──────────────────────────

describe('actor-naming sub-phase kinds', () => {
	const cases: Array<{ kind: RowStateKind; label: string }> = [
		{ kind: 'await_demand_counter', label: 'Make Demands — awaiting counter' },
		{ kind: 'await_demand_draft_pick', label: 'Make Demands — draft pick' },
		{ kind: 'await_festivity_guest_turn', label: 'Host Festivity — guest turn' },
		{ kind: 'await_festivity_challenge_response', label: 'Host Festivity — challenge response' },
		{ kind: 'await_duel_bout', label: 'Propose Duel — bout' },
		{ kind: 'await_take_consent', label: 'Spread Rumors — consent to take asset' },
		{ kind: 'await_question_answer', label: 'Seek Answers — answer a question' },
		{ kind: 'await_courtier_response', label: 'Exchange Courtiers — target responds' },
	];
	it.each(cases)('$kind → names acting_player_ids with label "$label"', ({ kind, label }) => {
		const got = mainEventWaitingOn(
			input({ focusPlayerID: 7, rowState: rowState(kind, { acting_player_ids: [5] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([5]);
		expect(got.stepLabel).toBe(label);
	});

	it('multi-actor kinds list every named player (duel staking)', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_duel_staking', { acting_player_ids: [3, 4] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3, 4]);
		expect(got.stepLabel).toBe('Propose Duel — staking');
	});

	it('liaise_resolving and await_chronicle_choices list every named player', () => {
		expect(
			playerIDs(
				mainEventWaitingOn(
					input({ rowState: rowState('liaise_resolving', { acting_player_ids: [1, 2] }) }),
				).waitees,
			),
		).toEqual([1, 2]);
		expect(
			playerIDs(
				mainEventWaitingOn(
					input({ rowState: rowState('await_chronicle_choices', { acting_player_ids: [2, 9] }) }),
				).waitees,
			),
		).toEqual([2, 9]);
	});
});

// ── Delay reveal ─────────────────────────────────────────────────────────────

describe('await_delay_reveal', () => {
	it('names the pending submitters and labels by plan type (make_war)', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_delay_reveal'),
				delayRevealPlanType: 'make_war',
				delayRevealPendingSubmitterIDs: [1, 2],
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 2]);
		expect(got.stepLabel).toBe('Make War — delay reveal');
	});

	it('labels clandestinely_liaise', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_delay_reveal'), delayRevealPlanType: 'clandestinely_liaise' }),
		);
		expect(got.stepLabel).toBe('Clandestinely Liaise — delay reveal');
	});

	it('falls back to a generic label for an unknown plan type', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_delay_reveal'), delayRevealPlanType: null }),
		);
		expect(got.stepLabel).toBe('Delay reveal');
	});
});

// ── Row-advance gates ────────────────────────────────────────────────────────

describe('await_battle_cost / await_surrender_claim', () => {
	it('unions cost-payers and claimants, dedupes, and builds the subtitle', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_battle_cost'),
				blockingCostPayers: [1, 2],
				blockingClaimants: [2, 3],
			}),
		);
		expect(playerIDs(got.waitees).sort()).toEqual([1, 2, 3]);
		expect(got.stepLabel).toBe('Row advance blocked');
		expect(got.stepSubtitle).toBe('cost of battle · surrender-asset claims');
	});

	it('only shows the relevant subtitle part when one source is empty', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_surrender_claim'), blockingClaimants: [3] }),
		);
		expect(got.stepSubtitle).toBe('surrender-asset claims');
	});
});

// ── Focus-player kinds ───────────────────────────────────────────────────────

describe('focus-player kinds', () => {
	it('scene_active / scene_setting name the focus player', () => {
		expect(
			playerIDs(mainEventWaitingOn(input({ focusPlayerID: 4, rowState: rowState('scene_active') })).waitees),
		).toEqual([4]);
		expect(
			playerIDs(mainEventWaitingOn(input({ focusPlayerID: 4, rowState: rowState('scene_setting') })).waitees),
		).toEqual([4]);
	});

	it('post_scene_action names the focus player and pluralizes the refresh subtitle', () => {
		const one = mainEventWaitingOn(input({ focusPlayerID: 4, rowState: rowState('post_scene_action'), maxRefresh: 1 }));
		expect(playerIDs(one.waitees)).toEqual([4]);
		expect(one.stepSubtitle).toBe('or refresh 1 asset');
		const many = mainEventWaitingOn(input({ focusPlayerID: 4, rowState: rowState('post_scene_action'), maxRefresh: 3 }));
		expect(many.stepSubtitle).toBe('or refresh 3 assets');
	});

	it('names no one when there is no focus player', () => {
		expect(mainEventWaitingOn(input({ focusPlayerID: null, rowState: rowState('scene_setting') })).waitees).toEqual([]);
	});
});

// ── Active dice roll overrides the row-state ──────────────────────────────────

describe('an unresolved dice roll overrides the row-state waitees', () => {
	it('decide_vote names the roll actor', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('plan_resolving', { acting_player_ids: [3] }),
				activeRoll: roll({ stage: 'decide_vote', actor_id: 8 }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([8]); // the roll, not the plan preparer
		expect(got.stepLabel).toBe('Dice roll — call a vote?');
	});

	it('voting names players who have not yet voted', () => {
		const got = mainEventWaitingOn(
			input({
				players: [player(1), player(2), player(3)],
				activeRoll: roll({ stage: 'voting' }),
				activeRollVotes: [vote(2)],
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 3]);
		expect(got.stepLabel).toBe('Dice roll — difficulty vote');
	});

	it('leverage names participants who are not ready', () => {
		const got = mainEventWaitingOn(
			input({
				activeRoll: roll({ stage: 'leverage' }),
				activeRollParticipants: [participant(1, true), participant(2, false)],
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([2]);
		expect(got.stepLabel).toBe('Dice roll — leverage & ready');
	});

	it('a RESOLVED roll (outcome set) does not override — row-state wins', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('plan_resolving', { acting_player_ids: [3] }),
				activeRoll: roll({ stage: 'leverage', outcome: 'make' }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('Resolving plan');
	});
});

// ── Loading / unknown states ──────────────────────────────────────────────────

describe('empty states', () => {
	it('null rowState (still loading) → no waitees', () => {
		expect(mainEventWaitingOn(input({ rowState: null })).waitees).toEqual([]);
	});
});
