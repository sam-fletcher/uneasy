import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import type { PlanContext } from './types';
import { MIRROR, mirrorOf } from './mirrorMatchers';

// A Make Demands perform_steps win hands the whole resolution to the winner and
// locks the preparer out (the server 403s them). The arrival form must follow
// the winner, or a winner-driven plan shows no way forward.
//
// A seat that is not acting gets no form and no line about who is: the open step's
// heading names who the table waits on and the step ("Waiting on sol", "Describe
// each arrival", from plans/stepLabels). The lines this panel used to print for it
// ("sol is welcoming the new peers…", "ravi is naming the new peers…", "… is
// resolving the marred introductions…") must stay gone.

const PREPARER = 1;
const WINNER = 2;

type Branch = {
	rollOutcome: 'make' | 'mar' | null; mi: Record<string, unknown>; choices?: unknown[];
	/** Top-level resolution_data markers (choice_auto_applied, …). */
	marker?: Record<string, unknown>;
	rollActive?: boolean;
	/** Skip the Make Demands stand-in, so the preparer is the actor. */
	noWinner?: boolean;
};

const MAKE: Branch = {
	rollOutcome: 'make',
	mi: { peer_count: 1, make_pending: true, drafts: [{ id: 'd1', name: 'Lady Vane' }] },
	choices: [{ option: 'peers_arrive', player_id: PREPARER }],
	marker: { choice_auto_applied: true },
};
const NAMING: Branch = { rollOutcome: null, mi: { peer_count: 1 } };
const MAR: Branch = {
	rollOutcome: 'mar',
	mi: { peer_count: 1, mar_pending: true, drafts: [{ id: 'd1', name: 'Lady Vane' }] },
	// The mar commits with no picks: make_mar_choices stays empty and the
	// auto-apply marker is the only record that the outcome was applied.
	marker: { choice_auto_applied: true },
};

