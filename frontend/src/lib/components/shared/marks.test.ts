import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import BarbuteMark from './BarbuteMark.svelte';
import ClockMark from './ClockMark.svelte';
import DieMark from './DieMark.svelte';
import NewDot from './NewDot.svelte';
import PlayerShape from './PlayerShape.svelte';
import LogMark from '../LogMark.svelte';

/*
 * The house marks' contract is what they emit: which drawing, which colour
 * rule, and whether a screen reader hears them. Rendered through svelte/server
 * (no jsdom in this suite), like the other shared-component tests.
 */

// The components differ in props, so the helper takes them loosely; the cast
// is confined to this one line.
const html = (component: unknown, props: Record<string, unknown> = {}) =>
	render(component as never, { props } as never).body;

describe('PlayerShape', () => {
	it("draws the seat's shape in the seat's colour", () => {
		const body = html(PlayerShape, { seat: 4 });
		expect(body).toContain('data-shape="drop"');
		expect(body).toContain('fill="#4386FE"');
	});

	it('takes both from a Player, and lets token_color change only the colour', () => {
		const body = html(PlayerShape, { player: { token_color: '#123456', seat_order: 2 } });
		expect(body).toContain('data-shape="triangle"');
		expect(body).toContain('fill="#123456"');
	});

	it('is hidden from a screen reader unless it stands alone with a label', () => {
		expect(html(PlayerShape, { seat: 1 })).toContain('aria-hidden="true"');
		const labelled = html(PlayerShape, { seat: 1, label: 'otto' });
		expect(labelled).toContain('role="img"');
		expect(labelled).toContain('aria-label="otto"');
		expect(labelled).not.toContain('aria-hidden');
	});

	it('defaults to 12px, the smallest a shape should be drawn', () => {
		expect(html(PlayerShape, { seat: 1 })).toContain('width="12"');
	});

	it('can take the surrounding text colour instead', () => {
		expect(html(PlayerShape, { seat: 1, color: 'currentColor' })).toContain('fill="currentColor"');
	});
});

describe('ClockMark', () => {
	it('draws the clock family, in one of three colour rules', () => {
		for (const tone of ['you', 'other', 'inherit'] as const) {
			const body = html(ClockMark, { tone });
			expect(body).toContain(`data-tone="${tone}"`);
			expect(body).toContain('M12 7.5V12l3 2');
		}
	});

	it('is decoration by default and an image when labelled', () => {
		expect(html(ClockMark)).toContain('aria-hidden="true"');
		expect(html(ClockMark, { label: 'waiting on you' })).toContain('aria-label="waiting on you"');
	});
});

describe('BarbuteMark', () => {
	it('draws the war family, in war red unless told to inherit', () => {
		const body = html(BarbuteMark);
		expect(body).toContain('M5 20.5V11a7 7 0 0 1 14 0V20.5');
		expect(body).toContain('data-tone="war"');
		expect(html(BarbuteMark, { tone: 'inherit' })).toContain('data-tone="inherit"');
	});
});

describe('DieMark', () => {
	it('is the log\'s die, borrowed by noun: a rounded square with two pips', () => {
		const body = html(DieMark);
		expect(body).toContain('<rect x="3.5" y="3.5" width="17" height="17" rx="3.5"');
		expect(body.match(/<circle/g)).toHaveLength(2);
	});

	it('is the same drawing the chat log draws for a roll, so they cannot drift', () => {
		const log = html(LogMark, { family: 'roll' });
		const inner = (s: string) => s.match(/<svg[^>]*>([\s\S]*)<\/svg>/)![1];
		expect(inner(html(DieMark))).toBe(inner(log));
	});
});

describe('NewDot', () => {
	it('says "new" to a screen reader, or whatever it is told to', () => {
		expect(html(NewDot)).toContain('aria-label="new"');
		expect(html(NewDot, { label: 'new law' })).toContain('aria-label="new law"');
		expect(html(NewDot)).toContain('role="img"');
	});
});

describe('LogMark gained the clock and war families', () => {
	it('renders them, and still renders nothing for an unknown family', () => {
		expect(html(LogMark, { family: 'clock' })).toContain('<circle');
		expect(html(LogMark, { family: 'war' })).toContain('M5 20.5V11');
		expect(html(LogMark, { family: 'no-such-mark' })).not.toContain('<path');
	});
});
