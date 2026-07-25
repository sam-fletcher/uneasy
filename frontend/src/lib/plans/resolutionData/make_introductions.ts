// make_introductions.ts — typed resolution_data view for Make Introductions.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';
import type { DraftPeer } from './draft_peer';

// A peer who exists in the fiction but owns no asset row yet: named in the
// pre-roll here, materialized on arrival. The shape is shared with Host
// Festivity, so it lives in draft_peer.ts; re-exported for the existing
// importers (notably $lib/api's type barrel).
export type { DraftPeer };

/** One draft that has materialized, paired with the asset it became. */
export interface MIArrival {
	draft_id: string;
	asset_id: number;
}

export interface MIMarOutcome {
	draft_id: string;
	outcome: 'other_retinue' | 'broken_arrival' | 'delayed' | 'broken_journey';
	/** The player who owes this peer's marginalia before they can arrive. */
	author_player_id?: number | null;
	/** The retinue the peer joins once that marginalia is written. */
	owner_player_id?: number | null;
	done: boolean;
}

export interface MakeIntroductionsResolutionData {
	peer_count?: number;
	/** Peers named in the pre-roll. Drafts, not assets: nothing is in a retinue
	 *  until it appears in `arrivals`. */
	drafts?: DraftPeer[];
	/** Drafts that have materialized into real assets. */
	arrivals?: MIArrival[];
	delayed_peer_plan_ids?: number[];
	/** Set when the roll made: every draft still owes its arrival form. */
	make_pending?: boolean;
	/** Set when the roll marred: the preparer must resolve each peer. */
	mar_pending?: boolean;
	/** Per-peer mar resolution (one entry per resolved draft). */
	mar_outcomes?: MIMarOutcome[];
	/** Fields below only set on synthetic delayed-arrival child plans. */
	delayed_arrival?: boolean;
	delayed_draft?: DraftPeer | null;
	original_plan_id?: number | null;
}

export function parseMakeIntroductionsData(
	plan: Plan | null | undefined
): MakeIntroductionsResolutionData {
	return parseResolutionData(plan).make_introductions ?? {};
}

/** The peers this plan is introducing, whichever shape the plan takes: a normal
 *  plan lists them in `drafts`, a synthetic delayed-arrival plan carries the one
 *  traveller in `delayed_draft`. */
export function miDrafts(mi: MakeIntroductionsResolutionData): DraftPeer[] {
	if (mi.delayed_arrival) return mi.delayed_draft ? [mi.delayed_draft] : [];
	return mi.drafts ?? [];
}

/** Whether a draft has already materialized into an asset. */
export function miHasArrived(mi: MakeIntroductionsResolutionData, draftID: string): boolean {
	return (mi.arrivals ?? []).some(a => a.draft_id === draftID);
}

/** Drafts still owing an arrival form on this plan, in naming order. */
export function miPendingArrivals(mi: MakeIntroductionsResolutionData): DraftPeer[] {
	if (!mi.make_pending && !mi.delayed_arrival) return [];
	return miDrafts(mi).filter(d => !miHasArrived(mi, d.id));
}
