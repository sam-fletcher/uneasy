import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import ChecklistRow from './ChecklistRow.svelte';
import HelpDisclosure from './HelpDisclosure.svelte';

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
		expect(html).toContain('›');
		expect(html).not.toContain('▾');
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
		expect(render(ChecklistRow, { props: { title: 'x', glyph: 'tick' } }).body).toContain('✓');
		expect(render(ChecklistRow, { props: { title: 'x', glyph: 'circle' } }).body).toContain('○');
		expect(render(ChecklistRow, { props: { title: 'x', glyph: 'help' } }).body).toContain('?');
	});
});

describe('HelpDisclosure — a ChecklistRow specialisation', () => {
	it('is an expanding help row, shut by default', () => {
		const { body: html } = render(HelpDisclosure, {
			props: { title: 'How the prologue works', id: 'prologue-help-body', children: body() },
		});
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('▾');
		expect(html).toContain('?');
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
	const source = readFileSync(join(__dirname, 'ChecklistRow.svelte'), 'utf8');

	it('keeps the 44px touch floor on the head', () => {
		expect(source).toMatch(/min-height:\s*44px/);
	});

	it('guards the caret transition for reduced motion', () => {
		expect(source).toMatch(/prefers-reduced-motion:\s*reduce/);
	});
});
