// propose_decree.ts — typed resolution_data view for Propose Decree.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ProposeDecreeResolutionData {
	signatory_player_ids?: number[];
	signatory_id?: number | null;
	addendum?: string;
	law_id?: number | null;
}

export function parseProposeDecreeData(
	plan: Plan | null | undefined
): ProposeDecreeResolutionData {
	return parseResolutionData(plan).propose_decree ?? {};
}
