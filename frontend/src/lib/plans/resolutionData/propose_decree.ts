// propose_decree.ts — typed resolution_data view for Propose Decree.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ProposeDecreeResolutionData {
	signatory_player_ids?: number[];
	/** Eligible players who explicitly declined to join the council. */
	declined_player_ids?: number[];
	/** True once the preparer has finalized the text and opened the debate. */
	debate_started?: boolean;
	/** The applied roll outcome ("make"/"mar"); set means the decree was passed. */
	outcome?: string;
	signatory_id?: number | null;
	addendum?: string;
	addendum_connector?: string;
	addendum_placed?: boolean;
	/** Mar: non-preparer council amenders, lowest power first. */
	amendment_order?: number[];
	/** Mar: players who have already taken their amend turn. */
	amended_by?: number[];
	/** Set only once the addendum is placed (enactment); the law row's id. */
	law_id?: number | null;
	/** Current working law body (staged in resolution_data until enactment). */
	law_text?: string;
	/** Resource asset created (already named) by a made decree at enactment. */
	resource_asset_id?: number | null;
}

export function parseProposeDecreeData(
	plan: Plan | null | undefined
): ProposeDecreeResolutionData {
	return parseResolutionData(plan).propose_decree ?? {};
}
