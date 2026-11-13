// toneGroups.ts — the Tones sheet's list, once tones are locked.
//
// Before the Main Event the sheet is a grid of every topic, to set tones on. Once
// they lock, a grid of 41 tiles is hard to scan for the few that matter, so the
// sheet lists only the topics someone took a position on, grouped by how firm
// that position is, with a line counting the rest. The order puts "Never" first:
// it is the one a player must not miss.
//
// Pure: no Svelte, no fetches.

import type { ToneTopic, ToneTopicStatus } from '$lib/api';

/** The three positions a topic can be set to, strongest first. */
export type SetTone = Exclude<ToneTopicStatus, 'default'>;

export const TONE_ORDER: readonly SetTone[] = ['never', 'avoid_detail', 'include'];

/** What the group is called. The same words the grid's key uses. */
export const TONE_LABEL: Record<ToneTopicStatus, string> = {
	never: 'Never',
	avoid_detail: 'Avoid detail',
	include: 'Include',
	default: 'No opinion',
};

export interface ToneGroup {
	status: SetTone;
	label: string;
	topics: ToneTopic[];
}

export interface LockedTones {
	/** Only groups with something in them, in TONE_ORDER. */
	groups: ToneGroup[];
	/** Topics nobody set. */
	restCount: number;
}

/** Splits the table's topics for the locked list. Within a group the topics
 *  keep the order the server sent, which is the order the grid had. */
export function lockedTones(topics: ToneTopic[]): LockedTones {
	const groups: ToneGroup[] = [];
	for (const status of TONE_ORDER) {
		const inGroup = topics.filter((t) => t.status === status);
		if (inGroup.length > 0) groups.push({ status, label: TONE_LABEL[status], topics: inGroup });
	}
	const set = groups.reduce((n, g) => n + g.topics.length, 0);
	return { groups, restCount: topics.length - set };
}

/** The line under the list that counts the topics with no position. */
export function restLine(setCount: number, restCount: number): string | null {
	if (restCount <= 0) return null;
	if (setCount === 0) {
		return restCount === 1 ? 'The one topic has no opinion.' : `All ${restCount} topics have no opinion.`;
	}
	return `${restCount} more with no opinion.`;
}
