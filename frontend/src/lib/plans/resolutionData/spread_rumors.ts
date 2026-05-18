// spread_rumors.ts — typed resolution_data view for Spread Rumors.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface SpreadRumorsResolutionData {
	source_hidden?: boolean;
	rumor_id?: number | null;
}

export function parseSpreadRumorsData(
	plan: Plan | null | undefined
): SpreadRumorsResolutionData {
	return parseResolutionData(plan).spread_rumors ?? {};
}
