// Plan registry — the single source of truth for "which Svelte component
// renders which plan type, and when does it appear out-of-band."
//
// PlanPanel reads this map and dispatches via <svelte:component>. Adding a
// new plan type = writing one panel that conforms to PlanPanelProps + one
// entry here.

import type { Component } from 'svelte';
import type { Plan, PlanType } from '$lib/api';
import type { PlanPanelProps } from './types';

import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import SpreadPropagandaPanel from './SpreadPropagandaPanel.svelte';
import SeekAnswersPanel from './SeekAnswersPanel.svelte';
import SpreadRumorsPanel from './SpreadRumorsPanel.svelte';
import ChronicleHistoriesPanel from './ChronicleHistoriesPanel.svelte';
import ProposeDecreePanel from './ProposeDecreePanel.svelte';
import ClandestinelyLiaisePanel from './ClandestinelyLiaisePanel.svelte';
import ProposeDuelPanel from './ProposeDuelPanel.svelte';
import HostFestivityPanel from './HostFestivityPanel.svelte';
import MakeWarPanel from './MakeWarPanel.svelte';
import MakeDemandsPanel from './MakeDemandsPanel.svelte';

/** Predicate over (plan, viewer) deciding whether the plan's panel should
 *  render out-of-band — i.e. independent of being the currently resolving
 *  plan. The panel may still self-hide for fetched state it learns later
 *  (e.g. Make War's "war ended" check). */
export type AlwaysOnPredicate = (plan: Plan, viewerID: number | null) => boolean;

export interface PlanRegistryEntry {
	component: Component<PlanPanelProps>;
	/** When set, the panel renders out-of-band for any plan matching this
	 *  predicate (in addition to its prep/resolve dispatches). */
	alwaysOn?: AlwaysOnPredicate;
}

const C = <T>(c: T) => c as unknown as Component<PlanPanelProps>;

export const REGISTRY: Record<PlanType, PlanRegistryEntry> = {
	exchange_courtiers: { component: C(ExchangeCourtiersPanel) },
	make_introductions: { component: C(MakeIntroductionsPanel) },
	spread_propaganda: { component: C(SpreadPropagandaPanel) },
	seek_answers: { component: C(SeekAnswersPanel) },
	spread_rumors: { component: C(SpreadRumorsPanel) },
	chronicle_histories: { component: C(ChronicleHistoriesPanel) },
	propose_decree: { component: C(ProposeDecreePanel) },
	propose_duel: { component: C(ProposeDuelPanel) },
	host_festivity: { component: C(HostFestivityPanel) },
	make_demands: { component: C(MakeDemandsPanel) },

	make_war: {
		component: C(MakeWarPanel),
		// Only surface the per-plan war view inline during the delay-reveal
		// phase (plan is pending and hasn't been placed on the public record
		// yet); MainEventView promotes this to a "play area takeover" so
		// participants roll and non-participants choose to join or stay out.
		// Once placed, the same panel is reachable via the header "War"
		// drawer instead — see warDrawer.ts.
		alwaysOn: (plan) => plan.status === 'pending' && plan.row_number == null,
	},

	clandestinely_liaise: {
		component: C(ClandestinelyLiaisePanel),
		// Surface the simultaneous delay-reveal UI to the two participants
		// (preparer + target) while the plan is still pending with no row
		// assigned, before normal resolution begins.
		alwaysOn: (plan, viewerID) =>
			plan.status === 'pending'
			&& plan.row_number == null
			&& viewerID != null
			&& (plan.preparer_id === viewerID || plan.target_player_id === viewerID),
	},
};
