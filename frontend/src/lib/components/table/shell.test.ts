import { describe, expect, it, vi, afterEach } from 'vitest';
import { render } from 'svelte/server';
import PaneTabs from './PaneTabs.svelte';
import PlayerRow from './PlayerRow.svelte';
import RecordTabs from './RecordTabs.svelte';
import ShellColumn from './ShellColumn.svelte';
import type { Player } from '$lib/api';

/*
 * What the frame SAYS, on screen and to a screen reader: which tab you are on,
 * who the game is waiting on, what is new. Rendered through svelte/server like
 * the shared pieces' tests (no jsdom), so every assertion is about emitted
 * markup.
 */

function player(id: number, name: string): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-11-12T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: id,
	};
}
const alice = player(1, 'alice');
const bob = player(2, 'bob');
const noop = () => {};
const none = { record: false, play: false, chat: false };

describe('PaneTabs', () => {
	it('marks the current tab with aria-current and says nothing of the others', () => {
		const { body } = render(PaneTabs, {
			props: { panes: ['record', 'play', 'chat'], current: 'play', marks: none, onSelect: noop },
		});
		expect(body.match(/aria-current="page"/g)).toHaveLength(1);
		expect(body).toMatch(/aria-current="page"[^>]*>\s*<span>Play<\/span>/);
		for (const word of ['Public Record', 'Play', 'Chat']) expect(body).toContain(word);
	});

	it('draws the clock on Play only when it is your move, and the dot on Chat when there is something new', () => {
		const marked = render(PaneTabs, {
			props: {
				panes: ['record', 'play', 'chat'],
				current: 'record',
				marks: { record: false, play: true, chat: true },
				onSelect: noop,
			},
		}).body;
		expect(marked).toContain('aria-label="your move"');
		expect(marked).toContain('aria-label="new messages"');
		expect(marked).not.toContain('aria-label="new laws or rumors"');

		const quiet = render(PaneTabs, {
			props: { panes: ['record', 'play', 'chat'], current: 'record', marks: none, onSelect: noop },
		}).body;
		expect(quiet).not.toContain('your move');
		expect(quiet).not.toContain('new messages');
	});

	it('carries no numbers: a mark is a picture and a label, never a count', () => {
		const { body } = render(PaneTabs, {
			props: {
				panes: ['record', 'play', 'chat'],
				current: 'play',
				marks: { record: true, play: true, chat: true },
				onSelect: noop,
			},
		});
		const text = body.replace(/<[^>]*>/g, ' ');
		expect(text).not.toMatch(/\d/);
	});

	it('draws only the tabs it is given: two columns have no Chat tab', () => {
		const { body } = render(PaneTabs, {
			props: { panes: ['record', 'play'], current: 'play', marks: none, onSelect: noop },
		});
		expect(body).not.toContain('Chat');
	});
});

describe('RecordTabs', () => {
	it('selects the current chip as a tab and puts the dot on Laws and Rumors, never Events', () => {
		const { body } = render(RecordTabs, {
			props: { current: 'laws', lawsNew: true, rumorsNew: true, onSelect: noop },
		});
		expect(body.match(/aria-current="page"/g)).toHaveLength(1);
		expect(body).toMatch(/aria-current="page"[^>]*>\s*<span>Laws<\/span>/);
		expect(body).toContain('aria-label="new laws"');
		expect(body).toContain('aria-label="new rumors"');
		const events = body.slice(body.indexOf('Events'), body.indexOf('Laws'));
		expect(events).not.toContain('new');
	});

	it('shows no dot when nothing is new', () => {
		const { body } = render(RecordTabs, {
			props: { current: 'events', lawsNew: false, rumorsNew: false, onSelect: noop },
		});
		expect(body).not.toContain('aria-label="new');
	});
});

