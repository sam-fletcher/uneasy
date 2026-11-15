import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import Chip from './Chip.svelte';
import Notice from './Notice.svelte';
import OptionCard from './OptionCard.svelte';
import RowMark from './RowMark.svelte';
import { declarations, styleOf, winning, type El } from './cssTestKit';

/*
 * The shared recipes (chip, notice, row mark, and the stylesheets behind the
 * buttons, status words and sheets). The components are checked by what they
 * emit. The stylesheets, which no test here can apply, are checked in two
 * ways: by the rules their headers promise (read with comments stripped, so a
 * comment that merely names a property can't fail or hide anything), and, for
 * the chip, by running the cascade (cssTestKit.winning) for each state, since
 * the bug that mattered there was one rule out-ranking another.
 */

const text = (s: string) => createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));
const css = (name: string) => readFileSync(join(__dirname, name), 'utf8');
/** The declarations of the first rule whose selector is exactly `selector`. */
function rule(source: string, selector: string): string {
	const escaped = selector.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	const m = source.match(new RegExp(`(?:^|\\n)${escaped}\\s*\\{([^}]*)\\}`));
	if (!m) throw new Error(`no rule for ${selector}`);
	return m[1];
}

describe('Chip', () => {
	it('is a choice by default: aria-pressed tells a screen reader which is picked', () => {
		const off = render(Chip, { props: { children: text('Aid') } }).body;
		expect(off).toContain('aria-pressed="false"');
		expect(off).not.toContain('selected');
		const on = render(Chip, { props: { selected: true, children: text('Aid') } }).body;
		expect(on).toContain('aria-pressed="true"');
		expect(on).toMatch(/class="[^"]*\bselected\b/);
	});

	it('can stand in for a tab: aria-current says where you are, and only when selected', () => {
		const on = render(Chip, { props: { kind: 'tab', selected: true, children: text('Events') } }).body;
		expect(on).toContain('aria-current="page"');
		expect(on).not.toContain('aria-pressed');
		const off = render(Chip, { props: { kind: 'tab', children: text('Laws') } }).body;
		expect(off).not.toContain('aria-current');
	});

	it('is a real button, and passes attributes through', () => {
		const body = render(Chip, {
			props: { children: text('x'), 'aria-label': 'Wren', compact: true, disabled: true },
		}).body;
		expect(body).toContain('<button');
		expect(body).toContain('type="button"');
		expect(body).toContain('aria-label="Wren"');
		expect(body).toContain('disabled');
		expect(body).toMatch(/class="[^"]*\bcompact\b/);
	});
});

