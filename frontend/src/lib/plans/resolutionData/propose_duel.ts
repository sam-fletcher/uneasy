// propose_duel.ts — typed resolution_data view for Propose Duel.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export type DuelPhase = 'setup' | 'staking' | 'bouts' | 'roll' | 'done';

export interface DuelResolutionData {
	duel_type?: string;
	preparer_champion_id?: number | null;
	target_champion_id?: number | null;
	preparer_champion_declared?: boolean;
	target_champion_declared?: boolean;
	phase?: DuelPhase;
	preparer_stake_count?: number;
	target_stake_count?: number;
	current_bout?: number;
	initiative_player_id?: number | null;
	/** Pre-reveal accumulator for stake-reveal submissions; keyed by player ID.
	 *  Vestigial once both have submitted and the canonical stake counts are set. */
	stake_counts?: Record<number, number>;
}

export function parseDuelData(plan: Plan | null | undefined): DuelResolutionData {
	return parseResolutionData(plan).duel ?? {};
}
