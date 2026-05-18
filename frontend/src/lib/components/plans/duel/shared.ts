// Shared types + display helpers for the propose-duel sub-components.

import type { Asset, DuelStake, DuelBout } from '$lib/api';
import { assetName } from '../shared';

export type DuelRes = {
	duelType: string;
	phase: string;
	initiativeID: number | null;
	prepChampID: number | null;
	targChampID: number | null;
	prepChampDeclared: boolean;
	targChampDeclared: boolean;
	prepStakeCount: number;
	targStakeCount: number;
	currentBout: number;
	stakeCounts: Record<number, number>;
};

// Per-stake display string. While unresolved we show "hidden" or
// "hidden dN" (visible only to the stake's owner — backend redacts
// hidden_die for opponents). Once resolved, dig the rolled die out of
// the bout this stake participated in.
export function stakeLabel(s: DuelStake, assets: Asset[], bouts: DuelBout[]): string {
	const nm = assetName(assets, s.asset_id);
	if (s.is_resolved) {
		for (const b of bouts) {
			if (b.declarer_stake_id === s.id && b.declarer_die != null) {
				return `${nm} — ${b.declarer_die}${b.is_match ? ' (set aside)' : ''}`;
			}
			if (b.responder_stake_id === s.id && b.responder_die != null) {
				return `${nm} — ${b.responder_die}${b.is_match ? ' (set aside)' : ''}`;
			}
		}
		return `${nm} — resolved`;
	}
	if (s.hidden_die != null) {
		return `${nm} — hidden d${s.hidden_die}`;
	}
	return `${nm} — hidden`;
}

// Accumulated dice per side, matching backend carryover: ties accumulate
// into `pending` and go to the winner of the next non-tie bout.
export function computeAccumulated(
	bouts: DuelBout[],
	preparerID: number | null,
): { prep: number[]; targ: number[]; pending: number[] } {
	const prep: number[] = [];
	const targ: number[] = [];
	let pending: number[] = [];
	if (preparerID == null) return { prep, targ, pending };
	for (const b of bouts) {
		if (b.declarer_die == null || b.responder_die == null) continue;
		if (b.is_match) {
			pending.push(b.declarer_die, b.responder_die);
			continue;
		}
		if (b.winner_id == null) continue;
		const gained = [b.declarer_die, b.responder_die, ...pending];
		pending = [];
		if (b.winner_id === preparerID) prep.push(...gained);
		else targ.push(...gained);
	}
	return { prep, targ, pending };
}
