// make_war.ts — typed resolution_data view for Make War.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface MakeWarResolutionData {
	war_id?: number | null;
	delay_reveal_id?: number | null;
	enemy_player_ids?: number[];
	scene_posted?: boolean;
}

export function parseMakeWarData(plan: Plan | null | undefined): MakeWarResolutionData {
	return parseResolutionData(plan).make_war ?? {};
}
