import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';
import type { Game, Plan, Player, RowState, RowStateKind } from '$lib/api';
import MainEventView from './MainEventView.svelte';

/*
 * The open step's heading replaced the page-level Waiting On bar, which was
 * always drawn. MainEventView draws the heading BY DEFAULT and lets a card, the
 * preparation step or an open roll claim it, so no state can be left without
 * one. Rendering on the server runs no effects, so no one has claimed it: what
 * shows is the default, which is exactly the state a headingless screen would be
 * in. Each row state here is one the old bar covered that has no card of its own
 * (or no card yet).
 */

// useWindowEvents removes its listeners on teardown, and a server render has no window.
beforeAll(() => vi.stubGlobal('window', { addEventListener: () => {}, removeEventListener: () => {} }));
afterAll(() => vi.unstubAllGlobals());

const players = [
	{ id: 1, display_name: 'alice' },
	{ id: 2, display_name: 'bob' },
] as Player[];

const game = {
	id: 1, phase: 'main_event', current_row: 5, focus_player_id: 1, ending_mode: null, facilitator_id: 1,
} as Game;

function view(rowState: RowState | null, currentPlayerID: number | null = 2, plans: Plan[] = []): string {
	const { body } = render(MainEventView, {
		props: {
			game, players, rankings: [], laws: [], rumors: [], assets: [],
			currentPlayerID, recordRows: [], rowState,
			playerNameMap: new Map([[1, 'alice'], [2, 'bob']]),
			activeRoll: null, activeRollDice: [], activeRollVotes: [], activeRollParticipants: [],
			bankedDice: [], plans, planTokens: [],
			onPlansChanged: () => {}, waitingOn: { waitees: [] },
		},
	});
	return body.replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ');
}
const headings = (html: string) => [...html.matchAll(/<h2 class="step-heading[^"]*"[^>]*>([^<]*)<\/h2>/g)].map(m => m[1]);
const state = (kind: RowStateKind, acting = [1]): RowState => ({ kind, acting_player_ids: acting });

describe('the open step always has a heading', () => {
	// None of these has a plan card on screen: a plan pending before it kicks off,
	// a scene before the scene has loaded (so no scene panel yet), a war's cost with
	// no war plan to show, and a plan whose panel has not loaded (no plans fetched).
	const cases: Array<[string, RowState, string]> = [
		['a plan pending before kick-off', state('plan_pending'), 'Waiting on alice — the next step'],
		['a plan resolving whose panel has not loaded', state('plan_resolving'), 'Waiting on alice — the next step'],
		['an active scene before the scene has loaded', state('scene_active'), 'Waiting on alice — the scene'],
		['a cost of battle with no war on screen', state('await_battle_cost'), 'Waiting on alice — pay the cost of battle'],
		['a surrender claim with no war on screen', state('await_surrender_claim'), 'Waiting on alice — surrender-asset claims'],
		['an open roll before it has loaded', state('await_dice_roll'), 'Waiting on alice — the roll'],
		['Make War resolving, which draws its own panel', { ...state('plan_resolving'), plan_id: 9 }, 'Waiting on alice — the next step'],
	];
	it.each(cases)('%s', (_name, rowState, text) => {
		expect(headings(view(rowState))).toEqual([text]);
	});

	// MakeWarPanel draws its own header and never uses the plan's card, so a resolving
	// Make War is the case a "the card draws the heading" design would leave bare.
	it('a resolving Make War plan, whose panel draws no card, is headed too', () => {
		const war = { id: 9, plan_type: 'make_war', status: 'resolving', preparer_id: 1, row_number: 5 } as Plan;
		expect(headings(view({ ...state('plan_resolving'), plan_id: 9 }, 2, [war]))).toEqual(['Waiting on alice — the next step']);
	});

	it('is the viewer\'s own when it is their move', () => {
		expect(headings(view(state('plan_pending', [2])))).toEqual(['Your move — the next step']);
	});

	it('draws exactly one, never two', () => {
		for (const [, rowState] of cases) expect(headings(view(rowState))).toHaveLength(1);
	});

	it('draws none when the game waits on no one (a beat the server ends itself)', () => {
		expect(headings(view({ kind: 'finale_row_complete' }))).toEqual([]);
		expect(headings(view(null))).toEqual([]);
	});
});
