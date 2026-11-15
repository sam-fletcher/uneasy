import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

/*
 * The text-field look on <input> is one global rule in routes/+layout.svelte.
 * Written as `:global(input)`, it reaches checkboxes and radios too: they get
 * width: 100% and a text-field padding, so a checkbox takes its whole row and
 * draws its box in the middle of it, pushing its words off the screen. That is
 * how the surrender checkbox on Make War's cost of battle shipped (the words
 * were in the markup and never showed on a phone).
 *
 * So a global rule that sizes inputs must name only the text-like types. Its
 * selector excludes checkbox and radio, and those keep their native size. A
 * component that wants a different box says so locally (Spread Rumors' toggle
 * sets its own width; the chat's Hide bookkeeping is drawn as a switch).
 *
 * If this test fails you added a sizing rule that also reaches checkboxes or
 * radios. Scope it with :not([type='checkbox']):not([type='radio']), or give
 * the one control you mean its own class.
 */

const LAYOUT = readFileSync(join(__dirname, '..', 'routes/+layout.svelte'), 'utf8');

// Every rule whose selector is `:global(input …)`. The selector may contain
// parentheses (:not(...)), so the match runs to the `)` that precedes the brace.
const inputRules = [...LAYOUT.matchAll(/:global\((input[^{]*?)\)\s*\{([^}]*)\}/g)].map(m => ({
	selector: m[1],
	// Comments are prose; only declarations count.
	body: m[2].replace(/\/\*[\s\S]*?\*\//g, ''),
}));

// A rule that sets an input's size, padding, border or font size.
const SIZING = /\b(?:width|height|padding|border|font-size)\s*:/;

describe('global input rule', () => {
	it('has a sizing rule for inputs (so the guard below is not vacuous)', () => {
		const sizing = inputRules.filter(r => SIZING.test(r.body));
		expect(sizing.length, 'no `:global(input …)` rule sets size or padding').toBeGreaterThan(0);
	});

	it('leaves checkboxes and radios out of every sizing input rule', () => {
		const offenders = inputRules
			.filter(r => SIZING.test(r.body))
			.filter(r => !r.selector.includes(":not([type='checkbox'])") || !r.selector.includes(":not([type='radio'])"))
			.map(r => `:global(${r.selector})`);
		expect(
			offenders,
			'these input rules would stretch checkboxes or radios; scope them with :not([type=\'checkbox\']):not([type=\'radio\'])',
		).toEqual([]);
	});
});
