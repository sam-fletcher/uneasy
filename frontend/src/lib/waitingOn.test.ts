import { describe, it, expect } from 'vitest';
import type { DiceRoll, RowState, RowStateKind } from '$lib/api';
import {
	mainEventWaitingOn, type MainEventWaitingOnInput, type Waitee,
	shakeUpWaitingOn, type ShakeUpWaitingOnInput,
} from './waitingOn';

// ── Fixtures ───────────────────────────────────────────────────────────────
// The derivation reads only a handful of fields off each input; minimal casts
// keep the fixtures focused on what actually drives the logic.

const roll = (over: Partial<DiceRoll>) => ({ stage: 'voting', outcome: null, ...over }) as DiceRoll;

function rowState(kind: RowStateKind, over: Partial<RowState> = {}): RowState {
	return { kind, ...over };
}

/** Build a full input with empty/neutral defaults; override per test. */
function input(over: Partial<MainEventWaitingOnInput> = {}): MainEventWaitingOnInput {
	return {
		rowState: null,
		activeRoll: null,
		delayRevealPlanType: null,
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
		// The resolving plan belongs to preparer 3; nothing here names 7.
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('plan_resolving', { plan_id: 99, acting_player_ids: [3] }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(playerIDs(got.waitees)).not.toContain(7); // never a focus bystander
		expect(got.stepLabel).toBe('the next step');
	});

	it('does NOT fall back to a focus player when the acting set is empty', () => {
		// The old client-side proxy used focus as a fallback — that was the bug.
		// With no acting set, the honest answer is "no one named", not "focus".
		const got = mainEventWaitingOn(
			input({ rowState: rowState('plan_resolving', { acting_player_ids: [] }) }),
		);
		expect(got.waitees).toEqual([]);
	});

	it('plan_pending names the preparer the same way', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('plan_pending', { acting_player_ids: [3] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('the next step');
	});
});

// ── Sub-phase gates read acting_player_ids verbatim ──────────────────────────

describe('actor-naming sub-phase kinds', () => {
	const cases: Array<{ kind: RowStateKind; label: string }> = [
		{ kind: 'await_demand_counter', label: 'the counter-demand' },
		{ kind: 'await_demand_draft_pick', label: 'draft pick' },
		{ kind: 'await_demand_leverage', label: 'leverage on the roll' },
		{ kind: 'await_demand_retarget', label: 'keep or change the target' },
		{ kind: 'await_festivity_guest_turn', label: 'the festivity' },
		{ kind: 'await_festivity_challenge_response', label: 'answer the challenge' },
		{ kind: 'await_duel_bout', label: 'the bout' },
		{ kind: 'await_take_consent', label: 'consent to the take' },
		{ kind: 'await_question_answer', label: 'answer the question' },
		{ kind: 'await_courtier_response', label: 'respond to the exchange' },
		{ kind: 'await_plan_reaim', label: 'choose a new target' },
		{ kind: 'await_main_character_choice', label: 'choose a new main character' },
		{ kind: 'await_endgame_vote', label: 'how will the game end?' },
	];
	it.each(cases)('$kind → names acting_player_ids with label "$label"', ({ kind, label }) => {
		const got = mainEventWaitingOn(input({ rowState: rowState(kind, { acting_player_ids: [5] }) }));
		expect(playerIDs(got.waitees)).toEqual([5]);
		expect(got.stepLabel).toBe(label);
	});

	it('await_endgame_vote names every player who still owes a vote', () => {
		// Everyone votes, so the bar routinely names the whole table — and there
		// is nothing that shortens the wait, which is why naming them matters.
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_endgame_vote', { acting_player_ids: [1, 2, 3] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 2, 3]);
		expect(got.stepLabel).toBe('how will the game end?');
	});

	it('multi-actor kinds list every named player (duel staking)', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_duel_staking', { acting_player_ids: [3, 4] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3, 4]);
		expect(got.stepLabel).toBe('stake for the duel');
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

	it('await_counter_propaganda names whoever the server lists', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_counter_propaganda', { acting_player_ids: [5, 9] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([5, 9]);
		expect(got.stepLabel).toBe('counter-propaganda');
	});

	it('await_introductions_marginalia names whoever owes a newcomer’s marginalia', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_introductions_marginalia', { acting_player_ids: [4, 5] }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([4, 5]);
		expect(got.stepLabel).toBe('describe a newcomer');
	});
});

