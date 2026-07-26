// chronicle_histories.ts — typed resolution_data view for Chronicle Histories.

import type { Plan } from '$lib/api';
import { parseResolutionData } from '$lib/components/plans/shared';

export interface ChronicleHistoriesResolutionData {
	invoked_artifact_ids?: number[];
	invoke_phase_closed?: boolean;
	/** True once the mar scene begins (first mar choice submitted). */
	mar_active?: boolean;
	/**
	 * Number of players who must each submit one mar choice before the plan can
	 * complete — the player count captured when the mar scene began.
	 */
	mar_required_choices?: number;
	/**
	 * Number of make options the preparer must choose (the dice result),
	 * captured server-side on the first make-step. The picker shows
	 * (make_budget − make_choices_done) remaining.
	 */
	make_budget?: number;
	/**
	 * Make options submitted so far via make-step (server-authoritative), so a
	 * refresh doesn't re-prompt a finished choice or allow over-picking.
	 */
	make_choices_done?: number;
	/**
	 * Specifics of every committed choice — which artifact was broken or invoked,
	 * and the chronicler's narration — so the panel can show what was actually
	 * done rather than a bare option count (Tier-1 committed state, ADR-006).
	 * Both paths append: make (player_id absent) and mar (player_id set).
	 */
	steps?: ChronicleStep[];
}

/** One committed Chronicle Histories choice, with its specifics. */
export interface ChronicleStep {
	/** Absent on the make path (the preparer); set on the mar path. */
	player_id?: number;
	option: string;
	/** The artifact broken or newly invoked; absent for the narrative options. */
	asset_id?: number;
	/** The chronicler's summary of the beat. Make path only. */
	narration?: string;
}

export function parseChronicleHistoriesData(
	plan: Plan | null | undefined
): ChronicleHistoriesResolutionData {
	return parseResolutionData(plan).chronicle_histories ?? {};
}
