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
	/** True on a made plan: the preparer must author the artifact (create-artifact). */
	artifact_required?: boolean;
	/** The authored societal-shift artifact; set once create-artifact runs. */
	artifact_id?: number | null;
	/** Mar (a) "give_peer": repeatable — how many of the picked give_peer
	 *  transfers have actually been carried out. "Owed" is derived from how
	 *  many times "give_peer" appears in the committed choices, not a flag. */
	give_peer_done?: number;
	/** Mar (c) "break_self": repeatable, same shape as give_peer_done. */
	break_self_done?: number;
}

/** Read-only convenience parser. Returns a non-nil object (defaults when
 *  the nested key is absent) so callers don't have to nil-check. */
export function parseSpreadPropagandaData(
	plan: Plan | null | undefined
): SpreadPropagandaResolutionData {
	return parseResolutionData(plan).spread_propaganda ?? {};
}
