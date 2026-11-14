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

/** What the row is waiting for when the war costs are being settled by a vote, in
 *  the same lower-case grammar as lib/waitingOn.ts's step labels. */
export const PEACE_VOTE_LABEL = 'vote on the peace proposal';
/** The same, when one war is at a vote and another is at a payment. */
export const SETTLE_WARS_LABEL = 'settle the costs of war';

/**
 * The step label to use in place of "pay the cost of battle" when that is not what
 * the table is doing.
 *
 * A war under an open peace proposal waits on its voters, not its payers, but the
 * row state is still `await_battle_cost`, and the label is read off the kind alone,
 * so without this a voter's heading says "Your move — pay the cost of battle".
 *
 * Two wars can owe on the same row. The row state names only one of them, and not
 * a stable one, while `acting_player_ids` covers all of them. So this reads every war
 * that is blocking the row, never the one the row state happens to name:
 *   - no blocking war has a proposal open: null (the payment label stands);
 *   - every blocking war has one: the vote label, for everyone;
 *   - a mix: the viewer's own business if it is only one kind (a vote, or a
 *     payment), and otherwise a label that is true of both.
 */
export function peaceVoteLabel(
	rowState: RowState | null | undefined,
	wars: WarStateResponse[],
	viewerID: number | null,
): string | null {
	if (rowState?.kind !== 'await_battle_cost') return null;
	const blocking = wars.filter(w => w.status === 'active' && w.outstanding_costs.length > 0);
	const voting = blocking.filter(w => w.open_proposal != null);
	if (voting.length === 0) return null;
	if (voting.length === blocking.length) return PEACE_VOTE_LABEL;

	const votes = viewerID != null && voting.some(w => w.open_proposal!.awaiting.includes(viewerID));
	const pays = viewerID != null && blocking.some(
		w => w.open_proposal == null && w.outstanding_costs[0].payer_id === viewerID,
	);
	if (votes && !pays) return PEACE_VOTE_LABEL;
	if (pays && !votes) return null;
	return SETTLE_WARS_LABEL;
}

/**
 * Which war's sheet stays open when the live wars change. The one that was open,
 * if its war is still live; otherwise none. The caller stores the result, so a war
 * that drops out of the list (it ended, or the list failed to load and came back
 * empty) closes its sheet for good: a later good list that holds the same war must
 * not open it again unasked.
 */
export function keepOpenWar(openID: number | null, livePlanIDs: readonly number[]): number | null {
	return openID != null && livePlanIDs.includes(openID) ? openID : null;
}

/**
 * Does a war panel in the play-area takeover need the war's name above it? Not when
 * one band sits over one panel and already names it. Yes when two wars owe at once
 * (each panel must say which) or when the war has no band (it has just ended and
 * left a surrender claim).
 */
export function nameWarInTakeover(
	planID: number,
	takeoverPlanIDs: readonly number[],
	bandPlanIDs: ReadonlySet<number>,
): boolean {
	return takeoverPlanIDs.length > 1 || !bandPlanIDs.has(planID);
}

/**
 * One entry per war. The list a client holds should never repeat a war, but a
 * response read mid-surrender once did (the war was in both the "active" and
 * the "ended, claims open" group), and a repeat is not harmless: the play area
 * renders each war's plan in a keyed {#each}, and a duplicate key throws,
 * which trips the page's error boundary and freezes the whole table region —
 * the waiting-on bar included — until a reload. Where a war repeats, the later
 * entry wins: it was read later, so it is the fresher of the two.
 */
export function dedupeWars(wars: WarStateResponse[]): WarStateResponse[] {
	const byPlan = new Map<number, WarStateResponse>();
	for (const w of wars) byPlan.set(w.origin_plan_id, w);
	return [...byPlan.values()];
}

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
		// An ended war owes no cost of battle, but a surrender that emptied a
		// side ends the war in the same call that opens its claims, and those
		// claims still gate the row. The server lists such a war (and only
		// while a claim is open) precisely so this can surface it.
		const owesCosts = w.status === 'active' && w.outstanding_costs.length > 0;
		if (!owesCosts && w.open_claims.length === 0) continue;
		const plan = plans.find(p => p.id === w.origin_plan_id);
		// Never the same plan twice, whatever `wars` holds: the caller keys its
		// {#each} on the plan id (see dedupeWars for what a repeat costs).
		if (plan && !out.includes(plan)) out.push(plan);
	}
	return out;
}

/**
 * The Make War plans the header chip counts and the war drawer lists.
 *
 * `wars` is the game's active wars plus any ended war still owing a surrender
 * claim (listWars), so a plan with no active entry there has no live war and
 * drops out. The old filter asked whether the war had ended AND the plan had
 * resolved, which an ended war could never satisfy — it wasn't in the list to
 * be inspected — so a finished war counted forever, and the drawer it opened
 * was empty, because the panel inside hides itself once the war ends. An ended
 * war that is listed only for its claim is not a war under way: the play-area
 * takeover owns it.
 *
 * A plan with no row is still mid delay-reveal; that panel takes over the play
 * area on its own, so it is not drawer business.
 */
export function activeWarPlans(plans: Plan[], wars: WarStateResponse[]): Plan[] {
	const liveWarPlanIDs = new Set(
		wars.filter(w => w.status === 'active').map(w => w.origin_plan_id),
	);
	return plans.filter(p =>
		p.plan_type === 'make_war'
		&& p.status !== 'cancelled'
		&& p.row_number != null
		&& liveWarPlanIDs.has(p.id),
	);
}

/**
 * Should the Make War panel render nothing? Yes once the plan has resolved and
 * the war has ended — unless a surrender claim is still open. A surrender that
 * empties a side ends the war in the same call that opens its claims, and the
 * claims keep the row gated, so the panel is the only place they can be taken.
 *
 * Lives here, not inline in the component: the war arrives through an
 * onMount fetch the server renderer never runs, so a component test can't put
 * the panel in this state, and the one-line condition was the whole bug.
 */
export function warPanelHidden(plan: Plan | null | undefined, war: WarStateResponse | null): boolean {
	return plan != null
		&& plan.status === 'resolved'
		&& war != null
		&& war.status === 'ended'
		&& war.open_claims.length === 0;
}
