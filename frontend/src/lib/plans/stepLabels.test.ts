import { describe, expect, it } from 'vitest';
import type { DiceRoll, Plan, PlanType } from '$lib/api';
import { REGISTRY } from '$lib/components/plans/registry';
import { PLAN_SHORT } from '$lib/components/plans/shared';
import { ownRoll, UNKNOWN_STEP, type StepLabelInput } from './stepLabel';
import { duelBoutLabel } from './stepLabels/propose_duel';

// What each plan calls its open step. The server knows only that a plan is
// resolving; the plan's own resolution data says which step, so each plan has a
// pure function for it (plans/stepLabels/*, registered in the registry) and these
// tests feed each one the data of each of its steps. Every case reads as a heading:
// "Your move", "<label>" and "Waiting on otto", "<label>".

const PLAN_ID = 9;
const PREPARER = 1;
const TARGET = 2;

function plan(planType: PlanType, resolution: Record<string, unknown> = {}, over: Partial<Plan> = {}): Plan {
	return {
		id: PLAN_ID, plan_type: planType, status: 'resolving', preparer_id: PREPARER,
		target_player_id: TARGET, resolution_data: JSON.stringify(resolution), ...over,
	} as Plan;
}

const roll = (over: Partial<DiceRoll> = {}): DiceRoll =>
	({ id: 4, plan_id: PLAN_ID, stage: 'resolved', outcome: null, result: null, ...over }) as DiceRoll;
const made = (result = 2) => roll({ outcome: 'make', result });
const marred = () => roll({ outcome: 'mar' });

function label(p: Plan, over: Partial<StepLabelInput> = {}): string | null {
	return REGISTRY[p.plan_type].stepLabel({ plan: p, plans: [p], roll: null, acting: [], ...over });
}

interface Case {
	step: string;
	plan: Plan;
	over?: Partial<StepLabelInput>;
	label: string | null;
}

