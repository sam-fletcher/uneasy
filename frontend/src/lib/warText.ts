// warText.ts
// The words the war band and the war's panel say, as pure functions so the
// wording is tested rather than inferred from markup.
//
// A war is named for the two ends of it: "otto’s war on Wren" (the declarer
// and the first player they named, plus "and others" when there were more).
// Each side is called after its leader in the same way, so a choice reads
// "Join otto" and not "Join Side 1".

import type { Plan, Player, WarStateResponse } from '$lib/api';
import { parseMakeWarData } from '$lib/plans/resolutionData/make_war';
import { playerName } from '$lib/components/plans/shared';

export interface WarLeaders {
	/** Side 1: whoever declared the war. */
	declarer: string;
	/** Side 2: the first player the declarer named, or null if the plan doesn't say. */
	enemy: string | null;
	/** True when the declarer named more than one enemy. */
	andOthers: boolean;
}

export function warLeaders(plan: Plan, players: Player[]): WarLeaders {
	const enemies = parseMakeWarData(plan).enemy_player_ids ?? [];
	return {
		declarer: playerName(players, plan.preparer_id),
		enemy: enemies.length > 0 ? playerName(players, enemies[0]) : null,
		andOthers: enemies.length > 1,
	};
}

/** "otto’s war on Wren", or "otto’s war on Wren and others". */
export function warName(plan: Plan, players: Player[]): string {
	const l = warLeaders(plan, players);
	if (l.enemy == null) return `${l.declarer}’s war`;
	return `${l.declarer}’s war on ${l.enemy}${l.andOthers ? ' and others' : ''}`;
}

/** The leader of a side: side 1 is the declarer, side 2 the first enemy named. */
export function sideLeader(side: 1 | 2, leaders: WarLeaders): string {
	return side === 1 ? leaders.declarer : (leaders.enemy ?? 'the enemy');
}

/** Where the war stands, in one or two words, for the band. */
export function warStatusWord(plan: Plan): 'planned' | 'under way' {
	return plan.status === 'pending' ? 'planned' : 'under way';
}

/**
 * The lede under the sheet's title: when the war starts or started, and what
 * being in it costs. The cost recurs every row, so each lede restates it.
 */
export function warLede(plan: Plan, war: WarStateResponse | null): string {
	const rule = 'everyone in it pays the cost of battle each time the record advances';
	if (plan.status === 'pending') {
		const row = plan.row_number != null ? ` for row ${plan.row_number}` : '';
		return `Planned${row}. Once it begins, ${rule}.`;
	}
	// The war begins on its plan's row. The war record's own start is the row it
	// was declared on, which is earlier by the length of the delay.
	const began = plan.row_number ?? war?.started_at_row ?? null;
	const since = began != null ? ` since row ${began}` : '';
	return `Under way${since}. ${rule[0].toUpperCase()}${rule.slice(1)}.`;
}

/** "Wren", "Wren and Mira", "Wren, Mira and Ash". */
export function nameList(names: string[]): string {
	if (names.length <= 1) return names[0] ?? '';
	return `${names.slice(0, -1).join(', ')} and ${names[names.length - 1]}`;
}

/**
 * The opponents a late joiner on `side` owes an entry payment to: every
 * opposing participant who is fully in and hasn't surrendered. The same rule
 * the server applies (handler/plan_make_war_payments.go), so the price shown
 * is the price charged.
 */
export function entryOpponents(war: WarStateResponse, side: 1 | 2): number[] {
	return war.participants
		.filter(p => p.entry_payment_complete && p.surrendered_at_row == null && p.side !== side)
		.map(p => p.player_id);
}

export type JoinChoiceID = 'join1' | 'join2' | 'stay';

export interface JoinChoice {
	id: JoinChoiceID;
	/** The side joined, or null for staying out. */
	side: 1 | 2 | null;
	/** The chip: "Join otto". */
	label: string;
	/** The main button once this is picked: "Join otto’s side". */
	button: string;
	/** What it costs, shown under the chips once this one is picked. */
	cost: string;
}

/**
 * The three things a player outside the war can choose, with the cost of each.
 * `begun` is whether the delay vote has closed (the plan has a row). While the
 * vote is open a join is free and puts the joiner in the vote. From the moment
 * it closes, including the rows when the war is only planned, a join owes one
 * entry payment per opponent already in. Either way a full participant then pays
 * the cost of battle each time the record advances, until the war ends.
 */
export function joinChoices(
	war: WarStateResponse,
	plan: Plan,
	players: Player[],
	begun: boolean,
): JoinChoice[] {
	const leaders = warLeaders(plan, players);
	const sides: (1 | 2)[] = [1, 2];
	const joins = sides.map<JoinChoice>(side => {
		const leader = sideLeader(side, leaders);
		const against = nameList(
			entryOpponents(war, side).map(id => playerName(players, id)),
		);
		const after = 'Then you pay the cost of battle each time the record advances, until the war ends.';
		let cost: string;
		if (!begun) {
			cost = `Joining is free while the war’s delay is still being voted, and you vote on it with the others. ${after}`;
		} else if (against === '') {
			cost = `Joining costs an entry payment for each opponent already in, and none are in yet. ${after}`;
		} else {
			cost = `Joining costs an entry payment for each opponent already in, now ${against}: break one of your assets, or leverage two. ${after}`;
		}
		return {
			id: side === 1 ? 'join1' : 'join2',
			side,
			label: `Join ${leader}`,
			button: `Join ${leader}’s side`,
			cost,
		};
	});
	return [
		...joins,
		{
			id: 'stay',
			side: null,
			label: 'Stay out',
			button: 'Stay out',
			cost: 'Staying out tells the table you won’t join. You can still join later, while the war lasts.',
		},
	];
}
