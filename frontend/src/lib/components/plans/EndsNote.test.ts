import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SpreadPropagandaPanel from './SpreadPropagandaPanel.svelte';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import MainCharacterChoicePanel from '../MainCharacterChoicePanel.svelte';
import SeekAnswersPanel from './SeekAnswersPanel.svelte';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import MakeDemandsPanel from './MakeDemandsPanel.svelte';
import PeaceFlow from './war/PeaceFlow.svelte';
import SurrenderClaims from './war/SurrenderClaims.svelte';
import type { PlanContext } from './types';

// Principle 3, third limit: a removed click's punctuation is a sentence under the
// button that ends a plan or sub-flow. These cover the representative cases, both
// that the line appears on the ending submit and that it does not on a submit
// that merely moves along.

const PREPARER = 1;
const OTHER = 2;
const LAST_SPREAD = 'Spread Propaganda resolves when you submit';

function ctxFor(plan: object, currentPlayerID: number, extra: Partial<PlanContext> = {}): PlanContext {
	return {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [],
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID, isFocusPlayer: false,
		rollActive: false, rollOutcome: null, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		...extra,
	} as PlanContext;
}

function spPlan(resolution: Record<string, unknown>) {
	return {
		id: 9, plan_type: 'spread_propaganda', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
	};
}

function renderSP(resolution: Record<string, unknown>, rollOutcome: 'make' | 'mar' | null = 'mar'): string {
	const plan = spPlan(resolution);
	const ctx = ctxFor(plan, PREPARER, {
		rollOutcome,
		activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never,
	});
	return render(SpreadPropagandaPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Spread Propaganda says when a submit is the last step', () => {
	const picks = (...opts: string[]) => opts.map(option => ({ option, player_id: PREPARER }));

	it('on the last peer to give up', () => {
		const body = renderSP({
			spread_propaganda: { pre_roll_done: true, give_peer_done: 1 },
			make_mar_choices: picks('give_peer', 'give_peer'), choice_submitted: true,
		});
		expect(body).toContain(LAST_SPREAD);
	});

	it('not while more than one step is owed', () => {
		const body = renderSP({
			spread_propaganda: { pre_roll_done: true },
			make_mar_choices: picks('give_peer', 'break_self'), choice_submitted: true,
		});
		expect(body).not.toContain(LAST_SPREAD);
	});

	it('on the artifact when it is the only thing left', () => {
		const body = renderSP({
			spread_propaganda: { pre_roll_done: true, artifact_required: true },
			make_mar_choices: picks('create_artifact'), choice_submitted: true,
		}, 'make');
		expect(body).toContain(LAST_SPREAD);
	});
});

describe('Make Introductions mar says when a submit is the last', () => {
	function renderMI(currentPlayerID: number, mi: Record<string, unknown>): string {
		const plan = {
			id: 9, plan_type: 'make_introductions', status: 'resolving', preparer_id: PREPARER,
			preparation_notes: 'x', resolution_data: JSON.stringify({ make_introductions: mi }),
		};
		const ctx = ctxFor(plan, currentPlayerID, { rollOutcome: 'mar' });
		return render(MakeIntroductionsPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
	}
	const drafts = [{ id: 'd1', name: 'Lady Vane' }, { id: 'd2', name: 'Sir Orrin' }];

	it('tells the last author they are the one the plan waits on', () => {
		const body = renderMI(OTHER, {
			peer_count: 2, mar_pending: true, drafts,
			mar_outcomes: [
				{ draft_id: 'd1', outcome: 'broken_journey', done: true },
				{ draft_id: 'd2', outcome: 'other_retinue', author_player_id: OTHER, done: false },
			],
		});
		expect(body).toContain('You are the last one the plan is waiting on');
	});

	it('does not say so while another newcomer is still unresolved', () => {
		const body = renderMI(OTHER, {
			peer_count: 2, mar_pending: true, drafts,
			mar_outcomes: [{ draft_id: 'd1', outcome: 'other_retinue', author_player_id: OTHER, done: false }],
		});
		expect(body).not.toContain('You are the last one');
	});
});

describe('the main-character-lost screen says why and who is waited on', () => {
	const props = (currentPlayerID: number) => ({
		gameID: 1, assets: [], players: [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }] as never,
		currentPlayerID, actingPlayerIDs: [OTHER], playerNameMap: new Map([[OTHER, 'sol']]),
	});

	it('explains the loss to the player who must choose', () => {
		const body = render(MainCharacterChoicePanel, { props: props(OTHER) }).body.replace(/\s+/g, ' ');
		expect(body).toContain('taken, traded away or destroyed');
		expect(body).toContain('waiting on you');
	});

	it('names who is waited on, and why, for everyone else', () => {
		// Svelte keeps the template's line breaks, so compare with whitespace folded.
		const body = render(MainCharacterChoicePanel, { props: props(PREPARER) }).body.replace(/\s+/g, ' ');
		expect(body).toContain('sol lost their main character');
		expect(body).toContain('The log says what happened');
		expect(body).not.toContain('plan that just ended');
	});
});

