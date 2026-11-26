import { describe, expect, it } from 'vitest';
import { readFileSync, readdirSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import Buffet from '../plans/shared/Buffet.svelte';
import FinishedStep from '../plans/shared/FinishedStep.svelte';
import ChecklistRow from './ChecklistRow.svelte';
import HelpDisclosure from './HelpDisclosure.svelte';
import { declarations, styleOf } from './cssTestKit';

/*
 * The disclosure standard (Round 7, 2026-11-25; DECISIONS "Screen anatomy").
 * Every row that opens in place:
 *   - has a chevron that points down shut and up open (RowMark; its own tests
 *     pin the path and the half turn);
 *   - has no box and no fill, shut or open;
 *   - keeps ONE closing line, under the row while shut and under the opened
 *     content when open, which is what marks where the open content ends;
 *   - if it is an explainer ("how this works"), starts with the deep-gold "?".
 * Rendered through svelte/server (no jsdom), so what is pinned is what each
 * component emits and what its stylesheet declares; the live look is the
 * screenshots in the PR and tests/e2e/styleguide.spec.ts.
 */

const text = (s: string) => createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));
const para = () => createRawSnippet(() => ({ render: () => '<p>opened text</p>' }));
const HELP_GLYPH = 'M9.5 9a2.5 2.5 0 0 1 4.5 1.5';
const read = (...p: string[]) => readFileSync(join(__dirname, ...p), 'utf8');

describe('the closing line moves under the content when a disclosure opens', () => {
	const tap = declarations(read('tapRow.css'));

	it('the recipe: the row\'s own line goes transparent, the content carries the line', () => {
		const rowLine = tap.find((d) => d.selector === '.disclosure.has-body > .tap-row');
		expect(rowLine?.prop).toBe('border-bottom-color');
		expect(rowLine?.value).toBe('transparent');
		const bodyLine = tap.find((d) => d.selector === '.disclosure-body' && d.prop === 'border-bottom');
		expect(bodyLine?.value).toBe('1px solid var(--color-border)');
	});

	it('a shut row is not has-body and renders no content; an open one is, and does', () => {
		const shut = render(ChecklistRow, {
			props: { title: 'x', id: 'a', action: 'expand', children: para() },
		}).body;
		const open = render(ChecklistRow, {
			props: { title: 'x', id: 'a', action: 'expand', defaultOpen: true, children: para() },
		}).body;
		expect(shut).toMatch(/class="[^"]*\bdisclosure\b/);
		expect(shut).not.toMatch(/class="[^"]*\bhas-body\b/);
		expect(shut).not.toContain('opened text');
		expect(open).toMatch(/class="[^"]*\bdisclosure\b[^"]*\bhas-body\b/);
		expect(open).toContain('disclosure-body');
		// The content follows the row, so the line it carries is the last one.
		expect(open.indexOf('row-head')).toBeLessThan(open.indexOf('disclosure-body'));
	});

	it('a finished line with a detail moves its line the same way', () => {
		const props = { mark: text('m'), children: text('Rolled'), detail: para() };
		const shut = render(FinishedStep, { props }).body;
		const open = render(FinishedStep, { props: { ...props, open: true } }).body;
		expect(shut).toMatch(/class="[^"]*\bdisclosure\b/);
		expect(shut).not.toMatch(/\bhas-body\b/);
		expect(open).toMatch(/\bhas-body\b/);
		expect(open).toContain('disclosure-body');
		// A finished line with nothing to open is plain text: not a disclosure.
		expect(render(FinishedStep, { props: { mark: text('m'), children: text('Rolled') } }).body).not.toContain(
			'disclosure'
		);
	});

	it('the plan span folds in the feed move theirs to the foot of the span (render: chatFeed.render.test.ts)', () => {
		const chat = read('..', 'ChatPanel.svelte');
		// A scene's fold is not reachable from a render here, so its wiring is read.
		expect(chat).toMatch(/class:disclosure=\{!live\}\s+class:has-body=\{isExpanded && !live\}/);
		const foot = declarations(styleOf(chat)).find((d) => d.selector === '.span.has-body' && d.prop === 'border-bottom');
		expect(foot?.value).toBe('1px solid var(--color-border)');
		// The body's hanging rule runs down to that line: no gap is left between them.
		const pad = declarations(styleOf(chat)).find((d) => d.selector === '.span.has-body > .span-body' && d.prop === 'padding-bottom');
		expect(pad?.value).toBe('var(--space-3)');
		expect(declarations(styleOf(chat)).find((d) => d.selector === '.span.has-body' && d.prop === 'padding-bottom')).toBeUndefined();
	});

	it('the claim steps and the Prologue category rows move theirs the same way', () => {
		const claim = read('..', 'phases', 'ClaimChoiceModal.svelte');
		expect(claim).toMatch(/class="step disclosure"[^>]*class:has-body=\{isOpen\}/);
		expect(claim).toMatch(/class="step-body disclosure-body"/);
		expect(claim).toMatch(/aria-controls=\{isOpen \? `step-body-\$\{idx\}` : undefined\}/);
		const prologue = read('..', 'phases', 'PrologueView.svelte');
		expect(prologue).toMatch(/class="sheet-panel disclosure"[^>]*class:has-body=\{isOpen\}/);
		expect(prologue).toMatch(/class="sheet-body disclosure-body"/);
		const standing = read('..', 'phases', 'prologue', 'StandingStrip.svelte');
		expect(standing).toMatch(/class="standing disclosure" class:has-body=\{boardOpen\}/);
		expect(standing).toMatch(/class="tap-row standing-head"/);
		// Its head is a tap row (hover and the shared focus ring), and it names no ring of its own.
		expect(declarations(styleOf(standing)).filter((d) => d.prop === 'outline')).toEqual([]);
	});
});