// One list per plan, in the order a player meets the steps. A step that rides one of
// the server's own row-state kinds (the roll, a target's answer, a vote) is not here:
// that kind already has its label (lib/waitingOn.ts).
const CASES: Array<[string, Case[]]> = [
	['chronicle_histories', [
		{ step: 'the scene, before the dice', plan: plan('chronicle_histories', {}), label: 'set the scene' },
		{
			step: 'the first make option',
			plan: plan('chronicle_histories', { chronicle_histories: { invoke_phase_closed: true } }),
			over: { roll: made(2) }, label: 'choose a make option',
		},
		{
			step: 'a make option, one of two made',
			plan: plan('chronicle_histories', {
				chronicle_histories: { invoke_phase_closed: true, make_budget: 2, make_choices_done: 1 },
			}),
			over: { roll: made(2) }, label: 'choose a make option',
		},
		{
			step: 'every make option made',
			plan: plan('chronicle_histories', {
				chronicle_histories: { invoke_phase_closed: true, make_budget: 2, make_choices_done: 2 },
			}),
			over: { roll: made(2) }, label: 'wrap the scene',
		},
		{
			// The panel reads `make_budget || roll.result`: a stored 0 is "not written yet",
			// so the roll's own result is the budget.
			step: 'a stored budget of 0 falls back on the roll\'s result',
			plan: plan('chronicle_histories', {
				chronicle_histories: { invoke_phase_closed: true, make_budget: 0, make_choices_done: 1 },
			}),
			over: { roll: made(2) }, label: 'choose a make option',
		},
		{
			step: 'a stored budget of 0, every option made by the roll\'s result',
			plan: plan('chronicle_histories', {
				chronicle_histories: { invoke_phase_closed: true, make_budget: 0, make_choices_done: 2 },
			}),
			over: { roll: made(2) }, label: 'wrap the scene',
		},
		{
			step: 'a mar everyone has answered',
			plan: plan('chronicle_histories', {
				chronicle_histories: { invoke_phase_closed: true, mar_active: true, mar_required_choices: 3 },
			}),
			over: { roll: marred() }, label: 'wrap the scene',
		},
	]],
	['seek_answers', [
		{ step: 'what was learned', plan: plan('seek_answers', {}), label: 'describe what was learned' },
		{
			step: 'the options, once the dice landed',
			plan: plan('seek_answers', { seek_answers: { pre_roll_done: true } }),
			over: { roll: made(3) }, label: 'choose the options',
		},
		{
			step: 'a question put to a player, nothing else owed: the target alone',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'ask_question' }],
				seek_answers: { pre_roll_done: true, pending_question: { target_id: TARGET, question: 'Why?', vetoable: false } },
			}),
			over: { acting: [TARGET] }, label: 'answer the question',
		},
		{
			// The server names the target AND the actor while the actor still has picks of
			// their own, so "answer the question" would tell the actor to answer it.
			step: 'a question in flight while the actor still owes picks: two asks at once',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'ask_question' }, { option: 'declare_truth' }],
				seek_answers: { pre_roll_done: true, pending_question: { target_id: TARGET, question: 'Why?', vetoable: false } },
			}),
			over: { acting: [TARGET, PREPARER] }, label: 'the question and the picks',
		},
		{
			step: 'one kind of pick left',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'declare_truth' }, { option: 'declare_truth' }],
				seek_answers: { pre_roll_done: true, declare_truth_done: 1 },
			}),
			label: 'declare a truth',
		},
		{
			step: 'a break, a reveal and a question left',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'break_resource' }, { option: 'reveal_secret' }, { option: 'ask_question' }],
				seek_answers: { pre_roll_done: true },
			}),
			label: 'carry out the picks',
		},
		{
			step: 'only the mar penalty left',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'ask_question' }],
				seek_answers: {
					pre_roll_done: true, ask_question_done: 1, mar_self_flaws_required: 2, mar_self_flaws_applied: 1,
				},
			}),
			label: 'pay the mar penalty',
		},
		{
			step: 'every pick performed',
			plan: plan('seek_answers', {
				make_mar_choices: [{ option: 'break_resource' }],
				seek_answers: { pre_roll_done: true, break_resource_done: 1 },
			}),
			label: 'complete the plan',
		},
	]],
	['spread_rumors', [
		{ step: 'the rumor, before the dice', plan: plan('spread_rumors', {}), label: 'state the rumor' },
		{
			step: 'the options after a make',
			plan: plan('spread_rumors', { spread_rumors: { pre_roll_done: true, rumor_text: 'x' } }),
			over: { roll: made(2) }, label: 'choose the options',
		},
		{
			step: 'the counter-rumor, which a mar opens with',
			plan: plan('spread_rumors', { spread_rumors: { pre_roll_done: true, rumor_text: 'x' } }),
			over: { roll: marred() }, label: 'write the counter-rumor',
		},
		{
			step: 'the options once the counter-rumor is written',
			plan: plan('spread_rumors', {
				spread_rumors: { pre_roll_done: true, rumor_text: 'x', counter_rumor_text: 'y' },
			}),
			over: { roll: marred() }, label: 'choose the options',
		},
		{
			step: 'a break left',
			plan: plan('spread_rumors', {
				make_mar_choices: [{ option: 'break_target' }, { option: 'hide_source' }],
				spread_rumors: { pre_roll_done: true, hide_source_done: 1 },
			}),
			label: 'break the target asset',
		},
		{
			step: 'a hide left',
			plan: plan('spread_rumors', {
				make_mar_choices: [{ option: 'break_target' }, { option: 'hide_source' }],
				spread_rumors: { pre_roll_done: true, break_target_done: 1 },
			}),
			label: 'hide the source',
		},
		{
			step: 'a break and a hide left',
			plan: plan('spread_rumors', {
				make_mar_choices: [{ option: 'break_target' }, { option: 'hide_source' }],
				spread_rumors: { pre_roll_done: true },
			}),
			label: 'carry out the picks',
		},
		{
			step: 'every pick performed',
			plan: plan('spread_rumors', {
				make_mar_choices: [{ option: 'break_target' }],
				spread_rumors: { pre_roll_done: true, break_target_done: 1 },
			}),
			label: 'complete the plan',
		},
	]],
	['spread_propaganda', [
		{ step: 'the message, before the dice', plan: plan('spread_propaganda', {}), label: 'state the message' },
		{
			step: 'the mar options',
			plan: plan('spread_propaganda', { spread_propaganda: { pre_roll_done: true } }),
			over: { roll: marred() }, label: 'choose the mar options',
		},
		{
			step: 'the artifact a make owes',
			plan: plan('spread_propaganda', {
				choice_auto_applied: true, spread_propaganda: { pre_roll_done: true, artifact_required: true },
			}),
			label: 'create the artifact',
		},
		{
			step: 'a peer to give up',
			plan: plan('spread_propaganda', {
				make_mar_choices: [{ option: 'give_peer' }], choice_submitted: true,
				spread_propaganda: { pre_roll_done: true },
			}),
			label: 'give up a peer',
		},
		{
			step: 'an asset to break',
			plan: plan('spread_propaganda', {
				make_mar_choices: [{ option: 'break_self' }], choice_submitted: true,
				spread_propaganda: { pre_roll_done: true },
			}),
			label: 'break an asset',
		},
		{
			step: 'a peer and an asset owed together',
			plan: plan('spread_propaganda', {
				make_mar_choices: [{ option: 'give_peer' }, { option: 'break_self' }],
				spread_propaganda: { pre_roll_done: true },
			}),
			label: 'face the fallout',
		},
		{
			step: 'every effect performed',
			plan: plan('spread_propaganda', {
				make_mar_choices: [{ option: 'give_peer' }],
				spread_propaganda: { pre_roll_done: true, give_peer_done: 1 },
			}),
			label: 'complete the plan',
		},
	]],
	['make_introductions', [
		{
			step: 'naming the peers, before the dice',
			plan: plan('make_introductions', { make_introductions: { peer_count: 2, drafts: [{ id: 'a', name: 'A' }] } }),
			label: 'name the new peers',
		},
		{
			step: 'the arrivals of a make',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 2, make_pending: true, drafts: [{ id: 'a', name: 'A' }, { id: 'b', name: 'B' }],
					arrivals: [{ draft_id: 'a', asset_id: 5 }],
				},
			}),
			label: 'describe each arrival',
		},
		{
			step: 'every arrival described',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 1, make_pending: true, drafts: [{ id: 'a', name: 'A' }],
					arrivals: [{ draft_id: 'a', asset_id: 5 }],
				},
			}),
			label: null,
		},
		{
			step: 'the fates of a mar',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 2, mar_pending: true, drafts: [{ id: 'a', name: 'A' }, { id: 'b', name: 'B' }],
					mar_outcomes: [{ draft_id: 'a', outcome: 'delayed', done: true }],
				},
			}),
			label: 'settle each newcomer',
		},
		{
			step: 'a mar: only a newcomer\'s marginalia is owed (an author\'s step)',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 1, mar_pending: true, drafts: [{ id: 'a', name: 'A' }],
					mar_outcomes: [{ draft_id: 'a', outcome: 'other_retinue', author_player_id: TARGET, done: false }],
				},
			}),
			label: 'describe a newcomer',
		},
		{
			// The actor can settle the next fate while a recipient is still writing the last, and
			// the server names both: neither "settle" nor "describe" is everyone's step.
			step: 'a mar: fates still to settle and a marginalia still owed: two asks at once',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 2, mar_pending: true, drafts: [{ id: 'a', name: 'A' }, { id: 'b', name: 'B' }],
					mar_outcomes: [{ draft_id: 'a', outcome: 'other_retinue', author_player_id: TARGET, done: false }],
				},
			}),
			label: 'the newcomers',
		},
		{
			step: 'every fate settled',
			plan: plan('make_introductions', {
				choice_auto_applied: true,
				make_introductions: {
					peer_count: 1, mar_pending: true, drafts: [{ id: 'a', name: 'A' }],
					mar_outcomes: [{ draft_id: 'a', outcome: 'delayed', done: true }],
				},
			}),
			label: null,
		},
		{
			step: 'a delayed traveller arriving',
			plan: plan('make_introductions', {
				make_introductions: { delayed_arrival: true, delayed_draft: { id: 'a', name: 'A' } },
			}),
			label: 'describe the delayed arrival',
		},
	]],
	['exchange_courtiers', [
		{
			step: 'an offer awaiting the preparer',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'fair_trade', fair_trade_asset_id: 5 } }),
			label: 'accept or decline the trade',
		},
		{
			step: 'no offer yet (the target owes it)',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'fair_trade' } }),
			label: 'propose a fair trade',
		},
		{
			step: 'the offer taken',
			plan: plan('exchange_courtiers', {
				exchange_courtiers: { phase: 'fair_trade', fair_trade_asset_id: 5, fair_trade_accepted: true },
			}),
			label: null,
		},
		{
			step: 'the opening step, before a phase is written',
			plan: plan('exchange_courtiers', { exchange_courtiers: { fair_trade_asset_id: 5 } }),
			label: 'accept or decline the trade',
		},
		{
			step: 'the make, after the dice',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'roll', fair_trade_accepted: false } }),
			over: { roll: made(2) }, label: 'say how the trade went',
		},
		{
			step: 'the mar (the target\'s), after the dice',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'roll', fair_trade_accepted: false } }),
			over: { roll: marred() }, label: 'choose how to respond to the mar',
		},
		{
			step: 'the roll itself (its own heading)',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'roll', fair_trade_accepted: false } }),
			label: null,
		},
		{
			step: 'a riposte',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'riposte' } }),
			label: 'break or surrender the peer',
		},
		{
			step: 'a messy break (the target owes it)',
			plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'messy_break' } }),
			label: 'break a marginalia',
		},
		{ step: 'done', plan: plan('exchange_courtiers', { exchange_courtiers: { phase: 'done' } }), label: null },
	]],
	['propose_decree', [
		{
			step: 'the council forming, before the debate',
			plan: plan('propose_decree', { propose_decree: { signatory_id: 3 } }),
			over: { acting: [PREPARER, 4] }, label: 'form the council',
		},
		{
			step: 'the debate, with decisions still owed',
			plan: plan('propose_decree', { propose_decree: { signatory_id: 3, debate_started: true } }),
			over: { acting: [4] }, label: 'join or decline the council',
		},
		{
			step: 'the debate, every decision in',
			plan: plan('propose_decree', { propose_decree: { signatory_id: 3, debate_started: true } }),
			over: { acting: [3] }, label: 'end the debate',
		},
		{
			step: 'the dice landed and the result is not yet recorded',
			plan: plan('propose_decree', { propose_decree: { signatory_id: 3, debate_started: true } }),
			over: { roll: made(2), acting: [PREPARER] }, label: null,
		},
		{
			step: 'a mar, with an amender to go',
			plan: plan('propose_decree', {
				propose_decree: { outcome: 'mar', debate_started: true, amendment_order: [4, 5], amended_by: [4] },
			}),
			label: 'amend the law',
		},
		{
			step: 'the addendum, after the amendments',
			plan: plan('propose_decree', {
				propose_decree: { outcome: 'mar', debate_started: true, amendment_order: [4], amended_by: [4] },
			}),
			label: 'place the addendum',
		},
		{
			step: 'the addendum of a make',
			plan: plan('propose_decree', { propose_decree: { outcome: 'make', debate_started: true } }),
			label: 'place the addendum',
		},
		{
			step: 'enacting the law',
			plan: plan('propose_decree', {
				propose_decree: { outcome: 'make', debate_started: true, addendum_placed: true },
			}),
			label: 'enact the law',
		},
		{
			step: 'the law enacted',
			plan: plan('propose_decree', {
				propose_decree: { outcome: 'make', debate_started: true, addendum_placed: true, law_id: 8 },
			}),
			label: null,
		},
	]],
	['propose_duel', [
		{ step: 'the stakes', plan: plan('propose_duel', { duel: { phase: 'setup' } }), label: null },
		{ step: 'the bouts', plan: plan('propose_duel', { duel: { phase: 'bouts' } }), label: null },
		{
			step: 'the claim after a make',
			plan: plan('propose_duel', { duel: { phase: 'roll' } }),
			over: { roll: made(2) }, label: 'claim the stakes',
		},
		{
			step: 'the claim after a mar, which may be spared',
			plan: plan('propose_duel', { duel: { phase: 'roll' } }),
			over: { roll: marred() }, label: 'take the stakes or spare them',
		},
		{ step: 'the roll itself', plan: plan('propose_duel', { duel: { phase: 'roll' } }), label: null },
		{ step: 'done', plan: plan('propose_duel', { duel: { phase: 'done' } }), label: null },
	]],
	['host_festivity', [
		{ step: 'the event', plan: plan('host_festivity', {}), label: 'the festivity' },
	]],
	['make_demands', [
		{ step: 'the draft, mid-way', plan: plan('make_demands', { make_demands: { draft_choices: [{ player_id: 1, option: 'a' }] } }), over: { roll: made(2) }, label: null },
		{
			step: 'every option drafted',
			plan: plan('make_demands', {
				make_demands: { draft_choices: [1, 2, 1, 2].map(p => ({ player_id: p, option: 'x' })) },
			}),
			over: { roll: made(2) }, label: 'complete the plan',
		},
		{
			step: 'the counter-demand placed',
			plan: plan('make_demands', { make_demands: { counter_demand_placed: true } }),
			over: { roll: marred() }, label: 'complete the plan',
		},
		{ step: 'a counter-demand still owed', plan: plan('make_demands', {}), over: { roll: marred() }, label: null },
	]],
	['make_war', [
		{ step: 'resolving (it resolves in one go)', plan: plan('make_war', {}), label: null },
	]],
	['clandestinely_liaise', [
		{ step: 'together at last', plan: plan('clandestinely_liaise', { liaise: { phase: 'together_at_last' } }), label: 'together at last' },
		{ step: 'secrets we keep', plan: plan('clandestinely_liaise', { liaise: { phase: 'secrets_we_keep' } }), label: 'secrets we keep' },
		{ step: 'things we share', plan: plan('clandestinely_liaise', { liaise: { phase: 'things_we_share' } }), label: 'things we share' },
		{
			step: 'when will I see you again',
			plan: plan('clandestinely_liaise', { liaise: { phase: 'when_will_i_see_you_again' } }),
			label: 'when will I see you again?',
		},
		{ step: 'done', plan: plan('clandestinely_liaise', { liaise: { phase: 'done' } }), label: null },
		{ step: 'before it has a phase', plan: plan('clandestinely_liaise', {}), label: null },
	]],
];

