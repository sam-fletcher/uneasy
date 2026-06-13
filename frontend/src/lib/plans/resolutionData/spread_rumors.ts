// spread_rumors.ts — typed resolution_data view for Spread Rumors.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

/** An open "take asset" consent gate, mirroring game.TakeConsentRequest.
 *  Present while the victim is being asked to agree/disagree. */
export interface TakeConsentRequest {
	choices: string[];
	result: 'make' | 'mar';
	asset_ids: number[];
	victim_id: number;
	requested_by: number;
}

export interface SpreadRumorsResolutionData {
	source_hidden?: boolean;
	rumor_id?: number | null;
	/** Set while a take-asset consent request awaits the victim's response. */
	pending_take_consent?: TakeConsentRequest | null;
	/** Set when the victim declined; disables the take-asset option on re-pick. */
	take_asset_denied?: boolean;
	/** Set once an agreed-to take has transferred; the take step is complete. */
	take_resolved?: boolean;
	/** How many picked break_target / hide_source sub-flow steps the server has
	 *  recorded as done. The per-step picker shows (picked − done) remaining, so
	 *  a refresh/remount doesn't re-prompt a completed step. */
	break_target_done?: number;
	hide_source_done?: number;
}

export function parseSpreadRumorsData(
	plan: Plan | null | undefined
): SpreadRumorsResolutionData {
	return parseResolutionData(plan).spread_rumors ?? {};
}
