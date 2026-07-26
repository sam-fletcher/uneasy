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
	/**
	 * Set when the preparer chose "keep it secret for now" at prep time: the rumor
	 * text lives in the Secret on secret_asset_id rather than in the (blanked)
	 * preparation_notes, until a make publishes it. Metadata only — the prepared
	 * log post already names the holding asset, and the Secret's text is reachable
	 * only through the per-asset, grant-checked secrets endpoint.
	 */
	is_secret?: boolean;
	secret_asset_id?: number | null;
	secret_id?: number | null;
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
	/**
	 * Specifics of each completed step, shown read-only to every viewer (Tier-1,
	 * ADR-006). All public facts: hide_source is fiction-level anonymity that the
	 * plan.prepared post and the hide-source log entry already give away, so naming
	 * the sheltering asset leaks nothing. Secret text never passes through here.
	 */
	broken_asset_ids?: number[];
	hide_source_asset_ids?: number[];
	taken_asset_ids?: number[];
}

export function parseSpreadRumorsData(
	plan: Plan | null | undefined
): SpreadRumorsResolutionData {
	return parseResolutionData(plan).spread_rumors ?? {};
}
