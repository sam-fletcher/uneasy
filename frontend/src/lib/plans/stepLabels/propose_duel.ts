// Propose Duel: the stakes (`await_duel_staking`), the bouts (`await_duel_bout`),
// then the final roll. Those two phases have their own kinds, so what rides the
// generic one is the winner's claim after the roll. A make is the preparer's to
// take; a mar's opponent MAY take, so they are asked to take or spare.

import { parseDuelData } from '$lib/plans/resolutionData/propose_duel';
import type { Plan } from '$lib/api';
import type { StepLabelFn } from '../stepLabel';

/**
 * The bout step's label, once the heading no longer carries "alice to declare" /
 * "to respond": the verb says which half is pending. The server names one actor for the
 * bout (the responder if a declared bout is open, else the declarer, who holds the
 * initiative), so an actor who is the initiative player is declaring and the other
 * duellist is responding. A plan that can't be found, or an actor who is neither duellist,
 * stays "the bout". (A Make Demands winner never acts here: the bouts are the duellists'.)
 */
export function duelBoutLabel(plan: Plan | null | undefined, acting: readonly number[]): string {
	if (!plan || plan.plan_type !== 'propose_duel' || acting.length !== 1) return 'the bout';
	const initiative = parseDuelData(plan).initiative_player_id;
	const duellists = [plan.preparer_id, plan.target_player_id];
	if (initiative == null || !duellists.includes(initiative) || !duellists.includes(acting[0])) return 'the bout';
	return acting[0] === initiative ? 'declare the bout' : 'respond to the bout';
}

export const proposeDuelStepLabel: StepLabelFn = ({ plan, roll }) => {
	if (parseDuelData(plan).phase !== 'roll' || roll?.outcome == null) return null;
	return roll.outcome === 'mar' ? 'take the stakes or spare them' : 'claim the stakes';
};
