// propose_decree.ts — typed resolution_data view for Propose Decree.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ProposeDecreeResolutionData {
	signatory_player_ids?: number[];
	signatory_id?: number | null;
	addendum?: string;
	addendum_connector?: string;
	addendum_placed?: boolean;
	/** Mar: non-preparer council amenders, lowest power first. */
	amendment_order?: number[];
	/** Mar: players who have already taken their amend turn. */
	amended_by?: number[];
	law_id?: number | null;
	/** Current law body, mirrored from the law row for the resolve panel. */
	law_text?: string;
}

export function parseProposeDecreeData(
	plan: Plan | null | undefined
): ProposeDecreeResolutionData {
	return parseResolutionData(plan).propose_decree ?? {};
}
