import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { declarations, stripComments } from './cssTestKit';

/*
 * There are no plain browser checkboxes or radios in the app: a tick-box is the
 * one the asset cards draw, through <Checkbox> (docs/STYLE_GUIDE.md
 * "Checkboxes"). The native box was a different size in every place that used
 * it, took the browser's blue, and was stretched across the whole row by the
 * global text-field rule until a surrender checkbox's words fell off the phone.
 *
 * This fails when a .svelte file other than the shared component draws one:
 * the fix is <Checkbox>, not an entry below. A choice among a few words is a
 * <Chip>, and a setting that acts at once is a switch.
 *
 * It reads markup, so it can't see a type set through an expression
 * (`type={kind}`); no input in the app does that today.
 */

const SRC = join(__dirname, '..', '..', '..'); // frontend/src
const rel = (f: string) => f.slice(SRC.length + 1).split('\\').join('/');

function walk(dir: string, out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const full = join(dir, name);
		if (statSync(full).isDirectory()) walk(full, out);
		else if (/\.(svelte|css)$/.test(name)) out.push(full);
	}
	return out;
}
const FILES = walk(SRC);
const SVELTE = FILES.filter((f) => f.endsWith('.svelte'));

/** The one file allowed to draw a native checkbox: it hides it under the drawn box. */
const SHARED = 'lib/components/shared/Checkbox.svelte';

/**
 * An <input> that is a switch, not a checkbox: it acts the moment it is flipped,
 * so there is no row, words-and-explanation or submit step for <Checkbox> to be.
 * Each entry names the file, how many such inputs it may hold, and why.
 */
const SWITCHES: Record<string, { count: number; why: string }> = {
	'lib/components/ChatPanel.svelte': {
		count: 1,
		why: '"Hide bookkeeping" filters the feed the moment it is flipped, with no Submit: a switch (role="switch"), not a form field.',
	},
};

/** The markup of a .svelte file: no script, no style, no comments. */
function markupOf(file: string): string {
	return stripComments(readFileSync(file, 'utf8'))
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '');
}

const TAG = /<input\b(?:"[^"]*"|'[^']*'|\{[^}]*\}|[^>"'{])*>/g;
const TICKY = /\btype\s*=\s*(?:"(?:checkbox|radio)"|'(?:checkbox|radio)'|(?:checkbox|radio)(?=[\s/>]))/;

/** Every checkbox or radio <input> in some markup, as its tag text. */
function tickInputsIn(markup: string): string[] {
	return (markup.match(TAG) ?? []).filter((tag) => TICKY.test(tag));
}
const tickInputs = (file: string) => tickInputsIn(markupOf(file));

describe('no plain checkboxes or radios', () => {
	it('only the shared <Checkbox> draws one, and the chat\'s switch', () => {
		const offenders: string[] = [];
		for (const file of SVELTE) {
			const path = rel(file);
			if (path === SHARED) continue;
			const ticks = tickInputs(file);
			const switches = ticks.filter((t) => /\brole\s*=\s*["']switch["']/.test(t));
			const plain = ticks.length - switches.length;
			if (plain > 0) offenders.push(`${path}: ${plain} plain checkbox/radio — use <Checkbox>`);
			if (switches.length > (SWITCHES[path]?.count ?? 0)) {
				offenders.push(`${path}: a role="switch" input not listed in SWITCHES`);
			}
		}
		expect(offenders).toEqual([]);
	});

	it('the shared component does draw its native input, so the guard is looking at something', () => {
		expect(tickInputs(join(SRC, SHARED))).toHaveLength(1);
	});

	it.each(Object.entries(SWITCHES))('%s still holds its switch (an exception outlives its job otherwise)', (path, { count, why }) => {
		expect(why.length).toBeGreaterThan(20);
		const switches = tickInputs(join(SRC, path)).filter((t) => /\brole\s*=\s*["']switch["']/.test(t));
		expect(switches).toHaveLength(count);
	});

	it('reads a checkbox however its attribute is written', () => {
		for (const tag of [
			'<input type="checkbox" bind:checked={x} />',
			"<input type='checkbox'>",
			'<input bind:checked={x} type=checkbox />',
			'<input type="radio" name="r" value="a">',
			'<input\n\ttype="checkbox"\n\tchecked={on}\n\tonchange={() => go(a > b)}\n/>',
		]) {
			expect(tickInputsIn(tag), tag).toHaveLength(1);
		}
		for (const tag of ['<input type="text" />', '<input type="range" min="1">', '<input bind:value={v} />']) {
			expect(tickInputsIn(tag), tag).toHaveLength(0);
		}
	});
});

describe('the box is drawn once', () => {
	const css = (f: string) => {
		const src = readFileSync(f, 'utf8');
		if (f.endsWith('.css')) return src;
		return [...stripComments(src).matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
	};
	const declaredIn = (name: string) =>
		FILES.filter((f) =>
			declarations(css(f)).some((d) => new RegExp(`(^|[^\\w-])\\.${name}(?![\\w-])`).test(d.selector))).map(rel);

	it('.check-box is declared only in checkbox.css', () => {
		expect(declaredIn('check-box')).toEqual(['lib/components/shared/checkbox.css']);
	});

	it('the asset card draws its boxes with it: no fill, border or corner of its own', () => {
		const file = join(SRC, 'lib', 'components', 'AssetCardSelectable.svelte');
		const mine = declarations(css(file)).filter((d) => /(^|[^\w-])\.select-tap(?![\w-])/.test(d.selector)
			&& /^(background|border|border-color|border-radius|box-shadow|outline|color)$/.test(d.prop));
		expect(mine).toEqual([]);
		const boxes = markupOf(file).match(/class="select-tap(?![\w-])[^"]*"/g) ?? [];
		expect(boxes.length).toBeGreaterThan(0);
		for (const b of boxes) expect(b, b).toContain('check-box');
	});
});
