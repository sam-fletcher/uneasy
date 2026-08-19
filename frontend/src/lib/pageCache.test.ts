import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import {
	readTableSnapshot, writeTableSnapshot,
	readProfileSnapshot, writeProfileSnapshot, clearPageCache,
	type TableSnapshot,
} from './pageCache';
import type { MyTable, Account } from '$lib/api';

const acct = { id: 1, username: 'alice' } as Account;

// Minimal but *total* snapshot: the point of the type is that a snapshot is
// never partial, so the fixture fills every field.
function snap(overrides: Partial<TableSnapshot> = {}): TableSnapshot {
	return {
		game: null, players: [], toneTopics: [], rankings: [], assets: [],
		laws: [], rumors: [], members: [], playerActivity: [], secrets: [],
		currentPlayerID: null, prologueActivePlayerID: null,
		recordRows: [], plans: [], planTokens: [],
		activeScene: null, activeScenePeers: [], rowState: null, bankedDice: [],
		activeRoll: null, activeRollDice: [], activeRollVotes: [],
		activeRollParticipants: [],
		chatPosts: [], chatHasMoreBefore: false,
		chatLastReadPostID: 0, chatInitialReadMarker: 0,
		...overrides,
	};
}

beforeEach(() => clearPageCache());
afterEach(() => vi.useRealTimers());

describe('table snapshots', () => {
	it('round-trips a snapshot for its own game id', () => {
		writeTableSnapshot(7, snap({ currentPlayerID: 42 }));
		expect(readTableSnapshot(7)?.currentPlayerID).toBe(42);
	});

	it('keys games separately, so one table never paints another', () => {
		writeTableSnapshot(7, snap({ currentPlayerID: 42 }));
		expect(readTableSnapshot(8)).toBeNull();
	});

	it('treats numeric and string ids as the same table', () => {
		writeTableSnapshot(7, snap({ currentPlayerID: 42 }));
		expect(readTableSnapshot('7')?.currentPlayerID).toBe(42);
	});

	it('returns null once the entry is older than the max age', () => {
		vi.useFakeTimers();
		writeTableSnapshot(7, snap());
		vi.advanceTimersByTime(5 * 60_000 - 1);
		expect(readTableSnapshot(7)).not.toBeNull();
		vi.advanceTimersByTime(2);
		expect(readTableSnapshot(7)).toBeNull();
	});
});

describe('bounded retention', () => {
	// Without a cap the map grows with every distinct table visited in a
	// session, because an expired entry is only dropped when its own key is
	// read — and a table you never return to is never read.
	it('keeps at most six tables, evicting the least recently written', () => {
		for (let id = 1; id <= 8; id++) writeTableSnapshot(id, snap({ currentPlayerID: id }));
		// 1 and 2 were pushed out; 3..8 survive.
		expect(readTableSnapshot(1)).toBeNull();
		expect(readTableSnapshot(2)).toBeNull();
		for (let id = 3; id <= 8; id++) {
			expect(readTableSnapshot(id)?.currentPlayerID, `table ${id}`).toBe(id);
		}
	});

	it('re-writing a table refreshes its place, so it is not evicted next', () => {
		for (let id = 1; id <= 6; id++) writeTableSnapshot(id, snap({ currentPlayerID: id }));
		writeTableSnapshot(1, snap({ currentPlayerID: 1 })); // touched again
		writeTableSnapshot(7, snap({ currentPlayerID: 7 })); // forces one eviction
		expect(readTableSnapshot(1)?.currentPlayerID).toBe(1); // survived
		expect(readTableSnapshot(2)).toBeNull();              // evicted instead
	});

	it('drops expired entries before evicting live ones', () => {
		vi.useFakeTimers();
		writeTableSnapshot(1, snap({ currentPlayerID: 1 }));
		vi.advanceTimersByTime(5 * 60_000 + 1);
		for (let id = 2; id <= 6; id++) writeTableSnapshot(id, snap({ currentPlayerID: id }));
		// Table 1 aged out, so the five fresh ones all remain without eviction.
		expect(readTableSnapshot(1)).toBeNull();
		for (let id = 2; id <= 6; id++) expect(readTableSnapshot(id)).not.toBeNull();
	});
});

describe('profile snapshot', () => {
	it('round-trips and expires on the same clock', () => {
		vi.useFakeTimers();
		writeProfileSnapshot({ account: acct, tables: [{ game_id: 1 } as MyTable] });
		expect(readProfileSnapshot()?.tables).toHaveLength(1);
		expect(readProfileSnapshot()?.account.username).toBe('alice');
		vi.advanceTimersByTime(5 * 60_000 + 1);
		expect(readProfileSnapshot()).toBeNull();
	});
});

describe('clearPageCache', () => {
	// The privacy-relevant one: these hold one account's roster and secrets,
	// and sign-out must leave nothing for the next person on this browser.
	it('drops both table snapshots and profile tables', () => {
		writeTableSnapshot(7, snap());
		writeProfileSnapshot({ account: acct, tables: [{ game_id: 1 } as MyTable] });
		clearPageCache();
		expect(readTableSnapshot(7)).toBeNull();
		expect(readProfileSnapshot()).toBeNull();
	});
});
