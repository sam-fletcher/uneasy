// Pure derivations for the table-shell header chips (per-player rank triples,
// the top rank on each track, the at-risk badge count, and the live typing
// label). Extracted from routes/table/[id]/+page.svelte so they're unit-tested
// and the shell stays a thin wiring layer. See assetRisk.ts / waitingOn.ts for
// the same "pure logic out of the .svelte" pattern.

import type { Ranking, Asset } from '$lib/api';
import { isNeedlesslyAtRisk } from '$lib/assetRisk';

const CATEGORIES = ['power', 'knowledge', 'esteem'] as const;

/** A player's standing on the three tracks. rank 1 = top, 5 = bottom; null
 *  while a track has no ranking yet. */
export type RankTriple = { power: number | null; knowledge: number | null; esteem: number | null };

function emptyTriple(): RankTriple {
	return { power: null, knowledge: null, esteem: null };
}

/**
 * Per-player rank triple, keyed by player id. Placeholder/dummy rankings
 * (player_id null) are skipped — only real players get a chip.
 */
export function rankTriplesByPlayer(rankings: Ranking[]): Map<number, RankTriple> {
	const map = new Map<number, RankTriple>();
	for (const r of rankings) {
		if (r.player_id == null) continue;
		let entry = map.get(r.player_id);
		if (!entry) {
			entry = emptyTriple();
			map.set(r.player_id, entry);
		}
		entry[r.category] = r.rank;
	}
	return map;
}

/**
 * The best (lowest-numbered) rank any *player* actually holds on each track.
 * A dummy token can occupy rank 1, so the player-held top isn't always 1 —
 * whoever holds it is highlighted gold on the header chips.
 */
export function topRanks(triples: Map<number, RankTriple>): RankTriple {
	const best = emptyTriple();
	for (const entry of triples.values()) {
		for (const cat of CATEGORIES) {
			const v = entry[cat];
			if (v != null && (best[cat] == null || v < best[cat]!)) best[cat] = v;
		}
	}
	return best;
}

/**
 * Per-owner count of "needlessly at-risk" assets, surfaced as a warning badge
 * on each header chip. See isNeedlesslyAtRisk for the exact (avoidable-only)
 * rule.
 */
export function atRiskCountByPlayer(assets: Asset[]): Map<number, number> {
	const map = new Map<number, number>();
	for (const a of assets) {
		if (isNeedlesslyAtRisk(a)) {
			map.set(a.owner_id, (map.get(a.owner_id) ?? 0) + 1);
		}
	}
	return map;
}

/** Header label for the live "… is writing" typing indicator. */
export function typingIndicatorLabel(names: string[]): string {
	if (names.length === 0) return '';
	if (names.length === 1) return `${names[0]} is writing…`;
	if (names.length === 2) return `${names[0]} and ${names[1]} are writing…`;
	return 'Several people are writing…';
}
