// Player color helper.
//
// Every player has a stable color used app-wide so chat readers can follow
// who is talking even when the persona swaps mid-scene. `players.token_color`
// exists in the schema for a future per-player picker, but no route or UI
// sets it yet (adr/COLOR_ROLES_PLAN.md, 2026-07-15 finding) — so
// DEFAULT_PALETTE below, indexed by `seat_order`, is the entire live
// player-colour system today, not a rarely-hit fallback.
//
// Grey (UNKNOWN_PLAYER_COLOR) is NOT an "OOC" or "system" colour — in-scene
// table-talk keeps the speaking player's own color same as in-character
// speech (adr/CHAT_OVERHAUL_PLAN.md Phase 4d retired grey from all
// player-authored content). It's a defensive fallback for when no player
// can be resolved at all (see playerColorByID below).

import type { Player } from './api';

/**
 * Default palette, in seat order: seat 1 is index 0. Seats are filled in join
 * order and the table's creator always sits first, so small tables (the
 * common case) use the earliest, most distinct colours; colour-blind pairs
 * are furthest apart at two players and closest at four or five. Re-check
 * that order whenever one of these is tuned. Keep in sync with any future
 * server-side palette so the two don't disagree if token_color picking ships.
 *
 * Each sits between about 5 and 9:1 on the page (Sapphire is 5.0 there, and
 * drops to 4.49 on a raised surface), and at least ΔE 15 from every meaning
 * colour (red, orange, green, blue, the golds). Wherever a colour stands for
 * a player with no name nearby it is paired with a shape (`playerShape.ts`,
 * drawn by `PlayerShape.svelte`).
 */
const DEFAULT_PALETTE = [
	'#C5AEFE', // Lavender
	'#8AD00B', // Peridot
	'#FF55A0', // Rose
	'#4386FE', // Sapphire
	'#06AEAB', // Turquoise
];

/**
 * Color used when a player can't be resolved at all: `id` is null/missing,
 * or doesn't match any entry in the given players list. Deliberately the
 * same grey as app.css's --neutral-400 (a colour audit found the two had
 * drifted to within ΔE 0.77 of each other — an accidental near-duplicate,
 * not a meaningful distinction) — kept in sync via app.css's
 * --player-unknown cross-check (see designTokens.test.ts).
 */
export const UNKNOWN_PLAYER_COLOR = '#8a8a8a';

/**
 * Returns the hex color string to use for a player anywhere in the UI.
 *
 * - If `token_color` is a non-empty string, it wins.
 * - Otherwise we pick a palette slot via `seat_order` (1-indexed; seat 1 →
 *   palette[0], seat 6 → palette[0] again via modulo).
 * - If `seat_order` is null too, falls back to palette[0]. The caller can
 *   override per-component if a less-prominent color is wanted.
 */
export function playerColor(player: Pick<Player, 'token_color' | 'seat_order'> | null | undefined): string {
	if (!player) return DEFAULT_PALETTE[0];
	if (player.token_color && player.token_color.trim() !== '') {
		return player.token_color;
	}
	return DEFAULT_PALETTE[seatIndex(player.seat_order)];
}

/**
 * Which of the five slots a seat takes, 0-based. Seat 1 is slot 0 and seat 6
 * wraps round to slot 0 again; a missing seat counts as seat 1. This is the
 * one place that arithmetic lives: a player's colour (above) and a player's
 * shape (`playerShape.ts`) both come from it, so the two can never disagree
 * about which seat they belong to. A `token_color` override changes the colour
 * only; the shape still follows the seat.
 */
export function seatIndex(seatOrder: number | null | undefined): number {
	const n = DEFAULT_PALETTE.length;
	return ((((seatOrder ?? 1) - 1) % n) + n) % n;
}

/** How many seat slots there are before colours (and shapes) start repeating. */
export const PLAYER_SLOTS = DEFAULT_PALETTE.length;

/**
 * Convenience: look up a player by id from a list and return their color.
 * Returns UNKNOWN_PLAYER_COLOR if id is null (e.g. a system post — no player
 * author) or the player isn't found in `players`.
 */
export function playerColorByID(
	id: number | null | undefined,
	players: Pick<Player, 'id' | 'token_color' | 'seat_order'>[]
): string {
	if (id == null) return UNKNOWN_PLAYER_COLOR;
	const p = players.find(pl => pl.id === id);
	return p ? playerColor(p) : UNKNOWN_PLAYER_COLOR;
}
