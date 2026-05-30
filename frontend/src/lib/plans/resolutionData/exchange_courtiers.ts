// exchange_courtiers.ts — typed resolution_data view for Exchange Courtiers.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ExchangeCourtiersResolutionData {
	fair_trade_asset_id?: number | null;
	fair_trade_accepted?: boolean | null;
	messy_break_required?: boolean;
	messy_break_done?: boolean;
	/** Mar (target-driven): riposte/forfeit each require one peer claim. */
	peer_claims_required?: number;
	peer_claims_done?: number;
	/** Set when "riposte" was chosen — enables the preparer's optional break. */
	riposte_allowed?: boolean;
}

export function parseExchangeCourtiersData(
	plan: Plan | null | undefined
): ExchangeCourtiersResolutionData {
	return parseResolutionData(plan).exchange_courtiers ?? {};
}
