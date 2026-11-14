// Host Festivity has no phases: the event is open while the plan is resolving, and
// every step of it (a guest's turn, a challenge, the host's marks, ending it) is the
// server's `await_festivity_guest_turn` or `await_festivity_challenge_response`. The
// generic kind only reaches it when the server can't load the guest list, and then
// the event is still the thing the table is in.

import type { StepLabelFn } from '../stepLabel';

export const hostFestivityStepLabel: StepLabelFn = ({ plan }) =>
	plan.status === 'resolving' ? 'the festivity' : null;
