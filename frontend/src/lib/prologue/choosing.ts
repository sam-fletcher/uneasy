// choosing.ts — pure helpers for the Prologue "choosing" accordion
// (Sessions 1–2 of adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md).

import type {
	PrologueSheet,
	PrologueChoice,
	PrologueClaim,
	PrologueSheetType,
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
	/** Three-letter code the UI shows instead of the suit (Round 2, decision
	 *  1). Same table as the other two readings so the headers, legend and
	 *  tile chips cannot drift apart. */
	code: string;
}[] = [
	{ suit: 'C', assetType: 'Holding', track: 'Power', code: 'POW' },
	{ suit: 'D', assetType: 'Resource', track: 'Knowledge', code: 'KNO' },
	{ suit: 'S', assetType: 'Artifact', track: 'Esteem', code: 'EST' },
	{ suit: 'H', assetType: 'Peer', track: null, code: 'WLD' },
];

/** Track name for a ranked suit, e.g. 'C' → 'Power'. Empty for an unknown
 *  suit or the wild heart, which ranks nothing by itself. */
export function trackLabel(suit: string): string {
	return SUIT_MEANINGS.find((m) => m.suit === suit)?.track ?? '';
}

/**
 * The three-letter code shown in place of a suit ('C' → 'POW'). Empty for an
 * unknown suit.
 *
 * The heart gets 'WLD' rather than a glyph: it is categorically different (a
 * track you haven't picked yet, not a track), but a picture for the fourth
 * member of a set of words breaks the set and forces a legend. It earns a
 * *treatment* instead — the dashed border in shared/trackCode.css, borrowing
 * DifficultyMeter's dashed-means-not-yet idiom.
 */
export function trackCode(suit: string): string {
	return SUIT_MEANINGS.find((m) => m.suit === suit)?.code ?? '';
}

/** Asset type a suit makes, lowercased for running text ('C' → 'holding'). */
export function assetTypeLabel(suit: string): string {
	return (SUIT_MEANINGS.find((m) => m.suit === suit)?.assetType ?? 'asset').toLowerCase();
}

/** Where the viewer's three choices have gone. */
export interface SpentChoices {
	/** Choices spent on each sheet — the pips drawn on that category header.
	 *  Sheets the viewer hasn't spent on are absent, not zero. */
	bySheet: Map<PrologueSheetType, number>;
	/** Choices spent in total. The turn card still holds `3 - total`. */
	total: number;
}

/**
 * Split the viewer's claims by category.
 *
 * Pips are one object with two homes (Round 2, decision 4): the ones you still
 * hold sit in the turn card, and spending one moves it down to the category
 * header you spent it on. Both readings come out of this single walk, so they
 * always add to three — a header pip that hadn't come out of the turn card
 * would break the whole conceit.
 *
 * It also kills the "one from each category" misread the three stacked panels
 * created: two pips on Titles and none on Hailing From looks *normal* rather
 * than incomplete, which is what the rules actually say ("if you want three
 * titles, take three titles").
 *
 * A viewer with no player row (spectator, or not yet resolved) has spent
 * nothing — and the caller draws no pips at all for them.
 */
export function spentByCategory(
	claims: PrologueClaim[],
	playerID: number | null
): SpentChoices {
	const bySheet = new Map<PrologueSheetType, number>();
	let total = 0;
	if (playerID == null) return { bySheet, total };
	for (const c of claims) {
		if (c.player_id !== playerID) continue;
		bySheet.set(c.sheet_type, (bySheet.get(c.sheet_type) ?? 0) + 1);
		total++;
	}
	return { bySheet, total };
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