describe('Spread Propaganda counter-propaganda Decline tells the truth about the parent', () => {
	function renderCounter(parentStatus: string | null): string {
		const child = spPlan({ spread_propaganda: { original_plan_id: 3 } });
		const parent = { ...spPlan({}), id: 3, status: parentStatus };
		const plans = parentStatus == null ? [child] : [child, parent];
		const ctx = ctxFor(child, PREPARER, { plans: plans as never });
		return render(SpreadPropagandaPanel, { props: { ctx, plan: child as never, mode: 'resolve' } }).body;
	}
	it('says the parent carries on while it is still resolving', () => {
		const body = renderCounter('resolving');
		expect(body).toContain('carries on');
		expect(body).not.toContain('has already ended');
	});
	it('says only the ending when the parent is not known', () => {
		const child = spPlan({ spread_propaganda: { original_plan_id: 3 } });
		const body = render(SpreadPropagandaPanel, { props: { ctx: ctxFor(child, PREPARER), plan: child as never, mode: 'resolve' } }).body;
		expect(body).toContain('Declining ends this counter-propaganda');
		expect(body).not.toContain('carries on');
		expect(body).not.toContain('has already ended');
	});
	it('says it has ended once it resolved', () => {
		const body = renderCounter('resolved');
		expect(body).toContain('has already ended');
		expect(body).not.toContain('carries on');
	});
});

describe('Seek Answers says when a submit is the last step', () => {
	function renderSA(currentPlayerID: number, resolution: Record<string, unknown>): string {
		const plan = {
			id: 9, plan_type: 'seek_answers', status: 'resolving', preparer_id: PREPARER,
			preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
		};
		const ctx = ctxFor(plan, currentPlayerID, { rollOutcome: 'make', activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never });
		return render(SeekAnswersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
	}
	const picks = (...o: string[]) => o.map(option => ({ option, player_id: PREPARER }));
	it('on the last declaration', () => {
		const body = renderSA(PREPARER, {
			seek_answers: { pre_roll_done: true }, make_mar_choices: picks('declare_truth'), choice_submitted: true,
		});
		expect(body).toContain('Seek Answers resolves when you submit');
	});
	it('not while two steps are owed', () => {
		const body = renderSA(PREPARER, {
			seek_answers: { pre_roll_done: true }, make_mar_choices: picks('declare_truth', 'declare_truth'), choice_submitted: true,
		});
		expect(body).not.toContain('Seek Answers resolves when you submit');
	});
	it('names Answer, not Veto, for the questioned player on the last question', () => {
		const body = renderSA(OTHER, {
			seek_answers: { pre_roll_done: true, pending_question: { target_id: OTHER, question: 'Why?', vetoable: true } },
			make_mar_choices: picks('ask_question'), choice_submitted: true,
		});
		expect(body).toContain('Answering is the last step');
		expect(body).toContain('A veto ends nothing');
	});
});

describe('Exchange Courtiers says what accepting the trade does', () => {
	it('tells the preparer an accepted trade ends the plan and a decline goes to the dice', () => {
		const plan = {
			id: 9, plan_type: 'exchange_courtiers', status: 'resolving', preparer_id: PREPARER,
			target_player_id: OTHER, target_asset_id: 31, preparation_notes: 'x',
			resolution_data: JSON.stringify({ exchange_courtiers: { fair_trade_asset_id: 32 } }),
		};
		const ctx = ctxFor(plan, PREPARER);
		const body = render(ExchangeCourtiersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
		expect(body).toContain('Accepting ends the plan with no roll');
	});
});

describe('Make Demands says when a pick or counter ends the plan', () => {
	function renderMD(resolution: Record<string, unknown>, rollOutcome: 'make' | 'mar', who = PREPARER): string {
		const plan = {
			id: 9, plan_type: 'make_demands', status: 'resolving', preparer_id: PREPARER,
			targeted_plan_id: 3, preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
		};
		const target = { id: 3, plan_type: 'spread_propaganda', status: 'pending', preparer_id: OTHER, row_number: 6 };
		const ctx = ctxFor(plan, who, { plans: [plan, target] as never, rollOutcome, activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never });
		return render(MakeDemandsPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
	}
	it('says the counter-demand is the last step', () => {
		const body = renderMD({ make_demands: { outcome: 'mar' } }, 'mar', OTHER);
		expect(body).toContain('Make Demands resolves when you submit');
	});
});

describe('Make War lines', () => {
	const players = [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }] as never;
	const proposal = (votes: object[], awaiting: number[]) => ({
		id: 1, proposer_id: PREPARER, terms: 'a truce', votes, awaiting,
	}) as never;
	const peace = (p: never) => render(PeaceFlow, {
		props: { proposal: p, planID: 9, players, currentPlayerID: OTHER, amFullParticipant: true, onChanged: () => {}, setError: () => {} },
	}).body;
	it('says accepting ends the war only when everyone else has accepted', () => {
		expect(peace(proposal([{ player_id: 3, accepted: true }], [OTHER]))).toContain('the war ends in peace');
	});
	it('stays silent when a vote has been rejected or others are still out', () => {
		expect(peace(proposal([{ player_id: 3, accepted: false }], [OTHER]))).not.toContain('the war ends in peace');
		expect(peace(proposal([], [OTHER, 3]))).not.toContain('the war ends in peace');
	});
	it('claims only what is true on a surrender claim: play goes on once all are settled', () => {
		const body = render(SurrenderClaims, {
			props: { claims: [{ id: 1, surrendered_id: PREPARER }] as never, planID: 9, players, assets: [], onChanged: () => {}, setError: () => {} },
		}).body;
		expect(body).toContain('Play goes on once every surrender claim and battle cost at the table is settled');
		expect(body).not.toContain('row moves on');
	});
});
