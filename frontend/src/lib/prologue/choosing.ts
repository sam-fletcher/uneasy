// choosing.ts — pure helpers for the Prologue "choosing" accordion
// (Session 1 of adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md).

import type { PrologueSheet, PrologueClaim, PlayerCardRow } from '$lib/api';

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
