// recordLines.ts — what a plan or scene line in the Public Record's Events list
// says, in one place so a test can pin it.
//
// A plan line carries at most ONE status word. Down the right edge of the list
// they give a quick tally of how the table's plans have gone.
//
//   "Resolving"  the plan being played now (secondary text, no colour)
//   "Make"       a resolved plan that worked (green)
//   "Mar"        a resolved plan that went wrong (red)
//   "No effect"  a plan that closed with no roll at all (secondary text)
//   nothing      a future plan, whose place in the list already says "not yet"
//
// Pure: no Svelte, no fetches.

import type { Plan } from '$lib/api';

export type StatusTone = 'plain' | 'success' | 'danger';

export interface StatusWord {
	text: 'Resolving' | 'Make' | 'Mar' | 'No effect';
	tone: StatusTone;
}

/**
 * Make War and Clandestinely Liaise have no Make or Mar in the book, but the
 * server stores result = "make" on both when they end (a war's plan is closed
 * the moment it begins; a liaison when its secrets are kept), and the action log
 * even says "Make War succeeded". Reading that column straight off the plan would
 * stamp a green "Make" on a war, which the book never grants. So these two never
 * show an outcome word, whatever the column says.
 *
 * "Resolving" is not an outcome, only where the table is, so it still shows while
 * one of them is the plan being played. (docs/work/questions/record-war-liaise-resolving.md
 * asks whether the owner wants even that gone.)
 */
const NO_OUTCOME_WORD = new Set<Plan['plan_type']>(['make_war', 'clandestinely_liaise']);

/**
 * The one status word for a plan line, or null for none.
 *
 * "No effect" is a resolved plan with a NULL result: the server keeps that for a
 * plan that closed without a roll (its target was gone and nothing else could be
 * aimed at, or a counter-propaganda was declined). Nothing was won or lost, so
 * it gets no colour.
 *
 * A cancelled plan never has a row, so it never reaches this; it gets no word
 * here either, rather than a wrong one.
 */
export function planStatusWord(
	plan: Pick<Plan, 'plan_type' | 'status' | 'result'>
): StatusWord | null {
	if (plan.status === 'resolving') return { text: 'Resolving', tone: 'plain' };
	if (plan.status !== 'resolved') return null;
	if (NO_OUTCOME_WORD.has(plan.plan_type)) return null;
	if (plan.result === 'make') return { text: 'Make', tone: 'success' };
	if (plan.result === 'mar') return { text: 'Mar', tone: 'danger' };
	return { text: 'No effect', tone: 'plain' };
}

const SCENE_PREFIX = 'Scene:';

/**
 * A scene line's summary. The server writes it as "Scene: …", and the line draws
 * the house scene mark in place of that word, so the prefix comes off. A line that
 * doesn't carry it is shown whole and is not given the mark.
 */
export function sceneSummary(body: string): { isScene: boolean; text: string } {
	if (body.startsWith(SCENE_PREFIX)) {
		return { isScene: true, text: body.slice(SCENE_PREFIX.length).trim() };
	}
	return { isScene: false, text: body };
}
