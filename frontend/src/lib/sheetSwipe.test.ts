import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { claimsDrag, shouldDismiss } from './sheetSwipe';

/*
 * Swipe-down-to-dismiss (RetinueSheet, $lib/sheetSwipe) and the pull-to-refresh
 * guard that goes with it. The gesture itself is driven with real touch events
 * in tests/e2e/sheet_swipe.spec.ts; this pins the thresholds and the source
 * rules that are easy to lose quietly.
 */

describe('shouldDismiss', () => {
	it('closes past a quarter of the sheet, springs back before it', () => {
		expect(shouldDismiss(201, 800, 0)).toBe(true);
		expect(shouldDismiss(199, 800, 0)).toBe(false);
	});
	it('closes on a fast flick whatever the distance', () => {
		expect(shouldDismiss(40, 800, 0.6)).toBe(true);
		expect(shouldDismiss(40, 800, 0.4)).toBe(false);
	});
	it('never closes on an upward or zero drag', () => {
		expect(shouldDismiss(0, 800, 5)).toBe(false);
		expect(shouldDismiss(-30, 800, 5)).toBe(false);
	});
});

describe('claimsDrag', () => {
	it('claims a mostly-downward move with the body at the top', () => {
		expect(claimsDrag(2, 12, 0)).toBe(true);
		expect(claimsDrag(0, 12, -6)).toBe(true); // iOS rubber-band
	});
	it('leaves a scrolled body to scroll', () => {
		expect(claimsDrag(0, 12, 300)).toBe(false);
	});
	it('leaves upward and sideways moves alone', () => {
		expect(claimsDrag(0, -12, 0)).toBe(false);
		expect(claimsDrag(20, 12, 0)).toBe(false);
	});
});

describe('RetinueSheet source rules', () => {
	const src = readFileSync(join(__dirname, 'components/RetinueSheet.svelte'), 'utf8');
	const swipe = readFileSync(join(__dirname, 'sheetSwipe.ts'), 'utf8');

	it('stops the body chaining its scroll to the page', () => {
		const body = /\.sheet-body\s*\{[^}]*\}/.exec(src)?.[0] ?? '';
		expect(body).toMatch(/overscroll-behavior:\s*contain/);
	});
	it('shuts the page overscroll while open, and releases it', () => {
		expect(src).toMatch(/overscrollBehaviorY\s*=\s*'none'/);
		expect(src).toMatch(/removeProperty\('overscroll-behavior-y'\)/);
		expect(src).toMatch(/\$effect\(\(\) => \{\s*if \(!open\) return;\s*return lockPageOverscroll\(\);/);
	});
	it('uses touch events with a non-passive touchmove, never pointer events', () => {
		expect(swipe).toMatch(/addEventListener\('touchmove',\s*onTouchMove,\s*\{\s*passive:\s*false\s*\}\)/);
		expect(swipe).not.toMatch(/pointer(down|move|up)/i);
	});
	it('does not start a drag on text fields', () => {
		expect(swipe).toMatch(/TEXT_FIELD = 'input, textarea, select, \[contenteditable\]/);
	});
});
