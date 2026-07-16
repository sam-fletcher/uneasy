// choosing.ts — pure helpers for the Prologue "choosing" accordion
// (Sessions 1–2 of adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md).

import type { PrologueSheet, PrologueClaim, PlayerCardRow, Asset, Player } from '$lib/api';

/** How many boxes on this sheet remain unclaimed. */
export function openCount(sheet: PrologueSheet, claims: PrologueClaim[]): number {
	const claimedNames = new Set(
		claims.filter((c) => c.sheet_type === sheet.type).map((c) => c.choice_name)
	);
	return sheet.choices.filter((c) => !claimedNames.has(c.name)).length;
}

/** Every "suit::value" pair currently sitting in some player's hand. */
export function heldCardSet(cards: PlayerCardRow[]): Set<string> {
	return new Set(cards.map((c) => `${c.card_suit}::${c.card_value}`));
}

export interface StealPreview {
	ownerName: string;
	/** Null when the holder's linked asset can't be resolved (destroyed or
	 *  not found) — callers fall back to owner-only wording. */
	assetName: string | null;
}

/**
 * What claiming this card would mean, for the tap-to-explore expansion
 * (Session 2). Null if the card is still fresh (nobody holds it, so
 * claiming it makes a new asset rather than taking one).
 */
export function stealPreview(
	suit: string,
	value: string,
	cards: PlayerCardRow[],
	assets: Asset[],
	players: Player[]
): StealPreview | null {
	const holder = cards.find((c) => c.card_suit === suit && c.card_value === value);
	if (!holder) return null;
	const ownerName = players.find((p) => p.id === holder.player_id)?.display_name ?? '?';
	const asset = assets.find(
		(a) => a.linked_card_suit === suit && a.linked_card_value === value && !a.is_destroyed
	);
	return { ownerName, assetName: asset?.name ?? null };
}
