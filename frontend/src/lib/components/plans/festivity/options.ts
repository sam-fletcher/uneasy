// Shared make/mar option lists for the festivity sub-components. The
// guest's own turn shows MAKE_OPTS or MAR_OPTS depending on roll outcome;
// the host's picker on behalf of marred/opted-out guests uses
// HOST_MAKE_OPTS (everything except challenge_duel — the host can't pick
// a duel target for a guest).

export const MAKE_OPTS = [
	{ key: 'spread_rumor',     label: 'Spread a new rumor (notecard added to public record)' },
	{ key: 'introduce_peer',   label: 'Introduce a new peer' },
	{ key: 'take_center_peer', label: 'Take a peer from the center of the table' },
	{ key: 'challenge_duel',   label: 'Challenge somebody to a duel' },
];

export const MAR_OPTS = [
	{ key: 'rumor_about_you', label: 'A rumor spreads about you' },
	{ key: 'disagreement',    label: 'Get into a disagreement with one of your peers (sets them in the center)' },
	{ key: 'accept_duels',    label: 'You must accept any duel challenges during the event' },
	{ key: 'break_self',      label: 'Break yourself (tear a marginalia on your main character)' },
];

export const HOST_MAKE_OPTS = MAKE_OPTS.filter(o => o.key !== 'challenge_duel');

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
