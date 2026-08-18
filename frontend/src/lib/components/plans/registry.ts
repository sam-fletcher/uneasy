// Plan registry — the single source of truth for "which Svelte component
// renders which plan type, and when does it appear out-of-band."
//
// PlanPanel reads this map and dispatches through LazyPlanPanel. Adding a
// new plan type = writing one panel that conforms to PlanPanelProps + one
// entry here.
//
// Entries are `load()` thunks rather than component references so each panel
// becomes its own Vite chunk instead of being pulled into the table route's
// bundle. All twelve together were the bulk of a 549KB (165KB gzipped) chunk
// that every table load paid for up front, in every phase — including lobby,
// prologue and the endgame, where no plan panel can render at all. They are
// now fetched the moment a player actually opens one.

import type { Component } from 'svelte';
import type { PlanType } from '$lib/api';
import type { PlanPanelProps } from './types';


export interface PlanRegistryEntry {
	/** Dynamic import of the panel. Vite statically analyses these literals to
	 *  emit one chunk per panel, so the specifier must stay inline — hoisting
	 *  it into a variable would defeat the split. */
	load: () => Promise<Component<PlanPanelProps>>;
}

const C = (m: { default: unknown }) => m.default as Component<PlanPanelProps>;

export const REGISTRY: Record<PlanType, PlanRegistryEntry> = {
	exchange_courtiers: { load: () => import('./ExchangeCourtiersPanel.svelte').then(C) },
	make_introductions: { load: () => import('./MakeIntroductionsPanel.svelte').then(C) },
	spread_propaganda: { load: () => import('./SpreadPropagandaPanel.svelte').then(C) },
	seek_answers: { load: () => import('./SeekAnswersPanel.svelte').then(C) },
	spread_rumors: { load: () => import('./SpreadRumorsPanel.svelte').then(C) },
	chronicle_histories: { load: () => import('./ChronicleHistoriesPanel.svelte').then(C) },
	propose_decree: { load: () => import('./ProposeDecreePanel.svelte').then(C) },
	propose_duel: { load: () => import('./ProposeDuelPanel.svelte').then(C) },
	host_festivity: { load: () => import('./HostFestivityPanel.svelte').then(C) },
	make_demands: { load: () => import('./MakeDemandsPanel.svelte').then(C) },

	// Make War and Clandestinely Liaise's simultaneous-reveal phase is
	// driven by the row_state kind 'await_delay_reveal'; MainEventView
	// renders the appropriate panel directly in the play area for every
	// player. Outside the delay reveal these plans take the standard
	// prep/resolve paths. Make War's post-placement "war drawer" view is
	// also rendered from MainEventView, not via the registry.
	make_war: { load: () => import('./MakeWarPanel.svelte').then(C) },
	clandestinely_liaise: { load: () => import('./ClandestinelyLiaisePanel.svelte').then(C) },
};