describe('chip.css', () => {
	const chip = css('chip.css');

	it('is fully rounded and takes the touch floor', () => {
		const base = rule(chip, '.chip');
		expect(base).toContain('border-radius: var(--radius-chip)');
		expect(base).toContain('min-height: var(--size-tap)');
	});

	// The cascade, not the source text, is what a player sees. These run the
	// real rules in chip.css (specificity, then source order) for each state a
	// chip can be in. The first version of the file let a hover rule out-rank
	// .selected, so a hovered selected chip showed a GREY outer border around a
	// gold inset pixel, and a grep for "border-color: var(--color-you)" passed.
	const el = (classes: string[], pseudos: string[] = [], attrs: Record<string, string> = {}): El => ({
		classes: ['chip', ...classes],
		pseudos,
		attrs,
	});
	const border = (e: El) => winning(chip, 'border-color', e);
	const color = (e: El) => winning(chip, 'color', e);

	it('a selected chip keeps its bright gold border and text at rest, on hover and on focus', () => {
		for (const pseudos of [[], ['hover'], ['focus-visible'], ['hover', 'focus-visible']]) {
			const label = pseudos.join('+') || 'rest';
			expect(border(el(['selected'], pseudos)), `border, ${label}`).toBe('var(--color-you)');
			expect(color(el(['selected'], pseudos)), `text, ${label}`).toBe('var(--color-you)');
		}
	});

	it('a locked, chosen chip keeps the gold border but takes faint text, said on purpose', () => {
		for (const e of [
			el(['selected'], ['disabled']),
			el(['selected'], ['hover', 'disabled']),
			el(['selected'], [], { 'aria-disabled': 'true' }),
			el(['selected'], ['hover'], { 'aria-disabled': 'true' }),
		]) {
			expect(border(e)).toBe('var(--color-you)');
			expect(color(e)).toBe('var(--color-text-faint)');
		}
	});

	it('an unselected chip lightens its border on hover and greys when locked', () => {
		expect(border(el([], ['hover']))).toBe('var(--color-border-control-hover)');
		expect(border(el([], ['disabled']))).toBe('var(--color-border)');
		expect(border(el([], ['hover', 'disabled']))).toBe('var(--color-border)');
		expect(border(el([], [], { 'aria-disabled': 'true' }))).toBe('var(--color-border)');
		// At rest the border comes from the `border` shorthand, not a border-color rule.
		expect(border(el([]))).toBeNull();
	});

	it('the selected second pixel is an inset shadow, and nothing but the base paints a fill', () => {
		expect(winning(chip, 'box-shadow', el(['selected']))).toBe('inset 0 0 0 1px var(--color-you)');
		const fills = declarations(chip).filter((d) => /^background/.test(d.prop));
		expect(fills.map((d) => d.selector)).toEqual(['.chip']);
	});

	it('hover lightens through the shared token, not a copy of the colour-mix recipe', () => {
		expect(chip).not.toContain('color-mix');
	});

	it('never takes a meaning colour: a choice wears none', () => {
		expect(chip).not.toMatch(/--color-(danger|warning|success|highlight|info)/);
	});
});

describe('OptionCard', () => {
	const aside = createRawSnippet(() => ({ render: () => '<i class="the-die">4</i>' }));
	const card = (props: Record<string, unknown> = {}) =>
		render(OptionCard, { props: { title: 'Perform steps', ...props } }).body;

	it('is a pressable button that says whether it is picked, and ticks only when it is', () => {
		const off = card();
		expect(off).toContain('<button');
		expect(off).toContain('type="button"');
		expect(off).toMatch(/class="[^"]*\boption-card\b/);
		expect(off).toContain('aria-pressed="false"');
		expect(off).not.toContain('option-picked');
		const on = card({ selected: true });
		expect(on).toContain('aria-pressed="true"');
		expect(on).toMatch(/class="[^"]*\bselected\b/);
		// The tick is the success tone's mark, named for a screen reader.
		expect(on).toMatch(/option-picked[\s\S]*aria-label="Picked"/);
	});

	it('lays out a level, a title, a sentence and a reason, in that order, and only what it is given', () => {
		const full = card({ badge: '+1', description: 'Does the thing.', reason: 'needs +2' });
		const at = (s: string) => full.indexOf(s);
		expect(at('+1')).toBeGreaterThan(-1);
		expect(at('+1')).toBeLessThan(at('Perform steps'));
		expect(at('Perform steps')).toBeLessThan(at('Does the thing.'));
		expect(at('Does the thing.')).toBeLessThan(at('needs +2'));
		const bare = card();
		for (const part of ['option-badge', 'option-desc', 'option-aside']) expect(bare).not.toContain(part);
		expect(bare).toContain('option-title');
	});

	it('puts an aside (the duel\'s die) at the end of the row, before the tick', () => {
		const html = card({ selected: true, aside });
		expect(html.indexOf('option-text')).toBeLessThan(html.indexOf('the-die'));
		expect(html.indexOf('the-die')).toBeLessThan(html.indexOf('option-picked'));
	});

	it('is locked by the attribute the caller sets, and passes attributes and extra classes through', () => {
		const html = card({ disabled: true, 'aria-disabled': 'true', 'data-x': '1', class: 'stake-line' });
		expect(html).toMatch(/<button[^>]*\sdisabled/);
		expect(html).toContain('aria-disabled="true"');
		expect(html).toContain('data-x="1"');
		expect(html).toMatch(/class="[^"]*\boption-card\b[^"]*\bstake-line\b/);
		// A card never reads its lock from a class: the look follows the attribute.
		expect(html).not.toMatch(/class="[^"]*\blocked\b/);
	});
});

