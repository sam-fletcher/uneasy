// chronicle_histories.ts — typed resolution_data view for Chronicle Histories.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ChronicleHistoriesResolutionData {
	invoked_artifact_ids?: number[];
	invoke_phase_closed?: boolean;
	/** True once the mar scene begins (first mar choice submitted). */
	mar_active?: boolean;
	/**
	 * Number of players who must each submit one mar choice before the plan can
	 * complete — the player count captured when the mar scene began.
	 */
	mar_required_choices?: number;
	/**
	 * Completed make-list break_artifact sub-flow steps (server-authoritative).
	 * The picker shows (picked − done) remaining, so a refresh doesn't re-prompt
	 * a finished break.
	 */
	break_artifact_done?: number;
}

export function parseChronicleHistoriesData(
	plan: Plan | null | undefined
): ChronicleHistoriesResolutionData {
	return parseResolutionData(plan).chronicle_histories ?? {};
}
