// tableTitle.test.ts — the one title a table wears in its title bar and on the
// profile page's row for it. Every state a table can be in has a case below.
//
// Which plan a row names is decided on the server (handler/table_title.go) and
// tested there; this file only turns the fields into words.

import { describe, it, expect } from 'vitest';
import type { PlanType, PrologueRankingStep } from '$lib/api';
import { PLAN_SHORT } from '$lib/components/plans/shared';
import { tableTitle, type TableTitleInput } from './tableTitle';

const base: TableTitleInput = {
	phase: 'main_event',
	current_row: 6,
	plan_type: null,
	prologue_ranking_step: null,
	ending_vote_open: false,
	shake_up_category: null,
};
const title = (over: Partial<TableTitleInput>) => tableTitle({ ...base, ...over });

describe('tableTitle: the owner’s four examples', () => {
	it('Row 6 · Seek Answers', () => {
		expect(title({ plan_type: 'seek_answers' })).toBe('Row 6 · Seek Answers');
	});
	it('Prologue · Choosing', () => {
		expect(title({ phase: 'prologue', current_row: 0 })).toBe('Prologue · Choosing');
	});
	it('Lobby', () => {
		expect(title({ phase: 'lobby', current_row: 0 })).toBe('Lobby');
	});
	it('Game over', () => {
		expect(title({ phase: 'ended', current_row: 13 })).toBe('Game over');
	});
});

describe('tableTitle: the Prologue', () => {
	const steps: [PrologueRankingStep | null, string][] = [
		[null, 'Prologue · Choosing'],
		['declare_power', 'Prologue · Power ranking'],
		['place_set_asides_power', 'Prologue · Power set-asides'],
		['declare_knowledge', 'Prologue · Knowledge ranking'],
		['place_set_asides_knowledge', 'Prologue · Knowledge set-asides'],
		['declare_esteem', 'Prologue · Esteem ranking'],
		['place_set_asides_esteem', 'Prologue · Esteem set-asides'],
		['closing', 'Prologue · Closing'],
	];
	it.each(steps)('step %s reads %s', (step, want) => {
		expect(title({ phase: 'prologue', current_row: 0, prologue_ranking_step: step })).toBe(want);
	});

	it('ignores a stray row, plan or vote flag: they belong to the Main Event', () => {
		expect(
			title({ phase: 'prologue', current_row: 4, plan_type: 'make_war', ending_vote_open: true }),
		).toBe('Prologue · Choosing');
	});
});

describe('tableTitle: the Main Event', () => {
	it('names every plan type the way the rest of the app does', () => {
		for (const [type, name] of Object.entries(PLAN_SHORT)) {
			expect(title({ plan_type: type as PlanType })).toBe(`Row 6 · ${name}`);
		}
	});

	it('is just the row when the row has no plan: a scene, or nothing yet', () => {
		expect(title({ plan_type: null })).toBe('Row 6');
		expect(title({ current_row: 13, plan_type: null })).toBe('Row 13');
	});

	it('names the vote over the plan: the flag is set only once the row’s business is done', () => {
		expect(title({ current_row: 7, ending_vote_open: true })).toBe('Row 7 · Ending vote');
		expect(title({ current_row: 7, plan_type: 'seek_answers', ending_vote_open: true })).toBe(
			'Row 7 · Ending vote',
		);
	});

	it('does not let a plan carry over into a Main Event row 0', () => {
		expect(title({ current_row: 0, plan_type: 'seek_answers' })).toBe('Main Event');
	});

	it('falls back to the raw type for a plan type this client does not know', () => {
		expect(title({ plan_type: 'a_new_plan' as PlanType })).toBe('Row 6 · a_new_plan');
	});
});

describe('tableTitle: the Shake-Up and the end', () => {
	it('names the category being shaken up', () => {
		expect(title({ phase: 'shake_up', shake_up_category: 'esteem' })).toBe('Shake-Up · Esteem');
		expect(title({ phase: 'shake_up', shake_up_category: 'knowledge' })).toBe('Shake-Up · Knowledge');
		expect(title({ phase: 'shake_up', shake_up_category: 'power' })).toBe('Shake-Up · Power');
	});

	it('is just the Shake-Up before its first category is set', () => {
		expect(title({ phase: 'shake_up', shake_up_category: null })).toBe('Shake-Up');
	});

	it('does not carry a Main Event plan or vote into the Shake-Up or the end', () => {
		expect(title({ phase: 'shake_up', plan_type: 'seek_answers', ending_vote_open: true })).toBe('Shake-Up');
		expect(title({ phase: 'ended', plan_type: 'seek_answers' })).toBe('Game over');
	});
});

describe('tableTitle: states nobody enters today', () => {
	it('still reads something for the retired tone-setting phase', () => {
		expect(title({ phase: 'tone_setting' })).toBe('Tone setting');
	});

	it('says a phase it has not heard of plainly', () => {
		expect(title({ phase: 'epilogue' as never })).toBe('Epilogue');
	});
});
