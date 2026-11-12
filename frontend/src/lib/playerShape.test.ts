import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { PLAYER_SLOTS, playerColor, seatIndex } from './playerColor';
import { PLAYER_SHAPES, playerShapeFor } from './playerShape';

describe('player shapes', () => {
	it('has one shape for every colour in the palette', () => {
		expect(PLAYER_SHAPES).toHaveLength(PLAYER_SLOTS);
	});

	it('takes shapes in seat order: seat 1 is the diamond, seat 5 the gem', () => {
		expect(PLAYER_SHAPES.map((s) => s.name)).toEqual(['diamond', 'triangle', 'square', 'drop', 'gem']);
		expect(playerShapeFor(1).name).toBe('diamond');
		expect(playerShapeFor(5).name).toBe('gem');
	});

	it('wraps past the fifth seat exactly as colours do, so the pair never separates', () => {
		for (const seat of [1, 2, 3, 4, 5, 6, 7, 10, 11]) {
			expect(playerShapeFor(seat)).toBe(PLAYER_SHAPES[seatIndex(seat)]);
		}
		expect(playerShapeFor(6).name).toBe('diamond');
		expect(playerColor({ token_color: null, seat_order: 6 })).toBe(
			playerColor({ token_color: null, seat_order: 1 })
		);
	});

	it('treats a missing seat as seat 1, like the colour does', () => {
		expect(playerShapeFor(null).name).toBe('diamond');
		expect(playerShapeFor(undefined).name).toBe('diamond');
	});

	it('draws five different outlines', () => {
		expect(new Set(PLAYER_SHAPES.map((s) => s.path)).size).toBe(PLAYER_SLOTS);
	});

	it('is never a circle (a status dot) or a star (the main character)', () => {
		for (const s of PLAYER_SHAPES) {
			expect(s.name).not.toMatch(/circle|round|dot|star/);
			// No full-circle arc command: a circle in a path is "a r r 0 1 0 ...".
			expect(s.path).not.toMatch(/A\s*(\d+(?:\.\d+)?)\s+\1\s+0\s+[01]\s+[01]\s+[\d.]+\s+[\d.]+\s*A/);
		}
	});

	it('keeps its colour names in the same order as the palette comments', () => {
		// The gem names live beside the hexes in playerColor.ts; if someone
		// reorders the palette without reordering the shapes (or the reverse),
		// seat 1 would stop being one object.
		const src = readFileSync(join(__dirname, 'playerColor.ts'), 'utf8');
		const block = src.match(/DEFAULT_PALETTE = \[([\s\S]*?)\];/)![1];
		const names = [...block.matchAll(/\/\/\s*(\w+)/g)].map((m) => m[1]);
		expect(PLAYER_SHAPES.map((s) => s.colorName)).toEqual(names);
	});
});
