// Shared "needlessly at risk" logic for assets.
//
// IMPORTANT: this is deliberately NARROWER than "at risk of destruction". It
// flags only the *avoidable* case — an asset that is one (or zero) tears from
// destruction but whose owner could still write into an empty marginalia slot
// to shore it up. A fragile-but-full asset (e.g. 1 intact + 3 torn marginalia)
// is genuinely at risk yet excluded here: there's no empty slot to fill, so the
// owner can't de-risk it by adding a note. Don't reach for these helpers when
// you mean "could this asset be destroyed?" — they answer "should we nudge the
// owner to fill a slot?".

import type { Asset } from '$lib/api';

/**
 * True when an asset is *needlessly* at risk: alive, ≤1 intact (untorn)
 * marginalia — so one tear from destruction — AND still has at least one empty
 * slot the owner could fill to avert it. Brand-new assets (0 intact, 4 empty)
 * count: they also need a note before they're safe.
 */
export function isNeedlesslyAtRisk(asset: Asset): boolean {
	if (asset.is_destroyed) return false;
	const intact = asset.marginalia.filter((m) => !m.is_torn).length;
	const hasEmptySlot = asset.marginalia.length < 4;
	return intact <= 1 && hasEmptySlot;
}

/**
 * Index (0–3) of the first empty marginalia slot in an asset's 4-slot grid, or
 * null if all four positions are occupied. Slots are 1-indexed by `position`;
 * the returned index is 0-based to match `slotsFor()`-style arrays. Use this to
 * point an at-risk asset's owner at the exact slot to fill.
 */
export function firstEmptySlotIndex(asset: Asset): number | null {
	const filled = new Set(asset.marginalia.map((m) => m.position));
	for (let pos = 1; pos <= 4; pos++) {
		if (!filled.has(pos)) return pos - 1;
	}
	return null;
}

/**
 * Pre-tear warning for plan panels: when a break target is needlessly at risk
 * (its last note, but empty slots remain), nudge the owner to top it up before
 * the tear lands. Returns '' when there's nothing to warn about, so callers can
 * render it unconditionally. Only reachable with one intact note in practice —
 * a zero-note asset has nothing to select as a tear target.
 */
export function destructionWarning(asset: Asset | null | undefined): string {
	if (!asset || !isNeedlesslyAtRisk(asset)) return '';
	return "Heads up: this is the asset's last marginalia, but there are empty slots."
		+ ' Tearing it will destroy the asset.'
		+ ' The owner should add another marginalia before you tear it.';
}
