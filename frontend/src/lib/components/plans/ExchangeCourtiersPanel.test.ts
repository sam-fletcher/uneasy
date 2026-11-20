import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import { MAR_OPTIONS } from './shared';
import type { PlanContext } from './types';

// A seat that isn't acting is told in the third person what the plan holds, and
// only the actual addressee sees second-person wording. Who the plan is waiting on
// at each step after the roll is the open step's heading's to say ("Waiting on sol
// — choose how to respond to the mar", from plans/stepLabels), so the panel prints
// no line for it.

const PREPARER = 1;
const TARGET = 2;
const BYSTANDER = 3;
const PLAYERS = [
	{ id: PREPARER, display_name: 'ravi' },
	{ id: TARGET, display_name: 'sol' },
	{ id: BYSTANDER, display_name: 'mel' },
];
const ASSETS = [
	{ id: 5, name: 'Lord Vane', owner_id: TARGET, asset_type: 'peer', is_destroyed: false },
	{ id: 6, name: 'Dame Orr', owner_id: PREPARER, asset_type: 'peer', is_destroyed: false },
];

function renderFor(
	seat: number,
	resolution: Record<string, unknown>,
	rollOutcome: 'make' | 'mar' | null = null,
	assets = ASSETS,
	/** Top-level resolution_data (make_mar_choices, choice_submitted, …) and the live-roll flag. */
	extra: { top?: Record<string, unknown>; rollActive?: boolean; plans?: unknown[] } = {},
): string {
	const plan = {
		id: 9, plan_type: 'exchange_courtiers', status: 'resolving', preparer_id: PREPARER,
		target_player_id: TARGET, target_asset_id: 5, preparation_notes: 'x',
		resolution_data: JSON.stringify({ exchange_courtiers: resolution, ...extra.top }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan, ...(extra.plans ?? [])] as never,
		assets: assets as never, players: PLAYERS as never, rankings: [], currentPlayerID: seat,
		focusPlayerID: null, isFocusPlayer: false, rollActive: extra.rollActive ?? false, rollOutcome,
		activeRoll: rollOutcome ? ({ id: 4, plan_id: 9, status: 'resolved', result: 1, difficulty: 2 } as never) : null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ExchangeCourtiersPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

const text = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

describe('before the offer', () => {
	it('tells a bystander the target is proposing', () => {
		expect(text(renderFor(BYSTANDER, {}))).toContain('Waiting for sol to propose a fair trade');
	});
	it('names the preparer when they have no peers to trade', () => {
		const t = text(renderFor(BYSTANDER, {}, null, ASSETS.filter(a => a.owner_id !== PREPARER)));
		expect(t).toContain('Waiting for ravi to proceed to the dice roll');
	});
	it('tells the preparer the target is proposing', () => {
		expect(text(renderFor(PREPARER, {}))).toContain('Waiting for sol to propose a fair trade');
	});
});

describe('after the offer', () => {
	const offered = { fair_trade_asset_id: 6 };
	it('addresses the target in the second person', () => {
		expect(text(renderFor(TARGET, offered))).toContain('You asked for Dame Orr');
	});
	it('names the target for a bystander', () => {
		const t = text(renderFor(BYSTANDER, offered));
		expect(t).not.toContain('You asked for');
		expect(t).toContain('sol asked for Dame Orr');
	});
});

describe('after the roll', () => {
	const declined = { fair_trade_asset_id: 6, fair_trade_accepted: false };
	// The seats that do not choose get no picker, and no line about who does: the
	// heading says it. (The old lines were "Waiting for ravi to say how the trade went"
	// and "Waiting for sol to choose how to respond".)
	it.each([
		['the target on a make', TARGET, 'make'],
		['the preparer on a mar', PREPARER, 'mar'],
		['a bystander on a mar', BYSTANDER, 'mar'],
		['a bystander on a make', BYSTANDER, 'make'],
	] as const)('says nothing to %s about who is choosing', (_, seat, outcome) => {
		const t = text(renderFor(seat, declined, outcome));
		expect(t).not.toContain('Waiting for');
		expect(t).not.toContain('You beat the difficulty');
		expect(t).not.toContain('choose how you respond');
	});
});

describe('between a decline and the roll arriving', () => {
	it('does not announce a make to a bystander', () => {
		const t = text(renderFor(BYSTANDER, { fair_trade_asset_id: 6, fair_trade_accepted: false }));
		expect(t).not.toContain('say how the trade went');
	});
});

describe('Riposte wording', () => {
	it('is written for the target who chooses it', () => {
		const riposte = MAR_OPTIONS.exchange_courtiers!.find(o => o.key === 'riposte')!;
		expect(riposte.desc).not.toMatch(/you may break/i);
		expect(riposte.desc).toMatch(/they may break/i);
	});
});

// choicesDone reads the committed marker, not the picked options. Every route
// to a commit carries exactly one pick (the server validates that while a roll
// exists), so the marker only has to agree with the picks here; these pin what
// each seat sees on both sides of the commit, including the marker-only shape.
describe('commit marker', () => {
	const declined = { fair_trade_asset_id: 6, fair_trade_accepted: false };
	const PICKED = { top: { make_mar_choices: [{ option: 'legal', player_id: PREPARER }] } };
	const MARKED = { top: { choice_submitted: true } };

	describe('before the commit (the dice have landed, nothing chosen)', () => {
		it('make: the preparer gets the picker', () => {
			const t = text(renderFor(PREPARER, declined, 'make'));
			expect(t).toContain('You beat the difficulty');
			expect(t).not.toContain('Resolving…');
		});
		it('mar: the target gets the picker', () => {
			const t = text(renderFor(TARGET, declined, 'mar'));
			expect(t).toContain('choose how you respond');
			expect(t).not.toContain('Resolving…');
		});
		it('the roll still on screen: what is at stake, no picker, and no dice wait (the roll has its own heading)', () => {
			for (const seat of [PREPARER, TARGET, BYSTANDER]) {
				const t = text(renderFor(seat, declined, 'make', ASSETS, { rollActive: true }));
				expect(t).toContain('On a make:');
				expect(t).not.toContain('Dice roll in progress');
				expect(t).not.toContain('You beat the difficulty');
			}
		});
	});

	describe('after the commit', () => {
		it.each([['picks', PICKED], ['the marker alone', MARKED]])('make, %s: no picker, no dice wait', (_, extra) => {
			for (const seat of [PREPARER, TARGET, BYSTANDER]) {
				const t = text(renderFor(seat, declined, 'make', ASSETS, { ...extra, rollActive: true }));
				expect(t).toContain('Resolving…');
				expect(t).not.toContain('You beat the difficulty');
				expect(t).not.toContain('Dice roll in progress');
			}
		});
		it('a legacy row with picks and no marker still counts as committed', () => {
			expect(text(renderFor(PREPARER, declined, 'make', ASSETS, PICKED))).toContain('Resolving…');
		});
		it('messy make: the target breaks, and the note says it is the last step', () => {
			const messy = { ...declined, messy_break_required: true };
			const t = text(renderFor(TARGET, messy, 'make', ASSETS, MARKED));
			expect(t).toContain('The exchange was messy');
			expect(t).toContain('Exchange Courtiers resolves when you submit');
			expect(text(renderFor(PREPARER, messy, 'make', ASSETS, MARKED))).not.toContain('Waiting for sol to break a marginalia');
		});
		it('riposte mar: the preparer breaks or surrenders, and the note says it is the last step', () => {
			const riposte = { ...declined, riposte_allowed: true };
			// Two notes on the requested peer, so a break can leave it standing.
			const withNotes = ASSETS.map(a => (a.id === 6 ? {
				...a, marginalia: [1, 2].map(id => ({ id, asset_id: 6, position: id, text: `n${id}`, is_torn: false })),
			} : a));
			const t = text(renderFor(PREPARER, riposte, 'mar', withNotes, MARKED));
			expect(t).toContain('You may break it first');
			expect(t).toContain('Either choice is the last step');
			expect(text(renderFor(BYSTANDER, riposte, 'mar', ASSETS, MARKED))).toContain('Riposte — waiting for ravi');
		});
	});
});

// On a riposte the preparer may break the requested peer first, but the server
// refuses to tear its last intact marginalia: the peer has to survive to be
// surrendered. So the picker offers a note to tear only when a second one remains,
// and otherwise says why the only way out is to surrender it intact.
describe('riposte: breaking the requested peer first', () => {
	const riposte = { fair_trade_asset_id: 6, fair_trade_accepted: false, riposte_allowed: true };
	const MARKED = { top: { choice_submitted: true } };
	const note = (id: number, torn = false) =>
		({ id, asset_id: 6, position: id, text: `note ${id}`, is_torn: torn });
	const orrWith = (notes: ReturnType<typeof note>[]) =>
		ASSETS.map(a => (a.id === 6 ? { ...a, marginalia: notes } : a));

	it('offers the notes to tear while the peer would survive', () => {
		const t = text(renderFor(PREPARER, riposte, 'mar', orrWith([note(1), note(2)]), MARKED));
		expect(t).toContain('Marginalium to break');
		expect(t).toContain('Break, then surrender');
		expect(t).not.toContain('can only be surrendered intact');
		// The sentence after the peer's name keeps its space (a block body's leading
		// space is trimmed, which once glued the two together).
		expect(t).toContain('Dame Orr. You may break it first');
	});

	it('does not offer its last intact note, and says why', () => {
		const t = text(renderFor(PREPARER, riposte, 'mar', orrWith([note(1), note(2, true)]), MARKED));
		expect(t).not.toContain('Marginalium to break');
		expect(t).not.toContain('Break, then surrender');
		expect(t).toContain('Surrender intact');
		expect(t).toContain('Dame Orr has one marginalia left');
		expect(t).toContain('can only be surrendered intact');
	});

	it('a peer with no notes at all can only be surrendered', () => {
		const t = text(renderFor(PREPARER, riposte, 'mar', orrWith([]), MARKED));
		expect(t).not.toContain('Break, then surrender');
		expect(t).toContain('Surrender intact');
		expect(t).toContain('Dame Orr has no marginalia to tear');
	});

	it('shows a seat that is not deciding neither the picker nor the buttons', () => {
		const t = text(renderFor(BYSTANDER, riposte, 'mar', orrWith([note(1)]), MARKED));
		expect(t).not.toContain('Break, then surrender');
		expect(t).not.toContain('Surrender intact');
	});

	// A Make Demands perform-steps winner decides the preparer's steps in their place
	// (mel here), and is the only seat given the buttons.
	describe('for the Make Demands winner who decides in the preparer\'s place', () => {
		const winnerPlans = [{
			id: 12, plan_type: 'make_demands', status: 'resolved', result: 'make', preparer_id: BYSTANDER,
			targeted_plan_id: 9,
			resolution_data: JSON.stringify({
				make_demands: { draft_choices: [{ option: 'perform_steps', player_id: BYSTANDER }] },
			}),
		}];
		const asWinner = (notes: ReturnType<typeof note>[]) =>
			text(renderFor(BYSTANDER, riposte, 'mar', orrWith(notes), { ...MARKED, plans: winnerPlans }));

		it('may break it first while it has a second note', () => {
			const t = asWinner([note(1), note(2)]);
			expect(t).toContain("ravi's Dame Orr. As the Make Demands winner you decide whether to break it first or surrender it intact.");
			expect(t).toContain('Break, then surrender');
			expect(t).toContain('Surrender intact');
		});

		it('can only surrender it when its last note would be torn, and is told so in the winner\'s words', () => {
			const t = asWinner([note(1)]);
			expect(t).toContain("ravi's Dame Orr. As the Make Demands winner you surrender it.");
			expect(t).toContain('Dame Orr has one marginalia left');
			expect(t).not.toContain('Break, then surrender');
			expect(t).toContain('Surrender intact');
		});

		it('is not handed to the preparer, who is locked out', () => {
			const t = text(renderFor(PREPARER, riposte, 'mar', orrWith([note(1), note(2)]), { ...MARKED, plans: winnerPlans }));
			expect(t).not.toContain('Surrender intact');
		});
	});
});
