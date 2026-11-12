import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import ChecklistRow from './ChecklistRow.svelte';
import HelpDisclosure from './HelpDisclosure.svelte';
import { declarations, styleOf } from './cssTestKit';

/*
 * ChecklistRow's contract is its markup: which element the head is, and what
 * aria it carries (adr/LOBBY_AND_CHECKLIST_PLAN.md D2). Rendered through
 * svelte/server rather than a DOM — the suite has no jsdom and doesn't need
 * one here, since every assertion below is about what the component emits,
 * not about what a click does to it. The one thing SSR can't reach — a
 * navigate row actually invoking onSelect — is covered by the lobby's Tones
 * row in tests/e2e/player_join.spec.ts.
 */

const body = () => createRawSnippet(() => ({ render: () => '<p>body text</p>' }));

describe('ChecklistRow — action modes', () => {
	it("action='expand' is a button with aria-expanded, and no aria-controls while shut", () => {
		const { body: html } = render(ChecklistRow, {
			props: { title: 'Read the primer', id: 'primer-body', action: 'expand', children: body() },
		});
		expect(html).toContain('<button');
		expect(html).toContain('aria-expanded="false"');
		// aria-controls may only name an element that exists: the body is
		// unrendered while shut, so the attribute has to go with it.
		expect(html).not.toContain('aria-controls');
		expect(html).not.toContain('body text');
	});

	it("action='expand' with defaultOpen points aria-controls at the body it rendered", () => {
		const { body: html } = render(ChecklistRow, {
			props: {
				title: 'Read the primer',
				id: 'primer-body',
				action: 'expand',
				defaultOpen: true,
				children: body(),
			},
		});
		expect(html).toContain('aria-expanded="true"');
		expect(html).toContain('aria-controls="primer-body"');
		expect(html).toContain('id="primer-body"');
		expect(html).toContain('body text');
	});

	it("action='navigate' is a button with no expand semantics", () => {
		const { body: html } = render(ChecklistRow, {
			props: { title: 'Look over the tones', action: 'navigate', onSelect: () => {} },
		});
		expect(html).toContain('<button');
		expect(html).not.toContain('aria-expanded');
		expect(html).not.toContain('aria-controls');
		// D1: an arrow travels, a caret expands. Never both.
		expect(html).toContain('data-kind="arrow"');
		expect(html).not.toContain('data-kind="caret"');
	});

	it("action='none' is not interactive, and shows its body unconditionally", () => {
		const { body: html } = render(ChecklistRow, {
			props: { title: 'Name your main character', children: body() },
		});
		expect(html).not.toContain('<button');
		expect(html).not.toContain('aria-expanded');
		// R4: a blocker's form is never behind an accordion.
		expect(html).toContain('body text');
		expect(html).not.toContain('▾');
		expect(html).not.toContain('›');
	});
});

describe('ChecklistRow — row furniture', () => {
	it('renders the state chip with its tone', () => {
		const { body: html } = render(ChecklistRow, {
			props: { title: 'Turn on notifications', state: { text: 'off', tone: 'off' } },
		});
		expect(html).toContain('data-tone="off"');
		expect(html).toContain('off');
	});

	it('carries the row tone for the border/fill rules', () => {
		const { body: html } = render(ChecklistRow, { props: { title: 'x', tone: 'blocker' } });
		expect(html).toContain('data-tone="blocker"');
	});

	it('renders the house glyphs', () => {
		// Drawn marks, not typed characters (✓ ○ ? came from a different font on
		// every platform): each is an svg inside the glyph slot.
		const slot = (glyph: 'tick' | 'circle' | 'help') =>
			render(ChecklistRow, { props: { title: 'x', glyph } }).body;
		expect(slot('tick')).toContain('M5 12.5l4.5 4.5L19 7.5');
		expect(slot('circle')).toContain('<circle');
		// The help mark is HelpGlyph's circled question mark.
		expect(slot('help')).toContain('M9.5 9a2.5 2.5 0 0 1 4.5 1.5');
		expect(slot('tick')).not.toContain('✓');
	});
});

