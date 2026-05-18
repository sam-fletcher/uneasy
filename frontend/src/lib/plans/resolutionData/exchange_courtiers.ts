// exchange_courtiers.ts — typed resolution_data view for Exchange Courtiers.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ExchangeCourtiersResolutionData {
	fair_trade_asset_id?: number | null;
	fair_trade_accepted?: boolean | null;
	messy_break_required?: boolean;
	messy_break_done?: boolean;
}

export function parseExchangeCourtiersData(
	plan: Plan | null | undefined
): ExchangeCourtiersResolutionData {
	return parseResolutionData(plan).exchange_courtiers ?? {};
}
