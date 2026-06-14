import { describe, it, expect } from 'vitest';
import type { Asset, Secret } from './api';
import { knownCount, hiddenCount } from './secretCounts';

function secret(assetId: number, id = 1): Secret {
	return {
		id,
		asset_id: assetId,
		author_id: 1,
		text: 's',
		is_revealed: false,
		revealed_at: null,
		created_at: '2026-01-01T00:00:00Z',
	};
}

function asset(secret_count: number): Asset {
	return {
		id: 1,
		game_id: 1,
		owner_id: 1,
		creator_id: 1,
		asset_type: 'peer',
		name: 'A',
		is_main_character: false,
		is_leveraged: false,
		is_destroyed: false,
		created_at: '2026-01-01T00:00:00Z',
		destroyed_at: null,
		marginalia: [],
		secret_count,
	};
}

describe('knownCount', () => {
	it('counts only secrets on the given asset', () => {
		const secrets = [secret(4, 1), secret(4, 2), secret(9, 3)];
		expect(knownCount(secrets, 4)).toBe(2);
		expect(knownCount(secrets, 9)).toBe(1);
		expect(knownCount(secrets, 7)).toBe(0);
		expect(knownCount([], 4)).toBe(0);
	});
});

describe('hiddenCount', () => {
	it('is the public total minus the known count', () => {
		expect(hiddenCount(asset(3), 1)).toBe(2);
		expect(hiddenCount(asset(2), 2)).toBe(0);
	});
	it('never goes negative when known exceeds a stale total', () => {
		expect(hiddenCount(asset(0), 2)).toBe(0);
	});
});
