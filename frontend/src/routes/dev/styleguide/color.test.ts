import { describe, expect, it } from 'vitest';
import { contrast, parseRGB, ratioLabel, toHex } from './color';

describe('styleguide colour helpers', () => {
	it('reads the forms getComputedStyle returns', () => {
		expect(parseRGB('rgb(232, 228, 217)')).toEqual([232, 228, 217]);
		expect(parseRGB('rgba(0, 0, 0, 0.6)')).toEqual([0, 0, 0]);
		expect(parseRGB('color(srgb 1 0 0)')).toEqual([255, 0, 0]);
		expect(parseRGB('#C5AEFE')).toEqual([197, 174, 254]);
		expect(parseRGB('transparent')).toBeNull();
	});

	it('matches the WCAG reference points', () => {
		expect(contrast([255, 255, 255], [0, 0, 0])).toBeCloseTo(21, 5);
		expect(contrast([0, 0, 0], [0, 0, 0])).toBeCloseTo(1, 5);
		// Order does not matter.
		expect(contrast([10, 10, 10], [200, 200, 200])).toBeCloseTo(contrast([200, 200, 200], [10, 10, 10]), 10);
	});

	it('reproduces the style guide\'s main-text contrast on the page', () => {
		// --color-text on --color-bg: the guide says 13.7:1.
		const text = parseRGB('rgb(232, 228, 217)')!;
		const page = parseRGB('rgb(26, 26, 26)')!;
		expect(ratioLabel(contrast(text, page))).toBe('13.7:1');
	});

	it('formats a hex', () => {
		expect(toHex([26, 26, 26])).toBe('#1a1a1a');
	});
});