describe('each plan names its open step', () => {
	for (const [type, cases] of CASES) {
		describe(PLAN_SHORT[type as PlanType], () => {
			it.each(cases)('$step', (c) => {
				expect(label(c.plan, c.over)).toBe(c.label);
			});
		});
	}

	it('covers every plan the registry holds', () => {
		expect(CASES.map(([type]) => type).sort()).toEqual(Object.keys(REGISTRY).sort());
	});
});

describe('a label reads as the end of a heading', () => {
	const all = CASES.flatMap(([, cases]) => cases.flatMap(c => (c.label == null ? [] : [c.label])));
	// The book's own name for a Liaise phase is a question put to the other party;
	// it is a title here, not an address to the reader.
	const BOOK_TITLES = new Set(['when will I see you again?']);

	it.each([...new Set(all)])('"%s" is a lower-case phrase', (text) => {
		expect(text).toMatch(/^[a-z]/);
	});

	it.each([...new Set(all)])('"%s" does not speak to the reader', (text) => {
		if (BOOK_TITLES.has(text)) return;
		expect(text).not.toMatch(/\byou(r|rs|rself)?\b/i);
	});

	it.each([...new Set(all)])('"%s" does not name its plan', (text) => {
		for (const name of Object.values(PLAN_SHORT)) {
			expect(text.toLowerCase()).not.toContain(name.toLowerCase());
		}
	});
});

