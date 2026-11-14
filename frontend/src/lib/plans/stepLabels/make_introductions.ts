// Make Introductions: name the peers (the dice wait for it), roll, then a make owes
// an arrival form per peer and a mar owes an outcome per peer. The plan's last step
// ends it by itself, so "nothing owed" has no label.
//
// A newcomer's marginalia that someone ELSE owes is the server's
// `await_introductions_marginalia`, and the actor can still be owed their own fates at
// the same time (the server names both). So a mar has three states: only the actor
// owes ("settle each newcomer"), only authors owe ("describe a newcomer"), and both
// at once, which is named for what they share so that a heading never tells an
// author to settle fates, or the actor to write someone else's marginalia.

import { choicesCommitted, parseResolutionData } from '$lib/components/plans/shared';
import { miPendingArrivals, parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';
import type { StepLabelFn } from '../stepLabel';

export const makeIntroductionsStepLabel: StepLabelFn = ({ plan, roll }) => {
	const mi = parseMakeIntroductionsData(plan);
	const owesArrival = miPendingArrivals(mi).length > 0;

	// A synthetic plan for one delayed traveller: it goes straight to their arrival.
	if (mi.delayed_arrival) return owesArrival ? 'describe the delayed arrival' : null;
	if (mi.make_pending) return owesArrival ? 'describe each arrival' : null;
	if (mi.mar_pending) {
		const outcomes = mi.mar_outcomes ?? [];
		const actorOwes = outcomes.length < (mi.peer_count ?? 0);
		// Mirrors PendingArrivalAuthors: a fate that hands a peer's marginalia to another
		// player, not yet written.
		const authorsOwe = outcomes.some(o => !o.done && o.author_player_id != null);
		if (actorOwes && authorsOwe) return 'the newcomers';
		if (actorOwes) return 'settle each newcomer';
		return authorsOwe ? 'describe a newcomer' : null;
	}

	// Before the dice: the preparer names every peer, and finalizing casts the roll.
	const peerCount = mi.peer_count ?? 0;
	const committed = choicesCommitted(parseResolutionData(plan));
	if (roll == null && !committed && peerCount > 0 && (mi.drafts ?? []).length < peerCount) {
		return 'name the new peers';
	}
	return null;
};