describe('PlayerRow', () => {
	const base = {
		players: [alice, bob],
		currentPlayerID: 1,
		waitingPlayerIDs: new Set<number>(),
		atRiskByPlayer: new Map<number, number>(),
		onSelect: noop,
	};

	it('reads "You" for the viewer and keeps their name for a screen reader', () => {
		const { body } = render(PlayerRow, { props: base });
		expect(body).toContain('>You<');
		expect(body).toContain('>bob<');
		expect(body).toContain("View alice's retinue (you)");
	});

	it('puts a clock on whoever the game is waiting on, gold on you', () => {
		const { body } = render(PlayerRow, {
			props: { ...base, waitingPlayerIDs: new Set([1, 2]) },
		});
		expect(body.match(/data-tone="you"/g)).toHaveLength(1);
		expect(body.match(/data-tone="other"/g)).toHaveLength(1);
		expect(body).toContain('your move');
		expect(body).toContain('their move');
	});

	it('draws no clock when the game is waiting on nobody here', () => {
		const { body } = render(PlayerRow, { props: base });
		expect(body).not.toContain('clock-mark');
	});

	it('shows a warning mark for at-risk assets with no number anywhere', () => {
		const { body } = render(PlayerRow, {
			props: { ...base, atRiskByPlayer: new Map([[2, 3]]) },
		});
		expect(body.match(/class="risk[ "]/g)).toHaveLength(1);
		expect(body).toContain('some assets need marginalia');
		// The count of 3 must not reach the page in any form.
		expect(body).not.toMatch(/>\s*3\s*</);
		expect(body).not.toContain('3 assets');
	});

	it('gives every player a shape beside their name', () => {
		const { body } = render(PlayerRow, { props: base });
		expect(body.match(/data-shape="/g)).toHaveLength(2);
	});
});

describe('ShellColumn', () => {
	// A snippet is a function of the render anchor; for a body this simple one
	// that writes nothing does.
	const children = (() => {}) as never;

	it('hides a pane that is not shown without unmounting it', () => {
		const { body } = render(ShellColumn, {
			props: { pane: 'chat', label: 'Chat', shown: false, children },
		});
		expect(body).toContain('data-shown="false"');
		expect(body).toContain('data-pane="chat"');
		expect(body).toContain('aria-label="Chat"');
	});

	it('is shown by default in the markup a column carries', () => {
		const { body } = render(ShellColumn, {
			props: { pane: 'play', label: 'Play', shown: true, children },
		});
		expect(body).toContain('data-shown="true"');
	});
});

describe('tableLayout', () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		vi.resetModules();
	});

	// matchMedia stub: a query matches when the stubbed width satisfies it.
	function stubWindow(width: number) {
		const listeners = new Set<() => void>();
		const matchMedia = (query: string) => {
			const min = Number(query.match(/min-width:\s*(\d+)px/)?.[1] ?? 0);
			return {
				get matches() { return width >= min; },
				addEventListener: (_: string, fn: () => void) => listeners.add(fn),
				removeEventListener: (_: string, fn: () => void) => listeners.delete(fn),
			};
		};
		vi.stubGlobal('window', { matchMedia });
		return {
			resize(next: number) { width = next; listeners.forEach((fn) => fn()); },
		};
	}

	it('reads one, two or three columns from the breakpoints', async () => {
		for (const [width, mode] of [[375, 'phone'], [743, 'phone'], [744, 'two'], [1111, 'two'], [1112, 'three']] as const) {
			vi.resetModules();
			stubWindow(width);
			const { tableLayout } = await import('$lib/tableLayout.svelte');
			expect(tableLayout.mode, `${width}px`).toBe(mode);
		}
	});

	it('follows the viewport while watched and lets go on cleanup', async () => {
		const win = stubWindow(375);
		const { tableLayout, watchTableLayout } = await import('$lib/tableLayout.svelte');
		const stop = watchTableLayout();
		expect(tableLayout.mode).toBe('phone');
		win.resize(1200);
		expect(tableLayout.mode).toBe('three');
		win.resize(800);
		expect(tableLayout.mode).toBe('two');
		stop();
		win.resize(375);
		expect(tableLayout.mode).toBe('two');
	});
});
