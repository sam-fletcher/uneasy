// spread_propaganda.ts — typed resolution_data view for Spread Propaganda.
//
// Mirrors uneasy/game/plan_spread_propaganda_data.go. See
// RESOLUTION_DATA_TYPING_PLAN.md for the design rationale.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface SpreadPropagandaResolutionData {
	recursive_plan_id?: number | null;
	esteem_lockout?: boolean;
	original_plan_id?: number | null;
}

/** Read-only convenience parser. Returns a non-nil object (defaults when
 *  the nested key is absent) so callers don't have to nil-check. */
export function parseSpreadPropagandaData(
	plan: Plan | null | undefined
): SpreadPropagandaResolutionData {
	return parseResolutionData(plan).spread_propaganda ?? {};
}