function renderFor(currentPlayerID: number, branch: Branch = MAKE): string {
	const target = {
		id: 9, plan_type: 'make_introductions', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x',
		resolution_data: JSON.stringify({
			make_introductions: branch.mi,
			make_mar_choices: branch.choices ?? [],
			...branch.marker,
		}),
	};
	const demand = {
		id: 12, plan_type: 'make_demands', status: 'resolved', result: 'make',
		preparer_id: WINNER, targeted_plan_id: 9,
		resolution_data: JSON.stringify({
			make_demands: { draft_choices: [{ option: 'perform_steps', player_id: WINNER }] },
		}),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null,
		plans: (branch.noWinner ? [target] : [target, demand]) as never,
		assets: [],
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: WINNER, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID, focusPlayerID: null, isFocusPlayer: false,
		rollActive: branch.rollActive ?? false, rollOutcome: branch.rollOutcome, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(MakeIntroductionsPanel, { props: { ctx, plan: target as never, mode: 'resolve' } }).body;
}

describe('MakeIntroductionsPanel with a perform_steps winner', () => {
	it('gives the arrival form to the winner', () => {
		expect(renderFor(WINNER)).toContain('They arrive');
	});

	it('shows the preparer no form, and no line about the winner', () => {
		const body = renderFor(PREPARER);
		expect(body).not.toContain('They arrive');
		expect(body).not.toContain('welcoming the new peers');
	});
});

describe('MakeIntroductionsPanel pre-roll naming with a perform_steps winner', () => {
	it('keeps the naming form with the preparer', () => {
		expect(renderFor(PREPARER, NAMING)).toContain('Name peers &amp; roll');
	});

	it('shows the winner no form, and no line about the preparer', () => {
		const body = renderFor(WINNER, NAMING);
		expect(body).not.toContain('Name peers');
		expect(body).not.toContain('naming the new peers');
	});
});

describe('MakeIntroductionsPanel mar fates with a perform_steps winner', () => {
	it('gives the resolve form to the winner', () => {
		expect(renderFor(WINNER, MAR)).toContain('Resolve peer');
	});

	it('shows the preparer no form, and no line about the winner', () => {
		const body = renderFor(PREPARER, MAR);
		expect(body).not.toContain('Resolve peer');
		expect(body).not.toContain('resolving the marred introductions');
	});
});

// choicesDone reads the committed marker (not the picked options): the mar
// commits with no picks, so these pin what each viewer sees on both sides of
// the commit, and that the mar needs no `mar_pending` crutch to look committed.
describe('MakeIntroductionsPanel commit marker', () => {
	const DRAFTS = [{ id: 'd1', name: 'Lady Vane' }, { id: 'd2', name: 'Sir Pell' }];
	const MAR_PENDING = { peer_count: 2, mar_pending: true, drafts: DRAFTS };
	const marAs = (id: number, extra: Partial<Branch> = {}) =>
		renderFor(id, { ...MAR, mi: MAR_PENDING, noWinner: true, ...extra });

	describe('committed (the dice have landed and the server applied the outcome)', () => {
		it('mar, actor: the fate form, no dice wait or difficulty preview', () => {
			const body = marAs(PREPARER);
			expect(body).toContain('Resolve peer');
			expect(body).not.toContain('Dice roll in progress');
			expect(body).not.toContain('plus 1 for each');
			expect(body).not.toContain('What can happen');
		});

		it('mar, actor, roll still on screen: still the fate form, not the dice wait', () => {
			const body = marAs(PREPARER, { rollActive: true });
			expect(body).toContain('Resolve peer');
			expect(body).not.toContain('Dice roll in progress');
		});

		it('mar, bystander: no form, no line about the actor, no dice wait', () => {
			const body = marAs(WINNER);
			expect(body).not.toContain('resolving the marred introductions');
			expect(body).not.toContain('Resolve peer');
			expect(body).not.toContain('Dice roll in progress');
		});

		it('mar, bystander, roll still on screen: the same', () => {
			const body = marAs(WINNER, { rollActive: true });
			expect(body).not.toContain('resolving the marred introductions');
			expect(body).not.toContain('Dice roll in progress');
		});

		it('mar, the author of a peer: the marginalia form', () => {
			const body = marAs(WINNER, {
				mi: {
					...MAR_PENDING,
					mar_outcomes: [{ draft_id: 'd1', outcome: 'other_retinue', author_player_id: WINNER, done: false }],
				},
			});
			expect(body).toContain('Write marginalia');
			expect(body).toContain('joining your retinue');
		});

		it('make, actor: the arrival form, no dice wait', () => {
			const body = renderFor(PREPARER, { ...MAKE, noWinner: true, rollActive: true });
			expect(body).toContain('They arrive');
			expect(body).not.toContain('Dice roll in progress');
		});

		it('make, bystander: no form, and no line about the actor', () => {
			const body = renderFor(WINNER, { ...MAKE, noWinner: true });
			expect(body).not.toContain('welcoming the new peers');
			expect(body).not.toContain('They arrive');
		});
	});

	describe('not committed (dice rolling or just landed, nothing applied yet)', () => {
		const DICE = { choices: [], marker: {} };

		it('mar landing, roll on screen: no fate form, and no dice wait (the roll has its own heading)', () => {
			for (const id of [PREPARER, WINNER]) {
				const body = renderFor(id, { ...MAR, ...DICE, mi: { peer_count: 2, drafts: DRAFTS }, rollActive: true, noWinner: true });
				expect(body).not.toContain('Dice roll in progress');
				expect(body).not.toContain('Resolve peer');
			}
		});

		it('make landing, roll on screen: no arrival form, and no dice wait', () => {
			const body = renderFor(PREPARER, {
				...MAKE, ...DICE, mi: { peer_count: 1, drafts: [DRAFTS[0]] }, rollActive: true, noWinner: true,
			});
			expect(body).not.toContain('Dice roll in progress');
			expect(body).not.toContain('They arrive');
		});

		it('mar outcome known, roll gone, nothing applied: no form, no line about the actor', () => {
			const body = renderFor(WINNER, { ...MAR, ...DICE, mi: { peer_count: 2, drafts: DRAFTS }, noWinner: true });
			expect(body).not.toContain('resolving the marred introductions');
			expect(body).not.toContain('Resolve peer');
			expect(body).not.toContain('Dice roll in progress');
		});

		it('before the roll: the difficulty and what the roll can bring', () => {
			const body = renderFor(PREPARER, {
				rollOutcome: null, mi: { peer_count: 2, drafts: DRAFTS }, noWinner: true,
			});
			expect(body).toContain('plus 1 for each');
			expect(body).toContain('What can happen');
		});
	});

	it('a legacy-shaped plan with picks but no marker still counts as committed', () => {
		const body = renderFor(PREPARER, { ...MAKE, marker: {}, noWinner: true, rollActive: true });
		expect(body).toContain('They arrive');
	});
});

// ── The difficulty before the dice, and the watcher's view of the mar's fates ─────────
//
// The meter was already here; it is the shared one now (the same words), and what a
// player who is not settling a newcomer saw of the mar's fates was nothing at all. They
// now see the four fates, greyed out and inert, with the ones already handed out struck
// through, as the actor does. (The naming, the arrivals and the marginalia are text the
// player writes, not a pick: they are not mirrored, and nothing is streamed.)

const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

describe('Make Introductions: the difficulty, from the shared meter', () => {
	it('is 2 plus a peer each, for the preparer and for everyone else, before the dice', () => {
		for (const seat of [PREPARER, WINNER]) {
			const t = textOf(renderFor(seat, { rollOutcome: null, mi: { peer_count: 2 }, noWinner: true }));
			expect(t).toContain('Difficulty 4');
			expect(t).toContain('2, plus 1 for each of the 2 peers introduced.');
			expect(t).toContain('1 easy');
		}
	});

	it('is gone once the dice have landed', () => {
		expect(textOf(renderFor(WINNER, MAR))).not.toContain('1 easy');
		expect(textOf(renderFor(WINNER, MAKE))).not.toContain('1 easy');
	});
});

describe('Make Introductions: what a watcher sees of the mar fates', () => {
	const TWO: Branch = {
		rollOutcome: 'mar',
		mi: {
			peer_count: 2, mar_pending: true,
			drafts: [{ id: 'd1', name: 'Lady Vane' }, { id: 'd2', name: 'Ser Oak' }],
			mar_outcomes: [{ draft_id: 'd1', outcome: 'delayed', done: true }],
		},
		marker: { choice_auto_applied: true },
		noWinner: true,
	};

	it('is the four fates, greyed and inert, with the one already handed out struck through', () => {
		const body = renderFor(WINNER, TWO);
		expect(body).toMatch(MIRROR);
		const t = textOf(body);
		for (const w of ['Another retinue', 'Not who you thought', 'Delayed', 'Broken journey']) expect(t).toContain(w);
		expect(t).toContain('Ser Oak');
		expect(t).toContain('1 remaining');
		// Delayed was Lady Vane's, so it reads as taken.
		expect(body).toMatch(/<span class="[^"]*\bstruck\b[^"]*">Delayed/);
		expect(body).not.toMatch(/<span class="[^"]*\bstruck\b[^"]*">Another retinue/);
	});

	it('shows none of the fates as picked: the actor\'s choice is not sent to anyone', () => {
		const body = renderFor(WINNER, TWO);
		const mirror = mirrorOf(body);
		const chips = [...mirror.matchAll(/<button[^>]*class="chip[^"]*"[^>]*>/g)].map(m => m[0]);
		expect(chips).toHaveLength(4);
		for (const c of chips) expect(c).toContain('aria-pressed="false"');
	});

	it('has no button, no field and no sub-form: nothing to commit, nothing typed', () => {
		const body = renderFor(WINNER, TWO);
		const mirror = mirrorOf(body);
		expect(mirror).not.toContain('Resolve peer');
		expect(mirror).not.toContain('<textarea');
		expect(mirror).not.toContain('Joins which retinue');
		expect(body).not.toContain('Resolve peer');
	});

	it('speaks of no one in the second person, and is the same page for every watcher', () => {
		const a = textOf(renderFor(WINNER, TWO));
		// (The fate named "Not who you thought" is the book's own wording, and the name the
		// Buffet and the recap already use, so it is the one "you" allowed.)
		expect(a.replace('Not who you thought', '')).not.toMatch(/\byou(r)?\b/i);
		// A second watcher: with a perform_steps winner, the preparer is a watcher too.
		const b = textOf(renderFor(PREPARER, { ...TWO, noWinner: false }));
		expect(b).toContain('Ser Oak');
		expect(b).toMatch(/1 remaining/);
	});

	it('is still the live form, with its button, for the player settling the newcomers', () => {
		const body = renderFor(PREPARER, TWO);
		expect(body).not.toMatch(MIRROR);
		expect(body).toContain('Resolve peer');
	});

	it('is gone once every newcomer has a fate', () => {
		const done: Branch = {
			...TWO,
			mi: { ...TWO.mi, mar_outcomes: [
				{ draft_id: 'd1', outcome: 'delayed', done: true },
				{ draft_id: 'd2', outcome: 'broken_journey', done: true },
			] },
		};
		expect(renderFor(WINNER, done)).not.toMatch(MIRROR);
	});
});
