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
	/** Artifact created by the make step ("societal shift"). */
	artifact_id?: number | null;
	/** True once the preparer has named the artifact. */
	artifact_named?: boolean;
	/** Mar (a) "give_peer": a peer must be handed to another player. */
	give_peer_required?: boolean;
	give_peer_done?: boolean;
	/** Mar (c) "break_self": the preparer must break one of their own assets. */
	break_self_required?: boolean;
	break_self_done?: boolean;
}

/** Read-only convenience parser. Returns a non-nil object (defaults when
 *  the nested key is absent) so callers don't have to nil-check. */
export function parseSpreadPropagandaData(
	plan: Plan | null | undefined
): SpreadPropagandaResolutionData {
	return parseResolutionData(plan).spread_propaganda ?? {};
}
