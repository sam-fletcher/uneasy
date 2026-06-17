// Shared make/mar option lists for the festivity sub-components. The
// guest's own turn shows MAKE_OPTS or MAR_OPTS depending on roll outcome;
// the host's picker (the free make they take for each guest who marred or
// opted out) uses HOST_MAKE_OPTS (everything except challenge_duel — a duel
// is a live challenge, not a take-for-yourself make).
//
// `desc` is the one-line effect shown in the read-only buffet reference.

export const MAKE_OPTS = [
	{ key: 'spread_rumor',     label: 'Spread a new rumour',            desc: 'A notecard goes under the public record, sourced to you.' },
	{ key: 'introduce_peer',   label: 'Introduce a new peer',           desc: 'A new peer joins your retinue (placed in the centre for now).' },
	{ key: 'take_center_peer', label: 'Take a peer from the centre',    desc: 'A peer in the centre of the table joins your retinue.' },
	{ key: 'challenge_duel',   label: 'Challenge someone to a duel',    desc: 'Call out a guest; if they accept, resolve a duel right away.' },
];

export const MAR_OPTS = [
	{ key: 'rumor_about_you', label: 'A rumour spreads about you',      desc: 'A notecard goes under the public record, aimed at your character.' },
	{ key: 'disagreement',    label: 'Fall out with one of your peers', desc: 'One of your own peers is set in the centre of the table.' },
	{ key: 'accept_duels',    label: 'Must accept any duel challenge',  desc: 'For the rest of the event you cannot decline a duel.' },
	{ key: 'break_self',      label: 'Break yourself',                  desc: 'Tear a marginalia on your main character.' },
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
