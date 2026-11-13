import { describe, it, expect } from 'vitest';
import { groupTables, isYourMove } from './tableGroups';
import type { MyTable } from '$lib/api';

function table(id: number, over: Partial<MyTable> = {}): MyTable {
	return {
		game_id: id,
		join_code: `CODE${id}`,
		is_facilitator: false,
		joined_at: '2026-08-01T00:00:00Z',
		phase: 'main_event',
		current_row: 6,
		plan_type: null,
		prologue_ranking_step: null,
		ending_vote_open: false,
		shake_up_category: null,
		player_id: 100 + id,
		players: [],
		waiting_on_player_ids: [],
		unread_count: 0,
		reminder_muted: false,
		reminder_exhausted: false,
		...over,
	};
}

const yours = (id: number, over: Partial<MyTable> = {}) =>
	table(id, { waiting_on_player_ids: [100 + id], ...over });
const theirs = (id: number, over: Partial<MyTable> = {}) =>
	table(id, { waiting_on_player_ids: [999], ...over });
const finished = (id: number) => table(id, { phase: 'ended' });

describe('groupTables', () => {
	it('puts each table under the heading that answers "is it my move?"', () => {
		const groups = groupTables([theirs(1), yours(2), finished(3)]);
		expect(groups.map((g) => [g.heading, g.tables.map((t) => t.game_id)])).toEqual([
			['Waiting on you', [2]],
			['Waiting on others', [1]],
			['Finished', [3]],
		]);
	});

	it('leaves out a group with no tables, so no empty heading is drawn', () => {
		expect(groupTables([theirs(1)]).map((g) => g.key)).toEqual(['others']);
		expect(groupTables([yours(1), finished(2)]).map((g) => g.key)).toEqual(['you', 'finished']);
		expect(groupTables([])).toEqual([]);
	});

	it("keeps each group's tables in the order the server sent them", () => {
		const groups = groupTables([yours(5), theirs(4), yours(3), theirs(2), finished(7), finished(6)]);
		expect(groups.find((g) => g.key === 'you')?.tables.map((t) => t.game_id)).toEqual([5, 3]);
		expect(groups.find((g) => g.key === 'others')?.tables.map((t) => t.game_id)).toEqual([4, 2]);
		expect(groups.find((g) => g.key === 'finished')?.tables.map((t) => t.game_id)).toEqual([7, 6]);
	});

	it('puts a live table waiting on no one under "Waiting on others"', () => {
		const groups = groupTables([table(1)]);
		expect(groups).toHaveLength(1);
		expect(groups[0].key).toBe('others');
	});

	// A finished game's wait list is empty from the server, but the rule should
	// not depend on that: a stale response must not file a finished game under
	// "Waiting on you".
	it('files a finished table under "Finished" even if the wait list names you', () => {
		const stale = table(1, { phase: 'ended', waiting_on_player_ids: [101] });
		expect(isYourMove(stale)).toBe(false);
		expect(groupTables([stale]).map((g) => g.key)).toEqual(['finished']);
	});

	it('counts a wait that includes you among others as your move', () => {
		expect(isYourMove(table(1, { waiting_on_player_ids: [999, 101] }))).toBe(true);
	});
});
