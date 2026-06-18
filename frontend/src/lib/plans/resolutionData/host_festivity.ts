// host_festivity.ts — typed resolution_data view for Host Festivity.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

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
	pending_host_mars?: string[];
	accept_duels?: number[];
	pending_duel_plan_id?: number | null;
	pending_challenge?: PendingChallenge | null;
	centered_asset_ids?: number[];
	disagreement_asset_ids?: number[];
}

export function parseFestivityData(plan: Plan | null | undefined): FestivityResolutionData {
	return parseResolutionData(plan).festivity ?? {};
}
