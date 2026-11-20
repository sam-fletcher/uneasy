import { describe, expect, it } from 'vitest';
import { liaiseSecretHeader } from './liaiseSecret';

// The line the panel promises is added in front of a liaison secret. The server writes
// the real one (clSecretHeader), so these pin the shape the hint shows.
describe('liaiseSecretHeader', () => {
	it('names the other player and the peer they brought', () => {
		expect(liaiseSecretHeader('bob', 'Ser Ott')).toBe("Met bob's Ser Ott");
	});

	it('falls back to the other player alone when the peer is not found', () => {
		expect(liaiseSecretHeader('bob', null)).toBe('Met bob');
		expect(liaiseSecretHeader('bob', undefined)).toBe('Met bob');
		expect(liaiseSecretHeader('bob', '')).toBe('Met bob');
	});
});
