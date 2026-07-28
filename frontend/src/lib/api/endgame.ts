// api/endgame.ts — the row 7 → 8 ending-mode vote
// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md).
//
// Two routes, no third. There is deliberately no close/force/skip: every seated
// player must vote, the tie-break is an automatic rule inside the server's
// tally, and a non-responder stalls the game — a social problem, not a software
// one (adr/FACILITATOR_POWERS_AUDIT.md).

import { apiFetch } from './client';

export type EndgameMode = 'smooth_landing' | 'explosive_finale';

/** One cast vote. Votes are public — who voted and for what, both — because
 *  this is a table conversation, not a secret ballot. */
export interface EndingVoteEntry {
	player_id: number;
	/** Widened to string: `long_campaign` is rejected by the API today but the
	 *  column may carry it later, and an unknown mode must not break the render. */
	mode: string;
}

/** The shape both ending-vote routes return. `votes` and `pending_player_ids`
 *  are always arrays, never null; `mode` is null until the ballot resolves. */
export interface EndingVoteState {
	/** The single window authority — votes are only accepted while true. */
	open: boolean;
	votes: EndingVoteEntry[];
	mode: string | null;
	/** Seated players who still owe a vote — the same set
	 *  RowStateAwaitEndgameVote carries in `acting_player_ids`. */
	pending_player_ids: number[];
}

export function getEndingVote(gameID: string | number): Promise<EndingVoteState> {
	return apiFetch(`/tables/${gameID}/ending-vote`);
}

/** Upsert this player's vote. A player may change their vote freely while the
 *  window is open; 409 once it has closed. */
export function castEndingVote(
	gameID: string | number,
	mode: EndgameMode
): Promise<EndingVoteState> {
	return apiFetch(`/tables/${gameID}/ending-vote`, {
		method: 'POST',
		body: JSON.stringify({ mode })
	});
}
