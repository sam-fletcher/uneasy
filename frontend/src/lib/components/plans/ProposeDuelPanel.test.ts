import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';
import ProposeDuelPanel from './ProposeDuelPanel.svelte';
import type { PlanContext } from './types';

// The duel's difficulty was a bare number tacked on the end of the "ravi vs sol" line.
// It is the shared meter now (the challenged player's esteem status: on a mar, that many
// of the loser's stakes change hands), and a "What can happen?" says what a make and a
// mar do. Both are for everyone until the dice land.

// The panel listens for the duel's window events and lets go of them when it is destroyed,
// which a server render does too: give it a window to subscribe to.
beforeAll(() => { vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} }); });
afterAll(() => { vi.unstubAllGlobals(); });

const CHALLENGER = 1;
const CHALLENGED = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

function renderDuel(opts: { seat: number; phase: 'setup' | 'bouts' | 'roll'; outcome?: 'make' | 'mar' | null }): string {
	const plan = {
		id: 9, plan_type: 'propose_duel', status: 'resolving', preparer_id: CHALLENGER, target_player_id: CHALLENGED,
		preparation_notes: 'at dawn', resolution_data: JSON.stringify({ duel: { phase: opts.phase, duel_type: 'arms' } }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [] as never,
		players: [{ id: CHALLENGER, display_name: 'ravi' }, { id: CHALLENGED, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' }] as never,
		rankings: [{ id: 1, game_id: 1, category: 'esteem', player_id: CHALLENGED, rank: 2 }] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: opts.outcome ?? null, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ProposeDuelPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Propose Duel: the difficulty', () => {
	it.each(['setup', 'bouts'] as const)('is the challenged player\'s esteem status, for everyone, in the %s step', (phase) => {
		for (const seat of [CHALLENGER, CHALLENGED, BYSTANDER]) {
			const t = textOf(renderDuel({ seat, phase }));
			expect(t).toContain('Difficulty 4');
			expect(t).toContain('sol’s status on the esteem track: 6 minus rank 2 is 4');
		}
	});

	it('is no longer a bare figure on the "ravi vs sol" line', () => {
		const t = textOf(renderDuel({ seat: BYSTANDER, phase: 'setup' }));
		expect(t).toContain('ravi vs sol');
		expect(t).not.toMatch(/ravi vs sol\s*·\s*difficulty/);
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) expect(textOf(renderDuel({ seat: BYSTANDER, phase: 'roll', outcome: o }))).not.toContain('1 easy');
	});
});

describe('Propose Duel: "What can happen?"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen\?/;

	it.each(['setup', 'bouts'] as const)('is a collapsed row for everyone in the %s step', (phase) => {
		for (const seat of [CHALLENGER, CHALLENGED, BYSTANDER]) expect(renderDuel({ seat, phase })).toMatch(toggle);
	});

	it('is gone once the dice have landed: the claim step says what each side may take', () => {
		for (const o of ['make', 'mar'] as const) expect(renderDuel({ seat: BYSTANDER, phase: 'roll', outcome: o })).not.toContain('What can happen?');
	});
});
