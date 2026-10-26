// warGate.ts
// Pure derivation of "which Make War panels does the play area owe the player
// right now?", plus the active-war list behind the header chip and drawer.
//
// Extracted from MainEventView.svelte for the same reason waitingOn.ts was:
// this is the routing layer where a server-side gate becomes something a
// player can actually see, and it is exactly where Make War's worst bug lived.
// An outstanding cost of battle has always been a hard row-state gate
// (await_battle_cost, rulebook step 1), but the only UI that could discharge
// it reached the play area through the plan registry — which renders a plan
// only while its status is 'resolving'. The cost of battle falls on every
// LATER row, long after Make War itself resolved, so the player who had to act
// was shown an empty play area and a 32px header chip. The gate was right; the
// surfacing was missing.
//
// Keeping the derivation here, free of runes and fetches, means the mapping
// from row-state kind → visible panel is unit-tested rather than inferred from
// a component's `{#if}` chain.

import type { Plan, RowState, WarStateResponse } from '$lib/api';

/** Row-state kinds whose gate is a Make War obligation the war panel can
 *  discharge: an unpaid cost of battle, or an unredeemed surrender claim.
 *  Both sit ABOVE plan resolution in ComputeRowState's precedence chain
 *  (priorities 3 and 4), so when either is open the row is blocked on it and
 *  nothing else in the play area can proceed — which is why the panel takes
 *  the play area over rather than sitting below the scene. */
const WAR_GATE_KINDS: ReadonlySet<string> = new Set([
	'await_battle_cost',
	'await_surrender_claim',
]);

/** Is the row currently blocked on a Make War obligation? */
export function warGateActive(rowState: RowState | null | undefined): boolean {
	return rowState != null && WAR_GATE_KINDS.has(rowState.kind);
}

/**
 * The Make War plans whose panels the play area must show while a war gate is
 * open — one per war that still owes something on this row.
 *
 * Driven off the war state rather than `rowState.war_id` alone: the row state
 * names one war (clients fetch the rest separately) and carries no war id at
 * all for await_surrender_claim, while two wars can owe on the same row. Every
 * war with something outstanding is surfaced, so no obligation is hidden
 * behind another war's panel.
 *
 * Returns [] when no gate is open, and also while `wars` is still in flight —
 * the caller then renders its normal play area, which is already gated and
 * carries the WaitingOnBar, rather than a placeholder.
 */
export function warGatePlans(
	rowState: RowState | null | undefined,
	wars: WarStateResponse[],
	plans: Plan[],
): Plan[] {
	if (!warGateActive(rowState)) return [];
	const out: Plan[] = [];
	for (const w of wars) {
		if (w.status !== 'active') continue;
		if (w.outstanding_costs.length === 0 && w.open_claims.length === 0) continue;
		const plan = plans.find(p => p.id === w.origin_plan_id);
		if (plan) out.push(plan);
	}
	return out;
}

/**
 * The Make War plans the header chip counts and the war drawer lists.
 *
 * `wars` is the game's ACTIVE wars (listWars returns no others), so a plan
 * with no entry there has no live war and drops out. That membership test is
 * the whole filter: the old one asked whether the war had ended AND the plan
 * had resolved, which an ended war could never satisfy — it isn't in the list
 * to be inspected — so a finished war counted forever, and the drawer it
 * opened was empty, because the panel inside hides itself once the war ends.
 *
 * A plan with no row is still mid delay-reveal; that panel takes over the play
 * area on its own, so it is not drawer business.
 */
export function activeWarPlans(plans: Plan[], wars: WarStateResponse[]): Plan[] {
	const liveWarPlanIDs = new Set(wars.map(w => w.origin_plan_id));
	return plans.filter(p =>
		p.plan_type === 'make_war'
		&& p.status !== 'cancelled'
		&& p.row_number != null
		&& liveWarPlanIDs.has(p.id),
	);
}