// ── Dice roll ────────────────────────────────────────────────────────────────
// Session 1 (adr/NOTIFICATIONS_PLAN.md) moved the roll override server-side:
// await_dice_roll is just another rowState.kind, reading acting_player_ids
// like every other kind. activeRoll now survives only for its `stage`, to
// pick the step label.

describe('await_dice_roll', () => {
	it('decide_vote label, actor named via acting_player_ids', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { roll_id: 1, acting_player_ids: [8] }),
				activeRoll: roll({ stage: 'decide_vote', actor_id: 8 }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([8]);
		expect(got.stepLabel).toBe('call a difficulty vote?');
	});

	it('voting label', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [1, 3] }),
				activeRoll: roll({ stage: 'voting' }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 3]);
		expect(got.stepLabel).toBe('difficulty vote');
	});

	// The leverage step's ask depends on who is being asked: everyone but the
	// actor aids or interferes, and the actor's move is the roll itself.
	it('leverage: a non-actor is asked to aid or interfere', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [2, 8] }),
				activeRoll: roll({ stage: 'leverage', actor_id: 8 }),
				currentPlayerID: 2,
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([2, 8]);
		expect(got.stepLabel).toBe('aid or interfere');
	});

	it('leverage: the actor, looking, is asked for the roll even while others are unready', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [2, 8] }),
				activeRoll: roll({ stage: 'leverage', actor_id: 8 }),
				currentPlayerID: 8,
			}),
		);
		expect(got.stepLabel).toBe('the roll');
	});

	it('leverage: once only the actor is left, it is the roll whoever is looking', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [8] }),
				activeRoll: roll({ stage: 'leverage', actor_id: 8 }),
				currentPlayerID: 2,
			}),
		);
		expect(got.stepLabel).toBe('the roll');
	});

	it('falls back to "the roll" when activeRoll has not loaded yet', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_dice_roll', { acting_player_ids: [2] }), activeRoll: null }),
		);
		expect(got.stepLabel).toBe('the roll');
	});
});

// ── Delay reveal ─────────────────────────────────────────────────────────────

describe('await_delay_reveal', () => {
	it('names acting_player_ids and labels the reveal (make_war)', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_delay_reveal', { acting_player_ids: [1, 2] }),
				delayRevealPlanType: 'make_war',
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 2]);
		expect(got.stepLabel).toBe('the delay reveal');
	});

	it('labels clandestinely_liaise the same way', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_delay_reveal'), delayRevealPlanType: 'clandestinely_liaise' }),
		);
		expect(got.stepLabel).toBe('the delay reveal');
	});

	it('needs no plan type to label the reveal', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_delay_reveal'), delayRevealPlanType: null }),
		);
		expect(got.stepLabel).toBe('the delay reveal');
	});
});

// ── Row-advance gates ────────────────────────────────────────────────────────

describe('await_battle_cost / await_surrender_claim', () => {
	it('await_battle_cost names acting_player_ids and the cost of battle', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_battle_cost', { acting_player_ids: [1, 2] }) }),
		);
		expect(playerIDs(got.waitees).sort()).toEqual([1, 2]);
		expect(got.stepLabel).toBe('pay the cost of battle');
	});

	it('await_surrender_claim names acting_player_ids and the surrender claims', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_surrender_claim', { acting_player_ids: [3] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('surrender-asset claims');
	});
});

// ── Focus-player kinds ───────────────────────────────────────────────────────
// These now read acting_player_ids like every other kind — the backend fills
// it with the focus player (or nothing, if none is set yet). There is no
// client-side focus fallback left.

