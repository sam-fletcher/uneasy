// Player color helper.
//
// Every player has a stable color used app-wide so chat readers can follow
// who is talking even when the persona swaps mid-scene. We prefer the
// server-stored `players.token_color` but fall back to a deterministic
// palette indexed by `seat_order` for players who never picked one (or for
// whom the field is still null in older records).
//
// Tweak the palette below if the color picker UI lands; downstream callers
// only care that `playerColor()` returns a hex string.
//
// OOC posts use OOC_COLOR, independent of any player.

import type { Player } from './api';

/**
 * Deterministic fallback palette, ordered to maximize visual distinction
 * across a 5-player table on a dark background. Index 0 = highest seat
 * order. Keep in sync with any future server-side palette so the two don't
 * disagree before token_color is set.
 */
const FALLBACK_PALETTE = [
	'#c8a96e', // warm gold (the existing brand accent)
	'#7fb3d5', // cool blue
	'#b48ead', // muted purple
	'#a3be8c', // sage green
	'#d08770', // terracotta
];

/** Color used for OOC chat messages and OOC persona affordances. */
export const OOC_COLOR = '#888888';

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
	if (!player) return FALLBACK_PALETTE[0];
	if (player.token_color && player.token_color.trim() !== '') {
		return player.token_color;
	}
	const seat = player.seat_order ?? 1;
	const idx = ((seat - 1) % FALLBACK_PALETTE.length + FALLBACK_PALETTE.length) % FALLBACK_PALETTE.length;
	return FALLBACK_PALETTE[idx];
}

/**
 * Convenience: look up a player by id from a list and return their color.
 * Returns OOC_COLOR if id is null or the player isn't found — the
 * "speaker is the system / OOC" fallback for chat rendering.
 */
export function playerColorByID(
	id: number | null | undefined,
	players: Pick<Player, 'id' | 'token_color' | 'seat_order'>[]
): string {
	if (id == null) return OOC_COLOR;
	const p = players.find(pl => pl.id === id);
	return p ? playerColor(p) : OOC_COLOR;
}
