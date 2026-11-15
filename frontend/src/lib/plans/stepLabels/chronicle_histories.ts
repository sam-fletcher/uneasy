// Chronicle Histories: set the scene, then make options one at a time, then wrap
// the scene. A mar is the one place it asks everyone at once ("everyone chooses",
// the server's own kind); once all have chosen, only the wrap is left.

import { parseChronicleHistoriesData } from '$lib/plans/resolutionData/chronicle_histories';
import type { StepLabelFn } from '../stepLabel';

export const chronicleHistoriesStepLabel: StepLabelFn = ({ plan, roll }) => {
	const ch = parseChronicleHistoriesData(plan);
	// The invocations and the scene go in one submit, which also casts the dice.
	if (!ch.invoke_phase_closed) return 'set the scene';

	const marred = roll?.outcome === 'mar' || ch.mar_active || (ch.mar_required_choices ?? 0) > 0;
	if (marred) return 'wrap the scene';

	// The budget is the dice result, written on the first step; before it is, the
	// roll's own result is the same number. `||`, as the panel reads it: a stored 0 is
	// "not written yet", not a budget of nothing, or the heading would stay on
	// "choose a make option" for ever.
	const budget = ch.make_budget || roll?.result || 0;
	const done = ch.make_choices_done ?? 0;
	if (done > 0 && budget > 0 && done >= budget) return 'wrap the scene';
	return 'choose a make option';
};