describe('an explainer is one component with the "?" at its start', () => {
	it('HelpDisclosure draws the deep-gold help glyph before the title', () => {
		const html = render(HelpDisclosure, { props: { title: 'How X works', id: 'x', children: para() } }).body;
		expect(html).toContain(HELP_GLYPH);
		expect(html.indexOf(HELP_GLYPH)).toBeLessThan(html.indexOf('How X works'));
		const row = declarations(styleOf(read('ChecklistRow.svelte'))).find((d) => d.selector === '.row-glyph.help');
		expect(row?.value).toBe('var(--color-accent)');
	});

	it('"What can happen?" is that component, shut by default', () => {
		const html = render(Buffet, { props: { tabs: [{ key: 'a', label: 'A', always: 'x' }] } }).body;
		expect(html).toContain('What can happen?');
		expect(html).toContain(HELP_GLYPH);
		expect(html).toContain('aria-expanded="false"');
		expect(html).toContain('data-kind="caret"');
	});

	it('two Buffets on one page have different body ids', () => {
		const ids = [0, 1].map(
			() => render(Buffet, { props: { open: true, tabs: [{ key: 'a', label: 'A', always: 'x' }] } }).body
		);
		// Each render is its own app root, so the ids are unique within a page
		// only through $props.id(): check the markup uses it rather than a literal.
		expect(read('..', 'plans', 'shared', 'Buffet.svelte')).toMatch(/\$props\.id\(\)/);
		for (const html of ids) expect(html).toMatch(/aria-controls="buffet-[^"]+"/);
	});

	it('the app has no explainer of its own: the "?" row is drawn only by the shared pieces', () => {
		// HelpGlyph is the "?" mark. It may sit on the Help button (a different
		// control), in the shared row (ChecklistRow's help glyph) and in its own
		// file; the lobby also names it in prose, as the thing to look for.
		const users = svelteFiles().filter((f) => /import HelpGlyph/.test(read(f)));
		expect(users.sort()).toEqual(
			['../HelpButton.svelte', '../phases/LobbyView.svelte', '../shared/ChecklistRow.svelte'].sort()
		);
		// And no row asks ChecklistRow for the "?" directly: that is HelpDisclosure's
		// one call, so the title and the closing line can't be styled differently.
		const direct = svelteFiles().filter((f) => /glyph="help"/.test(read(f)));
		expect(direct).toEqual(['../shared/HelpDisclosure.svelte']);
	});
});

