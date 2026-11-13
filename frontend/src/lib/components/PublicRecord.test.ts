import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import PublicRecord from './PublicRecord.svelte';
import { declarations, styleOf } from './shared/cssTestKit';
import type { GamePhase, Plan, Player, RecordRow, SceneEntry } from '$lib/api';

/*
 * The Public Record's Events list, rendered through svelte/server (no jsdom), so
 * every assertion is about the markup the list emits. What is pinned:
 *   - what is tappable (a plan or scene line, and nothing else);
 *   - the one status word on a plan line, and that a war or liaison never wears
 *     Make, whatever the server stored;
 *   - the current row's mark, the burst on the Shake-Up row, the note a table
 *     sees before play starts.
 */

const SOURCE = readFileSync(join(__dirname, 'PublicRecord.svelte'), 'utf8');

function player(id: number, name: string, seat: number): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-08-16T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: seat,
	};
}
const ash = player(1, 'Ash', 1);
const wren = player(2, 'Wren', 2);
const otto = player(3, 'otto', 3);
const players = [ash, wren, otto];
const playerNames = new Map(players.map((p) => [p.id, p.display_name]));

let nextID = 1;
function plan(over: Partial<Plan> & Pick<Plan, 'plan_type' | 'row_number'>): Plan {
	return {
		id: nextID++,
		game_id: 1,
		category: 'power',
		preparer_id: 1,
		target_player_id: null,
		target_asset_id: null,
		row_order: 0,
		prepared_at_row: 1,
		status: 'pending',
		result: null,
		resolved_at: null,
		preparation_notes: null,
		resolution_data: null,
		targeted_plan_id: null,
		is_finale_bonus: false,
		...over,
	};
}
function scene(row: number, author: number, body: string): SceneEntry {
	return { id: nextID++, game_id: 1, row_number: row, author_id: author, body, created_at: '' };
}
const row = (n: number, plans: Plan[] = [], entries: SceneEntry[] = []): RecordRow => ({
	row_number: n,
	plans,
	entries,
});

function draw(
	rows: RecordRow[],
	over: { currentRow?: number; phase?: GamePhase; shakeUpCategory?: string | null } = {}
) {
	const { body } = render(PublicRecord, {
		props: {
			rows,
			currentRow: over.currentRow ?? 6,
			phase: over.phase ?? 'main_event',
			shakeUpCategory: over.shakeUpCategory ?? null,
			playerNames,
			players,
		},
	});
	return body;
}

/** Matches `class="name …"`: Svelte appends a scoping hash to every class. */
const cls = (name: string) => new RegExp(`class="${name}(?: [^"]*)?"`, 'g');
/** How many elements carry the class. */
const count = (html: string, name: string) => html.match(cls(name))?.length ?? 0;

/** The `<button>` elements of the markup. */
const buttons = (html: string) => html.match(/<button[\s\S]*?<\/button>/g) ?? [];
/** A line's status word: the text of its `.status-word`, and the class it wears. */
function statusWords(html: string): { text: string; cls: string }[] {
	return [...html.matchAll(/<span class="status-word([^"]*)">([^<]*)<\/span>/g)].map((m) => ({
		text: m[2],
		// What is left once the scoping hash is dropped: 'success', 'danger' or ''.
		cls: m[1].replace(/\bsvelte-\w+/, '').trim(),
	}));
}

