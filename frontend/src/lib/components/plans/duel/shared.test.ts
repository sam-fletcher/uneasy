import { describe, it, expect } from 'vitest';
import type { Asset, DuelBout, DuelStake } from '$lib/api';
import { computeAccumulated, stakeLabel } from './shared';

const PREP = 1;
const TARG = 2;

function bout(overrides: Partial<DuelBout> = {}): DuelBout {
	return {
		id: 1,
		plan_id: 1,
		bout_number: 1,
		declarer_id: PREP,
		declarer_stake_id: 10,
		responder_id: TARG,
		responder_stake_id: 20,
		declaration: 'high',
		declarer_die: null,
		responder_die: null,
		winner_id: null,
		is_match: false,
		created_at: '2026-01-01T00:00:00Z',
		resolved_at: null,
		...overrides,
	};
}

function stake(overrides: Partial<DuelStake> = {}): DuelStake {
	return {
		id: 10,
		plan_id: 1,
		player_id: PREP,
		asset_id: 100,
		is_resolved: false,
		is_winner: null,
		hidden_die: null,
		...overrides,
	};
}

function asset(id: number, name: string): Asset {
	return {
		id,
		game_id: 1,
		owner_id: 1,
		creator_id: 1,
		asset_type: 'peer',
		name,
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: false,
		created_at: '2026-01-01T00:00:00Z',
		destroyed_at: null,
		marginalia: [],
		secret_count: 0,
	};
}

describe('computeAccumulated', () => {
	it('returns empty pools when preparerID is null', () => {
		expect(computeAccumulated([bout()], null)).toEqual({ prep: [], targ: [], pending: [] });
	});

	it('skips bouts with unresolved dice', () => {
		expect(computeAccumulated([bout({ declarer_die: 5 })], PREP)).toEqual({
			prep: [],
			targ: [],
			pending: [],
		});
	});

	it('awards both dice to the winning side on a non-tie bout', () => {
		const b = bout({ declarer_die: 6, responder_die: 2, winner_id: PREP });
		expect(computeAccumulated([b], PREP)).toEqual({ prep: [6, 2], targ: [], pending: [] });
	});

	it('routes dice to the target when the target wins', () => {
		const b = bout({ declarer_die: 1, responder_die: 5, winner_id: TARG });
		expect(computeAccumulated([b], PREP)).toEqual({ prep: [], targ: [1, 5], pending: [] });
	});

	// The carryover rule is subtle and shared with the backend
	// (pduelCreateFinalRoll): tied dice are NOT discarded — they pile into
	// `pending` and are awarded to the winner of the next non-tie bout.
	it('carries tied dice into the next non-tie bout winner', () => {
		const tie = bout({ id: 1, bout_number: 1, declarer_die: 3, responder_die: 3, is_match: true });
		const win = bout({ id: 2, bout_number: 2, declarer_die: 6, responder_die: 1, winner_id: PREP });
		expect(computeAccumulated([tie, win], PREP)).toEqual({
			prep: [6, 1, 3, 3],
			targ: [],
			pending: [],
		});
	});

	it('leaves trailing tied dice in pending when no later non-tie bout exists', () => {
		const win = bout({ id: 1, bout_number: 1, declarer_die: 5, responder_die: 2, winner_id: TARG });
		const tie = bout({ id: 2, bout_number: 2, declarer_die: 4, responder_die: 4, is_match: true });
		expect(computeAccumulated([win, tie], PREP)).toEqual({
			prep: [],
			targ: [5, 2],
			pending: [4, 4],
		});
	});

	it('does not clear pending on a non-tie bout that has no winner_id', () => {
		const tie = bout({ id: 1, bout_number: 1, declarer_die: 2, responder_die: 2, is_match: true });
		const noWinner = bout({ id: 2, bout_number: 2, declarer_die: 6, responder_die: 1, winner_id: null });
		const win = bout({ id: 3, bout_number: 3, declarer_die: 5, responder_die: 1, winner_id: PREP });
		expect(computeAccumulated([tie, noWinner, win], PREP)).toEqual({
			prep: [5, 1, 2, 2],
			targ: [],
			pending: [],
		});
	});
});

describe('stakeLabel', () => {
	const assets = [asset(100, 'Crown')];

	it('shows "hidden" for an unresolved stake the viewer cannot see', () => {
		expect(stakeLabel(stake({ asset_id: 100, hidden_die: null }), assets, [])).toBe('Crown — hidden');
	});

	it('shows the hidden die value for the stake owner', () => {
		expect(stakeLabel(stake({ asset_id: 100, hidden_die: 4 }), assets, [])).toBe('Crown — hidden d4');
	});

	it('shows the rolled die when resolved as declarer', () => {
		const s = stake({ id: 10, asset_id: 100, is_resolved: true });
		const b = bout({ declarer_stake_id: 10, declarer_die: 6, is_match: false });
		expect(stakeLabel(s, assets, [b])).toBe('Crown — 6');
	});

	it('marks a matched (set-aside) stake', () => {
		const s = stake({ id: 20, asset_id: 100, is_resolved: true });
		const b = bout({ responder_stake_id: 20, responder_die: 3, is_match: true });
		expect(stakeLabel(s, assets, [b])).toBe('Crown — 3 (set aside)');
	});

	it('falls back to "resolved" when no bout references the stake', () => {
		const s = stake({ id: 99, asset_id: 100, is_resolved: true });
		expect(stakeLabel(s, assets, [])).toBe('Crown — resolved');
	});
});
