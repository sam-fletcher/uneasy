import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import DiceRollPanel from './DiceRollPanel.svelte';
import type { DiceRollDie, RollIntent, RollParticipant } from '$lib/api';

// The footer tally counts players by side, in the same unit as the
// "N of M ready" beside it. It once counted dice in each pool, so the
// actor's two base dice read as "2 aid" and a player who had picked
// interfere but not yet committed a die read as "0 interfere".

const ACTOR = 2;
const players = [
	{ id: 1, display_name: 'otto' },
	{ id: 2, display_name: 'wren' },
	{ id: 3, display_name: 'pia' },
	{ id: 4, display_name: 'quin' },
] as never;
const playerNameMap = new Map([[1, 'otto'], [2, 'wren'], [3, 'pia'], [4, 'quin']]);

let nextDie = 1;
const die = (player_id: number, extra: Partial<DiceRollDie> = {}): DiceRollDie =>
	({
		id: nextDie++, roll_id: 9, player_id, is_interference: false,
		leveraged_asset_id: null, face: null, is_cancelled: false,
		cancelled_by_die_id: null, ...extra,
	}) as DiceRollDie;
const baseDice = () => [die(ACTOR), die(ACTOR)];

const participant = (player_id: number, intent: RollIntent | null, is_ready = false) =>
	({ roll_id: 9, player_id, intent, is_ready }) as RollParticipant;

function footer(participants: RollParticipant[], dice: DiceRollDie[]): string {
	const { body } = render(DiceRollPanel, {
		props: {
			roll: {
				id: 9, game_id: 1, plan_id: 1, row_number: 5, is_shake_up: false,
				actor_id: ACTOR, difficulty: 2, adjusted_difficulty: null,
				result: null, outcome: null, stage: 'leverage',
			} as never,
			dice,
			votes: [],
			participants,
			bankedDice: [],
			assets: [],
			currentPlayerID: 4,
			players,
			playerNameMap,
		},
	});
	const m = body.match(/<p class="footer-summary[^"]*">([\s\S]*?)<\/p>/);
	if (!m) throw new Error('no footer summary rendered');
	return m[1].replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ').trim();
}

describe('DiceRollPanel footer tally', () => {
	it('counts a side picked before any die is committed', () => {
		const text = footer(
			[participant(1, 'interfere'), participant(ACTOR, 'aid'), participant(3, 'aid'), participant(4, 'aid')],
			baseDice(),
		);
		expect(text).toBe('3 aid · 1 interfere · 0 of 4 ready');
	});

	it('counts players, not dice, once dice are committed', () => {
		const text = footer(
			[participant(1, 'interfere', true), participant(ACTOR, 'aid'), participant(3, 'aid')],
			[
				...baseDice(),
				die(3, { leveraged_asset_id: 50 }), die(3, { leveraged_asset_id: 51 }),
				die(1, { is_interference: true, leveraged_asset_id: 60 }),
				die(1, { is_interference: true, leveraged_asset_id: 61 }),
				die(1, { is_interference: true, leveraged_asset_id: 62 }),
			],
		);
		expect(text).toBe('2 aid · 1 interfere · 1 of 3 ready');
	});

	it('counts the actor as aid and an undecided player on neither side', () => {
		const text = footer(
			[participant(1, null), participant(ACTOR, null), participant(3, null, true)],
			baseDice(),
		);
		expect(text).toBe('1 aid · 0 interfere · 1 of 3 ready');
	});
});

// ── The roll as a step ───────────────────────────────────────────────────────
// Open, it is the open step (heading, a sentence, "For" and "Against", then the
// controls); resolved, it is one finished line. It draws the die mark, never the
// emoji, and a choice (aid or interfere) wears no meaning colour.

import { readFileSync } from 'node:fs';
import { join } from 'node:path';

type Stage = 'decide_vote' | 'voting' | 'leverage' | 'resolved';

function panel(
	over: { stage?: Stage; currentPlayerID?: number; dice?: DiceRollDie[]; participants?: RollParticipant[]; outcome?: 'make' | 'mar' | null; heading?: { text: string; yours: boolean } | null } = {},
): string {
	const { body } = render(DiceRollPanel, {
		props: {
			roll: {
				id: 9, game_id: 1, plan_id: 1, row_number: 5, is_shake_up: false,
				actor_id: ACTOR, difficulty: 3, adjusted_difficulty: null,
				result: over.stage === 'resolved' ? 2 : null, outcome: over.outcome ?? null,
				stage: over.stage ?? 'leverage',
			} as never,
			dice: over.dice ?? baseDice(),
			votes: [],
			participants: over.participants ?? [participant(ACTOR, null), participant(1, null), participant(3, null)],
			bankedDice: [],
			assets: [],
			currentPlayerID: over.currentPlayerID ?? 1,
			players,
			playerNameMap,
			heading: over.heading ?? null,
		},
	});
	return body.replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ');
}

