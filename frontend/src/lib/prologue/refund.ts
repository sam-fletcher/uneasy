// refund.ts — TypeScript port of game/prologue_refund.go.
//
// Computes the per-track ranking and bright/grey state of committed
// hearts client-side so the UI can update instantly as commitments
// change. The server runs the same algorithm at resolution to lock in
// bright hearts and refund grey ones; the two implementations must stay
// in agreement on rank/bright outputs.
//
// See PROLOGUE_RANKING_UI_PLAN.md for the design intent.

import type { PrologueTrack, CommittedHeart, PlayerCardRow } from '$lib/api';

const SUIT_FOR_TRACK: Record<PrologueTrack, 'C' | 'D' | 'S'> = {
	power: 'C',
	knowledge: 'D',
	esteem: 'S'
};

const VALUE_RANK: Record<string, number> = {
	A: 14,
	K: 13,
	Q: 12,
	J: 11,
	'10': 10,
	'9': 9,
	'8': 8,
	'7': 7,
	'6': 6,
	'5': 5,
	'4': 4,
	'3': 3,
	'2': 2
};

export function cardRank(value: string): number {
	return VALUE_RANK[value] ?? 0;
}

export interface TrackResult {
	/** Player IDs in rank order (rank 1 first). */
	ranked: number[];
	/** Player IDs that are set aside (zero count for this track). */
	setAside: number[];
}

/**
 * Compute the rank ordering for a single track given the current
 * committed-hearts state. Mirrors `ComputeTrackRankingFromCommitments`
 * in game/prologue_refund.go.
 */
export function computeTrackRanking(
	track: PrologueTrack,
	allPlayerIDs: number[],
	cards: PlayerCardRow[],
	committed: CommittedHeart[]
): TrackResult {
	const suit = SUIT_FOR_TRACK[track];
	const natural = new Map<number, string[]>();
	for (const c of cards) {
		if (c.card_suit === suit) {
			pushTo(natural, c.player_id, c.card_value);
		}
	}
	const heartsForTrack = new Map<number, string[]>();
	for (const h of committed) {
		if (h.track === track) {
			pushTo(heartsForTrack, h.player_id, h.value);
		}
	}
	return rankFromContributions(allPlayerIDs, natural, heartsForTrack);
}

interface RankItem {
	playerID: number;
	count: number;
	valSorted: string[];
	isHeart: boolean[];
}

function rankFromContributions(
	allPlayerIDs: number[],
	natural: Map<number, string[]>,
	hearts: Map<number, string[]>
): TrackResult {
	const items: RankItem[] = [];
	const setAside: number[] = [];
	for (const pid of allPlayerIDs) {
		const nat = natural.get(pid) ?? [];
		const hs = hearts.get(pid) ?? [];
		if (nat.length + hs.length === 0) {
			setAside.push(pid);
			continue;
		}
		const combined: string[] = [];
		const flags: boolean[] = [];
		for (const v of nat) {
			combined.push(v);
			flags.push(false);
		}
		for (const v of hs) {
			combined.push(v);
			flags.push(true);
		}
		const idx = combined.map((_, i) => i);
		// Sort descending by card rank; non-heart sorts before heart at ties.
		idx.sort((a, b) => {
			const ra = cardRank(combined[a]);
			const rb = cardRank(combined[b]);
			if (ra !== rb) return rb - ra;
			if (flags[a] === flags[b]) return 0;
			return flags[a] ? 1 : -1;
		});
		items.push({
			playerID: pid,
			count: combined.length,
			valSorted: idx.map((i) => combined[i]),
			isHeart: idx.map((i) => flags[i])
		});
	}

	items.sort((a, b) => {
		if (a.count !== b.count) return b.count - a.count;
		const n = Math.min(a.valSorted.length, b.valSorted.length);
		for (let k = 0; k < n; k++) {
			const ra = cardRank(a.valSorted[k]);
			const rb = cardRank(b.valSorted[k]);
			if (ra !== rb) return rb - ra;
			if (a.isHeart[k] !== b.isHeart[k]) {
				// Heart loses the tie at this position → it sorts later.
				return a.isHeart[k] ? 1 : -1;
			}
		}
		return b.valSorted.length - a.valSorted.length;
	});

	return { ranked: items.map((it) => it.playerID), setAside };
}

/**
 * Determine which committed hearts on a track are bright (necessary
 * to maintain the player's final slot) vs grey (would be refunded if
 * the track resolved now). Mirrors `ComputeBrightHearts` in
 * game/prologue_refund.go.
 *
 * "Final slot" treats set-aside players as just having zero cards:
 * they're appended at the end of the ranked sequence in player_id
 * order and slotted into the remaining open ranks. This means a
 * heart that promotes a zero-card player from set-aside to ranked
 * but doesn't change which slot they end up occupying is correctly
 * identified as grey.
 *
 * Returns a map: playerID → set of bright card IDs. Hearts committed
 * to this track but absent from the set are grey.
 */
export function computeBrightHearts(
	track: PrologueTrack,
	allPlayerIDs: number[],
	cards: PlayerCardRow[],
	committed: CommittedHeart[]
): Map<number, Set<number>> {
	const baseline = computeFinalSlots(track, allPlayerIDs, cards, committed);

	const perPlayer = new Map<number, CommittedHeart[]>();
	for (const h of committed) {
		if (h.track !== track) continue;
		pushTo(perPlayer, h.player_id, h);
	}
	for (const [pid, hs] of perPlayer) {
		hs.sort((a, b) => cardRank(b.value) - cardRank(a.value));
		perPlayer.set(pid, hs);
	}

	const result = new Map<number, Set<number>>();
	for (const [pid, hs] of perPlayer) {
		const greyed = new Set<number>();
		for (const h of hs) {
			const trial = committed.filter(
				(c) => c.card_id !== h.card_id && !greyed.has(c.card_id)
			);
			const trialSlots = computeFinalSlots(track, allPlayerIDs, cards, trial);
			if (trialSlots.get(pid) === baseline.get(pid)) {
				greyed.add(h.card_id);
			}
		}
		const bright = new Set<number>();
		for (const h of hs) {
			if (!greyed.has(h.card_id)) bright.add(h.card_id);
		}
		result.set(pid, bright);
	}
	return result;
}

/**
 * Returns each player's final rank slot (1..5) for the given
 * commitment state. Set-asides are appended in player_id order.
 */
export function computeFinalSlots(
	track: PrologueTrack,
	allPlayerIDs: number[],
	cards: PlayerCardRow[],
	committed: CommittedHeart[]
): Map<number, number> {
	const r = computeTrackRanking(track, allPlayerIDs, cards, committed);
	const sortedSetAside = [...r.setAside].sort((a, b) => a - b);
	const seq = [...r.ranked, ...sortedSetAside];
	const open = openRanksForCount(allPlayerIDs.length);
	const out = new Map<number, number>();
	seq.forEach((pid, i) => {
		if (i < open.length) out.set(pid, open[i]);
	});
	return out;
}

export function openRanksForCount(n: number): number[] {
	let dummies: number[] = [];
	switch (n) {
		case 4: dummies = [3]; break;
		case 3: dummies = [1, 5]; break;
		case 2: dummies = [1, 3, 5]; break;
	}
	const out: number[] = [];
	for (let r = 1; r <= 5; r++) {
		if (!dummies.includes(r)) out.push(r);
	}
	return out;
}

function pushTo<K, V>(m: Map<K, V[]>, k: K, v: V) {
	const cur = m.get(k);
	if (cur) cur.push(v);
	else m.set(k, [v]);
}
