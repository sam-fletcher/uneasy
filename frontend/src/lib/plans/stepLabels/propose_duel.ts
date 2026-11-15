// Propose Duel: the stakes (`await_duel_staking`), the bouts (`await_duel_bout`),
// then the final roll. Those two phases have their own kinds, so what rides the
// generic one is the winner's claim after the roll. A make is the preparer's to
// take; a mar's opponent MAY take, so they are asked to take or spare.

import { parseDuelData } from '$lib/plans/resolutionData/propose_duel';
import type { StepLabelFn } from '../stepLabel';

export const proposeDuelStepLabel: StepLabelFn = ({ plan, roll }) => {
	if (parseDuelData(plan).phase !== 'roll' || roll?.outcome == null) return null;
	return roll.outcome === 'mar' ? 'take the stakes or spare them' : 'claim the stakes';
};
