// Line-of-succession display logic for the crown UI (ADR-007, Phase D).
//
// "The monarch" is computed game state, not a stored pointer: it is the
// controller of the asset bearing the highest-priority *title* that is still
// live (its marginalia untorn, its asset not destroyed). Tear or destroy that
// claim and the next title in SUCCESSION_ORDER ascends. The authoritative
// computation lives server-side (handler/monarch.go); this module mirrors it for
// rendering only — every input is already on the client (assets carry
// marginalia.title; the game carries throne_established), so the crowns follow
// ADR-006's "show rich state to all" principle without a round trip.
//
// Keep SUCCESSION_ORDER in sync with game/succession.go (the on-disk contract).

import type { Asset } from './api';

// Throne-line title ids, earliest = highest claim. Mirrors game.SuccessionOrder.
// A title not in this list never confers a place in the succession.
export const SUCCESSION_ORDER = [
	'monarch',
	'true_heir',
	'favored_heir',
	'claimant',
	'consort',
	'general',
] as const;

// The general is in the mechanical line of succession (last-ditch heir) but is
// deliberately hidden from the *successor* UI: narratively they are not in the
// official line, merely positioned to seize control once it collapses. So they
// never show a waiting-successor crown — but if they end up the reigning monarch
// (the whole line above them is gone), the monarch crown does appear, an
// evocative "the General has seized the throne" reveal.
const HIDDEN_AS_SUCCESSOR = 'general';

const successionRank = new Map<string, number>(
	SUCCESSION_ORDER.map((id, i) => [id, i]),
);

export type CrownRole = 'monarch' | 'successor';

export interface CrownMark {
	role: CrownRole;
	/** 1-based distance from the throne for successors (1 = next in line).
	 *  Absent on the monarch. */
	ordinal?: number;
}

/** A live throne-line claim: the title's marginalia id and its succession rank. */
interface LiveClaim {
	marginaliaID: number;
	rank: number;
}

// A claim is live only if its marginalia is untorn AND its asset is not
// destroyed. Filtering is_destroyed is required: a direct asset destroy can
// remove an asset while its title marginalia is still untorn (ADR-007 §3) —
// without this we would crown a ghost.
function collectLiveClaims(assets: Asset[]): LiveClaim[] {
	const claims: LiveClaim[] = [];
	for (const asset of assets) {
		if (asset.is_destroyed) continue;
		for (const m of asset.marginalia) {
			if (m.is_torn || m.title == null) continue;
			const rank = successionRank.get(m.title);
			if (rank === undefined) continue; // title outside the line of succession
			claims.push({ marginaliaID: m.id, rank });
		}
	}
	return claims;
}

/**
 * Build the crown lookup for a game: marginalia id → its crown mark.
 *
 * Returns an empty map (no crowns anywhere) when the throne was never
 * established — a lone heir with no monarch is a powerless pretender (ADR-007
 * §2). The earliest live claim is the reigning monarch (general included, so a
 * collapsed line reveals a General-monarch). Every later live claim is a
 * successor, numbered by distance from the throne — except the general, which is
 * never marked as a waiting successor.
 */
export function computeCrowns(
	assets: Asset[],
	throneEstablished: boolean,
): Map<number, CrownMark> {
	const marks = new Map<number, CrownMark>();
	if (!throneEstablished) return marks;

	const claims = collectLiveClaims(assets);
	if (claims.length === 0) return marks; // interregnum: throne vacant

	// Sort by claim strength; the marginalia id is a stable tiebreaker for the
	// abnormal case of two assets sharing one title.
	claims.sort((a, b) => a.rank - b.rank || a.marginaliaID - b.marginaliaID);

	marks.set(claims[0].marginaliaID, { role: 'monarch' });

	let ordinal = 0;
	for (let i = 1; i < claims.length; i++) {
		const c = claims[i];
		if (SUCCESSION_ORDER[c.rank] === HIDDEN_AS_SUCCESSOR) continue;
		ordinal += 1;
		marks.set(c.marginaliaID, { role: 'successor', ordinal });
	}
	return marks;
}