describe('no disclosure gets a fill, a box or a shadow when it opens', () => {
	// A selector that only applies while something is open or expanded.
	const OPEN = /\.open\b|\.has-body\b|aria-expanded=['"]?true|\.expanded\b/;
	// Game pieces keep their boxes (a tile in the Prologue grid, a card), and
	// the tile's expansion is the gold join the owner ruled on: not a disclosure row.
	const PIECES = /choice-btn|choice-detail|\.card\b/;

	it('no stylesheet paints a background or a shadow on an open row, body or span', () => {
		const found: string[] = [];
		for (const f of svelteFiles()) {
			for (const d of declarations(styleOf(read(f)))) {
				if (!OPEN.test(d.selector) || PIECES.test(d.selector)) continue;
				if (/^(background|box-shadow)/.test(d.prop) && !/^background(-color)?:?\s*(none|transparent)$/.test(`${d.prop}:${d.value}`)) {
					found.push(`${f}: ${d.selector} { ${d.prop}: ${d.value} }`);
				}
			}
		}
		expect(found).toEqual([]);
	});

	// What a disclosure draws at REST matters as much as open: a box that comes
	// back on the row, its head or its body fails here. Each file names the
	// classes of its disclosure (the row, its head, its content). Only a line
	// under (border-bottom) is allowed, and the chat span's hanging rule
	// (border-left on its body). A focus or hover state may paint what it likes.
	const DISCLOSURES: Record<string, string[]> = {
		'ChecklistRow.svelte': ['checklist-row', 'row-head', 'row-body'],
		'../plans/shared/FinishedStep.svelte': ['done-step', 'done-line', 'done-detail'],
		'../ChatPanel.svelte': ['span', 'span-body', 'plan-group', 'scene-group'],
		'../chat/SpanHeader.svelte': ['span-header'],
		'../phases/PrologueView.svelte': ['sheet-accordion', 'sheet-panel', 'sheet-header', 'sheet-body'],
		'../phases/ClaimChoiceModal.svelte': ['step', 'step-header', 'step-body'],
		'../phases/prologue/StandingStrip.svelte': ['standing', 'standing-head'],
	};

	it('no disclosure row, head or body is boxed or filled even at rest', () => {
		const found: string[] = [];
		for (const [file, classes] of Object.entries(DISCLOSURES)) {
			for (const d of declarations(styleOf(read(file)))) {
				for (const full of d.selector.split(',').map((x) => x.trim())) {
					if (/:(hover|focus)/.test(full)) continue;
					// What the rule paints is its LAST compound: `.step.done .step-marker` is the marker.
					const sel = full.split(/\s+/).pop()!;
					if (!classes.some((c) => new RegExp(`\\.${c}(?![\\w-])`).test(sel))) continue;
					const painted = /^(background|box-shadow|border-radius|outline)/.test(d.prop) && !/^(none|transparent|0)$/.test(d.value);
					const boxed =
						/^border/.test(d.prop) &&
						!/^border-bottom/.test(d.prop) &&
						!(d.prop === 'border-left' && sel.endsWith('.span-body')) &&
						!/^(none|0)$/.test(d.value);
					if (painted || boxed) found.push(`${file}: ${sel} { ${d.prop}: ${d.value} }`);
				}
			}
		}
		expect(found).toEqual([]);
	});

	it('the shared recipe paints no fill, at rest or open; only the hover lift', () => {
		const fills = declarations(read('tapRow.css')).filter((d) => /^background/.test(d.prop) && d.value !== 'none');
		expect(fills.map((d) => d.selector)).toEqual(['button.tap-row:hover,\na.tap-row:hover']);
	});
});

function svelteFiles(dir = join(__dirname, '..'), out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const full = join(dir, name);
		if (statSync(full).isDirectory()) svelteFiles(full, out);
		else if (name.endsWith('.svelte')) out.push(full.slice(join(__dirname, '..').length + 1).replace(/^/, '../'));
	}
	return out;
}
