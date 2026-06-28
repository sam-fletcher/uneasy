// Crown lookup for the line-of-succession UI, shared with the asset-card and
// retinue surfaces via context (ADR-007, Phase D).
//
// The crown role of a title marginalia depends on the *whole game's* live
// claims, not just one asset — so it can't be derived inside a leaf from its own
// props. The full asset list + throne_established flag live high up (the table
// page). Rather than thread them through every asset-card caller (CardPicker,
// scene/dice panels, …), the provider publishes a lookup once and the
// marginalia-rendering surfaces (AssetCardSelectable, RetinueView) read it
// directly. Mirrors the secretCounts seam (secretCountsContext.ts).

import { getContext, setContext } from 'svelte';
import type { Asset } from './api';
import { computeCrowns, type CrownMark } from './succession';

const KEY = Symbol('succession');

export interface SuccessionLookup {
	/** The crown mark for a title marginalia, or undefined if it bears none. */
	crown(marginaliaID: number): CrownMark | undefined;
}

/** Provide the lookup once near the root. `getAssets`/`getThrone` are invoked on
 *  each call so reads stay reactive as assets are taken/torn/destroyed. The map
 *  is memoized on the (assets, throne) inputs so a render that crowns many
 *  marginalia recomputes it at most once, not once per lookup. */
export function provideSuccession(
	getAssets: () => Asset[],
	getThrone: () => boolean,
): void {
	let cachedAssets: Asset[] | null = null;
	let cachedThrone = false;
	let cachedMap = new Map<number, CrownMark>();

	setContext<SuccessionLookup>(KEY, {
		crown(marginaliaID) {
			const assets = getAssets();
			const throne = getThrone();
			if (assets !== cachedAssets || throne !== cachedThrone) {
				cachedAssets = assets;
				cachedThrone = throne;
				cachedMap = computeCrowns(assets, throne);
			}
			return cachedMap.get(marginaliaID);
		},
	});
}

/** Consume the lookup at a marginalia-rendering surface. Undefined when no
 *  provider is mounted (isolated tests / stories) — surfaces treat that as "no
 *  crown data" and render no crowns. */
export function useSuccession(): SuccessionLookup | undefined {
	return getContext<SuccessionLookup>(KEY);
}
