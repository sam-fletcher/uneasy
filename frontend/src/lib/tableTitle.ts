// tableTitle.ts — a table's one title, in one place.
//
// The redesign gives every table a single title, shown in the table page's
// title bar and, in the same words, on the profile page's row for it:
//
//   Lobby · Prologue · Choosing · Row 6 · Seek Answers · Shake-Up · Esteem · Game over
//
// It names the middle level of detail. The phase alone is too coarse to earn the
// space, and the step belongs to the play area (DECISIONS, Screen anatomy), so
// the title names the row and its plan, the Prologue's step, the Shake-Up's
// category. Both surfaces call tableTitle, so they cannot disagree.
//
// Pure: no Svelte, no fetches. The input is shaped like the fields `Game` and
// `MyTable` already carry, so either passes straight in. `plan_type` is the one
// field neither `Game` has: the plan the Main Event row is about. The server
// decides it once (handler/table_title.go) and sends it on the row state, which
// the table page reads as `rowState.title_plan_type` and the profile list gets as
// `MyTable.plan_type`. There is no client-side pick to keep in step with it.

import type { GamePhase, PlanType, PrologueRankingStep } from '$lib/api';
import { PLAN_SHORT } from '$lib/components/plans/shared';

/** Everything a title is built from. Field names match `Game` and `MyTable`. */
export interface TableTitleInput {
	phase: GamePhase;
	/** The Main Event row (1–13); 0 before the first. */
	current_row: number;
	/** The plan the current row is about: `rowState.title_plan_type` on the table
	 *  page, `plan_type` from the profile list. Only read in the Main Event. */
	plan_type: PlanType | null;
	prologue_ranking_step: PrologueRankingStep | null;
	/** True while the row 7 → 8 advance waits on the ending vote. */
	ending_vote_open: boolean;
	shake_up_category: string | null;
}

const SEPARATOR = ' · ';

const TRACK_NAME: Record<string, string> = {
	power: 'Power',
	knowledge: 'Knowledge',
	esteem: 'Esteem',
};

const capitalise = (s: string): string => (s ? s[0].toUpperCase() + s.slice(1) : s);
const trackName = (track: string): string => TRACK_NAME[track] ?? capitalise(track);

/** The Prologue step's own words. null is the first stage, where players take
 *  turns choosing boxes; the ranking stages repeat per track. */
function prologueStepName(step: PrologueRankingStep | null): string {
	if (step == null) return 'Choosing';
	if (step === 'closing') return 'Closing';
	if (step.startsWith('declare_')) return `${trackName(step.slice('declare_'.length))} ranking`;
	if (step.startsWith('place_set_asides_')) {
		return `${trackName(step.slice('place_set_asides_'.length))} set-asides`;
	}
	return capitalise(step.replaceAll('_', ' '));
}

/**
 * The table's title.
 *
 * | State                              | Title                          |
 * |------------------------------------|--------------------------------|
 * | lobby                              | Lobby                          |
 * | prologue, choosing                 | Prologue · Choosing            |
 * | prologue, a track's ranking        | Prologue · Power ranking       |
 * | prologue, a track's set-asides     | Prologue · Power set-asides    |
 * | prologue, closing                  | Prologue · Closing             |
 * | Main Event, row with a plan        | Row 6 · Seek Answers           |
 * | Main Event, row with no plan       | Row 6                          |
 * | Main Event, ending vote            | Row 7 · Ending vote            |
 * | Shake-Up, a category               | Shake-Up · Esteem              |
 * | Shake-Up, before the first         | Shake-Up                       |
 * | game over                          | Game over                      |
 * | Main Event before row 1            | Main Event      (unreachable)  |
 * | the retired tone-setting phase     | Tone setting    (unreachable)  |
 *
 * A row with a scene and a row with nothing yet read alike, "Row 6": the title
 * names the plan, and the scene is a step, which belongs to the play area. Only
 * "Row 6 · Seek Answers", "Prologue · Choosing", "Lobby" and "Game over" are the
 * owner's wording; the rest is proposed (docs/work/questions/table-title-wording.md).
 */
export function tableTitle(t: TableTitleInput): string {
	switch (t.phase) {
		case 'lobby':
			return 'Lobby';
		case 'tone_setting':
			// Unreachable: the server stopped entering this phase and migration 023
			// moved every existing game out of it, though the column's CHECK still
			// allows it. Named so a stray row would read as something.
			return 'Tone setting';
		case 'prologue':
			return ['Prologue', prologueStepName(t.prologue_ranking_step)].join(SEPARATOR);
		case 'main_event': {
			// Unreachable too: advanceToMainEvent sets current_row to 1 before it
			// flips the phase. Here for a test seed that skips that.
			if (t.current_row < 1) return 'Main Event';
			const row = `Row ${t.current_row}`;
			// The vote outranks the plan: the flag is only set once the row's own
			// business is done (ComputeRowState, endgameVoteGate).
			if (t.ending_vote_open) return [row, 'Ending vote'].join(SEPARATOR);
			return t.plan_type ? [row, PLAN_SHORT[t.plan_type] ?? t.plan_type].join(SEPARATOR) : row;
		}
		case 'shake_up':
			return t.shake_up_category
				? ['Shake-Up', trackName(t.shake_up_category)].join(SEPARATOR)
				: 'Shake-Up';
		case 'ended':
			return 'Game over';
	}
	// A phase this client has not heard of: say it plainly rather than nothing.
	return capitalise(String(t.phase).replaceAll('_', ' '));
}