describe('what can be tapped', () => {
	const rows = [
		row(1, [plan({ plan_type: 'make_introductions', row_number: 1 })]),
		row(2, [], [scene(2, 3, 'Scene: A quiet word in the stables')]),
		row(3),
	];

	it('is a plan line or a scene line: one button each, and nothing else', () => {
		const html = draw(rows);
		expect(buttons(html)).toHaveLength(2);
	});

	it('leaves a row number and an empty row inert', () => {
		const html = draw(rows);
		// Every number is plain text beside the lines, never inside a button.
		for (const b of buttons(html)) expect(b).not.toMatch(/class="row-num/);
		expect(html).not.toMatch(/aria-label="Jump to row/);
		// Row 3 is empty: its number, and no line at all.
		const blocks = html.match(/<li class="record-row[\s\S]*?<\/li>/g) ?? [];
		const empty = blocks.find((b) => />3<\/span>/.test(b)) ?? '';
		expect(empty).toContain('record-row');
		expect(empty).not.toContain('<button');
		expect(empty).not.toContain('class="lines');
	});

	it('draws a line as a tapRow ending in the arrow, since it opens Chat', () => {
		const html = draw(rows);
		for (const b of buttons(html)) {
			expect(b).toMatch(/class="tap-row line/);
			expect(b).toContain('data-kind="arrow"');
			// The caret means "expands in place": a line opens somewhere else.
			expect(b).not.toContain('data-kind="caret"');
		}
	});

	it('names a line for a screen reader with the plan, its preparer and where it goes', () => {
		const html = draw(rows);
		expect(html).toContain('aria-label="Make Introductions by Ash. Open in Chat"');
		expect(html).toContain('aria-label="Scene by otto: A quiet word in the stables. Open in Chat"');
	});

	it('draws the preparer\'s shape on a plan line and the author\'s on a scene line', () => {
		const html = draw(rows);
		const [planLine, sceneLine] = buttons(html);
		expect(planLine).toContain('data-shape="diamond"'); // Ash, seat 1
		expect(sceneLine).toContain('data-shape="square"'); // otto, seat 3
	});

	it('draws a scene with the house scene mark and drops the "Scene:" word', () => {
		const html = draw(rows);
		const sceneLine = buttons(html)[1];
		expect(sceneLine).toContain('scene-mark');
		expect(sceneLine).toContain('A quiet word in the stables');
		expect(sceneLine).not.toMatch(/>\s*Scene:/);
	});
});

describe('the status word on a plan line', () => {
	it('says Resolving on the plan being played, in plain text', () => {
		const html = draw([row(6, [plan({ plan_type: 'seek_answers', row_number: 6, status: 'resolving' })])]);
		expect(statusWords(html)).toEqual([{ text: 'Resolving', cls: '' }]);
	});

	it('says Make in green and Mar in red on resolved plans', () => {
		const html = draw([
			row(1, [plan({ plan_type: 'propose_decree', row_number: 1, status: 'resolved', result: 'make' })]),
			row(2, [plan({ plan_type: 'make_introductions', row_number: 2, status: 'resolved', result: 'mar' })]),
		]);
		expect(statusWords(html)).toEqual([
			{ text: 'Make', cls: 'success' },
			{ text: 'Mar', cls: 'danger' },
		]);
	});

	it('says "No effect", with no colour, on a plan closed without a roll', () => {
		const html = draw([
			row(3, [plan({ plan_type: 'spread_propaganda', row_number: 3, status: 'resolved', result: null })]),
		]);
		expect(statusWords(html)).toEqual([{ text: 'No effect', cls: '' }]);
	});

	it('says nothing on a future plan', () => {
		const html = draw([row(9, [plan({ plan_type: 'exchange_courtiers', row_number: 9 })])]);
		expect(statusWords(html)).toEqual([]);
	});

	// The server stores "make" when a war or a liaison ends. The record must not
	// show it: the book gives neither a Make or a Mar.
	it('never shows Make on a Make War line, though the server stored "make"', () => {
		const html = draw([
			row(4, [plan({ plan_type: 'make_war', row_number: 4, status: 'resolved', result: 'make' })]),
		]);
		expect(html).toContain('Make War');
		expect(statusWords(html)).toEqual([]);
		expect(html).not.toMatch(/status-word[^>]*>\s*Make\b/);
	});

	it('never shows Make on a Clandestinely Liaise line, though the server stored "make"', () => {
		const html = draw([
			row(5, [plan({ plan_type: 'clandestinely_liaise', row_number: 5, status: 'resolved', result: 'make' })]),
		]);
		expect(html).toContain('Clandestinely Liaise');
		expect(statusWords(html)).toEqual([]);
	});

	it('gives each line one word at most, and a war or liaison beside a plan adds none', () => {
		const html = draw([
			row(5, [
				plan({ plan_type: 'propose_decree', row_number: 5, status: 'resolved', result: 'make' }),
				plan({ plan_type: 'clandestinely_liaise', row_number: 5, status: 'resolved', result: 'make', row_order: 1 }),
			]),
		]);
		expect(statusWords(html)).toEqual([{ text: 'Make', cls: 'success' }]);
	});
});

describe('the current row', () => {
	const rows = Array.from({ length: 13 }, (_, i) => row(i + 1));

	it('is marked on exactly one row', () => {
		const html = draw(rows, { currentRow: 6 });
		expect(html.match(/data-state="current"/g)).toHaveLength(1);
		expect(html.match(/data-state="past"/g)).toHaveLength(5);
		// 7 to 13, and the Shake-Up row after them.
		expect(html.match(/data-state="future"/g)).toHaveLength(8);
	});

	// The current row is marked by a bar and a number colour and nothing behind
	// it, and the rules below are what say so.
	describe('is a deep gold number and a 2px bar down the left edge, with no fill', () => {
		const css = styleOf(SOURCE);
		const decls = declarations(css);
		const of = (selector: string, prop: string) =>
			decls.filter((d) => d.selector === selector && d.prop === prop).map((d) => d.value);

		it('draws the bar as an inset deep gold shadow', () => {
			const bars = decls.filter(
				(d) => d.prop === 'box-shadow' && d.selector.includes(".record-row[data-state='current']")
			);
			expect(bars.map((d) => d.value)).toEqual(['inset 2px 0 0 var(--color-accent)']);
		});

		it('paints the number deep gold, and every other number faint', () => {
			expect(of(".record-row[data-state='current'] .row-num", 'color')).toEqual(['var(--color-accent)']);
			expect(of('.row-num', 'color')).toEqual(['var(--color-text-faint)']);
		});

		it('gives no row a background', () => {
			const fills = decls.filter((d) => /^background(-color)?$/.test(d.prop) && /record-row|row-num|lines/.test(d.selector));
			expect(fills).toEqual([]);
		});
	});
});

describe('the Shake-Up row', () => {
	it('draws the house burst, not the typed star', () => {
		const html = draw([]);
		expect(html).toContain('The Shake-Up');
		expect(count(html, 'burst')).toBe(1);
		// The burst's three strokes (LogMark's shake_up family).
		expect(html).toContain('M12 3v18');
		expect(html).not.toContain('✶');
		expect(html).not.toContain('★');
	});

	it('follows the game: future, then current, then past', () => {
		const state = (phase: GamePhase) =>
			draw([], { phase, currentRow: 13 }).match(/class="shakeup-row[^"]*" data-state="(\w+)"/)?.[1];
		expect(state('main_event')).toBe('future');
		expect(state('shake_up')).toBe('current');
		expect(state('ended')).toBe('past');
	});

	it('shows its three pips only once the Shake-Up is under way', () => {
		expect(count(draw([], { phase: 'main_event' }), 'pip')).toBe(0);
		const html = draw([], { phase: 'shake_up', shakeUpCategory: 'knowledge' });
		expect(count(html, 'pip')).toBe(3);
		expect(html).toContain('aria-label="esteem: done"');
		expect(html).toContain('aria-label="knowledge: current"');
		expect(html).toContain('aria-label="power: pending"');
	});
});

describe('before play starts', () => {
	for (const phase of ['lobby', 'tone_setting', 'prologue'] as const) {
		it(`says when play starts, over the thirteen blank rows (${phase})`, () => {
			const html = draw([], { phase, currentRow: 0 });
			expect(html).toContain('Play starts when the Prologue ends');
			expect(count(html, 'record-row')).toBe(13);
			// A blank list has nothing to tap.
			expect(buttons(html)).toHaveLength(0);
			expect(html).not.toContain('data-state="current"');
		});
	}

	for (const phase of ['main_event', 'shake_up', 'ended'] as const) {
		it(`says nothing of the kind once play has started (${phase})`, () => {
			expect(draw([], { phase })).not.toContain('Play starts');
		});
	}
});

describe('the list around the lines', () => {
	it('puts the ranking mark between rows 4|5, 8|9 and 12|13', () => {
		const html = draw(Array.from({ length: 13 }, (_, i) => row(i + 1)));
		expect(count(html, 'engrailed')).toBe(3);
		// The podium (LogMark's ranking family), once per divider.
		expect(html.match(/M9 20\.5V6h6v14\.5/g)).toHaveLength(3);
		// Each divider is announced, since its picture carries the meaning.
		expect(html.match(/Ranking update/g)).toHaveLength(3);
	});

	it('names a delayed arrival by the traveller, not by "Make Introductions"', () => {
		const html = draw([
			row(7, [
				plan({
					plan_type: 'make_introductions',
					row_number: 7,
					resolution_data: JSON.stringify({
						make_introductions: {
							delayed_arrival: true,
							delayed_draft: { id: 'd1', name: 'Sister Ilse', creator_id: 1 },
						},
					}),
				}),
			]),
		]);
		expect(html).toContain('Arriving: Sister Ilse');
	});
});

describe('the stylesheet', () => {
	const css = styleOf(SOURCE);
	const decls = declarations(css);

	it('keeps every line and the first-line number at the touch floor, by token', () => {
		const heights = decls.filter((d) => /^(min-)?height$/.test(d.prop) && d.value === 'var(--size-tap)');
		const selectors = heights.map((d) => d.selector);
		expect(selectors).toContain('.row-num.with-lines');
		expect(selectors).toContain('.shakeup-row');
		// A line is a tapRow, whose own recipe holds the 44px (tapRow.css); this
		// file only strips its padding and rule, and must not shrink it.
		const shrunk = decls.filter((d) => d.selector === '.line' && /^(min-)?height$/.test(d.prop));
		expect(shrunk).toEqual([]);
	});

	it('writes no colour literal and no typed star', () => {
		expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
		expect(css).not.toMatch(/\brgba?\(/);
	});
});
