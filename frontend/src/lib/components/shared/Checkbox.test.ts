import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'svelte/server';
import Checkbox from './Checkbox.svelte';
import { declarations, stripComments } from './cssTestKit';

/*
 * <Checkbox>'s contract is its markup and its stylesheet: a real checkbox under
 * the drawn box, named by the words and described by the explanation, locked by
 * the input's own :disabled. Rendered through svelte/server (the suite has no
 * DOM), so what a tap does is covered by the e2e specs that use it
 * (make_war.spec.ts, spread_rumors.spec.ts).
 */

const html = (props: Record<string, unknown>) => render(Checkbox, { props: { label: 'Surrender', ...props } }).body;
const css = stripComments(readFileSync(join(__dirname, 'checkbox.css'), 'utf8'));
const decls = declarations(css);
const rule = (selector: string) =>
	decls.filter((d) => d.selector.split(',').map((s) => s.trim()).includes(selector));
const prop = (selector: string, p: string) => rule(selector).find((d) => d.prop === p)?.value;

describe('Checkbox markup', () => {
	it('is a label around one real checkbox, in a tap row', () => {
		const out = html({});
		expect(out).toContain('<label class="tap-row check-row"');
		expect(out.match(/<input\b/g)).toHaveLength(1);
		expect(out).toContain('type="checkbox"');
		expect(out).toContain('class="check-input"');
	});

	it('the drawn box is the input\'s next sibling, which is how the recipe sees ticked, locked and focused', () => {
		// checkbox.css styles `.check-input:checked + .check-box`; any wrapper between the two
		// silently turns every one of those rules off.
		const out = html({ checked: true });
		expect(out).toMatch(/<input[^>]*class="check-input"[^>]*\/?>\s*<span class="check-box"/);
	});

	it('the drawn box is decoration (the input carries the state) and the tick shows only when ticked', () => {
		expect(html({})).toContain('<span class="check-box" aria-hidden="true">');
		expect(html({})).not.toContain('<svg');
		const ticked = html({ checked: true });
		expect(ticked).toContain('checked');
		expect(ticked).toContain('<svg');
		expect(ticked).toContain('data-tone="success"');
	});

	it('the words name the input; the explanation describes it', () => {
		const out = html({ label: 'Keep it secret', description: 'Hide it under an asset.' });
		const labelId = out.match(/id="([^"]+)">Keep it secret</)![1];
		const descId = out.match(/id="([^"]+)">Hide it under an asset\.</)![1];
		expect(out).toContain(`aria-labelledby="${labelId}"`);
		expect(out).toContain(`aria-describedby="${descId}"`);
	});

	it('a checkbox with no explanation has nothing to be described by', () => {
		expect(html({})).not.toContain('aria-describedby');
		expect(html({})).not.toContain('check-desc');
	});

	it('the explanation is on the screen, not in a title', () => {
		const out = html({ description: 'After this payment you are marked surrendered.' });
		expect(out).toContain('After this payment you are marked surrendered.');
		expect(out).not.toContain('title=');
	});

	it('disabled locks the input and shows the reason in words, and only then', () => {
		const props = { description: 'What it does.', reason: 'Not on a peace proposal.' };
		const open = html(props);
		expect(open).not.toContain('disabled');
		expect(open).not.toContain('Not on a peace proposal.');
		const locked = html({ ...props, disabled: true });
		expect(locked).toContain('disabled');
		expect(locked).toContain('Not on a peace proposal.');
		// Both lines describe the input, the reason after the explanation.
		const ids = locked.match(/aria-describedby="([^"]+)"/)![1].split(' ');
		expect(ids).toHaveLength(2);
		for (const id of ids) expect(locked).toContain(`id="${id}"`);
	});

	it('a reason with nothing locked, or a lock with no reason, adds no empty line', () => {
		expect(html({ disabled: true })).not.toContain('check-desc');
	});

	it('passes extra attributes to the input and the class to the row', () => {
		const out = html({ name: 'surrender', 'data-testid': 'x', class: 'mine' });
		expect(out).toMatch(/<input[^>]*name="surrender"/);
		expect(out).toMatch(/<input[^>]*data-testid="x"/);
		expect(out).toContain('class="tap-row check-row mine"');
	});
});

describe('the checkbox recipe', () => {
	it('is the asset card\'s box: 24px, the control border, the raised-twice fill, the control corner', () => {
		expect(prop('.check-box', 'width')).toBe('24px');
		expect(prop('.check-box', 'height')).toBe('24px');
		expect(prop('.check-box', 'border')).toBe('1px solid var(--color-border-control)');
		expect(prop('.check-box', 'background')).toBe('var(--color-surface-2)');
		expect(prop('.check-box', 'border-radius')).toBe('var(--radius-control)');
	});

	it('ticked is the 2px gold border with no fill, for the card\'s attribute and the input alike', () => {
		for (const sel of ['.check-box[aria-checked=\'true\']', '.check-input:checked + .check-box']) {
			expect(prop(sel, 'border-color'), sel).toBe('var(--color-you)');
			expect(prop(sel, 'box-shadow'), sel).toBe('inset 0 0 0 1px var(--color-you)');
			expect(prop(sel, 'background'), sel).toBeUndefined();
		}
	});

	it('locked is a hairline, for both spellings', () => {
		for (const sel of ['.check-box[aria-disabled=\'true\']', '.check-input:disabled + .check-box']) {
			expect(prop(sel, 'border-color'), sel).toBe('var(--color-border)');
		}
	});

	it('a ticked box that is locked stays gold: the ticked rules come after the locked ones', () => {
		const order = (sel: string) => css.indexOf(sel);
		expect(order('.check-input:checked + .check-box')).toBeGreaterThan(order('.check-input:disabled + .check-box'));
		expect(order(".check-box[aria-checked='true']")).toBeGreaterThan(order(".check-box[aria-disabled='true']"));
	});

	it('focus is the bright gold ring, on the box', () => {
		for (const sel of ['.check-box:focus-visible', '.check-input:focus-visible + .check-box']) {
			expect(prop(sel, 'outline'), sel).toBe('2px solid var(--color-you)');
		}
	});

	it('never fades with opacity, and the hidden input is clipped, not removed from the tab order', () => {
		expect(decls.filter((d) => d.prop === 'opacity')).toEqual([]);
		expect(decls.filter((d) => d.prop === 'display' && d.value === 'none')).toEqual([]);
		expect(prop('.check-input', 'clip-path')).toBe('inset(50%)');
	});

	it('the row is at least a tap target tall: it is a .tap-row, and the words\' line is 44px less the padding', () => {
		expect(prop('.check-lead, .check-label', 'min-height')).toBeUndefined();
		const lead = decls.find((d) => d.prop === 'min-height' && d.selector.includes('.check-lead'));
		expect(lead?.value).toBe('calc(var(--size-tap) - 2 * var(--space-2))');
	});

	it('locked words go faint, never a fade', () => {
		expect(prop('.check-row:has(.check-input:disabled)', 'color')).toBe('var(--color-text-faint)');
		expect(prop('.check-row:has(.check-input:disabled) .check-desc', 'color')).toBe('var(--color-text-faint)');
	});
});
