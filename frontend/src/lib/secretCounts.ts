// Secret-count rules, in one place.
//
// A secret's *existence* is public (asset.secret_count, viewer-independent),
// but its *content* is gated. The viewer holds the set of secrets they can read
// (the visible-secrets array); "known" is how many of those belong to an asset,
// and "hidden" is the public total minus that. See SECRETS_RULES.md and the
// secret-existence model note.

import type { Asset, Secret } from './api';

/** How many secrets in the viewer's visible set belong to `assetId` — i.e. the
 *  count whose content the viewer can read. */
export function knownCount(secrets: Secret[], assetId: number): number {
	let n = 0;
	for (const s of secrets) if (s.asset_id === assetId) n++;
	return n;
}

/** Secrets that exist on the asset but the viewer can't read: the public total
 *  minus the known count, clamped at zero (a stale known count can't go
 *  negative). */
export function hiddenCount(asset: Asset, known: number): number {
	return Math.max(0, asset.secret_count - known);
}
