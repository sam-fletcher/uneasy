import type { Asset, DraftPeer } from '$lib/api';
import { assetName } from '../shared';

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
	pendingHostMars: string[];
	acceptDuels: number[];
	pendingDuelPlanID: number | null;
	pendingChallenge: { challenger_id: number; target_id: number; notes?: string } | null;
	/** Real peers shoved to the centre by a disagreement — still owned. */
	centeredAssetIDs: number[];
	/** Peers introduced at the party, who own no asset row until claimed (D7). */
	centeredDrafts: DraftPeer[];
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
 *  makes are taken, every outstanding mar (a guest IOU) has been inflicted, and
 *  every insisted mar the host must resolve themselves has been settled. Peers
 *  still in the centre don't hold the event open — ending it is what settles
 *  them. */
export function festivityEndable(fest: FestRes, hostID: number): boolean {
	const allChosen = fest.guests.every((id) => String(id) in fest.outcomes);
	const makesLeft = earnedHostMakes(fest, hostID) - fest.hostMakesTaken.length;
	return allChosen && makesLeft <= 0 && fest.guestIOUs.length === 0
		&& fest.pendingHostMars.length === 0;
}

/** Display-only asset id for a centered draft, so drafts and real peers can
 *  share one picker list. Derived from the draft's own (random) id rather than
 *  its position, so a pick can't silently re-point at a neighbour when another
 *  guest claims a draft mid-selection — the same reason DraftPeer carries a
 *  string id at all. Always negative, so it can never collide with a real asset
 *  id, and stable across re-renders. */
export function draftPickID(draftID: string): number {
	let h = 0;
	for (let i = 0; i < draftID.length; i++) h = (Math.imul(h, 31) + draftID.charCodeAt(i)) | 0;
	return -(Math.abs(h) + 1);
}

/** One centered draft rendered as an asset card. It has no owner (that's the
 *  point — D7), so owner_id is 0 and the card falls back to the unknown-player
 *  colour; the caller's ownerLabel says so in words. */
function draftAsAsset(draft: DraftPeer, gameID: number): Asset {
	const id = draftPickID(draft.id);
	return {
		id,
		game_id: gameID,
		owner_id: 0,
		creator_id: draft.creator_id,
		asset_type: 'peer',
		name: draft.name,
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: false,
		created_at: '',
		destroyed_at: null,
		linked_card_suit: null,
		linked_card_value: null,
		marginalia: draft.marginalia
			? [{
				id: 0, asset_id: id, position: 1, text: draft.marginalia,
				is_torn: false, torn_at: null, torn_by_id: null, title: null,
			}]
			: [],
		secret_count: 0,
	};
}

/** Everything at the centre of the party that a guest may take, as one list —
 *  players shouldn't have to know which peers are drafts and which are real.
 *  Disagreement peers come first (they were there before anyone was
 *  introduced). */
export function centeredPeerCards(fest: FestRes, assets: Asset[], gameID: number): Asset[] {
	return [
		...assets.filter(a => fest.centeredAssetIDs.includes(a.id) && !a.is_destroyed),
		...fest.centeredDrafts.map(d => draftAsAsset(d, gameID)),
	];
}

/** True for a card minted by centeredPeerCards from a draft rather than a real
 *  asset — it has no owner to name and no history to read. */
export function isDraftCard(a: Asset): boolean {
	return a.id < 0;
}

/** Turns a pick from centeredPeerCards into the take_center_peer request
 *  fields: a real peer travels as asset_id, a draft as draft_id. Returns null
 *  when the pick no longer matches anything in the centre. */
export function centeredPeerPick(
	fest: FestRes, pickedID: number,
): { asset_id?: number; draft_id?: string } | null {
	if (pickedID >= 0) {
		return fest.centeredAssetIDs.includes(pickedID) ? { asset_id: pickedID } : null;
	}
	const draft = fest.centeredDrafts.find(d => draftPickID(d.id) === pickedID);
	return draft ? { draft_id: draft.id } : null;
}

/** Names of every peer waiting in the centre, for the read-only roster. */
export function centeredPeerNames(fest: FestRes, assets: Asset[]): string[] {
	return [
		...fest.centeredAssetIDs.map(id => assetName(assets, id)),
		...fest.centeredDrafts.map(d => d.name),
	];
}
