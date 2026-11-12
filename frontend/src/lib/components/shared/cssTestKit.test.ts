import { describe, expect, it } from 'vitest';
import { declarations, stripComments, styleOf, winning } from './cssTestKit';

describe('cssTestKit', () => {
	it('strips CSS and HTML comments but keeps line numbers', () => {
		const out = stripComments('a /* border-radius: 4px;\n */ b <!-- background: red -->');
		expect(out).not.toMatch(/border-radius|background/);
		expect(out.split('\n')).toHaveLength(2);
	});

	it('reads only the <style> of a component, without its comments', () => {
		const css = styleOf('<div style="padding:1px"></div><style>/* x: y */ .a { color: red; }</style>');
		expect(declarations(css)).toEqual([{ selector: '.a', prop: 'color', value: 'red' }]);
	});

	it('lets a more specific rule beat a later, less specific one', () => {
		const css = '.a.b { color: gold; } .a:hover { color: grey; }';
		// .a.b is (0,2,0), .a:hover is (0,2,0): the later rule wins a tie.
		expect(winning(css, 'color', { classes: ['a', 'b'], pseudos: ['hover'] })).toBe('grey');
		const css2 = '.a.b.c { color: gold; } .a:hover { color: grey; }';
		expect(winning(css2, 'color', { classes: ['a', 'b', 'c'], pseudos: ['hover'] })).toBe('gold');
	});

	it('counts the argument of :not() toward specificity, and matches it', () => {
		const css = '.chip.selected { c: gold; } .chip:hover:not(:disabled):not(.selected) { c: grey; }';
		expect(winning(css, 'c', { classes: ['chip', 'selected'], pseudos: ['hover'] })).toBe('gold');
		expect(winning(css, 'c', { classes: ['chip'], pseudos: ['hover'] })).toBe('grey');
		// The regression the kit exists for: without :not(.selected) the hover rule wins.
		const bad = '.chip.selected { c: gold; } .chip:hover:not(:disabled) { c: grey; }';
		expect(winning(bad, 'c', { classes: ['chip', 'selected'], pseudos: ['hover'] })).toBe('grey');
	});

	it('reads attribute selectors', () => {
		const css = "[aria-disabled='true'] { c: grey; }";
		expect(winning(css, 'c', { attrs: { 'aria-disabled': 'true' } })).toBe('grey');
		expect(winning(css, 'c', { attrs: { 'aria-disabled': 'false' } })).toBeNull();
	});

	it('throws on a selector it cannot read rather than guessing', () => {
		expect(() => winning('.a > .b { c: d; }', 'c', {})).toThrow(/can't read/);
	});
});
