import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import type { Component } from 'svelte';
import ChronicleHistoriesPanel from './ChronicleHistoriesPanel.svelte';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import ProposeDecreePanel from './ProposeDecreePanel.svelte';
import SeekAnswersPanel from './SeekAnswersPanel.svelte';
import SpreadPropagandaPanel from './SpreadPropagandaPanel.svelte';
import SpreadRumorsPanel from './SpreadRumorsPanel.svelte';
import type { PlanContext } from './types';

// A Make Demands perform_steps win hands a plan's post-roll steps to the winner
// and locks the preparer out (the server 403s them). Each panel must show the
// winner its post-roll form and the preparer a wait line — never the reverse —
// while the pre-roll steps stay with the preparer.

const PREPARER = 1;
const WINNER = 2;
const BYSTANDER = 3;

type Case = {
	name: string;
	panel: Component<any>;
	planType: string;
	rollOutcome: 'make' | 'mar';
	resolution: Record<string, unknown>;
	extra?: Record<string, unknown>;
	/** Present only on the post-roll form the actor works. */
	form: string;
	/** Present only on the wait line shown to everyone else. */
	wait: string;
	/** No roll yet: leave the roll state empty (pre-roll steps stay with the preparer). */
	noRoll?: boolean;
};

const CASES: Case[] = [
	{
		name: 'Seek Answers', panel: SeekAnswersPanel, planType: 'seek_answers', rollOutcome: 'make',
		resolution: { seek_answers: { pre_roll_done: true } },
		form: 'Apply choices', wait: 'sol is choosing make/mar options',
	},
	{
		name: 'Spread Rumors', panel: SpreadRumorsPanel, planType: 'spread_rumors', rollOutcome: 'make',
		resolution: { spread_rumors: { pre_roll_done: true, rumor_text: 'x' } },
		form: 'Apply choices', wait: 'sol is choosing options',
	},
	{
		name: 'Spread Propaganda', panel: SpreadPropagandaPanel, planType: 'spread_propaganda', rollOutcome: 'mar',
		resolution: { spread_propaganda: { pre_roll_done: true } },
		form: 'Apply choices', wait: 'sol is resolving Spread Propaganda',
	},
	{
		name: 'Chronicle Histories', panel: ChronicleHistoriesPanel, planType: 'chronicle_histories', rollOutcome: 'make',
		resolution: { chronicle_histories: { invoke_phase_closed: true, make_budget: 2 } },
		form: 'Choose an option', wait: 'sol is choosing options',
	},
	{
		name: 'Exchange Courtiers', panel: ExchangeCourtiersPanel, planType: 'exchange_courtiers', rollOutcome: 'make',
		resolution: { exchange_courtiers: { fair_trade_accepted: false } },
		extra: { target_player_id: BYSTANDER, target_asset_id: 5 },
		form: 'Apply choices', wait: 'Waiting for sol to say how the trade went',
	},
	{
		name: 'Propose Decree', panel: ProposeDecreePanel, planType: 'propose_decree', rollOutcome: 'make',
		resolution: {
			propose_decree: {
				signatory_id: BYSTANDER, signatory_player_ids: [PREPARER, BYSTANDER],
				debate_started: true, outcome: 'make', addendum_placed: true,
			},
		},
		form: 'Enact the law', wait: 'Waiting for sol to enact the law',
	},
];

/** The form is "live" when its label is present and not inside a disabled fieldset. */
function live(body: string, form: string): boolean {
	return body.includes(form) && !body.includes('resolve-mirror-wrap choices-section resolve-mirror')
		&& !/<fieldset[^>]*resolve-mirror[^>]*disabled/.test(body);
}

function renderFor(c: Case, currentPlayerID: number): string {
	const target = {
		id: 9, plan_type: c.planType, status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x', resolution_data: JSON.stringify(c.resolution), ...c.extra,
	};
	const demand = {
		id: 12, plan_type: 'make_demands', status: 'resolved', result: 'make',
		preparer_id: WINNER, targeted_plan_id: 9,
		resolution_data: JSON.stringify({
			make_demands: { draft_choices: [{ option: 'perform_steps', player_id: WINNER }] },
		}),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [target, demand] as never,
		assets: [],
		players: [
			{ id: PREPARER, display_name: 'ravi' }, { id: WINNER, display_name: 'sol' },
			{ id: BYSTANDER, display_name: 'mel' },
		] as never,
		rankings: [], currentPlayerID, isFocusPlayer: false,
		rollActive: false, rollOutcome: c.noRoll ? null : c.rollOutcome,
		activeRoll: c.noRoll ? null : ({ id: 4, plan_id: 9, status: 'resolved' } as never),
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(c.panel, { props: { ctx, plan: target as never, mode: 'resolve' } }).body;
}

const SEEK_DONE: Case = {
	name: 'Seek Answers, sub-flows done', panel: SeekAnswersPanel, planType: 'seek_answers', rollOutcome: 'make',
	resolution: { seek_answers: { pre_roll_done: true }, choice_submitted: true },
	form: 'Complete plan', wait: 'Waiting for ravi to complete the plan',
};

const DECREE_ADDENDUM: Case = {
	name: 'Propose Decree addendum', panel: ProposeDecreePanel, planType: 'propose_decree', rollOutcome: 'make',
	resolution: {
		propose_decree: {
			signatory_id: PREPARER, signatory_player_ids: [PREPARER, BYSTANDER],
			debate_started: true, outcome: 'make',
		},
	},
	form: 'Place addendum', wait: 'Waiting for sol to place the addendum',
};

const SEEK_PRE_ROLL: Case = {
	name: 'Seek Answers pre-roll', panel: SeekAnswersPanel, planType: 'seek_answers', rollOutcome: 'make', noRoll: true,
	resolution: { seek_answers: {} },
	form: 'Submit', wait: 'ravi is researching',
};

describe.each([...CASES, DECREE_ADDENDUM])('$name with a perform_steps winner', (c) => {
	it('gives the post-roll form to the winner', () => {
		const body = renderFor(c, WINNER);
		expect(live(body, c.form)).toBe(true);
		expect(body).not.toContain(c.wait);
	});

	it('shows the preparer a wait on the winner, not the form', () => {
		const body = renderFor(c, PREPARER);
		expect(live(body, c.form)).toBe(false);
		expect(body).toContain(c.wait);
	});
});

describe('pre-roll steps stay with the preparer under perform_steps', () => {
	it('shows the preparer the narration form and the winner a wait', () => {
		const prep = renderFor(SEEK_PRE_ROLL, PREPARER);
		const winner = renderFor(SEEK_PRE_ROLL, WINNER);
		expect(prep).toContain('Submit');
		expect(winner).not.toContain('Submit');
		expect(winner).toContain('ravi is researching');
	});
});

describe('completion stays with the preparer under perform_steps', () => {
	it('offers Complete plan to the preparer and a wait to the winner', () => {
		expect(renderFor(SEEK_DONE, PREPARER)).toContain('Complete plan');
		const winner = renderFor(SEEK_DONE, WINNER);
		expect(winner).not.toContain('Complete plan');
		expect(winner).toContain(SEEK_DONE.wait);
	});
});
