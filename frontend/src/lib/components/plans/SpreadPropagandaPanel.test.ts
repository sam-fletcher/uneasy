import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import SpreadPropagandaPanel from './SpreadPropagandaPanel.svelte';
import type { PlanContext } from './types';
import { LIVE_WRAP, MIRROR } from './mirrorMatchers';

// The server records a mar committed with nothing picked as choice_submitted, with
// make_mar_choices empty. The panel must read that as "choices made" (done-state comes
// from the server), not show the picker again.

const PREPARER = 1;

function renderMar(resolution: Record<string, unknown>): string {
	const plan = {
		id: 9, plan_type: 'spread_propaganda', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'x', resolution_data: JSON.stringify(resolution),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: [],
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: 2, display_name: 'sol' }] as never,
		rankings: [], currentPlayerID: PREPARER, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: 'mar',
		activeRoll: { id: 4, plan_id: 9, status: 'resolved' } as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SpreadPropagandaPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Spread Propaganda mar after a zero-pick commit', () => {
	it('shows the picker while nothing is committed', () => {
		expect(renderMar({ spread_propaganda: { pre_roll_done: true } })).toContain('Apply choices');
	});

	it('moves on to the follow-up steps, not the picker', () => {
		const body = renderMar({ spread_propaganda: { pre_roll_done: true }, choice_submitted: true });
		expect(body).not.toContain('Apply choices');
		expect(body).toContain('Complete plan');
	});
});

// The options carried the book's (a)-(d) lettering, which meant something on the
// printed page and nothing on screen: nothing here refers to a letter, so it was a
// stray prefix on every line of the picker and the summary.
describe('Spread Propaganda mar option labels', () => {
	const strip = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ');

	it('are written in words, without the letters', () => {
		const t = strip(renderMar({ spread_propaganda: { pre_roll_done: true } }));
		expect(t).toContain('A peer leaves your retinue');
		expect(t).toContain('Keep your head down');
		expect(t).toContain('Word of your laughable ideas gets around');
		expect(t).toContain('The top interferer may spread their own propaganda now, or decline');
		expect(t).not.toMatch(/\([a-d]\) /);
	});
});

// ── The difficulty before the dice, and the watcher's view of the picks ───────────────
//
// Spread Propaganda showed a watcher nothing but the open step's heading while the
// preparer picked the mar's options, handed a peer over or broke their own asset, and
// showed no difficulty before the dice panel. Both are drawn the way Spread Rumors
// draws them: the meter for everyone until the dice land, and the actor's picker as a
// greyed, inert mirror for everyone else.

const OTHER = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();
const peer = (id: number, owner: number, name: string) => ({
	id, name, owner_id: owner, asset_type: 'peer', is_destroyed: false, is_main_character: false, is_leveraged: false,
	secret_count: 2,
	marginalia: [{ id: id * 10, asset_id: id, position: 1, text: 'sworn to the crown', is_torn: false }],
});

