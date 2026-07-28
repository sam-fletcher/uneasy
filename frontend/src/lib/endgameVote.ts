// endgameVote.ts — pure helpers for the row 7 → 8 ending-mode vote
// (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7).
//
// Extracted from EndgameVotePanel.svelte so the ballot arithmetic is unit-
// testable without a component: the panel assembles its whole render from
// `summariseEndingVote`, and merges live votes with `applyVoteCast`.
//
// Free of Svelte runes; every input is passed explicitly and comes from the
// server (the GET/POST response, the endgame.vote_cast broadcast, and the
// roster).

import type { EndgameMode, EndingVoteEntry, Player } from '$lib/api';

/** The two live modes, in the order the panel lists them — which is also the
 *  server's clause-3 preference order (asks least of the table first). */
export const ENDING_MODES: EndgameMode[] = ['smooth_landing', 'explosive_finale'];

/** Player-facing names. Mirrors handler/ending_vote.go's endingModeLabel;
 *  an unknown mode falls back to its raw value rather than vanishing. */
export function endingModeLabel(mode: string): string {
	switch (mode) {
		case 'smooth_landing':
			return 'Smooth Landing';
		case 'explosive_finale':
			return 'Explosive Finale';
		case 'long_campaign':
			return 'Long Campaign';
	}
	return mode;
}

/**
 * Merge one `endgame.vote_cast` payload into the ballot.
 *
 * The vote is an **upsert** server-side, so a changed vote must REPLACE that
 * player's entry rather than append a second one — the optimistic-append/WS
 * duplicate trap (a keyed `{#each}` with two rows for one id freezes
 * reactivity). Keyed by player_id, which is exactly the ballot's primary key.
 *
 * Returns a new array; the input is never mutated.
 */
export function applyVoteCast(
	votes: EndingVoteEntry[],
	cast: EndingVoteEntry
): EndingVoteEntry[] {
	const idx = votes.findIndex((v) => v.player_id === cast.player_id);
	if (idx < 0) return [...votes, cast];
	return votes.map((v, i) => (i === idx ? cast : v));
}

/** Everything the panel needs to render, derived in one pass. */
export interface EndingVoteSummary {
	/** This viewer's own vote, or null while they still owe one. */
	myMode: string | null;
	/** True while the viewer owes a vote — the table is waiting on them. */
	iOwe: boolean;
	/** Votes per mode, including modes nobody picked (so the tally is stable). */
	counts: Record<string, number>;
	/** Votes cast by seated players. */
	votedCount: number;
	/** Seats at the table — every one of them must vote. */
	seatedCount: number;
	/** Display names of the OTHER players who still owe a vote, in roster
	 *  order. The viewer is excluded: `iOwe` carries their half, and the panel
	 *  addresses them directly rather than listing them among the stragglers. */
	pendingNames: string[];
}

export interface EndingVoteSummaryInput {
	/** Every vote on record, from the route or merged live broadcasts. */
	votes: EndingVoteEntry[];
	/** The seated roster — the denominator, and the source of names. */
	players: Pick<Player, 'id' | 'display_name'>[];
	currentPlayerID: number | null;
}

/**
 * Reduce the ballot to what the panel renders.
 *
 * The pending set is derived from the ballot rather than read from
 * `row_state.acting_player_ids`, even though the server computes the identical
 * set: the tally and the "still waiting on" line then come from one source and
 * cannot disagree with each other mid-update. The Waiting On bar reads the
 * server's set independently, and the two converge on the same broadcast.
 *
 * A vote from a player no longer on the roster is ignored, so the tally can
 * never exceed the number of seats.
 */
export function summariseEndingVote(input: EndingVoteSummaryInput): EndingVoteSummary {
	const { votes, players, currentPlayerID } = input;

	const byPlayer = new Map<number, string>();
	for (const v of votes) byPlayer.set(v.player_id, v.mode);

	const counts: Record<string, number> = {};
	for (const mode of ENDING_MODES) counts[mode] = 0;

	let votedCount = 0;
	const pendingNames: string[] = [];
	for (const p of players) {
		const mode = byPlayer.get(p.id);
		if (mode == null) {
			if (p.id !== currentPlayerID) pendingNames.push(p.display_name);
			continue;
		}
		votedCount++;
		counts[mode] = (counts[mode] ?? 0) + 1;
	}

	const myMode = currentPlayerID == null ? null : byPlayer.get(currentPlayerID) ?? null;

	return {
		myMode,
		// A spectator with no seat (currentPlayerID null) owes nothing.
		iOwe: currentPlayerID != null && myMode == null,
		counts,
		votedCount,
		seatedCount: players.length,
		pendingNames,
	};
}
