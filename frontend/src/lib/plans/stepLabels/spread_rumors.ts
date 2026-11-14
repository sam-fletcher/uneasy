// Spread Rumors: state the rumor, roll, then either options (a make) or the
// counter-rumor first (a mar, written by the target asset's owner), then the picks'
// own steps. A take's consent is the victim's (the server's `await_take_consent`).

import { choicesCommitted, parseResolutionData } from '$lib/components/plans/shared';
import { parseSpreadRumorsData } from '$lib/plans/resolutionData/spread_rumors';
import { countOf, oneOf, type StepLabelFn } from '../stepLabel';

export const spreadRumorsStepLabel: StepLabelFn = ({ plan, roll }) => {
	const rd = parseResolutionData(plan);
	const sr = parseSpreadRumorsData(plan);
	const picks = (rd.make_mar_choices ?? []).map(c => c.option);
	const committed = choicesCommitted(rd);

	if (!(sr.pre_roll_done ?? false) && roll == null && !committed) return 'state the rumor';
	if (!committed) {
		// A mar opens with the counter-rumor, and the options wait for it.
		if (roll?.outcome === 'mar' && !sr.counter_rumor_text) return 'write the counter-rumor';
		return 'choose the options';
	}

	const owed: string[] = [];
	if (countOf(picks, 'break_target') > (sr.break_target_done ?? 0)) owed.push('break the target asset');
	if (countOf(picks, 'hide_source') > (sr.hide_source_done ?? 0)) owed.push('hide the source');
	return oneOf(owed, 'carry out the picks', 'complete the plan');
};
