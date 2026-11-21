import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SeekAnswersPanel from './SeekAnswersPanel.svelte';
import { seekAnswersMarPenalty } from './shared';
import type { PlanContext } from './types';

// Two lines in the option-picking step and one in the break-a-resource sub-step
// were wrong:
//   · "On a mar you must also describe a flaw…" printed on a make as well, and gave
//     no number. It now shows only on a mar, with how many resources are owed
//     (the shortfall, capped at the resources the preparer can still flaw).
//   · "No resource can be broken" printed twice for the player who owes the pick.

const PREPARER = 1;
const OTHER = 2;
const PLAYERS = [
	{ id: PREPARER, display_name: 'ravi' },
	{ id: OTHER, display_name: 'sol' },
];
const resource = (id: number, owner: number) => ({
	id, name: `Ledger ${id}`, owner_id: owner, asset_type: 'resource', is_destroyed: false, is_leveraged: false,
	secret_count: 0,
	marginalia: [{ id: id * 10, asset_id: id, position: 1, text: 'worn', is_torn: false }],
});

function renderFor(opts: {
	seat: number;
	outcome: 'make' | 'mar';
	result?: number;
	difficulty?: number;
	assets?: unknown[];
	top?: Record<string, unknown>;
}): string {
	const plan = {
		id: 9, plan_type: 'seek_answers', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'x',
		resolution_data: JSON.stringify({ seek_answers: { pre_roll_done: true }, ...opts.top }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: (opts.assets ?? []) as never, players: PLAYERS as never, rankings: [],
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: opts.outcome,
		activeRoll: {
			id: 4, plan_id: 9, status: 'resolved', result: opts.result ?? 1, difficulty: opts.difficulty ?? 3,
		} as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SeekAnswersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

const text = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

describe('the mar penalty, as the picks are made', () => {
	const mine = [resource(31, PREPARER), resource(32, PREPARER), resource(33, PREPARER)];

	it('is not mentioned on a make', () => {
		for (const seat of [PREPARER, OTHER]) {
			const t = text(renderFor({ seat, outcome: 'make', result: 3, difficulty: 2, assets: mine }));
			expect(t).toContain('Pick options equal to');
			expect(t).not.toContain('must also describe a flaw');
			expect(t).not.toContain('On a mar');
		}
	});

	it('says how many resources the preparer must flaw: the shortfall', () => {
		const t = text(renderFor({ seat: PREPARER, outcome: 'mar', result: 1, difficulty: 3, assets: mine }));
		expect(t).toContain('You must also describe a flaw in (break) 2 of your own resource assets');
	});

	it('names the preparer for everyone else, in the third person', () => {
		const t = text(renderFor({ seat: OTHER, outcome: 'mar', result: 1, difficulty: 3, assets: mine }));
		expect(t).toContain('ravi must also describe a flaw in (break) 2 of their own resource assets');
		expect(t).not.toContain('your own');
	});

	it('says nothing about a cap while the count is the whole shortfall', () => {
		const t = text(renderFor({ seat: PREPARER, outcome: 'mar', result: 1, difficulty: 3, assets: mine }));
		expect(t).not.toContain('can still be flawed');
	});

	it('is capped by the resources the preparer can still flaw, and says why', () => {
		const t = text(renderFor({
			seat: PREPARER, outcome: 'mar', result: 0, difficulty: 3, assets: [resource(31, PREPARER)],
		}));
		expect(t).toContain('(break) 1 of your own resource assets');
		expect(t).toContain('The plan marred by 3, but only 1 of your resources can still be flawed.');
	});

	it('names the preparer and their resources for everyone else when capped', () => {
		const t = text(renderFor({
			seat: OTHER, outcome: 'mar', result: 0, difficulty: 3, assets: [resource(31, PREPARER)],
		}));
		expect(t).toContain('ravi must also describe a flaw in (break) 1 of their own resource assets');
		expect(t).toContain('but only 1 of their resources can still be flawed.');
		expect(t).not.toContain('your');
	});

	it('explains a count of zero rather than going quiet: nothing is owed, and why', () => {
		// Another player's resource is not the preparer's to flaw.
		const own = text(renderFor({ seat: PREPARER, outcome: 'mar', result: 1, difficulty: 3, assets: [resource(41, OTHER)] }));
		expect(own).not.toContain('must also describe a flaw');
		expect(own).toContain('The plan marred by 2, but you have no resource left to flaw, so you owe none.');
		const other = text(renderFor({ seat: OTHER, outcome: 'mar', result: 1, difficulty: 3, assets: [resource(41, OTHER)] }));
		expect(other).toContain('The plan marred by 2, but ravi has no resource left to flaw, so owes none.');
	});
});

describe('seekAnswersMarPenalty', () => {
	it('is the shortfall, capped by the resources that can still be flawed', () => {
		expect(seekAnswersMarPenalty(3, 1, 5)).toBe(2);
		expect(seekAnswersMarPenalty(3, 0, 2)).toBe(2);
		expect(seekAnswersMarPenalty(4, 1, 0)).toBe(0);
	});
	it('is never negative', () => {
		expect(seekAnswersMarPenalty(2, 3, 4)).toBe(0);
	});
});

describe('a break-a-resource pick with nothing to break', () => {
	const picked = { make_mar_choices: [{ option: 'break_resource', player_id: PREPARER }], choice_submitted: true };
	const count = (t: string) => t.split('No resource can be broken').length - 1;

	it('says so once to the player who owes it, and why they can skip', () => {
		const t = text(renderFor({ seat: PREPARER, outcome: 'make', result: 1, difficulty: 1, assets: [], top: picked }));
		expect(count(t)).toBe(1);
		expect(t).toContain('This pick has no valid target');
		expect(t).toContain('Skip — no valid targets');
	});

	it('says so once to everyone else, who see the same step read-only', () => {
		const t = text(renderFor({ seat: OTHER, outcome: 'make', result: 1, difficulty: 1, assets: [], top: picked }));
		expect(count(t)).toBe(1);
	});
});

// The pre-roll was its own greyed-out fieldset with a bare "Submit" that also cast the
// dice, and no word of how hard the roll would be. It is the shared pre-roll step now:
// the preparer's form says what pressing the button does, and everyone sees the
// difficulty before deciding whether to help or interfere.
describe('the pre-roll, before the dice', () => {
	const rankings = [{ id: 1, game_id: 1, category: 'knowledge', player_id: PREPARER, rank: 2 }];

	function renderPreRoll(opts: {
		seat: number; outcome?: 'make' | 'mar' | null; preRollDone?: boolean;
		/** The choices are committed, and another plan's roll is the one open on the table. */
		committedWithOtherRoll?: boolean;
	}): string {
		const plan = {
			id: 9, plan_type: 'seek_answers', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'old ledgers',
			resolution_data: JSON.stringify({
				seek_answers: { pre_roll_done: opts.preRollDone ?? false },
				...(opts.committedWithOtherRoll ? { choice_submitted: true } : {}),
			}),
		};
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
			assets: [] as never, players: PLAYERS as never, rankings: rankings as never,
			currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
			rollActive: false, rollOutcome: opts.outcome ?? null,
			activeRoll: (opts.committedWithOtherRoll ? { id: 8, plan_id: 77, outcome: null, difficulty: 2 } : null) as never,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		return render(SeekAnswersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
	}

	it('gives the preparer the shared step: the prompt, a field, and a button that says it opens the roll', () => {
		const body = renderPreRoll({ seat: PREPARER });
		const t = text(body);
		expect(t).toContain('Restate your methods of research');
		expect(body.match(/<textarea/g)).toHaveLength(1);
		expect(body).toMatch(/<button[^>]*>\s*Post &amp; open the roll/);
		// The button no longer says only "Submit" while it also casts the dice.
		expect(body).not.toMatch(/<button[^>]*>\s*Submit\s*<\/button>/);
		expect(t).toContain('The table reads this before anyone decides');
	});

	// Whether a pre-roll offers the preparation note back is still open for the plans
	// that restate a draft; Seek Answers has always been written from scratch, and
	// moving to the shared step must not change that. (The card above still quotes the
	// note as a finished line; what is checked is that the step doesn't offer it back.)
	it('starts the field blank: the step does not offer the preparation note back', () => {
		const body = renderPreRoll({ seat: PREPARER });
		expect(text(body)).not.toContain('Starts from your preparation note');
	});

	it('shows everyone the difficulty, and where it comes from, before the dice', () => {
		for (const seat of [PREPARER, OTHER]) {
			const t = text(renderPreRoll({ seat }));
			expect(t).toContain('Difficulty 2');
			expect(t).toContain('ravi’s rank on the knowledge track: 2');
		}
	});

	it('gives a watcher no form and no button: the heading says whose move it is', () => {
		const body = renderPreRoll({ seat: OTHER });
		expect(body).not.toContain('<textarea');
		expect(body).not.toMatch(/<button[^>]*>\s*Post &amp; open the roll/);
		expect(body).not.toMatch(/<button[^>]*>\s*Submit/);
	});

	it('does not bring the meter back when another plan\'s roll opens after this plan\'s dice have landed', () => {
		// `rollOutcome` follows whichever roll is on the table: it is null again for the newly opened one.
		const t = text(renderPreRoll({ seat: OTHER, preRollDone: true, committedWithOtherRoll: true }));
		expect(t).not.toContain('1 easy');
		expect(t).not.toContain('What can happen?');
	});

	it('draws the meter only until the dice land', () => {
		expect(text(renderPreRoll({ seat: PREPARER, preRollDone: true, outcome: null }))).toContain('Difficulty 2');
		expect(text(renderPreRoll({ seat: PREPARER, preRollDone: true, outcome: 'make' }))).not.toContain('1 easy');
		expect(text(renderPreRoll({ seat: OTHER, preRollDone: true, outcome: 'mar' }))).not.toContain('1 easy');
	});
});
