// Clandestinely Liaise: four phases, in the book's own names. The first rides the
// generic kind (moving the meeting on is the preparer's, or a Make Demands winner's);
// the other three are collaborative submits the server calls `liaise_resolving`, a
// kind that spans all three, so the plan names which. The delay vote before them is
// the server's `await_delay_reveal`.

import { parseLiaiseData } from '$lib/plans/resolutionData/liaise';
import type { StepLabelFn } from '../stepLabel';

export const clandestinelyLiaiseStepLabel: StepLabelFn = ({ plan }) => {
	switch (parseLiaiseData(plan).phase) {
		case 'together_at_last': return 'together at last';
		case 'secrets_we_keep': return 'secrets we keep';
		case 'things_we_share': return 'things we share';
		case 'when_will_i_see_you_again': return 'when will I see you again?';
		default: return null;
	}
};
