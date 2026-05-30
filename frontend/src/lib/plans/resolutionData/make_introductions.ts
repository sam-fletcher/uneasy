// make_introductions.ts — typed resolution_data view for Make Introductions.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface MIMarOutcome {
	peer_asset_id: number;
	outcome: 'other_retinue' | 'broken_arrival' | 'delayed' | 'broken_journey';
	author_player_id?: number | null;
	done: boolean;
}

export interface MakeIntroductionsResolutionData {
	peer_count?: number;
	/** Asset IDs created so far via /create-peer in the pre-roll naming step. */
	created_peer_ids?: number[];
	delayed_peer_plan_ids?: number[];
	/** Set when the roll marred: the focus player must resolve each peer. */
	mar_pending?: boolean;
	/** Per-peer mar resolution (one entry per resolved peer). */
	mar_outcomes?: MIMarOutcome[];
	/** Fields below only set on synthetic delayed-arrival child plans. */
	delayed_arrival?: boolean;
	delayed_peer_asset_id?: number | null;
	original_plan_id?: number | null;
}

export function parseMakeIntroductionsData(
	plan: Plan | null | undefined
): MakeIntroductionsResolutionData {
	return parseResolutionData(plan).make_introductions ?? {};
}
