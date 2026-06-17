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

// The "always" effects that apply on top of any chosen option, surfaced in the
// buffet so players understand the full consequence before they roll.
export const MAKE_ALWAYS = 'You can insist the host choose one Mar at any point during the festivity.';
export const MAR_ALWAYS = 'The host can choose a free Make at any point during the festivity.';
export const OPT_OUT_EFFECT = 'The host can choose a free Make at any point during the festivity.';

export type FestRes = {
	phase: string;
	guests: number[];
	outcomes: Record<string, string>;
	guestMakes: Record<string, string>;
	guestMars: Record<string, string>;
	hostChoices: Record<string, string>;
	guestRollIDs: Record<string, number>;
	guestIOUs: number[];
	hostMarInsists: string[];
	acceptDuels: number[];
	pendingDuelPlanID: number | null;
	pendingChallenge: { challenger_id: number; target_id: number; notes?: string } | null;
	centeredAssetIDs: number[];
};