describe('DiceRollPanel as a step', () => {
	it('open, it is a region named for the roll with the heading first', () => {
		const html = panel({ heading: { text: 'Waiting on wren — the roll', yours: false } });
		expect(html).toContain('aria-label="Dice roll"');
		expect(html.indexOf('Waiting on wren — the roll')).toBeLessThan(html.indexOf('wren rolls two dice against difficulty 3'));
	});

	it('says what is being rolled in a sentence', () => {
		expect(panel()).toContain('wren rolls two dice against difficulty 3.');
	});

	it('a Shake-Up roll adds no sentence of its own: the Shake-Up screen already says it', () => {
		const { body } = render(DiceRollPanel, {
			props: {
				roll: {
					id: 9, game_id: 1, plan_id: null, row_number: 14, is_shake_up: true, shake_up_category: 'esteem',
					actor_id: ACTOR, difficulty: 0, adjusted_difficulty: null,
					result: null, outcome: null, stage: 'leverage',
				} as never,
				dice: baseDice(), votes: [], participants: [participant(ACTOR, null)], bankedDice: [],
				assets: [], currentPlayerID: 1, players, playerNameMap,
			},
		});
		expect(body).not.toContain('roll-brief');
		expect(body).not.toContain('earns one token');
	});

	it('draws the die mark for an unrolled die, never the emoji', () => {
		const html = panel();
		expect(html).not.toContain('🎲');
		expect(html).toContain('class="die-mark');
		expect(html).toMatch(/role="img" aria-label="wren's die/);
	});

	it('labels the dice "For" and "Against" only when there is a second group to tell apart', () => {
		const alone = panel();
		expect(alone).not.toMatch(/>For</);
		expect(alone).not.toMatch(/>Against</);
		const split = panel({
			dice: [...baseDice(), die(1, { is_interference: true, leveraged_asset_id: 60 })],
		});
		expect(split).toMatch(/>For</);
		expect(split).toMatch(/>Against</);
		// …and the interfering die is named as such.
		expect(split).toContain("otto's die, interfering");
	});

	it('resolved, it is one finished line that opens to show the dice', () => {
		const html = panel({ stage: 'resolved', outcome: 'make' });
		expect(html).toContain('aria-expanded="false"');
		expect(html).toMatch(/Rolled\s*<span class="status-word success">Make<\/span>/);
		expect(html).toContain('2 distinct faces, difficulty 3');
		// Shut: neither the open step's controls nor the dice are drawn.
		expect(html).not.toContain('aria-label="Dice roll"');
		expect(html).not.toContain('die-tile');
	});

	it('a mar is danger beside its word, a make is success beside its word', () => {
		expect(panel({ stage: 'resolved', outcome: 'mar' })).toMatch(/status-word danger">Mar</);
		expect(panel({ stage: 'resolved', outcome: 'make' })).toMatch(/status-word success">Make</);
	});

	it('names the main button for what it commits: Pass, or Roll for the roller', () => {
		// Not the actor, nothing drafted: the way through is to pass.
		expect(panel({ currentPlayerID: 1 })).toMatch(/action-btn primary"[^>]*>\s*Pass\s*</);
		// The actor's move is the roll itself.
		expect(panel({ currentPlayerID: ACTOR })).toMatch(/action-btn primary"[^>]*>\s*Roll\s*</);
	});

	it('shows aid and interfere as chips a player can press, to everyone but the actor', () => {
		const other = panel({ currentPlayerID: 1 });
		expect(other).toMatch(/class="chip[^"]*"[^>]*>\s*Aid\s*</);
		expect(other).toMatch(/class="chip[^"]*"[^>]*>\s*Interfere\s*</);
		expect(panel({ currentPlayerID: ACTOR })).not.toMatch(/>\s*Interfere\s*</);
	});

	it('a choice wears no meaning colour: the panel never styles aid or interfere green or red', () => {
		const source = readFileSync(join(__dirname, 'DiceRollPanel.svelte'), 'utf8');
		const style = source.slice(source.indexOf('<style>'));
		expect(style).not.toMatch(/color-(success|danger|chip-(green|red))/);
	});

	it('lists who is where as plain lines, not chips', () => {
		const html = panel();
		expect(html).toMatch(/class="plain-list roster[^"]*"/);
		expect(html).not.toContain('class="chip" style');
	});
});
