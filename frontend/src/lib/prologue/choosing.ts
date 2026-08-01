// choosing.ts — pure helpers for the Prologue "choosing" accordion
// (Sessions 1–2 of adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md).

import type {
	PrologueSheet,
	PrologueChoice,
	PrologueClaim,
	PlayerCardRow,
	Asset,
	Player,
} from '$lib/api';

export type CardSuit = 'C' | 'D' | 'S' | 'H';
/** The three ranked tracks' suits. Hearts are wild and rank nothing on their
 *  own — they're declared as a suit during the hearts step instead. */
export type TrackSuit = 'C' | 'D' | 'S';

const TRACK_SUITS: TrackSuit[] = ['C', 'D', 'S'];

/**
 * The two things a suit tells you, in one table.
 *
 * Every card glyph in the choosing view carries both readings at once — the
 * asset it makes *and* the ranking track it feeds — and until now the UI only
 * ever taught the first. Worse, the two readings collide (♠ makes an artifact
 * but raises Esteem), so a player who learns one meaning is actively misled
 * about the other. One table, so the legend and the sheet headers can't drift.
 *
 * Ordered Power → Knowledge → Esteem to match TrackBoard's columns, with the
 * wild heart last.
 */
export const SUIT_MEANINGS: {
	suit: CardSuit;
	/** Asset type this suit makes (PROLOGUE_RULES.md "Making Card Assets"). */
	assetType: string;
	/** Ranking track this suit is counted on; null for the wild heart. */
	track: string | null;
}[] = [
	{ suit: 'C', assetType: 'Holding', track: 'Power' },
	{ suit: 'D', assetType: 'Resource', track: 'Knowledge' },
	{ suit: 'S', assetType: 'Artifact', track: 'Esteem' },
	{ suit: 'H', assetType: 'Peer', track: null },
];

/** Track name for a ranked suit, e.g. 'C' → 'Power'. Empty for an unknown
 *  suit or the wild heart, which ranks nothing by itself. */
export function trackLabel(suit: string): string {
	return SUIT_MEANINGS.find((m) => m.suit === suit)?.track ?? '';
}

/** Asset type a suit makes, lowercased for running text ('C' → 'holding'). */
export function assetTypeLabel(suit: string): string {
	return (SUIT_MEANINGS.find((m) => m.suit === suit)?.assetType ?? 'asset').toLowerCase();
}

/** How many boxes on this sheet remain unclaimed. */
export function openCount(sheet: PrologueSheet, claims: PrologueClaim[]): number {
	const claimedNames = new Set(
		claims.filter((c) => c.sheet_type === sheet.type).map((c) => c.choice_name)
	);
	return sheet.choices.filter((c) => !claimedNames.has(c.name)).length;
}

export interface SheetTrackProfile {
	/** Ranked suits some box on this sheet supplies, in track order. */
	tracks: TrackSuit[];
	/** Ranked suits NO box on this sheet supplies — the sheet's blind spot. */
	missing: TrackSuit[];
	/** Whether any box supplies a wild heart. */
	wild: boolean;
}

/**
 * Which ranking tracks a sheet can and cannot feed.
 *
 * Each of the three sheets is missing an entire track — Titles has no ♠ at
 * all, Hailing From no ♣, Laws & Rumors no ♦ — so the category you open is
 * already a ranking decision, made before you have seen a single box. Derived
 * from the sheet data rather than hardcoded, so it tracks the rules if the
 * card pairs in game/prologue_sheets.go ever change.
 */
export function sheetTrackProfile(sheet: PrologueSheet): SheetTrackProfile {
	const seen = new Set<string>();
	for (const choice of sheet.choices) {
		for (const card of choice.cards) seen.add(card.suit);
	}
	return {
		tracks: TRACK_SUITS.filter((s) => seen.has(s)),
		missing: TRACK_SUITS.filter((s) => !seen.has(s)),
		wild: seen.has('H'),
	};
}

/**
 * What a tile's card would do *for the viewer*:
 *
 *   fresh — nobody holds it; claiming makes a new asset
 *   steal — another player holds it; claiming takes their asset
 *   mine  — the viewer already holds it; claiming does NOTHING
 *
 * The third state is the one worth marking. processPrologueCardClaim
 * (handler/prologue.go) returns early when the claimer already owns the card,
 * so a tile carrying one of your cards yields one asset instead of two, and a
 * tile carrying both yields none at all. Before this distinction existed every
 * held card wore the same steal ring, which advertised a dead tile as an
 * opportunity.
 */
export type CardHoldState = 'fresh' | 'steal' | 'mine';

/** Held cards keyed "suit::value" → what claiming them means for the viewer.
 *  Absent from the map means 'fresh'. */
export function cardHoldStates(
	cards: PlayerCardRow[],
	currentPlayerID: number | null
): Map<string, CardHoldState> {
	const out = new Map<string, CardHoldState>();
	for (const c of cards) {
		out.set(
			`${c.card_suit}::${c.card_value}`,
			currentPlayerID != null && c.player_id === currentPlayerID ? 'mine' : 'steal'
		);
	}
	return out;
}

/** How many of a tile's two cards the viewer already holds — i.e. how much of
 *  the tile a claim would silently waste. 2 means the tile grants the viewer
 *  no card assets whatsoever. */
export function ownedCardCount(
	choice: PrologueChoice,
	states: Map<string, CardHoldState>
): number {
	return choice.cards.filter((c) => states.get(`${c.suit}::${c.value}`) === 'mine').length;
}

export interface StealPreview {
	/** Who holds the card right now. Callers must compare this against the
	 *  viewer before using take wording: PROLOGUE_RULES.md "Taking Card
	 *  Assets" only transfers an asset held by *another* player, and the
	 *  server no-ops a self-take. Card pairs repeat across tiles, so a card
	 *  you already hold turning up on another tile is routine. */
	ownerID: number;
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
	return { ownerID: holder.player_id, ownerName, assetName: asset?.name ?? null };
}