describe('optionCard.css', () => {
	const sheet = css('optionCard.css');
	const el = (classes: string[] = [], pseudos: string[] = [], attrs: Record<string, string> = {}): El => ({
		classes: ['option-card', ...classes], pseudos, attrs,
	});
	const border = (e: El) => winning(sheet, 'border-color', e);

	it('is a control: the control border and corner, a raised fill, and the touch floor', () => {
		const base = rule(sheet, '.option-card');
		expect(base).toContain('border: 1px solid var(--color-border-control)');
		expect(base).toContain('border-radius: var(--radius-control)');
		expect(base).toContain('background: var(--color-surface)');
		expect(base).toContain('min-height: var(--size-tap)');
	});

	it('a selected card is the bright gold border with an inset second pixel, at rest and on hover', () => {
		for (const pseudos of [[], ['hover'], ['focus-visible']]) {
			expect(border(el(['selected'], pseudos)), pseudos.join('+') || 'rest').toBe('var(--color-you)');
			expect(winning(sheet, 'box-shadow', el(['selected'], pseudos))).toBe('inset 0 0 0 1px var(--color-you)');
		}
		// No fill for a pick: only the base paints a background.
		expect(declarations(sheet).filter((d) => /^background/.test(d.prop)).map((d) => d.selector)).toEqual(['.option-card']);
	});

	it('an unselected card lightens its border on hover, and a locked one is a hairline with faint words', () => {
		expect(border(el([], ['hover']))).toBe('var(--color-border-control-hover)');
		for (const e of [el([], ['disabled']), el([], [], { 'aria-disabled': 'true' }), el([], ['hover', 'disabled'])]) {
			expect(border(e)).toBe('var(--color-border)');
			expect(winning(sheet, 'color', e)).toBe('var(--color-text-faint)');
			expect(winning(sheet, 'cursor', e)).toBe('not-allowed');
		}
	});

	it('a card that is locked wins over a pick: it keeps the tick but not the gold', () => {
		expect(border(el(['selected'], ['disabled']))).toBe('var(--color-border)');
		expect(border(el(['selected'], [], { 'aria-disabled': 'true' }))).toBe('var(--color-border)');
	});

	it('never fades and never wears a meaning colour: a choice is told apart by its words', () => {
		const decls = declarations(sheet); // comments stripped: the header may say "never opacity"
		expect(decls.filter((d) => d.prop === 'opacity')).toEqual([]);
		expect(decls.filter((d) => /--color-(danger|warning|success|highlight|info)/.test(d.value))).toEqual([]);
	});

	it('the focus ring is the bright gold, the tick is bright gold, and the sentence is secondary', () => {
		expect(rule(sheet, '.option-card:focus-visible')).toContain('outline: 2px solid var(--color-you)');
		expect(rule(sheet, '.option-picked')).toContain('color: var(--color-you)');
		expect(rule(sheet, '.option-desc')).toContain('color: var(--color-text-secondary)');
	});
});

describe('the step\'s button row (actionButton.css)', () => {
	const sheet = css('actionButton.css');
	const flexOf = (selector: string) => declarations(sheet).find((d) => d.selector === selector && d.prop === 'flex')?.value;

	it('is a wrapping row with the small gap, and the main button takes twice the room', () => {
		const row = rule(sheet, '.step-actions');
		expect(row).toContain('display: flex');
		expect(row).toContain('flex-wrap: wrap');
		expect(row).toContain('gap: var(--space-2)');
		expect(flexOf('.step-actions > .action-btn.primary')).toBe('2 1 10rem');
		expect(flexOf('.step-actions > .action-btn.secondary')).toBe('1 1 6rem');
	});

	it('has a stacking variant: each button asks for 12rem, so two never fit a phone column', () => {
		expect(flexOf('.step-actions.stacking > .action-btn.primary')).toBe('2 1 12rem');
		expect(flexOf('.step-actions.stacking > .action-btn.secondary')).toBe('1 1 12rem');
		// 2 × 12rem + the gap is wider than the narrowest play column (344px).
		expect(2 * 12 * 16 + 8).toBeGreaterThan(344);
		// And the row without it fits beside each other there.
		expect(10 * 16 + 6 * 16 + 8).toBeLessThan(344);
	});

	it('stretches both buttons to the row\'s height, so a long label never leaves its neighbour short', () => {
		expect(declarations(sheet).find((d) => d.selector === '.step-actions > .action-btn' && d.prop === 'align-self')?.value).toBe('stretch');
	});
});

