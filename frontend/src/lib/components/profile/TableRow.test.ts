import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import TableRow from './TableRow.svelte';
import { declarations, styleOf } from '../shared/cssTestKit';
import type { MyTable, MyTablePlayer } from '$lib/api';

/*
 * TableRow's contract is what a row SAYS: the table's title, who is there, who
 * the game is waiting on, and whether chat has gone unread. Pictures carry
 * most of it (a shape, a clock, a dot), so the words that stand in for them
 * are pinned here along with the things a row must NOT show. Rendered through
 * svelte/server like TableRoster.test.ts: no jsdom, so every assertion is
 * about emitted markup and the component's own stylesheet.
 */

function player(id: number, name: string, seat: number): MyTablePlayer {
	return { id, display_name: name, token_color: null, seat_order: seat, online: false };
}

const ash = player(10, 'ash', 1);
const wren = player(11, 'wren', 2);

function table(over: Partial<MyTable> = {}): MyTable {
	return {
		game_id: 7,
		join_code: 'K7QX2P',
		is_facilitator: false,
		joined_at: '2026-08-01T00:00:00Z',
		phase: 'main_event',
		current_row: 6,
		plan_type: 'seek_answers',
		prologue_ranking_step: null,
		ending_vote_open: false,
		shake_up_category: null,
		player_id: ash.id,
		players: [ash, wren],
		waiting_on_player_ids: [],
		unread_count: 0,
		reminder_muted: false,
		reminder_exhausted: false,
		...over,
	};
}

const html = (t: MyTable) => render(TableRow, { props: { table: t } }).body;

describe('TableRow: first line', () => {
	it("opens the table and says the table's own title", () => {
		const body = html(table());
		expect(body).toContain('href="/table/7"');
		expect(body).toContain('Row 6 · Seek Answers');
	});

	it('shows no join code anywhere, and no phase badge', () => {
		const body = html(table({ waiting_on_player_ids: [ash.id], unread_count: 3 }));
		expect(body).not.toContain('K7QX2P');
		expect(body).not.toContain('phase-badge');
	});

	it('shows the pale dot and "N new" in plain words when chat is unread', () => {
		const body = html(table({ unread_count: 4 }));
		expect(body).toContain('new-dot');
		expect(body).toContain('4 new');
	});

	it('shows nothing about chat when none is unread', () => {
		const body = html(table({ unread_count: 0 }));
		expect(body).not.toContain('new-dot');
		expect(body).not.toContain(' new<');
	});

	it('caps a very long count', () => {
		expect(html(table({ unread_count: 250 }))).toContain('99+ new');
	});

	// The server already sends 0 for a finished table's wait list; unread is the
	// one the old card hid explicitly, so this keeps it hidden.
	it('says nothing about unread chat on a finished table', () => {
		const body = html(table({ phase: 'ended', unread_count: 5 }));
		expect(body).not.toContain('new-dot');
		expect(body).toContain('Game over');
	});
});

describe('TableRow: second line', () => {
	it('names every player, with their own shape', () => {
		const body = html(table());
		expect(body).toContain('>ash<');
		expect(body).toContain('>wren<');
		expect(body).toContain('data-shape="diamond"');
		expect(body).toContain('data-shape="triangle"');
	});

	it('keeps your own name rather than "You", and says "(you)" to a screen reader', () => {
		const body = html(table());
		expect(body).not.toContain('>You<');
		expect(body).toContain('(you)');
	});

	it('puts a bright clock on you and a grey one on someone else, with words for each', () => {
		const mine = html(table({ waiting_on_player_ids: [ash.id] }));
		expect(mine).toContain('data-tone="you"');
		expect(mine).not.toContain('data-tone="other"');
		expect(mine).toContain('the game is waiting on you');

		const theirs = html(table({ waiting_on_player_ids: [wren.id] }));
		expect(theirs).toContain('data-tone="other"');
		expect(theirs).not.toContain('data-tone="you"');
		expect(theirs).toContain('the game is waiting on them');
	});

	it('draws no clock when the game waits on no one', () => {
		expect(html(table())).not.toContain('clock-mark');
	});

	it('draws no clock on a finished table', () => {
		const body = html(table({ phase: 'ended', waiting_on_player_ids: [ash.id] }));
		expect(body).not.toContain('clock-mark');
	});
});

describe('TableRow: the bell', () => {
	it('is a button beside the link, only while the table waits on you', () => {
		const waiting = html(table({ waiting_on_player_ids: [ash.id] }));
		expect(waiting).toContain('<button');
		expect(waiting).toContain('Stop reminding me about Row 6 · Seek Answers');
		expect(html(table({ waiting_on_player_ids: [wren.id] }))).not.toContain('<button');
	});

	it('is never nested inside the link, which cannot hold a button', () => {
		const body = html(table({ waiting_on_player_ids: [ash.id] }));
		const link = body.slice(body.indexOf('<a '), body.indexOf('</a>'));
		expect(link).not.toContain('<button');
	});

	it('is a status, not a button, once reminders have stopped on their own', () => {
		const body = html(table({ waiting_on_player_ids: [ash.id], reminder_exhausted: true }));
		expect(body).not.toContain('<button');
		expect(body).toContain('role="img"');
		expect(body).toContain('Reminders have stopped');
	});
});

// The row is drawn from lines and type, never a box: a box marks something you
// can press; the row is pressed as a whole, and its arrow is the sign.
describe('TableRow: style', () => {
	const css = styleOf(readFileSync(join(__dirname, 'TableRow.svelte'), 'utf8'));
	const decls = declarations(css);
	const on = (selector: string, prop: string) =>
		decls.filter((d) => d.selector === selector && d.prop === prop).map((d) => d.value);

	// The hairline, the 44px minimum and the lack of any box come from the shared
	// .tap-row recipe; this row must use that recipe and add no box of its own.
	it('is the shared tappable-row recipe, with no box added on top', () => {
		expect(readFileSync(join(__dirname, 'TableRow.svelte'), 'utf8')).toMatch(
			/import '\$lib\/components\/shared\/tapRow\.css';/,
		);
		expect(html(table())).toContain('class="table-row tap-row');
		for (const prop of ['border', 'border-bottom', 'border-radius', 'background']) {
			expect(on('.table-row', prop)).toEqual([]);
		}
	});

	it('is never faded (a finished table is not dimmed)', () => {
		expect(decls.filter((d) => d.prop === 'opacity' && d.selector !== 'button.bell:disabled')).toEqual([]);
	});

	it('keeps the bell at the 44px touch minimum', () => {
		expect(on('.bell', 'width')).toEqual(['var(--size-tap)']);
		expect(on('.bell', 'height')).toEqual(['var(--size-tap)']);
	});
});
