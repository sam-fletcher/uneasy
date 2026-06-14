import { describe, it, expect } from 'vitest';
import { isNeedlesslyAtRisk, firstEmptySlotIndex, destructionWarning } from './assetRisk';
import type { Asset, Marginalium } from '$lib/api';

// Minimal marginalia builder; only the fields the helpers read matter.
function marg(position: number, is_torn = false): Marginalium {
	return { id: position, asset_id: 1, position, text: `m${position}`, is_torn } as Marginalium;
}

function asset(marginalia: Marginalium[], is_destroyed = false): Asset {
	return { id: 1, marginalia, is_destroyed } as Asset;
}

describe('isNeedlesslyAtRisk', () => {
	it('flags a brand-new asset with no marginalia (0 intact, 4 empty)', () => {
		expect(isNeedlesslyAtRisk(asset([]))).toBe(true);
	});

	it('flags 1 intact note with empty slots remaining', () => {
		expect(isNeedlesslyAtRisk(asset([marg(1)]))).toBe(true);
	});

	it('flags 1 intact + 2 torn while a slot is still empty', () => {
		expect(isNeedlesslyAtRisk(asset([marg(1), marg(2, true), marg(3, true)]))).toBe(true);
	});

	it('does NOT flag a fragile-but-full asset (1 intact + 3 torn, no empty slot)', () => {
		const a = asset([marg(1), marg(2, true), marg(3, true), marg(4, true)]);
		expect(isNeedlesslyAtRisk(a)).toBe(false);
	});

	it('does NOT flag an asset with 2+ intact notes', () => {
		expect(isNeedlesslyAtRisk(asset([marg(1), marg(2)]))).toBe(false);
	});

	it('does NOT flag a destroyed asset', () => {
		expect(isNeedlesslyAtRisk(asset([], true))).toBe(false);
	});
});

describe('firstEmptySlotIndex', () => {
	it('returns 0 when the grid is empty', () => {
		expect(firstEmptySlotIndex(asset([]))).toBe(0);
	});

	it('returns the first gap, skipping occupied positions', () => {
		// positions 1 and 3 filled → first empty is position 2 → index 1
		expect(firstEmptySlotIndex(asset([marg(1), marg(3)]))).toBe(1);
	});

	it('counts torn slots as occupied (they hold a position)', () => {
		// 1 intact + 3 torn fills all four positions → no empty slot
		expect(firstEmptySlotIndex(asset([marg(1), marg(2, true), marg(3, true), marg(4, true)]))).toBe(
			null,
		);
	});
});

describe('destructionWarning', () => {
	it('returns the warning sentence when needlessly at risk', () => {
		expect(destructionWarning(asset([marg(1)]))).toBe(
			"Heads up: this is the asset's last marginalia, but there are empty slots." +
			' Tearing it will destroy the asset.' +
			' The owner should add another marginalia before you tear it.',
		);
	});

	it('returns empty string when not at risk', () => {
		expect(destructionWarning(asset([marg(1), marg(2)]))).toBe('');
	});

	it('returns empty string for null/undefined', () => {
		expect(destructionWarning(null)).toBe('');
		expect(destructionWarning(undefined)).toBe('');
	});
});