function renderSP(opts: {
	seat: number;
	outcome: 'make' | 'mar' | null;
	top?: Record<string, unknown>;
	sp?: Record<string, unknown>;
	assets?: unknown[];
}): string {
	const plan = {
		id: 9, plan_type: 'spread_propaganda', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'pamphlets', resolution_data: JSON.stringify({
			spread_propaganda: { pre_roll_done: true, ...opts.sp }, ...opts.top,
		}),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never,
		assets: (opts.assets ?? []) as never,
		players: [
			{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' },
		] as never,
		rankings: [{ id: 1, game_id: 1, category: 'esteem', player_id: PREPARER, rank: 3 }] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: false, rollOutcome: opts.outcome,
		activeRoll: opts.outcome == null ? null : { id: 4, plan_id: 9, status: 'resolved', result: 1, difficulty: 3 } as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(SpreadPropagandaPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

// What a mirror is: the picker inside a disabled fieldset with the mirror class, which
// the shared CSS (planPanel.css) turns into "the same controls, greyed, not yours".

describe('Spread Propaganda: the difficulty before the dice', () => {
	it('shows everyone how hard the roll is, and why, until the dice land', () => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) {
			const t = textOf(renderSP({ seat, outcome: null }));
			expect(t).toContain('Difficulty 3');
			expect(t).toContain('ravi’s rank on the esteem track: 3');
		}
	});

	it('is gone once the dice have landed', () => {
		expect(textOf(renderSP({ seat: OTHER, outcome: 'mar' }))).not.toContain('1 easy');
	});
});

// The mirror is for people who are not choosing, so nothing in it addresses them: the
// prompts say what the actor is doing, and no line speaks of "you" or "yours".
describe('Spread Propaganda: a watcher is never addressed', () => {
	const SECOND_PERSON = /\b(you|your|yours|yourself)\b/i;

	it('the mar picker says what the actor is doing, not what the viewer should do', () => {
		for (const seat of [OTHER, BYSTANDER]) {
			const t = textOf(renderSP({ seat, outcome: 'mar' }));
			expect(t).toContain('ravi picks options equal to (difficulty − result) (repeatable)');
			expect(t).not.toContain('Pick options equal to');
			expect(t).not.toContain('choose exactly');
			expect(t).toContain('nothing of the preparer’s left to break');
			expect(t).not.toMatch(SECOND_PERSON);
		}
	});

	it('says so for the actor in the second person: this is the player choosing', () => {
		const t = textOf(renderSP({ seat: PREPARER, outcome: 'mar' }));
		expect(t).toContain('Pick options equal to (difficulty − result) (repeatable)');
		expect(t).toContain('nothing of yours left to break');
	});

	it('the picks owed read as what the preparer is doing, in every sub-step', () => {
		for (const option of ['give_peer', 'break_self']) {
			const body = renderSP({
				seat: BYSTANDER, outcome: 'mar',
				top: { make_mar_choices: [{ option, player_id: PREPARER }], choice_submitted: true },
				assets: [peer(31, PREPARER, 'Lord Ashe')],
			});
			expect(textOf(body), option).not.toMatch(SECOND_PERSON);
		}
	});

	it('does not glue "decline" to the dash that opens a note (Svelte trims a span\'s leading space)', () => {
		// Strip tags WITHOUT putting a space where each was, or the trimmed space cannot be seen.
		const flat = renderSP({ seat: BYSTANDER, outcome: 'mar' }).replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, '');
		expect(flat).toContain('or decline — ');
		expect(flat).not.toMatch(/\w— /);
	});
});

describe('Spread Propaganda: the difficulty when another plan rolls', () => {
	it('does not come back when a counter-propaganda opens its roll while this plan owes steps', () => {
		// This plan's mar is committed; `rollOutcome` is null again for the counter-propaganda's new roll.
		const plan = {
			id: 9, plan_type: 'spread_propaganda', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'x',
			resolution_data: JSON.stringify({ spread_propaganda: { pre_roll_done: true }, choice_submitted: true }),
		};
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [] as never,
			players: [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }] as never,
			rankings: [{ id: 1, game_id: 1, category: 'esteem', player_id: PREPARER, rank: 3 }] as never,
			currentPlayerID: OTHER, focusPlayerID: null, isFocusPlayer: false,
			rollActive: true, rollOutcome: null,
			activeRoll: { id: 8, plan_id: 77, outcome: null, difficulty: 2, adjusted_difficulty: null } as never,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		const t = textOf(render(SpreadPropagandaPanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body);
		expect(t).not.toContain('1 easy');
	});
});

describe('Spread Propaganda: what a watcher sees of the mar picks', () => {
	it('is the same picker, greyed out and inert, for everyone who is not choosing', () => {
		for (const seat of [OTHER, BYSTANDER]) {
			const body = renderSP({ seat, outcome: 'mar' });
			expect(body).toMatch(MIRROR);
			const t = textOf(body);
			expect(t).toContain('Result: Mar');
			expect(t).toContain('ravi picks options equal to (difficulty − result)');
			expect(t).toContain('Total picks: 0');
			// Every option is on offer, worded for a bystander ("the preparer's", not "your").
			expect(t).toContain('A peer leaves the preparer\'s retinue');
			expect(t).toContain('The preparer keeps their head down');
			expect(t).not.toContain('your retinue');
		}
	});

	it('has no button to commit anything', () => {
		const body = renderSP({ seat: OTHER, outcome: 'mar' });
		expect(body).not.toContain('Apply choices');
		expect(body).not.toContain('last step');
	});

	it('is still the live picker, with its button, for the player who chooses', () => {
		const body = renderSP({ seat: PREPARER, outcome: 'mar' });
		expect(body).toMatch(LIVE_WRAP);
		expect(body).not.toMatch(MIRROR);
		expect(body).toContain('Apply choices');
		expect(textOf(body)).toContain('A peer leaves your retinue');
	});

	it('shows nothing a watcher could not already see: every count is zero, whatever the actor has ticked', () => {
		// The actor's uncommitted ticks are local state and are never sent anywhere (a
		// resolution draft channel is not built), so a mirror can only ever show zeros.
		const body = renderSP({ seat: BYSTANDER, outcome: 'mar' });
		const counts = [...body.matchAll(/class="stepper-count[^"]*"[^>]*>(\d+)</g)].map(m => m[1]);
		expect(counts).toEqual(['0', '0', '0', '0']);
	});

	it('is the same for every watcher: nothing in it depends on who is looking', () => {
		expect(textOf(renderSP({ seat: OTHER, outcome: 'mar' }))).toBe(textOf(renderSP({ seat: BYSTANDER, outcome: 'mar' })));
	});
});

describe('Spread Propaganda: a watcher sees the picks the preparer still owes', () => {
	const owed = (option: string) => ({
		top: { make_mar_choices: [{ option, player_id: PREPARER }], choice_submitted: true },
		assets: [peer(31, PREPARER, 'Lord Ashe'), peer(32, OTHER, 'Dame Orrin')],
	});

	it('shows a peer being handed over: the preparer\'s peers and who may receive them, inert', () => {
		const body = renderSP({ seat: BYSTANDER, outcome: 'mar', ...owed('give_peer') });
		expect(body).toMatch(MIRROR);
		const t = textOf(body);
		expect(t).toContain('A peer leaves ravi’s retinue'.replace('’', "'"));
		expect(t).toContain('Lord Ashe');
		// Not the other player's peer: it is not the preparer's to give.
		expect(t).not.toContain('Dame Orrin');
		// The player chips are the preparer's possible recipients, and read-only.
		expect(body).toMatch(/<button[^>]*disabled[^>]*>[\s\S]*?sol/);
		expect(body).not.toContain('Give up peer');
		expect(body).not.toContain('Skip — no valid targets');
	});

	it('shows an asset being broken: the preparer\'s own, rows out of the tab order', () => {
		const body = renderSP({ seat: BYSTANDER, outcome: 'mar', ...owed('break_self') });
		expect(body).toMatch(MIRROR);
		const t = textOf(body);
		expect(t).toContain('ravi breaks one of their own assets');
		expect(t).toContain('Lord Ashe');
		expect(t).not.toContain('Dame Orrin');
		expect(body).not.toContain('Break asset');
		const rows = [...body.matchAll(/<[^>]*role="checkbox"[^>]*>/g)].map(m => m[0]);
		expect(rows.length).toBeGreaterThan(0);
		for (const r of rows) expect(r).toContain('aria-disabled="true"');
	});

	it('does not repeat "yourself" to a bystander', () => {
		const t = textOf(renderSP({ seat: BYSTANDER, outcome: 'mar', ...owed('break_self') }));
		expect(t).not.toContain('Break yourself');
	});

	it('leaves the preparer\'s own steps live, with their buttons', () => {
		const give = renderSP({ seat: PREPARER, outcome: 'mar', ...owed('give_peer') });
		expect(give).toMatch(LIVE_WRAP);
		expect(give).toContain('Give up peer');
		const brk = renderSP({ seat: PREPARER, outcome: 'mar', ...owed('break_self') });
		expect(brk).toContain('Break asset');
		expect(textOf(brk)).toContain('Break yourself — break one of your assets');
	});

	it('is the same for every watcher, and built from public assets only', () => {
		// Assets carry how many secrets they hide, never what they say (that is a
		// per-viewer record this reads nothing of), so two watchers read the same page.
		const a = textOf(renderSP({ seat: OTHER, outcome: 'mar', ...owed('give_peer') }));
		const b = textOf(renderSP({ seat: BYSTANDER, outcome: 'mar', ...owed('give_peer') }));
		expect(a).toBe(b);
	});
});
