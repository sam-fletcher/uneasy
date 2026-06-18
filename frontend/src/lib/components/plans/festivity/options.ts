// Shared make/mar option lists for the festivity sub-components. The
// guest's own turn shows MAKE_OPTS or MAR_OPTS depending on roll outcome;
// the host's picker (the free make they take for each guest who marred or
// opted out) uses HOST_MAKE_OPTS (everything except challenge_duel — a duel
// is a live challenge, not a take-for-yourself make).
//
// `desc` is the one-line effect shown in the read-only buffet reference.

export const MAKE_OPTS = [
	{ key: 'spread_rumor',     label: 'Spread a new rumor',            	desc: '' },
	{ key: 'introduce_peer',   label: 'Introduce a new peer',           desc: '— They\'ll join your retinue if not taken during the festivity.' },
	{ key: 'take_center_peer', label: 'Take an available peer',   		desc: '— A free peer at the festivity joins your retinue.' },
	{ key: 'challenge_duel',   label: 'Propose a duel',    				desc: '— Challenge a player; if they accept, duel right away.' },
];

export const MAR_OPTS = [
	{ key: 'rumor_about_you', label: 'A rumor spreads about you',      	desc: '— Create a rumor targeting your main character.' },
	{ key: 'disagreement',    label: 'A peer considers leaving', 		desc: '— They\'re available for anyone to take. If not taken, they\'ll rejoin you, broken.' },
	{ key: 'accept_duels',    label: 'You must accept all duels',  		desc: 'during the festivity.' },
	{ key: 'break_self',      label: 'Break yourself',                  desc: '— Tear a marginalia on your main character.' },
];

export const HOST_MAKE_OPTS = MAKE_OPTS.filter(o => o.key !== 'challenge_duel');

// Third-person past-tense phrases for the scorecard ("The Talk of the Event"),
// where the MAKE_OPTS/MAR_OPTS labels (second person) read awkwardly.
export const MAKE_PHRASE: Record<string, string> = {
	spread_rumor: 'spread a rumor',
	introduce_peer: 'introduced a peer',
	take_center_peer: 'took a peer from the table',
	challenge_duel: 'called for a duel',
};
export const MAR_PHRASE: Record<string, string> = {
	rumor_about_you: 'a rumor spread about them',
	disagreement: 'fell out with a peer',
	accept_duels: 'agreed to answer any duel',
	break_self: 'embarrassed themselves',
};

// The "always" effects that apply on top of any chosen option, surfaced in the
// buffet so players understand the full consequence before they roll.
export const MAKE_ALWAYS = 'You can insist the host choose one Mar at any point during the festivity.';
export const MAR_ALWAYS = 'The host can choose a free Make at any point during the festivity.';
export const OPT_OUT_EFFECT = 'The host can choose a free Make at any point during the festivity.';

export type FestRes = {
	guests: number[];
	outcomes: Record<string, string>;
	guestMakes: Record<string, string>;
	guestMars: Record<string, string>;
	hostMakesTaken: string[];
	guestRollIDs: Record<string, number>;
	guestIOUs: number[];
	hostMarInsists: string[];
	acceptDuels: number[];
	pendingDuelPlanID: number | null;
	pendingChallenge: { challenger_id: number; target_id: number; notes?: string } | null;
	centeredAssetIDs: number[];
};

/** Extra makes the host has earned: one for hosting, one per guest who marred
 *  or opted out. They're the host's spoils, counted — not tied to a guest. */
export function earnedHostMakes(fest: FestRes, hostID: number): number {
	let n = 0;
	for (const id of fest.guests) {
		const oc = fest.outcomes[String(id)];
		if (oc === 'mar' || oc === 'opt_out' || (id === hostID && oc === 'host')) n++;
	}
	return n;
}

/** Whether the host may wind the event down: every guest has chosen, all earned
 *  makes are taken, and every outstanding mar (a guest IOU) has been inflicted. */
export function festivityEndable(fest: FestRes, hostID: number): boolean {
	const allChosen = fest.guests.every((id) => String(id) in fest.outcomes);
	const makesLeft = earnedHostMakes(fest, hostID) - fest.hostMakesTaken.length;
	return allChosen && makesLeft <= 0 && fest.guestIOUs.length === 0;
}
