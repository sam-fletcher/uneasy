// Per-viewer secret-count lookup, shared with the asset-card seams via context.
//
// "Known" counts are viewer-scoped and derived from the visible-secrets array,
// which lives high up (the table page). The asset cards that need them sit deep
// behind generic, secrets-agnostic wrappers (CardPicker, FormField). Rather
// than thread the data through those layers, the provider publishes a lookup
// once and the seams (CardPicker + the scene/dice panels) read it directly.
//
// The leaf AssetCardSelectable stays dumb and prop-driven — the seams pass it a
// plain `knownSecretCount`. This context is only the delivery channel to those
// seams, not something the leaf reaches into.

import { getContext, setContext } from 'svelte';
import type { Secret } from './api';
import { knownCount } from './secretCounts';

const KEY = Symbol('secretCounts');

export interface SecretCounts {
	/** Secrets on the asset whose content the viewer can read. */
	known(assetId: number): number;
}

/** Provide the lookup once near the root, backed by the live visible-secrets
 *  state. `getSecrets` is invoked on each call so reads stay reactive. */
export function provideSecretCounts(getSecrets: () => Secret[]): void {
	setContext<SecretCounts>(KEY, {
		known: (assetId) => knownCount(getSecrets(), assetId),
	});
}

/** Consume the lookup at an asset-card seam. Undefined when no provider is
 *  mounted (e.g. isolated tests / stories) — seams treat that as "no secret
 *  data", so the eyes simply don't render. */
export function useSecretCounts(): SecretCounts | undefined {
	return getContext<SecretCounts>(KEY);
}
