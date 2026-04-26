// Plan registry — per-plan metadata that PlanPanel needs at dispatch time
// without baking plan-type knowledge into PlanPanel itself.
//
// TODO(refactor B2-B5 follow-up): expand this into a full PlanComponent
// contract once Make Demands lands. Each entry should carry the prep,
// resolve, and alwaysOn Svelte components for that plan type, so PlanPanel's
// remaining prep/resolve dispatch chains collapse into a registry loop.
// Tracking this in REFACTORING_PLAN.md (Stream B, "Adjustments" section).

import type { Plan, PlanType } from '$lib/api';

/** Per-plan rule for "render this plan even when it isn't the resolving plan."
 *  shouldRender is a pure predicate over the plan + the current viewer; the
 *  underlying panel may still self-hide for state it knows about (e.g. Make
 *  War's "war ended" check, which requires fetched war state). */
export interface AlwaysOnSpec {
	shouldRender: (plan: Plan, viewerID: number | null) => boolean;
}

/** Plans that render out-of-band (independent of resolving status). The
 *  predicate replaces ad-hoc filters in PlanPanel. */
export const ALWAYS_ON: Partial<Record<PlanType, AlwaysOnSpec>> = {
	// Make War: render the per-plan war view (delay reveal, status, cost
	// picker, peace flow, surrender claims) for every non-cancelled war
	// plan. The panel itself hides further once the underlying war has
	// ended AND the plan has resolved.
	make_war: {
		shouldRender: (plan) => plan.status !== 'cancelled',
	},

	// Clandestinely Liaise: surface the simultaneous delay-reveal UI to
	// the two participants (preparer + target) while the plan is still
	// pending at row 0, before normal resolution begins.
	clandestinely_liaise: {
		shouldRender: (plan, viewerID) =>
			plan.status === 'pending'
			&& plan.row_number === 0
			&& viewerID != null
			&& (plan.preparer_id === viewerID || plan.target_player_id === viewerID),
	},
};
