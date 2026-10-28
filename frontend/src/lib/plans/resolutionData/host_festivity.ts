// host_festivity.ts — typed resolution_data view for Host Festivity.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';
import type { DraftPeer } from './draft_peer';

export interface PendingChallenge {
	challenger_id: number;
	target_id: number;
	notes?: string;
}

// Host Festivity has no phases — the event is open while the plan is resolving
// and ends when the host winds it down. There is no stored guest list either:
// every player at the table attends, so the guest set is derived from the
// game's players, not from resolution_data.
export interface FestivityResolutionData {
	outcomes?: Record<string, string>;
	guest_makes?: Record<string, string>;
	guest_mars?: Record<string, string>;
	host_makes_taken?: string[];
	guest_roll_ids?: Record<string, number>;
	guest_ious?: number[];
	host_mar_insists?: string[];
	/** Guests who insisted the host take a mar the host hasn't chosen yet. */
	unchosen_host_mars?: number[];
	/** Legacy: a mar the insisting guest chose (pre-2026-10-28 flow), awaiting
	 *  only the host's asset pick. Only games in flight across the change hold it. */
	pending_host_mars?: string[];
	accept_duels?: number[];
	pending_duel_plan_id?: number | null;
	pending_challenge?: PendingChallenge | null;
	/** Real peers shoved to the centre by a disagreement — still owned. */
	centered_asset_ids?: number[];
	disagreement_asset_ids?: number[];
	/** Peers introduced at the party. Drafts, not assets: they belong to nobody
	 *  until a guest claims one, or the event ends and they leave with their
	 *  introducer (D7). Players see these merged with `centered_asset_ids`. */
	centered_drafts?: DraftPeer[];
}

/** The plan the table is actually playing. Normally the one `resolving` plan —
 *  but a Host Festivity that spawned a duel stays `resolving` underneath it, so
 *  two are. The duel is then the live one: the play area shows it (for every
 *  player alike) and the festivity returns when it ends. Mirrors the server's
 *  planSuspendedByNested, which picks the same plan for the waiting bar. */
export function activeResolvingPlan(plans: Plan[]): Plan | null {
	const resolving = plans.filter((p) => p.status === 'resolving');
	const suspended = (p: Plan): boolean => {
		const nested = p.plan_type === 'host_festivity' ? parseFestivityData(p).pending_duel_plan_id : null;
		return nested != null && resolving.some((q) => q.id === nested);
	};
	return resolving.find((p) => !suspended(p)) ?? resolving[0] ?? null;
}

export function parseFestivityData(plan: Plan | null | undefined): FestivityResolutionData {
	return parseResolutionData(plan).festivity ?? {};
}
