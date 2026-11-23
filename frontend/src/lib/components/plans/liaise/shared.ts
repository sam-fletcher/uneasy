// Shared state + derivations for the Clandestinely Liaise step components.
//
// A liaison has two SEATS — the preparer's and the partner's — and each submission step
// fills one. The record keys on the seat's player (kept_secrets, share_submitter_ids,
// the reveal entries), not on whoever is typing, because a Make Demands perform_steps
// winner stands in for the preparer: they fill the preparer's seat and the preparer is
// locked out of it. A demander can also be the partner, and then holds both. Mirrors the
// server's clSeatsFor.
//
// `liaiseSeats` works out, from the plan and who is looking, which seats the viewer
// fills and which of them still owes the current step. Plain data, no runes: the panel
// calls it inside a template `{@const}`, so it is recomputed with the context as the
// `$derived`s it replaces were. A step that needs an EFFECT to follow a seat reads the
// seat through its own `$derived` (a number or a string), never the whole object, so the
// effect re-runs only when the seat itself changes.

import type { KeptSecret, Plan } from '$lib/api';
import { parseLiaiseData, type LiaisePhase } from '$lib/plans/resolutionData/liaise';
import { playerName } from '../shared';
import type { PlanContext } from '../types';

export type CLState = {
	phase: LiaisePhase | '';
	partnerID: number | null;
	preparerPeerID: number | null;
	partnerPeerID: number | null;
	delayRevealID: number | null;
	redelayRevealID: number | null;
	keptSecrets: KeptSecret[];
	shareSubmitterIDs: number[];
};

export function clStateOf(plan: Plan | null): CLState {
	const ld = parseLiaiseData(plan);
	return {
		phase: ld.phase ?? '',
		partnerID: ld.partner_id ?? null,
		preparerPeerID: ld.preparer_peer_id ?? null,
		partnerPeerID: ld.partner_peer_id ?? null,
		delayRevealID: ld.delay_reveal_id ?? null,
		redelayRevealID: ld.redelay_reveal_id ?? null,
		keptSecrets: ld.kept_secrets ?? [],
		shareSubmitterIDs: ld.share_submitter_ids ?? [],
	};
}

export function liaiseSeats(
	ctx: PlanContext,
	plan: Plan,
	cl: CLState,
	performStepsWinnerID: number | null,
) {
	const { players, currentPlayerID } = ctx;

	const amPreparer = currentPlayerID === plan.preparer_id;
	const amPartner = currentPlayerID != null && cl.partnerID === currentPlayerID;
	/** Who fills the preparer's seat: the winner if a demand handed it over. */
	const preparerSeatHolder = performStepsWinnerID ?? plan.preparer_id;
	/** The seats the viewer fills, partner's first. */
	const mySeats: number[] = [];
	if (amPartner && cl.partnerID != null) mySeats.push(cl.partnerID);
	if (currentPlayerID != null && currentPlayerID === preparerSeatHolder) {
		mySeats.push(plan.preparer_id);
	}
	const amParticipant = mySeats.length > 0;
	/** Moving the meeting on is a step: whoever holds the preparer's seat presses it. */
	const canAdvance = mySeats.includes(plan.preparer_id);
	/** In the meeting but with no seat to fill: the preparer, locked out by a demand. */
	const amLockedOut = (amPreparer || amPartner) && mySeats.length === 0;
	/** Seat-holders' names, for "waiting on …" copy — the preparer's seat shows its holder. */
	const holderName = playerName(players, preparerSeatHolder);
	const standingIn = currentPlayerID != null && currentPlayerID !== plan.preparer_id
		&& currentPlayerID === performStepsWinnerID;

	const participants: { player_id: number; display_name: string }[] = [];
	participants.push({
		player_id: plan.preparer_id,
		display_name: playerName(players, preparerSeatHolder ?? plan.preparer_id),
	});
	if (cl.partnerID != null) {
		participants.push({
			player_id: cl.partnerID,
			display_name: playerName(players, cl.partnerID),
		});
	}

	const keepSecretSubmittedIDs = new Set(cl.keptSecrets.map(ks => ks.player_id));
	/** The first of the viewer's seats that hasn't kept a secret yet. */
	const activeKeepSeat = mySeats.find(seat => !keepSecretSubmittedIDs.has(seat)) ?? null;
	/** The first of the viewer's seats that hasn't made a Things We Share pick yet. */
	const activeShareSeat = mySeats.find(seat => !cl.shareSubmitterIDs.includes(seat)) ?? null;
	/** True when the viewer is acting for the preparer's seat and not their own. */
	const actingForPreparerSeat = mySeats.length > 0
		&& (activeKeepSeat ?? activeShareSeat ?? mySeats[0]) === plan.preparer_id
		&& !amPreparer;

	return {
		amPreparer, amPartner, preparerSeatHolder, mySeats, amParticipant,
		canAdvance, amLockedOut, holderName, standingIn, participants,
		keepSecretSubmittedIDs, activeKeepSeat, activeShareSeat, actingForPreparerSeat,
		/** Has every seat the viewer fills submitted keep-secret? */
		iKeptSecret: mySeats.length > 0 && activeKeepSeat == null,
		/** Whether the viewer has submitted their Things We Share choice, for every seat they fill.
		 *  Server-authoritative (from resolution_data) so a refresh mid-phase doesn't re-prompt. */
		iShared: mySeats.length > 0 && activeShareSeat == null,
	};
}

export type Seats = ReturnType<typeof liaiseSeats>;
