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
import { noStepLabel, type StepLabelFn } from '$lib/plans/stepLabel';
import { chronicleHistoriesStepLabel } from '$lib/plans/stepLabels/chronicle_histories';
import { clandestinelyLiaiseStepLabel } from '$lib/plans/stepLabels/clandestinely_liaise';
import { exchangeCourtiersStepLabel } from '$lib/plans/stepLabels/exchange_courtiers';
import { hostFestivityStepLabel } from '$lib/plans/stepLabels/host_festivity';
import { makeDemandsStepLabel } from '$lib/plans/stepLabels/make_demands';
import { makeIntroductionsStepLabel } from '$lib/plans/stepLabels/make_introductions';
import { proposeDecreeStepLabel } from '$lib/plans/stepLabels/propose_decree';
import { proposeDuelStepLabel } from '$lib/plans/stepLabels/propose_duel';
import { seekAnswersStepLabel } from '$lib/plans/stepLabels/seek_answers';
import { spreadPropagandaStepLabel } from '$lib/plans/stepLabels/spread_propaganda';
import { spreadRumorsStepLabel } from '$lib/plans/stepLabels/spread_rumors';


export interface PlanRegistryEntry {
	/** Dynamic import of the panel. Vite statically analyses these literals to
	 *  emit one chunk per panel, so the specifier must stay inline — hoisting
	 *  it into a variable would defeat the split. */
	load: () => Promise<Component<PlanPanelProps>>;
	/** What the plan calls the step it is on, for the open step's heading
	 *  ("Your move — …"). Pure and eager, unlike `load`: the heading must be right
	 *  before the panel's chunk has arrived, and the page reads it with no panel
	 *  mounted (lib/waitingOn.ts). Required, so a thirteenth plan can't forget it. */
	stepLabel: StepLabelFn;
}

const C = (m: { default: unknown }) => m.default as Component<PlanPanelProps>;

export const REGISTRY: Record<PlanType, PlanRegistryEntry> = {
	exchange_courtiers: { load: () => import('./ExchangeCourtiersPanel.svelte').then(C), stepLabel: exchangeCourtiersStepLabel },
	make_introductions: { load: () => import('./MakeIntroductionsPanel.svelte').then(C), stepLabel: makeIntroductionsStepLabel },
	spread_propaganda: { load: () => import('./SpreadPropagandaPanel.svelte').then(C), stepLabel: spreadPropagandaStepLabel },
	seek_answers: { load: () => import('./SeekAnswersPanel.svelte').then(C), stepLabel: seekAnswersStepLabel },
	spread_rumors: { load: () => import('./SpreadRumorsPanel.svelte').then(C), stepLabel: spreadRumorsStepLabel },
	chronicle_histories: { load: () => import('./ChronicleHistoriesPanel.svelte').then(C), stepLabel: chronicleHistoriesStepLabel },
	propose_decree: { load: () => import('./ProposeDecreePanel.svelte').then(C), stepLabel: proposeDecreeStepLabel },
	propose_duel: { load: () => import('./ProposeDuelPanel.svelte').then(C), stepLabel: proposeDuelStepLabel },
	host_festivity: { load: () => import('./HostFestivityPanel.svelte').then(C), stepLabel: hostFestivityStepLabel },
	make_demands: { load: () => import('./MakeDemandsPanel.svelte').then(C), stepLabel: makeDemandsStepLabel },

	// Make War and Clandestinely Liaise's simultaneous-reveal phase is
	// driven by the row_state kind 'await_delay_reveal'; MainEventView
	// renders the appropriate panel directly in the play area for every
	// player. Outside the delay reveal these plans take the standard
	// prep/resolve paths. Make War's post-placement "war drawer" view is
	// also rendered from MainEventView, not via the registry. Make War has no step
	// of its own to name (the delay vote, a cost of battle and a surrender claim are
	// all the server's own row-state kinds), hence noStepLabel.
	make_war: { load: () => import('./MakeWarPanel.svelte').then(C), stepLabel: noStepLabel },
	clandestinely_liaise: { load: () => import('./ClandestinelyLiaisePanel.svelte').then(C), stepLabel: clandestinelyLiaiseStepLabel },
};