describe('focus-player kinds', () => {
	it('scene_active / scene_setting name acting_player_ids', () => {
		expect(
			playerIDs(
				mainEventWaitingOn(input({ rowState: rowState('scene_active', { acting_player_ids: [4] }) }))
					.waitees,
			),
		).toEqual([4]);
		expect(
			playerIDs(
				mainEventWaitingOn(input({ rowState: rowState('scene_setting', { acting_player_ids: [4] }) }))
					.waitees,
			),
		).toEqual([4]);
	});

	it('post_scene_action names acting_player_ids and pluralizes the refresh offer', () => {
		const one = mainEventWaitingOn(
			input({ rowState: rowState('post_scene_action', { acting_player_ids: [4] }), maxRefresh: 1 }),
		);
		expect(playerIDs(one.waitees)).toEqual([4]);
		expect(one.stepLabel).toBe('prepare a plan, or refresh 1 asset');
		const many = mainEventWaitingOn(
			input({ rowState: rowState('post_scene_action', { acting_player_ids: [4] }), maxRefresh: 3 }),
		);
		expect(many.stepLabel).toBe('prepare a plan, or refresh 3 assets');
	});

	it('names no one when the backend sends an empty acting set (no focus player yet)', () => {
		expect(
			mainEventWaitingOn(input({ rowState: rowState('scene_setting', { acting_player_ids: [] }) }))
				.waitees,
		).toEqual([]);
	});
});

// ── Loading / unknown states ──────────────────────────────────────────────────

describe('empty states', () => {
	it('null rowState (still loading) → no waitees', () => {
		expect(mainEventWaitingOn(input({ rowState: null })).waitees).toEqual([]);
	});
});

// ── Shake-Up ─────────────────────────────────────────────────────────────────
// Both steps are strictly sequential (reverse rank order), so the derivation
// should never fall back to naming "everyone" the way the old self-reported
// rolling skeleton did.

function shakeUpInput(over: Partial<ShakeUpWaitingOnInput> = {}): ShakeUpWaitingOnInput {
	return {
		step: null,
		currentRollerID: null,
		openSpend: null,
		currentActor: null,
		...over,
	};
}

describe('shakeUpWaitingOn', () => {
	it('step 1 names the current roller', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 1, currentRollerID: 5 }));
		expect(playerIDs(got.waitees)).toEqual([5]);
		expect(got.stepLabel).toBe('roll for tokens');
	});

	it('step 1 with no current roller (everyone has rolled) → no waitees', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 1, currentRollerID: null }));
		expect(got.waitees).toEqual([]);
	});

	it('step 2 with an open spend, all reactors clear, names the spender (not the next actor)', () => {
		const got = shakeUpWaitingOn(
			shakeUpInput({
				step: 2,
				openSpend: { spend: { player_id: 3 }, pendingReactorIDs: [], commitReady: true },
				currentActor: 9,
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('commit the spend');
	});

	it('step 2 with an open spend and pending reactors names them, not the spender', () => {
		const got = shakeUpWaitingOn(
			shakeUpInput({
				step: 2,
				openSpend: { spend: { player_id: 3 }, pendingReactorIDs: [1, 2], commitReady: false },
				currentActor: 9,
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 2]);
		expect(got.stepLabel).toBe('react to the spend');
	});

	it('step 2 with no open spend names the current actor', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 2, currentActor: 4 }));
		expect(playerIDs(got.waitees)).toEqual([4]);
		expect(got.stepLabel).toBe('spend tokens');
	});

	it('step 2 with no open spend and no current actor (no one holds tokens) → no waitees', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 2 }));
		expect(got.waitees).toEqual([]);
	});

	it('unknown/null step → no waitees', () => {
		expect(shakeUpWaitingOn(shakeUpInput({ step: null })).waitees).toEqual([]);
	});
});

// ── The Explosive Finale's row 13 ────────────────────────────────────────────
// Row 13's plans are all resolved and the mode gives that row no turns, so
// nobody is being waited on. The server ends the row itself off this kind, which
// makes it the one main-event kind the bar stays silent through.
describe('finale_row_complete', () => {
	it('names nobody — the row is over, not waiting on anyone', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('finale_row_complete', { acting_player_ids: [] }) }),
		);
		expect(got.waitees).toEqual([]);
		expect(got.stepLabel).toBeUndefined();
	});
});
