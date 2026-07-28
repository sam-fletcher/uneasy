import { describe, it, expect } from 'vitest';
import type { EndingVoteEntry } from '$lib/api';
import {
	ENDING_MODES,
	applyVoteCast,
	endingModeLabel,
	summariseEndingVote,
	type EndingVoteSummaryInput,
} from './endgameVote';

// ── Fixtures ───────────────────────────────────────────────────────────────

const vote = (player_id: number, mode: string): EndingVoteEntry => ({ player_id, mode });

const roster = [
	{ id: 1, display_name: 'Alice' },
	{ id: 2, display_name: 'Bob' },
	{ id: 3, display_name: 'Carol' },
];

function input(over: Partial<EndingVoteSummaryInput> = {}): EndingVoteSummaryInput {
	return { votes: [], players: roster, currentPlayerID: 1, ...over };
}

// ── applyVoteCast — the WS-duplicate trap ──────────────────────────────────
// The vote is an upsert server-side, so a changed vote arrives as a second
// endgame.vote_cast for the same player. Appending it would give the keyed
// {#each} two rows for one id.

describe('applyVoteCast', () => {
	it('appends a first vote', () => {
		expect(applyVoteCast([], vote(1, 'smooth_landing'))).toEqual([vote(1, 'smooth_landing')]);
	});

	it('REPLACES a changed vote rather than appending a duplicate', () => {
		const before = [vote(1, 'smooth_landing'), vote(2, 'explosive_finale')];
		const after = applyVoteCast(before, vote(1, 'explosive_finale'));
		expect(after).toHaveLength(2);
		expect(after.filter((v) => v.player_id === 1)).toEqual([vote(1, 'explosive_finale')]);
		// The other player's entry is untouched, and order is stable.
		expect(after[1]).toEqual(vote(2, 'explosive_finale'));
	});

	it('is idempotent on a re-cast of the same mode', () => {
		const before = [vote(1, 'smooth_landing')];
		expect(applyVoteCast(before, vote(1, 'smooth_landing'))).toEqual(before);
	});

	it('does not mutate the input array', () => {
		const before = [vote(1, 'smooth_landing')];
		applyVoteCast(before, vote(2, 'explosive_finale'));
		expect(before).toHaveLength(1);
	});
});

// ── summariseEndingVote ────────────────────────────────────────────────────

describe('summariseEndingVote', () => {
	it('an empty ballot leaves every seat pending and every count zero', () => {
		const got = summariseEndingVote(input());
		expect(got.votedCount).toBe(0);
		expect(got.seatedCount).toBe(3);
		expect(got.counts).toEqual({ smooth_landing: 0, explosive_finale: 0 });
		expect(got.myMode).toBeNull();
		expect(got.iOwe).toBe(true);
		// The viewer is excluded from the list — iOwe carries their half.
		expect(got.pendingNames).toEqual(['Bob', 'Carol']);
	});

	it('names my own vote and drops me from the pending list', () => {
		const got = summariseEndingVote(input({ votes: [vote(1, 'explosive_finale')] }));
		expect(got.myMode).toBe('explosive_finale');
		expect(got.iOwe).toBe(false);
		expect(got.counts.explosive_finale).toBe(1);
		expect(got.pendingNames).toEqual(['Bob', 'Carol']);
	});

	it('lists the stragglers in roster order', () => {
		const got = summariseEndingVote(input({ votes: [vote(2, 'smooth_landing')] }));
		expect(got.pendingNames).toEqual(['Carol']);
		expect(got.votedCount).toBe(1);
	});

	it('tallies a complete ballot', () => {
		const got = summariseEndingVote(
			input({
				votes: [
					vote(1, 'smooth_landing'),
					vote(2, 'explosive_finale'),
					vote(3, 'explosive_finale'),
				],
			}),
		);
		expect(got.votedCount).toBe(3);
		expect(got.counts).toEqual({ smooth_landing: 1, explosive_finale: 2 });
		expect(got.pendingNames).toEqual([]);
		expect(got.iOwe).toBe(false);
	});

	it('ignores a vote from a player who is not seated', () => {
		// Defensive: the tally must never exceed the number of seats.
		const got = summariseEndingVote(input({ votes: [vote(99, 'smooth_landing')] }));
		expect(got.votedCount).toBe(0);
		expect(got.counts.smooth_landing).toBe(0);
		expect(got.pendingNames).toEqual(['Bob', 'Carol']);
	});

	it('counts an unknown mode without losing it or breaking the known ones', () => {
		// long_campaign is rejected by the API today, but the render must not
		// depend on that staying true.
		const got = summariseEndingVote(input({ votes: [vote(2, 'long_campaign')] }));
		expect(got.counts).toEqual({ smooth_landing: 0, explosive_finale: 0, long_campaign: 1 });
		expect(got.votedCount).toBe(1);
	});

	it('a viewer with no seat owes nothing', () => {
		const got = summariseEndingVote(input({ currentPlayerID: null }));
		expect(got.iOwe).toBe(false);
		expect(got.myMode).toBeNull();
		// With nobody to exclude, every seat shows in the pending list.
		expect(got.pendingNames).toEqual(['Alice', 'Bob', 'Carol']);
	});
});

// ── Labels ─────────────────────────────────────────────────────────────────

describe('endingModeLabel', () => {
	it('names every live mode', () => {
		expect(ENDING_MODES.map(endingModeLabel)).toEqual(['Smooth Landing', 'Explosive Finale']);
	});

	it('falls back to the raw value for an unknown mode', () => {
		expect(endingModeLabel('long_campaign')).toBe('Long Campaign');
		expect(endingModeLabel('something_else')).toBe('something_else');
	});
});