describe('HelpDisclosure — a ChecklistRow specialisation', () => {
	it('is an expanding help row, shut by default', () => {
		const { body: html } = render(HelpDisclosure, {
			props: { title: 'How the prologue works', id: 'prologue-help-body', children: body() },
		});
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('data-kind="caret"');
		expect(html).toContain('M9.5 9a2.5 2.5 0 0 1 4.5 1.5');
		expect(html).not.toContain('body text');
	});

	it('passes tone and defaultOpen through (the lobby primer)', () => {
		const { body: html } = render(HelpDisclosure, {
			props: {
				title: 'Read the two-minute primer',
				id: 'lobby-primer-body',
				tone: 'primary',
				defaultOpen: true,
				children: body(),
			},
		});
		expect(html).toContain('data-tone="primary"');
		expect(html).toContain('aria-controls="lobby-primer-body"');
		expect(html).toContain('body text');
	});
});

describe('ChecklistRow — house rules that are easy to lose', () => {
	// The row's furniture moved into shared recipes (the line in tapRow.css,
	// the caret in RowMark.svelte), so the rules are checked where they live.
	const tapRow = readFileSync(join(__dirname, 'tapRow.css'), 'utf8');
	const rowMark = readFileSync(join(__dirname, 'RowMark.svelte'), 'utf8');
	const source = readFileSync(join(__dirname, 'ChecklistRow.svelte'), 'utf8');

	it('keeps the touch floor on the head', () => {
		expect(source).toContain("'$lib/components/shared/tapRow.css'");
		expect(source).toMatch(/class="tap-row row-head"/);
		expect(tapRow).toMatch(/min-height:\s*var\(--size-tap\)/);
	});

	it('guards the caret transition for reduced motion', () => {
		expect(rowMark).toMatch(/prefers-reduced-motion:\s*reduce/);
	});

	it('draws no box: a row is separated by a rule, not a border, a fill or a shadow', () => {
		// The box was the old look; a tone colours text now. A regression here
		// would quietly bring the nested boxes back. Read with comments stripped,
		// so prose that names a property neither fails this nor hides a real one.
		const decls = declarations(styleOf(source));
		const boxy = decls.filter(
			(d) =>
				/^(background|box-shadow|outline|border-radius)/.test(d.prop) ||
				// Only the rule beneath a row (or beneath its body) may be drawn.
				(/^border/.test(d.prop) && !/^border-bottom(-color)?$/.test(d.prop))
		);
		expect(boxy).toEqual([]);
	});

	it('a tone colours text and never a border: the closing rule stays the neutral divider', () => {
		const decls = declarations(styleOf(source));
		const toned = decls.filter((d) => d.selector.includes('data-tone'));
		expect(toned.length).toBeGreaterThan(0);
		for (const d of toned) expect(d.prop, d.selector).toBe('color');
	});

	it('a row has one mark size: the leading glyph slot matches the end mark', () => {
		const slot = declarations(styleOf(source)).filter((d) => d.selector === '.row-glyph');
		const mark = declarations(styleOf(readFileSync(join(__dirname, 'RowMark.svelte'), 'utf8'))).filter(
			(d) => d.selector === '.row-mark'
		);
		const val = (ds: { prop: string; value: string }[], p: string) => ds.find((d) => d.prop === p)?.value;
		expect(val(slot, 'width')).toBe(val(mark, 'width'));
		expect(val(slot, 'height')).toBe(val(mark, 'height'));
	});

	it('a deadline row is one colour: its title and its state word are both the warning colour', () => {
		const body = render(ChecklistRow, {
			props: { title: 'x', tone: 'warn', state: { text: 'last chance', tone: 'warn' } },
		}).body;
		expect(body).toMatch(/class="row-state status-word warning/);
		expect(source).toMatch(/\.checklist-row\[data-tone='warn'\] \.row-title \{ color: var\(--color-warning\); \}/);
	});
});
