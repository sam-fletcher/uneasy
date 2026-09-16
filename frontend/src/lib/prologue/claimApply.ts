// claimApply.ts — apply a prologue claim to the client's projection in place.
//
// A claim used to trigger three full reloads on the claimer's screen (one per
// WebSocket echo, one on the HTTP reply) and two on everyone else's — ten and
// six requests respectively, each paying the production round-trip floor. The
// server now sends everything a claim changes about the sheets and hands in
// the `prologue.choice_claimed` payload (and the same body in the POST reply),
// so both are applied here and nothing is refetched. Assets arrive on their
// own asset.created / asset.taken events, handled by the table page.

import type { PlayerCardRow, PrologueClaim, PrologueSheetType } from '$lib/api';

/** The claim facts the server sends: WS payload and POST reply share it.
 *  `cards` is the hand rows for the choice's cards after the claim; a
 *  server that predates it omits the field, which is the fallback signal. */
export interface PrologueClaimOutcome {
	player_id: number;
	sheet_type: PrologueSheetType;
	choice_name: string;
	turn_number: number;
	cards?: PlayerCardRow[] | null;
	/** Present on the POST reply only (the WS turn marker is its own event). */
	current_player_id?: number | null;
}

/** Whether the outcome carries enough to apply locally. */
export function isApplicableClaim(d: unknown): d is PrologueClaimOutcome & { cards: PlayerCardRow[] } {
	if (!d || typeof d !== 'object') return false;
	const o = d as Partial<PrologueClaimOutcome>;
	return (
		typeof o.player_id === 'number' &&
		typeof o.sheet_type === 'string' &&
		typeof o.choice_name === 'string' &&
		Array.isArray(o.cards)
	);
}

/** Upsert one claim, keyed by sheet+choice. Re-seeing a claim (the WS echo
 *  and the HTTP reply both carry it) is a no-op rather than a duplicate. */
export function applyClaim(claims: PrologueClaim[], claim: PrologueClaim): PrologueClaim[] {
	const same = (c: PrologueClaim) =>
		c.sheet_type === claim.sheet_type && c.choice_name === claim.choice_name;
	const idx = claims.findIndex(same);
	if (idx === -1) return [...claims, claim];
	const next = claims.slice();
	next[idx] = claim;
	return next;
}

/** Replace the rows for each incoming card (matched by suit+value, which is
 *  unique per table) and append any not yet seen. A take moves an existing
 *  row to a new holder; a make adds one. */
export function applyCards(cards: PlayerCardRow[], incoming: PlayerCardRow[]): PlayerCardRow[] {
	if (incoming.length === 0) return cards;
	const key = (c: PlayerCardRow) => `${c.card_suit}|${c.card_value}`;
	const byKey = new Map(incoming.map((c) => [key(c), c]));
	const next = cards.map((c) => byKey.get(key(c)) ?? c);
	const seen = new Set(cards.map(key));
	for (const c of incoming) if (!seen.has(key(c))) next.push(c);
	return next;
}
