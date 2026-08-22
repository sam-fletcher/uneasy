import { describe, it, expect } from 'vitest';
import { tableBell } from './tableBell';
import type { MyTable } from '$lib/api';

function table(over: Partial<MyTable> = {}): MyTable {
	return {
		game_id: 1,
		join_code: 'AMGVBY',
		is_facilitator: false,
		joined_at: '2026-08-01T00:00:00Z',
		phase: 'main_event',
		player_id: 7,
		players: [],
		waiting_on_player_ids: [7],
		unread_count: 0,
		reminder_muted: false,
		reminder_exhausted: false,
		...over,
	};
}

describe('tableBell', () => {
	it('offers to silence a table that is waiting on you', () => {
		const bell = tableBell(table());
		expect(bell?.struck).toBe(false);
		expect(bell?.setMutedTo).toBe(true);
		expect(bell?.label).toContain('AMGVBY');
	});

	it('shows the struck bell and offers to undo once silenced', () => {
		const bell = tableBell(table({ reminder_muted: true }));
		expect(bell?.struck).toBe(true);
		expect(bell?.setMutedTo).toBe(false);
	});

	// The mute is per-wait, so a table not currently waiting on you has no
	// pending reminder to act on — and no muted state that could be stranded
	// out of sight, which is the whole reason the scope is the wait.
	it('renders nothing when the table is not waiting on you', () => {
		expect(tableBell(table({ waiting_on_player_ids: [] }))).toBeNull();
		expect(tableBell(table({ waiting_on_player_ids: [99] }))).toBeNull();
	});

	it('renders nothing for an ended table, even mid-wait', () => {
		expect(tableBell(table({ phase: 'ended' }))).toBeNull();
	});

	it('reports an exhausted wait as struck but not as a control', () => {
		const bell = tableBell(table({ reminder_exhausted: true }));
		expect(bell?.struck).toBe(true);
		// Nothing to toggle: the reminders already stopped on their own, and
		// offering to silence them would claim an effect the tap cannot have.
		expect(bell?.setMutedTo).toBeNull();
		expect(bell?.title).toContain('stopped');
	});

	it('lets an explicit mute outrank the give-up, since only it can be undone here', () => {
		const bell = tableBell(table({ reminder_muted: true, reminder_exhausted: true }));
		expect(bell?.setMutedTo).toBe(false);
	});
});
