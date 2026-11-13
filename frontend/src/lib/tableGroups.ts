// tableGroups.ts — which heading each of your tables sits under on the profile.
//
// People open this page to find where they're needed, so the list leads with
// the answer: the tables waiting on you, then every other live table, then
// the finished ones. Each group keeps the order the server sent
// (most recently joined first), so a table doesn't jump about between visits
// except when its turn actually changes hands.
//
// Pure: no Svelte, no fetches, so the grouping rules are unit-testable away
// from the page's markup.

import type { MyTable } from '$lib/api';

export type TableGroupKey = 'you' | 'others' | 'finished';

export interface TableGroup {
	key: TableGroupKey;
	/** The small heading above the group's rows. */
	heading: string;
	tables: MyTable[];
}

/** Whether a live table is blocked on this viewer. A finished table waits on no one. */
export function isYourMove(t: MyTable): boolean {
	return t.phase !== 'ended' && t.waiting_on_player_ids.includes(t.player_id);
}

/**
 * The three groups, in display order, with an empty group left out entirely
 * (an empty heading would only be noise).
 *
 * "Waiting on others" is where every live table that isn't waiting on you
 * goes, including one waiting on no one at all (a lobby nobody has been
 * assigned to yet, say). The alternative, a fourth heading for "waiting on
 * no one", would be a distinction with no use to a player choosing a table.
 */
export function groupTables(tables: MyTable[]): TableGroup[] {
	const groups: TableGroup[] = [
		{ key: 'you', heading: 'Waiting on you', tables: tables.filter(isYourMove) },
		{
			key: 'others',
			heading: 'Waiting on others',
			tables: tables.filter((t) => t.phase !== 'ended' && !isYourMove(t)),
		},
		{ key: 'finished', heading: 'Finished', tables: tables.filter((t) => t.phase === 'ended') },
	];
	return groups.filter((g) => g.tables.length > 0);
}
