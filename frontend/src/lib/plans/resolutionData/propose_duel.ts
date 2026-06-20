// propose_duel.ts — typed resolution_data view for Propose Duel.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export type DuelPhase = 'setup' | 'bouts' | 'roll' | 'done';

export interface DuelResolutionData {
	duel_type?: string;
	preparer_champion_id?: number | null;
	target_champion_id?: number | null;
	preparer_champion_declared?: boolean;
	target_champion_declared?: boolean;
	phase?: DuelPhase;
	/** Committed stake counts, written only once BOTH duellists commit (the duel
	 *  then advances to the bouts). Left undefined during setup so a count never
	 *  leaks to the opponent before both have committed. */
	preparer_stake_count?: number;
	target_stake_count?: number;
	current_bout?: number;
	initiative_player_id?: number | null;
}

export function parseDuelData(plan: Plan | null | undefined): DuelResolutionData {
	return parseResolutionData(plan).duel ?? {};
}
