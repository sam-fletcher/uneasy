// make_introductions.ts — typed resolution_data view for Make Introductions.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface MakeIntroductionsResolutionData {
	peer_count?: number;
	delayed_peer_plan_ids?: number[];
	/** Fields below only set on synthetic delayed-arrival child plans. */
	delayed_arrival?: boolean;
	delayed_peer_asset_id?: number | null;
	original_plan_id?: number | null;
}

export function parseMakeIntroductionsData(
	plan: Plan | null | undefined
): MakeIntroductionsResolutionData {
	return parseResolutionData(plan).make_introductions ?? {};
}
