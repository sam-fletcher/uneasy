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
		expect(got.stepLabel).toBe('Resolving plan');
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
		expect(got.stepLabel).toBe('Resolving plan');
	});
});

// ── Sub-phase gates read acting_player_ids verbatim ──────────────────────────

describe('actor-naming sub-phase kinds', () => {
	const cases: Array<{ kind: RowStateKind; label: string }> = [
		{ kind: 'await_demand_counter', label: 'Make Demands — awaiting counter' },
		{ kind: 'await_demand_draft_pick', label: 'Make Demands — draft pick' },
		{ kind: 'await_demand_leverage', label: 'Make Demands — control leverage' },
		{ kind: 'await_festivity_guest_turn', label: 'Host Festivity — in progress' },
		{ kind: 'await_festivity_challenge_response', label: 'Host Festivity — challenge response' },
		{ kind: 'await_duel_bout', label: 'Propose Duel — bout' },
		{ kind: 'await_take_consent', label: 'Spread Rumors — consent to take asset' },
		{ kind: 'await_question_answer', label: 'Seek Answers — answer a question' },
		{ kind: 'await_courtier_response', label: 'Exchange Courtiers — target responds' },
		{ kind: 'await_main_character_choice', label: 'Choose a new main character' },
	];
	it.each(cases)('$kind → names acting_player_ids with label "$label"', ({ kind, label }) => {
		const got = mainEventWaitingOn(input({ rowState: rowState(kind, { acting_player_ids: [5] }) }));
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
		expect(got.stepLabel).toBe('Dice roll — call a vote?');
	});

	it('voting label', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [1, 3] }),
				activeRoll: roll({ stage: 'voting' }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([1, 3]);
		expect(got.stepLabel).toBe('Dice roll — difficulty vote');
	});

	it('leverage label', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_dice_roll', { acting_player_ids: [2] }),
				activeRoll: roll({ stage: 'leverage' }),
			}),
		);
		expect(playerIDs(got.waitees)).toEqual([2]);
		expect(got.stepLabel).toBe('Dice roll — leverage & ready');
	});

	it('falls back to a generic label when activeRoll has not loaded yet', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_dice_roll', { acting_player_ids: [2] }), activeRoll: null }),
		);
		expect(got.stepLabel).toBe('Dice roll');
	});
});

// ── Delay reveal ─────────────────────────────────────────────────────────────

describe('await_delay_reveal', () => {
	it('names acting_player_ids and labels by plan type (make_war)', () => {
		const got = mainEventWaitingOn(
			input({
				rowState: rowState('await_delay_reveal', { acting_player_ids: [1, 2] }),
				delayRevealPlanType: 'make_war',
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
	it('await_battle_cost names acting_player_ids with the cost-of-battle subtitle', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_battle_cost', { acting_player_ids: [1, 2] }) }),
		);
		expect(playerIDs(got.waitees).sort()).toEqual([1, 2]);
		expect(got.stepLabel).toBe('Row advance blocked');
		expect(got.stepSubtitle).toBe('cost of battle');
	});

	it('await_surrender_claim names acting_player_ids with the surrender-claim subtitle', () => {
		const got = mainEventWaitingOn(
			input({ rowState: rowState('await_surrender_claim', { acting_player_ids: [3] }) }),
		);
		expect(playerIDs(got.waitees)).toEqual([3]);
		expect(got.stepLabel).toBe('Row advance blocked');
		expect(got.stepSubtitle).toBe('surrender-asset claims');
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

	it('post_scene_action names acting_player_ids and pluralizes the refresh subtitle', () => {
		const one = mainEventWaitingOn(
			input({ rowState: rowState('post_scene_action', { acting_player_ids: [4] }), maxRefresh: 1 }),
		);
		expect(playerIDs(one.waitees)).toEqual([4]);
		expect(one.stepSubtitle).toBe('or refresh 1 asset');
		const many = mainEventWaitingOn(
			input({ rowState: rowState('post_scene_action', { acting_player_ids: [4] }), maxRefresh: 3 }),
		);
		expect(many.stepSubtitle).toBe('or refresh 3 assets');
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
		expect(got.stepLabel).toBe('Roll for tokens');
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
		expect(got.stepLabel).toBe('Commit the spend');
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
		expect(got.stepLabel).toBe('React to the spend');
	});

	it('step 2 with no open spend names the current actor', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 2, currentActor: 4 }));
		expect(playerIDs(got.waitees)).toEqual([4]);
		expect(got.stepLabel).toBe('Spend tokens');
	});

	it('step 2 with no open spend and no current actor (no one holds tokens) → no waitees', () => {
		const got = shakeUpWaitingOn(shakeUpInput({ step: 2 }));
		expect(got.waitees).toEqual([]);
	});

	it('unknown/null step → no waitees', () => {
		expect(shakeUpWaitingOn(shakeUpInput({ step: null })).waitees).toEqual([]);
	});
});