describe('actionButton.css', () => {
	const btn = css('actionButton.css');

	it('the main button is deep gold, and greyed out is not made with opacity', () => {
		expect(rule(btn, '.action-btn.primary')).toContain('background: var(--color-accent)');
		const disabled = btn.match(/\.action-btn:disabled,[\s\S]*?\{([^}]*)\}/)![1];
		expect(disabled).toContain('--color-surface-2');
		expect(disabled).toContain('--color-text-faint');
		expect(disabled).not.toMatch(/opacity/);
	});

	it('keeps the touch floor and uses the control corner', () => {
		const base = rule(btn, '.action-btn');
		expect(base).toContain('min-height: var(--size-tap)');
		expect(base).toContain('border-radius: var(--radius-control)');
	});

	it('hover lightens through the shared token, not a copy of the recipe', () => {
		expect(btn).not.toContain('color-mix');
		expect(rule(btn, ".action-btn.secondary:hover:not(:disabled):not([aria-disabled='true'])")).toContain(
			'var(--color-border-control-hover)'
		);
	});

	it('never draws the focus ring in the deep gold', () => {
		expect(rule(btn, '.action-btn:focus-visible')).toContain('var(--color-you)');
	});
});

describe('statusText.css', () => {
	const status = css('statusText.css');

	it('a status word is small capitals in a colour: no border, no fill, no radius', () => {
		const word = rule(status, '.status-word');
		expect(word).toContain('text-transform: uppercase');
		expect(word).toContain('font-size: var(--text-label)');
		expect(word).not.toMatch(/border|background|radius|padding/);
	});

	it('defaults to secondary text, and takes a meaning colour only by class', () => {
		expect(rule(status, '.status-word')).toContain('var(--color-text-secondary)');
		expect(rule(status, '.status-word.success')).toContain('--color-success');
		expect(rule(status, '.status-word.danger')).toContain('--color-danger');
		expect(rule(status, '.status-word.warning')).toContain('--color-warning');
	});

	it('has no variant that nothing uses', () => {
		// .accent and .faint went when a deadline's state word became the warning
		// colour; a recipe nobody uses is one more thing to keep in step.
		expect(status).not.toMatch(/\.status-word\.(accent|faint)/);
	});
});

describe('modalShell.css', () => {
	const shell = css('modalShell.css');

	it('a sheet is a raised surface, not the retiring sunken one', () => {
		expect(rule(shell, '.modal-sheet')).toContain('background: var(--color-surface)');
		expect(shell).not.toContain('surface-sunken');
	});

	it('the sheet title is a deep gold heading', () => {
		const title = rule(shell, '.sheet-title');
		expect(title).toContain('font-size: var(--text-heading)');
		expect(title).toContain('color: var(--color-accent)');
	});

	it('the close button and the title take the touch floor', () => {
		const close = rule(shell, '.modal-close');
		expect(close).toContain('width: var(--size-tap)');
		expect(close).toContain('height: var(--size-tap)');
		expect(rule(shell, '.sheet-title')).toContain('min-height: var(--size-tap)');
	});

	it('the focus ring is bright gold', () => {
		expect(rule(shell, '.modal-close:focus-visible')).toContain('var(--color-you)');
	});
});

