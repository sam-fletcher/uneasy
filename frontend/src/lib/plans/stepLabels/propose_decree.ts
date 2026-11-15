// Propose Decree: a council forms round the preparer's text, the signatory calls the
// roll, then the law is written (a mar's council amends it in turn), the signatory
// places an addendum and the preparer enacts it. The server names who is owed at
// each of these (ResolvingWaitees); this says what.
//
// The council has two asks open at once, the preparer's ("open the debate") and each
// other player's ("join or decline"), so before the debate it is named for what they
// make together. After it the acting set tells the two apart: the signatory is only
// named once every decision is in.

import { parseProposeDecreeData } from '$lib/plans/resolutionData/propose_decree';
import type { StepLabelFn } from '../stepLabel';

export const proposeDecreeStepLabel: StepLabelFn = ({ plan, roll, acting }) => {
	const pd = parseProposeDecreeData(plan);

	if (!pd.outcome) {
		// A roll with no outcome applied yet is the moment between the dice landing
		// and the server recording the result; there is nothing for a player to do.
		if (roll != null) return null;
		if (!pd.debate_started) return 'form the council';
		return pd.signatory_id != null && acting.includes(pd.signatory_id)
			? 'end the debate'
			: 'join or decline the council';
	}

	const amended = pd.amended_by ?? [];
	if ((pd.amendment_order ?? []).some(id => !amended.includes(id))) return 'amend the law';
	if (!pd.addendum_placed) return 'place the addendum';
	return pd.law_id == null ? 'enact the law' : null;
};
