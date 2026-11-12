// Player shapes: the second half of a player's identity.
//
// A colour alone fails for colour-blind players, so wherever a colour stands
// for a player and no name is nearby, it is paired with a shape. Seats take
// their shape in the same order as their colour (`seatIndex` in
// playerColor.ts is the one place that maps a seat to a slot), so Seat 1 is
// always the Lavender diamond, whatever table it is at.
//
// Each outline is drawn on a 16x16 grid and is meant to be filled, at 12px or
// larger. They are sized to look equally heavy, which is why the square is
// drawn a little smaller than its neighbours. No player is ever a circle (that
// is the status dot) or a star (that is the main character).

import { PLAYER_SLOTS, seatIndex } from './playerColor';

export interface PlayerShapeSpec {
	/** The shape's name, which is also how the styleguide labels it. */
	name: string;
	/** The gem the player colour is named for. */
	colorName: string;
	/** SVG path data on a 16x16 grid. */
	path: string;
}

/** In seat order: the array index is `seatIndex(seat_order)`. */
export const PLAYER_SHAPES: readonly PlayerShapeSpec[] = [
	{ name: 'diamond', colorName: 'Lavender', path: 'M8 0.5L15.5 8L8 15.5L0.5 8Z' },
	{ name: 'triangle', colorName: 'Peridot', path: 'M8 1L15.5 14.5H0.5Z' },
	{ name: 'square', colorName: 'Rose', path: 'M2.5 2.5H13.5V13.5H2.5Z' },
	{
		name: 'drop',
		colorName: 'Sapphire',
		path: 'M8 0.5C10 4 13.5 7 13.5 10.5A5.5 5.5 0 0 1 2.5 10.5C2.5 7 6 4 8 0.5Z',
	},
	{ name: 'gem', colorName: 'Turquoise', path: 'M4 2H12L15.5 6L8 15L0.5 6Z' },
];

if (PLAYER_SHAPES.length !== PLAYER_SLOTS) {
	// Caught at import time rather than in a rendered game: a palette that
	// grows a sixth colour without a sixth shape would otherwise leave that
	// seat's shape undefined and crash the first table to reach it.
	throw new Error('PLAYER_SHAPES and the player palette must be the same length');
}

/** The shape for a seat (1-based `seat_order`; null counts as seat 1). */
export function playerShapeFor(seatOrder: number | null | undefined): PlayerShapeSpec {
	return PLAYER_SHAPES[seatIndex(seatOrder)];
}
