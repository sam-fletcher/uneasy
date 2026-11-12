// lawsRumorsSeen.test.ts — the one comparison behind every "new" dot on Laws and
// Rumors. The server half (markers, clamping, edits drawing a new rev) is in
// handler/laws_rumors_seen_integration_test.go.

import { describe, it, expect } from 'vitest';
import type { Law, Rumor } from '$lib/api';
import { hasNewEntries, isNewEntry, newEntryCount, seenThrough } from './lawsRumorsSeen';

const entry = (rev: number) => ({ rev });

describe('isNewEntry', () => {
	it('is new when its rev is above the marker', () => {
		expect(isNewEntry(entry(5), 4)).toBe(true);
	});

	it('is not new at the marker: the marker is the highest rev seen', () => {
		expect(isNewEntry(entry(5), 5)).toBe(false);
	});

	it('is not new below the marker', () => {
		expect(isNewEntry(entry(2), 5)).toBe(false);
	});

	it('counts everything as new for a marker of 0, the seat that has seen nothing', () => {
		expect(isNewEntry(entry(1), 0)).toBe(true);
	});

	it('counts an edit as new: the edit drew a higher rev than the one seen', () => {
		// Seen through rev 7; the law at rev 3 is then edited and takes rev 9.
		const before = entry(3);
		const edited = entry(9);
		expect(isNewEntry(before, 7)).toBe(false);
		expect(isNewEntry(edited, 7)).toBe(true);
	});

	it('takes real laws and rumors as they come off the wire, hidden source included', () => {
		const law = { rev: 4, signatory_id: null } as unknown as Law;
		const hidden = { rev: 6, source_player_id: null } as unknown as Rumor;
		expect(isNewEntry(law, 3)).toBe(true);
		expect(isNewEntry(hidden, 6)).toBe(false);
	});
});

describe('hasNewEntries', () => {
	it('is false for an empty list', () => {
		expect(hasNewEntries([], 0)).toBe(false);
	});

	it('is true when any one entry is new', () => {
		expect(hasNewEntries([entry(1), entry(2), entry(8)], 5)).toBe(true);
	});

	it('is false when the marker has caught up with the newest', () => {
		expect(hasNewEntries([entry(1), entry(2), entry(8)], 8)).toBe(false);
	});

	it('does not depend on list order', () => {
		expect(hasNewEntries([entry(8), entry(1)], 5)).toBe(true);
	});
});

describe('newEntryCount', () => {
	it('counts only the entries above the marker', () => {
		expect(newEntryCount([entry(1), entry(6), entry(7), entry(9)], 6)).toBe(2);
	});

	it('is zero for an empty list', () => {
		expect(newEntryCount([], 3)).toBe(0);
	});
});

describe('seenThrough', () => {
	it('reports the highest rev on screen when something is new', () => {
		expect(seenThrough([entry(3), entry(9), entry(5)], 4)).toBe(9);
	});

	it('reports nothing when nothing is new: the request would change nothing', () => {
		expect(seenThrough([entry(3), entry(4)], 4)).toBeNull();
	});

	it('reports nothing for an empty list', () => {
		expect(seenThrough([], 0)).toBeNull();
	});

	it('clears every dot that was showing: reporting it leaves no entry new', () => {
		const list = [entry(2), entry(7), entry(11)];
		const reported = seenThrough(list, 5);
		expect(reported).toBe(11);
		expect(hasNewEntries(list, reported!)).toBe(false);
	});
});