describe('ownRoll', () => {
	const p = plan('propose_duel', { duel: { phase: 'roll' } });

	it('keeps the plan\'s own roll', () => {
		const own = made(2);
		expect(ownRoll(p, own)).toBe(own);
	});

	it('drops another plan\'s roll, so a label never reads it', () => {
		// A Make Demands roll can sit on the table beside the plan it targets.
		expect(ownRoll(p, roll({ plan_id: PLAN_ID + 1, outcome: 'mar' }))).toBeNull();
		expect(ownRoll(p, roll({ plan_id: null }))).toBeNull();
		expect(ownRoll(p, null)).toBeNull();
	});
});

describe('the unknown step', () => {
	it('is the one phrase for "the plan cannot tell"', () => {
		expect(UNKNOWN_STEP).toBe('the next step');
	});
});

// The bout: the heading's who-line says who is acting, so the label says which half of the
// bout is theirs (the panel used to print "alice to declare" / "to respond").
describe('the duel bout label', () => {
	const duel = (initiative: number | null) => ({
		id: 4, plan_type: 'propose_duel', status: 'resolving', preparer_id: 1, target_player_id: 2,
		resolution_data: JSON.stringify({ duel: { phase: 'bouts', initiative_player_id: initiative } }),
	}) as never;

	it('says declare for the initiative player and respond for the other duellist', () => {
		expect(duelBoutLabel(duel(1), [1])).toBe('declare the bout');
		expect(duelBoutLabel(duel(1), [2])).toBe('respond to the bout');
		expect(duelBoutLabel(duel(2), [2])).toBe('declare the bout');
	});

	it('stays "the bout" when it cannot tell (no plan, no initiative, a stand-in)', () => {
		expect(duelBoutLabel(null, [1])).toBe('the bout');
		expect(duelBoutLabel(duel(null), [1])).toBe('the bout');
		expect(duelBoutLabel(duel(1), [9])).toBe('the bout');
	});
});
