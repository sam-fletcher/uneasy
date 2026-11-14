// Spread Propaganda: state the message, roll, then a mar picks options and a make
// authors the artifact. The mar's effects (give a peer up, break an asset) and the
// artifact are open together, so one is named and several are "the fallout". A
// counter-propaganda's own pre-roll is the server's `await_counter_propaganda`.

import { choicesCommitted, parseResolutionData } from '$lib/components/plans/shared';
import { parseSpreadPropagandaData } from '$lib/plans/resolutionData/spread_propaganda';
import { countOf, oneOf, type StepLabelFn } from '../stepLabel';

export const spreadPropagandaStepLabel: StepLabelFn = ({ plan, roll }) => {
	const rd = parseResolutionData(plan);
	const sp = parseSpreadPropagandaData(plan);
	const picks = (rd.make_mar_choices ?? []).map(c => c.option);
	const committed = choicesCommitted(rd);

	if (!(sp.pre_roll_done ?? false) && roll == null && !committed) return 'state the message';
	// A make has the one option and the server applies it as the dice land, so
	// anything left to pick is a mar's.
	if (!committed) return 'choose the mar options';

	const owed: string[] = [];
	if (sp.artifact_required && sp.artifact_id == null) owed.push('create the artifact');
	if (countOf(picks, 'give_peer') > (sp.give_peer_done ?? 0)) owed.push('give up a peer');
	if (countOf(picks, 'break_self') > (sp.break_self_done ?? 0)) owed.push('break an asset');
	return oneOf(owed, 'face the fallout', 'complete the plan');
};
