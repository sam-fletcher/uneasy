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
// Whole assets, as the table has them: a picker draws a card for each, and a card needs
// its marginalia and counts. A bare { id, name, owner } was enough until a test rendered one.
const peerAsset = (id: number, name: string, owner_id: number) => ({
	id, name, owner_id, asset_type: 'peer', is_destroyed: false, is_main_character: false,
	is_leveraged: false, secret_count: 0, marginalia: [] as unknown[],
});
const ASSETS = [peerAsset(5, 'Lord Vane', TARGET), peerAsset(6, 'Dame Orr', PREPARER)];

function renderFor(
	seat: number,
	resolution: Record<string, unknown>,
	rollOutcome: 'make' | 'mar' | null = null,
	assets = ASSETS,
	/** Top-level resolution_data (make_mar_choices, choice_submitted, …) and the live-roll flag. */
	extra: { top?: Record<string, unknown>; rollActive?: boolean; plans?: unknown[]; rankings?: unknown[]; activeRoll?: unknown } = {},
): string {
	const plan = {
		id: 9, plan_type: 'exchange_courtiers', status: 'resolving', preparer_id: PREPARER,
		target_player_id: TARGET, target_asset_id: 5, preparation_notes: 'x',
		resolution_data: JSON.stringify({ exchange_courtiers: resolution, ...extra.top }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan, ...(extra.plans ?? [])] as never,
		assets: assets as never, players: PLAYERS as never, rankings: (extra.rankings ?? []) as never, currentPlayerID: seat,
		focusPlayerID: null, isFocusPlayer: false, rollActive: extra.rollActive ?? false, rollOutcome,
		activeRoll: ('activeRoll' in extra ? extra.activeRoll : rollOutcome ? { id: 4, plan_id: 9, status: 'resolved', result: 1, difficulty: 2 } : null) as never,
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
	// The seats that do not choose get no line about who does: the heading says it. (The
	// old lines were "Waiting for ravi to say how the trade went" and "Waiting for sol to
	// choose how to respond".) They do see the picker, greyed out, in the third person:
	// see "what a watcher sees of the picks" below.
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

	it('shows a seat that is not deciding neither of the buttons', () => {
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

// ── The difficulty before the dice, and the watcher's view of the picks ───────────────
//
// A seat that was not choosing used to get a line ("Waiting for sol to propose a fair
// trade") or, once the dice landed, nothing at all; and nobody saw how hard the roll
// was before the dice panel. The difficulty is the shared meter for everyone until the
// dice land, and each pick is shown to everyone else as the same picker, greyed out and
// inert, worded in the third person.

const MIRROR = /<fieldset[^>]*class="[^"]*\bresolve-mirror\b[^"]*"[^>]*disabled/;
const LIVE = /<fieldset[^>]*class="[^"]*\bresolve-mirror-wrap\b(?![^"]*\bresolve-mirror\b)[^"]*"/;
const rowsOf = (html: string) => [...html.matchAll(/<[^>]*role="checkbox"[^>]*>/g)].map(m => m[0]);

describe('the difficulty, before the dice', () => {
	const rankings = [{ id: 1, game_id: 1, category: 'power', player_id: TARGET, rank: 2 }];

	it('is the target\'s power status, for every seat, before the offer and while the roll is out', () => {
		for (const seat of [PREPARER, TARGET, BYSTANDER]) {
			const before = text(renderFor(seat, {}, null, ASSETS, { rankings }));
			expect(before).toContain('Difficulty 4');
			expect(before).toContain('sol’s status on the power track: 6 minus rank 2 is 4');
			const rolling = text(renderFor(seat, { fair_trade_asset_id: 6, fair_trade_accepted: false }, null, ASSETS, { rankings, rollActive: true }));
			expect(rolling).toContain('Difficulty 4');
		}
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) {
			expect(text(renderFor(BYSTANDER, { fair_trade_asset_id: 6, fair_trade_accepted: false }, o, ASSETS, { rankings }))).not.toContain('1 easy');
		}
	});

	it('does not come back when another plan\'s roll opens after this plan\'s choice is committed', () => {
		// `rollOutcome` follows whichever roll is on the table, so it is null again for the new one.
		const other = { id: 8, plan_id: 77, outcome: null, difficulty: 2, adjusted_difficulty: null };
		const t = text(renderFor(BYSTANDER, { fair_trade_asset_id: 6, fair_trade_accepted: false }, null, ASSETS, {
			rankings, top: { choice_submitted: true }, activeRoll: other,
		}));
		expect(t).not.toContain('1 easy');
		expect(t).not.toContain('What can happen?');
	});
});

describe('what a watcher sees of the picks', () => {
	const declined = { fair_trade_asset_id: 6, fair_trade_accepted: false };
	const note = (id: number, assetID: number, torn = false) => ({ id, asset_id: assetID, position: id, text: `note ${id}`, is_torn: torn });
	const withNotes = (assetID: number, notes: ReturnType<typeof note>[]) =>
		ASSETS.map(a => (a.id === assetID ? { ...a, marginalia: notes } : a));

	describe('the trade the target may name', () => {
		it.each([['the preparer', PREPARER], ['a bystander', BYSTANDER]] as const)('shows %s the preparer\'s peers, greyed, under the line', (_, seat) => {
			const body = renderFor(seat, {});
			expect(body).toMatch(MIRROR);
			const t = text(body);
			expect(t).toContain('Waiting for sol to propose a fair trade');
			expect(t).toContain('Peer to receive in trade');
			expect(t).toContain('Dame Orr');
			// Only the preparer's peers can be named: the target's own is the one being asked for.
			expect(body.match(/<div class="card /g)).toHaveLength(1);
			// Inert to the keyboard as well as to a tap: the cards say they are disabled.
			const rows = rowsOf(body);
			expect(rows.length).toBeGreaterThan(0);
			for (const r of rows) expect(r).toContain('aria-disabled="true"');
		});

		it('leaves the target the live picker, with its button', () => {
			const body = renderFor(TARGET, {});
			expect(body).not.toMatch(MIRROR);
			expect(text(body)).toContain('Propose trade');
		});

		it.each([['the preparer', PREPARER], ['a bystander', BYSTANDER]] as const)('addresses %s as no one: the line and the picker are in the third person', (_, seat) => {
			expect(text(renderFor(seat, {}))).not.toMatch(/\byou(r)?\b/i);
		});

		it('draws no picker when the preparer has no peer to name', () => {
			const t = renderFor(BYSTANDER, {}, null, ASSETS.filter(a => a.owner_id !== PREPARER));
			expect(t).not.toMatch(MIRROR);
		});
	});

	describe('the make: how the trade went', () => {
		it.each([['the target', TARGET], ['a bystander', BYSTANDER]] as const)('shows %s the three options, greyed, with no button', (_, seat) => {
			const body = renderFor(seat, declined, 'make');
			expect(body).toMatch(MIRROR);
			const t = text(body);
			for (const w of ['Messy', 'Legal', 'Conspiracy']) expect(t).toContain(w);
			expect(body).not.toContain('Apply choices');
		});

		it('speaks of the preparer, not "you", to anyone who is not choosing', () => {
			const t = text(renderFor(BYSTANDER, declined, 'make'));
			expect(t).toContain('ravi beat the difficulty');
			expect(t).toContain('transferred to ravi');
			expect(t).toContain('The target will break one of the preparer\'s assets');
			expect(t).not.toMatch(/\byou(r)?\b/i);
		});

		it('is still the live picker for the player who chooses', () => {
			const body = renderFor(PREPARER, declined, 'make');
			expect(body).not.toMatch(MIRROR);
			expect(body).toContain('Apply choices');
			expect(text(body)).toContain('The target will break one of your assets');
		});
	});

	describe('who the targeted peer goes to', () => {
		// Make Demands' keep-assets winner takes the incoming side of an exchange instead of the preparer.
		const keeper = (winner: number) => [{
			id: 12, plan_type: 'make_demands', status: 'resolved', result: 'make', preparer_id: BYSTANDER, targeted_plan_id: 9,
			resolution_data: JSON.stringify({ make_demands: { draft_choices: [{ option: 'keep_assets', player_id: winner }] } }),
		}];

		it('is the preparer when nobody holds keep-assets', () => {
			expect(text(renderFor(BYSTANDER, declined, 'make'))).toContain('transferred to ravi');
			expect(text(renderFor(PREPARER, declined, 'make'))).toContain('transferred to you');
		});

		it('is the keep-assets winner, in the actor\'s line and in a watcher\'s', () => {
			const demand = { plans: keeper(BYSTANDER) };
			const watcher = text(renderFor(TARGET, declined, 'make', ASSETS, demand));
			expect(watcher).toContain('transferred to mel');
			expect(watcher).not.toContain('transferred to ravi');
			const actor = text(renderFor(PREPARER, declined, 'make', ASSETS, demand));
			expect(actor).toContain('transferred to mel');
			expect(actor).not.toContain('transferred to you');
		});

		it('reads "you" for the winner themself, when they are the one looking', () => {
			const demand = { plans: keeper(BYSTANDER) };
			expect(text(renderFor(BYSTANDER, declined, 'make', ASSETS, demand))).toContain('transferred to you');
		});
	});

	describe('the mar: how to respond', () => {
		it.each([['the preparer', PREPARER], ['a bystander', BYSTANDER]] as const)('shows %s the three options, greyed, with the tiers the margin puts out of reach', (_, seat) => {
			const body = renderFor(seat, declined, 'mar');
			expect(body).toMatch(MIRROR);
			const t = text(body);
			for (const w of ['A Fair Trade', 'Riposte', 'Forfeit']) expect(t).toContain(w);
			// result 1 against difficulty 2: a margin of 1, so tiers 2 and 3 are out of reach.
			expect(t).toContain('needs −2');
			expect(t).toContain('needs −3');
			expect(body).not.toContain('Apply choices');
		});

		it('speaks of the target and the preparer, not "you"', () => {
			const t = text(renderFor(BYSTANDER, declined, 'mar'));
			// How far it marred, and no line about who responds: the open step's heading says.
			expect(t).toContain('ravi\'s plan was marred by 1.');
			expect(t).not.toContain('chooses how to respond');
			expect(t).toContain('The target takes the requested peer');
			expect(t).toContain('The preparer may break the requested peer first');
			expect(t).not.toMatch(/\byou(r)?\b/i);
		});

		it('is still the live picker for the target, who is the one choosing', () => {
			const body = renderFor(TARGET, declined, 'mar');
			expect(body).not.toMatch(MIRROR);
			expect(body).toContain('Apply choices');
			// The space before the dash is kept (a block body's leading space is trimmed).
			expect(text(body)).toContain('marred by 1 — choose how you respond.');
		});
	});

	describe('the messy break', () => {
		const messy = { ...declined, messy_break_required: true, messy_break_done: false };
		const MARKED = { top: { choice_submitted: true } };
		const assets = withNotes(6, [note(1, 6), note(2, 6)]);

		it.each([['the preparer', PREPARER], ['a bystander', BYSTANDER]] as const)('shows %s the preparer\'s marginalia, greyed, and no button', (_, seat) => {
			const body = renderFor(seat, messy, 'make', assets, MARKED);
			expect(body).toMatch(MIRROR);
			const t = text(body);
			// No line of its own: the open step's heading says who is breaking what.
			expect(t).toContain('Marginalium to break');
			expect(t).not.toContain('break one of');
			expect(body).not.toContain('Break marginalia');
			const rows = rowsOf(body);
			expect(rows.length).toBeGreaterThan(0);
			for (const r of rows) expect(r).toContain('aria-disabled="true"');
			expect(t).not.toMatch(/\byou(r)?\b/i);
		});

		it('leaves the target the live picker', () => {
			const body = renderFor(TARGET, messy, 'make', assets, MARKED);
			expect(body).not.toMatch(MIRROR);
			expect(body).toContain('Break marginalia');
		});
	});

	describe('the riposte', () => {
		const riposte = { ...declined, riposte_allowed: true };
		const MARKED = { top: { choice_submitted: true } };
		const assets = withNotes(6, [note(1, 6), note(2, 6)]);

		it.each([['the target', TARGET], ['a bystander', BYSTANDER]] as const)('shows %s the notes that could be torn, greyed, and no button', (_, seat) => {
			const body = renderFor(seat, riposte, 'mar', assets, MARKED);
			expect(body).toMatch(MIRROR);
			expect(text(body)).toContain('Marginalium to break');
			expect(body).not.toContain('Break, then surrender');
			expect(body).not.toContain('Surrender intact');
			for (const r of rowsOf(body)) expect(r).toContain('aria-disabled="true"');
			expect(text(body)).not.toMatch(/\byou(r)?\b/i);
		});

		it('draws no picker when the peer has no second note to tear', () => {
			const body = renderFor(BYSTANDER, riposte, 'mar', withNotes(6, [note(1, 6)]), MARKED);
			expect(body).not.toMatch(MIRROR);
		});

		it('leaves the preparer the live picker and buttons', () => {
			const body = renderFor(PREPARER, riposte, 'mar', assets, MARKED);
			expect(body).not.toMatch(MIRROR);
			expect(body).toContain('Break, then surrender');
		});
	});

	describe('nothing hidden, nothing operable', () => {
		it('every watcher of a pick reads the same page: nothing in it depends on who is looking', () => {
			const asMake = (seat: number) => text(renderFor(seat, declined, 'make'));
			expect(asMake(TARGET)).toBe(asMake(BYSTANDER));
			const asMar = (seat: number) => text(renderFor(seat, declined, 'mar'));
			expect(asMar(PREPARER)).toBe(asMar(BYSTANDER));
		});

		it('no mirror holds a text field or a submit button', () => {
			const cases = [
				renderFor(BYSTANDER, {}),
				renderFor(BYSTANDER, declined, 'make'),
				renderFor(BYSTANDER, declined, 'mar'),
				renderFor(BYSTANDER, { ...declined, messy_break_required: true }, 'make', withNotes(6, [note(1, 6), note(2, 6)]), { top: { choice_submitted: true } }),
			];
			for (const body of cases) {
				const m = body.match(/<fieldset[^>]*resolve-mirror[^>]*disabled[\s\S]*?<\/fieldset>/);
				expect(m, 'expected a mirror').not.toBeNull();
				expect(m![0]).not.toContain('<textarea');
				expect(m![0]).not.toMatch(/<button[^>]*class="[^"]*primary/);
			}
		});
	});
});
