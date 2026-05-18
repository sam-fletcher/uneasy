// chronicle_histories.ts — typed resolution_data view for Chronicle Histories.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ChronicleHistoriesResolutionData {
	invoked_artifact_ids?: number[];
	invoke_phase_closed?: boolean;
}

export function parseChronicleHistoriesData(
	plan: Plan | null | undefined
): ChronicleHistoriesResolutionData {
	return parseResolutionData(plan).chronicle_histories ?? {};
}
