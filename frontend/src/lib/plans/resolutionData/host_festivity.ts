// host_festivity.ts — typed resolution_data view for Host Festivity.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export type FestivityPhase = 'socializing' | 'host_choosing' | 'done';

export interface PendingChallenge {
	challenger_id: number;
	target_id: number;
	notes?: string;
}

export interface FestivityResolutionData {
	phase?: FestivityPhase;
	guests?: number[];
	outcomes?: Record<string, string>;
	guest_makes?: Record<string, string>;
	guest_mars?: Record<string, string>;
	host_choices?: Record<string, string>;
	guest_roll_ids?: Record<string, number>;
	guest_ious?: number[];
	host_mar_insists?: string[];
	accept_duels?: number[];
	pending_duel_plan_id?: number | null;
	pending_challenge?: PendingChallenge | null;
	centered_asset_ids?: number[];
}

export function parseFestivityData(plan: Plan | null | undefined): FestivityResolutionData {
	return parseResolutionData(plan).festivity ?? {};
}
