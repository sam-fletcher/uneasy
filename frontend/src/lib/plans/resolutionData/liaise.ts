// liaise.ts — typed resolution_data view for Clandestinely Liaise.
//
// Mirrors uneasy/game/plan_liaise_data.go. See
// RESOLUTION_DATA_TYPING_PLAN.md for the design rationale.

import type { Plan, KeptSecret } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export type LiaisePhase =
	| 'together_at_last'
	| 'secrets_we_keep'
	| 'things_we_share'
	| 'when_will_i_see_you_again'
	| 'done';

export interface LiaiseResolutionData {
	phase?: LiaisePhase;
	partner_id?: number | null;
	/** The preparer's meeting peer (the peer they brought to the liaison). */
	preparer_peer_id?: number | null;
	/** The partner's meeting peer (the peer they brought to the liaison). */
	partner_peer_id?: number | null;
	delay_reveal_id?: number | null;
	redelay_reveal_id?: number | null;
	kept_secrets?: KeptSecret[];
}

/** Read-only convenience parser. Returns a non-nil object (defaults when
 *  the nested key is absent) so callers don't have to nil-check. */
export function parseLiaiseData(plan: Plan | null | undefined): LiaiseResolutionData {
	return parseResolutionData(plan).liaise ?? {};
}
