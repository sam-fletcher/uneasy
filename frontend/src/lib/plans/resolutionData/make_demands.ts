// make_demands.ts — typed resolution_data view for Make Demands.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface DraftChoice {
	player_id: number;
	option: string;
}

export interface MakeDemandsResolutionData {
	draft_choices?: DraftChoice[];
	counter_demand_placed?: boolean;
}

export function parseMakeDemandsData(
	plan: Plan | null | undefined
): MakeDemandsResolutionData {
	return parseResolutionData(plan).make_demands ?? {};
}
