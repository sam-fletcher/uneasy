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
 * Default palette, ordered to maximize visual distinction across a 5-player
 * table on a dark background. Index 0 = highest seat order. Keep in sync
 * with any future server-side palette so the two don't disagree if
 * token_color picking ships.
 */
const DEFAULT_PALETTE = [
	// Retuned 2026-07-19: the original neon primaries ran 3.1:1–8.1:1 as
	// byline text on --neutral-950 (sapphire failed AA; jade shouted).
	// Same five hue identities, luminance balanced into a 5.2–7.0 band.
	'#C46BE8', // Royal Amethyst
	'#5E8CFF', // Sapphire Blue
	'#2FB56E', // Vivid Jade
	'#F0566B', // Crimson Velvet
	'#EF8B33', // Blazing Citrine
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
	const seat = player.seat_order ?? 1;
	const idx = ((seat - 1) % DEFAULT_PALETTE.length + DEFAULT_PALETTE.length) % DEFAULT_PALETTE.length;
	return DEFAULT_PALETTE[idx];
}

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