describe('tapRow.css', () => {
	const tap = css('tapRow.css');

	it('is a touch-floor line with a rule under it and no box', () => {
		const row = rule(tap, '.tap-row');
		expect(row).toContain('min-height: var(--size-tap)');
		expect(row).toContain('border-bottom: 1px solid var(--color-border)');
		expect(row).toContain('border-radius: 0');
		expect(row).toMatch(/background:\s*none/);
	});

	it('draws no border but the rule beneath, and fills only on hover', () => {
		const props = declarations(tap).filter((d) => /^border/.test(d.prop) && d.prop !== 'border-radius');
		expect(props.map((d) => d.prop).sort()).toEqual(['border', 'border-bottom']);
		expect(props.find((d) => d.prop === 'border')!.value).toBe('0');
		const fills = declarations(tap).filter((d) => d.prop === 'background' && d.value !== 'none');
		expect(fills.map((d) => d.selector)).toEqual(['button.tap-row:hover,\na.tap-row:hover']);
	});
});

describe('Notice', () => {
	it('is a mark and text in the tone colour', () => {
		const body = render(Notice, { props: { tone: 'danger', children: text('Lost') } }).body;
		expect(body).toContain('data-tone="danger"');
		expect(body).toContain('<svg');
		expect(body).toContain('Lost');
	});

	it('draws no box: no border, fill, shadow, outline or corner', () => {
		const style = styleOf(readFileSync(join(__dirname, 'Notice.svelte'), 'utf8'));
		const boxy = declarations(style).filter((d) =>
			/^(border|background|box-shadow|outline)/.test(d.prop)
		);
		expect(boxy).toEqual([]);
	});

	it('draws a different default mark for each tone', () => {
		const marks = (['warning', 'danger', 'success', 'info'] as const).map(
			(tone) => render(Notice, { props: { tone, children: text('x') } }).body.match(/<path d="([^"]+)"/)![1]
		);
		expect(new Set(marks).size).toBe(4);
	});

	it('announces itself only when asked to', () => {
		expect(render(Notice, { props: { children: text('x') } }).body).not.toContain('role=');
		expect(render(Notice, { props: { live: true, children: text('x') } }).body).toContain('role="status"');
	});

	it('takes a house mark of the noun in place of the default', () => {
		const mark = createRawSnippet(() => ({ render: () => '<i data-custom-mark></i>' }));
		const body = render(Notice, { props: { mark, children: text('x') } }).body;
		expect(body).toContain('data-custom-mark');
		expect(body).not.toContain('<path');
	});
});

describe('RowMark', () => {
	it('a caret expands in place and an arrow opens elsewhere, and they differ in kind', () => {
		const caret = render(RowMark, { props: { kind: 'caret' } }).body;
		const arrow = render(RowMark, { props: { kind: 'arrow' } }).body;
		expect(caret).toContain('data-kind="caret"');
		expect(arrow).toContain('data-kind="arrow"');
		// A filled triangle against a stroked chevron: never mistaken for each other.
		expect(caret).toContain('fill="currentColor"');
		expect(arrow).toContain('stroke="currentColor"');
		expect(arrow).not.toContain('fill="currentColor"');
	});

	it('is decoration, and draws from paths rather than typed characters', () => {
		for (const kind of ['caret', 'arrow'] as const) {
			const body = render(RowMark, { props: { kind } }).body;
			expect(body).toContain('aria-hidden="true"');
			expect(body).not.toMatch(/[▾›▸▶]/);
		}
	});

	it('a caret says which way it faces', () => {
		expect(render(RowMark, { props: { kind: 'caret', open: true } }).body).toMatch(/class="[^"]*\bopen\b/);
		expect(render(RowMark, { props: { kind: 'caret' } }).body).not.toMatch(/class="[^"]*\bopen\b/);
	});

	it('is one fixed size, 16px, and takes no size prop', () => {
		const style = styleOf(readFileSync(join(__dirname, 'RowMark.svelte'), 'utf8'));
		const mark = declarations(style).filter((d) => d.selector === '.row-mark');
		const val = (p: string) => mark.find((d) => d.prop === p)?.value;
		expect(val('width')).toBe('16px');
		expect(val('height')).toBe('16px');
		expect(render(RowMark, { props: { kind: 'arrow', size: 40 } as never }).body).not.toContain('40px');
	});
});
