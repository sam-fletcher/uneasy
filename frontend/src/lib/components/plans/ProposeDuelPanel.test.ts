import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';
import ProposeDuelPanel from './ProposeDuelPanel.svelte';
import type { PlanContext } from './types';

// The duel's difficulty was a bare number tacked on the end of the "ravi vs sol" line.
// It is the shared meter now (the challenged player's esteem status: on a mar, that many
// of the loser's stakes change hands), and a "What can happen" says what a make and a
// mar do. Both are for everyone until the dice land.

// The panel listens for the duel's window events and lets go of them when it is destroyed,
// which a server render does too: give it a window to subscribe to.
beforeAll(() => { vi.stubGlobal('window', { addEventListener() {}, removeEventListener() {} }); });
afterAll(() => { vi.unstubAllGlobals(); });

const CHALLENGER = 1;
const CHALLENGED = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

const PEERS = [
	{ id: 11, owner_id: CHALLENGER, asset_type: 'peer', name: 'Ser Ott', is_main_character: true, is_destroyed: false },
	{ id: 12, owner_id: CHALLENGED, asset_type: 'peer', name: 'Dame Orr', is_main_character: true, is_destroyed: false },
];

function renderDuel(opts: { seat: number; phase: 'setup' | 'bouts' | 'roll' | 'done'; peers?: boolean; outcome?: 'make' | 'mar' | null }): string {
	const plan = {
		id: 9, plan_type: 'propose_duel', status: 'resolving', preparer_id: CHALLENGER, target_player_id: CHALLENGED,
		preparation_notes: 'at dawn', resolution_data: JSON.stringify({ duel: { phase: opts.phase, duel_type: 'arms' } }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: (opts.peers ? PEERS : []) as never,
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

	it('is no longer a bare figure on a "ravi vs sol" line: the pairing is a set piece, and the meter shows it', () => {
		const t = textOf(renderDuel({ seat: BYSTANDER, phase: 'setup' }));
		expect(t).not.toContain('ravi vs sol');
		expect(t).toMatch(/ravi\s+faces\s+sol/);
		expect(t).not.toMatch(/sol\s*·\s*difficulty/);
		expect(t).toMatch(/Difficulty 4/);
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) expect(textOf(renderDuel({ seat: BYSTANDER, phase: 'roll', outcome: o }))).not.toContain('1 easy');
	});
});

describe('Propose Duel: the pairing after setup', () => {
	// Owner ruling 2026-11-25: the set piece is for the setup step; later steps carry what
	// setup settled as plain text in its finished line.
	it('is the set piece in setup, with no finished line yet', () => {
		const html = renderDuel({ seat: BYSTANDER, phase: 'setup', peers: true });
		expect(html).toContain('<section class="set-piece ');
		expect(textOf(html)).not.toContain('Stakes set');
	});

	it.each(['bouts', 'roll', 'done'] as const)('is a plain finished line in the %s step, with no set piece', (phase) => {
		for (const seat of [CHALLENGER, CHALLENGED, BYSTANDER]) {
			const html = renderDuel({ seat, phase, peers: true });
			expect(html).not.toContain('set-piece');
			// (textOf puts a space where a tag was, so the possessive's gap is the helper's)
			expect(textOf(html).replace(/ 's/g, "'s")).toContain("Stakes set: ravi's Ser Ott faces sol's Dame Orr");
			expect(html.match(/Stakes set/g)).toHaveLength(1);
		}
	});

	it('stands a player with no peer on their own name', () => {
		expect(textOf(renderDuel({ seat: BYSTANDER, phase: 'bouts' }))).toContain('Stakes set: ravi faces sol');
	});
});

describe('Propose Duel: "What can happen"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen/;

	it.each(['setup', 'bouts'] as const)('is a collapsed row for everyone in the %s step', (phase) => {
		for (const seat of [CHALLENGER, CHALLENGED, BYSTANDER]) expect(renderDuel({ seat, phase })).toMatch(toggle);
	});


	it('is gone once the dice have landed: the claim step says what each side may take', () => {
		for (const o of ['make', 'mar'] as const) expect(renderDuel({ seat: BYSTANDER, phase: 'roll', outcome: o })).not.toContain('What can happen');
	});
});
