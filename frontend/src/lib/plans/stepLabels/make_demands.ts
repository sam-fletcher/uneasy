// Make Demands: roll, then a make drafts four options (`await_demand_draft_pick`) and
// a mar waits for the counter-demand (`await_demand_counter`). Both end the plan by
// themselves, so what is left on the generic kind is the moment after the last pick
// or the counter, where the demander may still press "Complete plan".

import { parseMakeDemandsData } from '$lib/plans/resolutionData/make_demands';
import type { StepLabelFn } from '../stepLabel';

export const makeDemandsStepLabel: StepLabelFn = ({ plan, roll }) => {
	const md = parseMakeDemandsData(plan);
	if (roll?.outcome === 'make' && (md.draft_choices ?? []).length >= 4) return 'complete the plan';
	if (roll?.outcome === 'mar' && md.counter_demand_placed) return 'complete the plan';
	return null;
};
