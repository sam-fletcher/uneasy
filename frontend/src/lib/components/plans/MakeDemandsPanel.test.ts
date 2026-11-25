import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import MakeDemandsPanel from './MakeDemandsPanel.svelte';
import type { PlanContext } from './types';
import { MIRROR, mirrorOf } from './mirrorMatchers';

// A demand's difficulty used to be the dice panel's bare figure, and its four options were
// described only once the draft began. Before the dice land everyone now sees how hard
// the roll is and why (the target plan's own difficulty, plus the gap in power), and a
// "What can happen" that says what each option does against this target.

const ADA = 1; // demands
const BEA = 2; // prepared the target plan
const CYR = 3; // bystander
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

function renderMD(opts: {
	seat: number; outcome?: 'make' | 'mar' | null; rolled?: boolean;
	/** The draft picks so far, in order. */
	draft?: { option: string; player_id: number }[];
}): string {
	const target = {
		id: 20, plan_type: 'seek_answers', status: 'pending', preparer_id: BEA, row_number: 5, resolution_data: null,
	};
	const demand = {
		id: 21, plan_type: 'make_demands', status: 'resolving', preparer_id: ADA, targeted_plan_id: 20,
		row_number: 5, preparation_notes: 'That plan is mine to direct.',
		resolution_data: opts.draft ? JSON.stringify({ make_demands: { draft_choices: opts.draft } }) : null,
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [target, demand] as never, assets: [] as never,
		players: [{ id: ADA, display_name: 'ada' }, { id: BEA, display_name: 'bea' }, { id: CYR, display_name: 'cyr' }] as never,
		rankings: [
			{ id: 1, game_id: 1, category: 'knowledge', player_id: BEA, rank: 2 },
			{ id: 2, game_id: 1, category: 'power', player_id: BEA, rank: 1 },
			{ id: 3, game_id: 1, category: 'power', player_id: ADA, rank: 4 },
		] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: opts.outcome == null, rollOutcome: opts.outcome ?? null,
		// A demand rolls the moment it starts resolving, at the difficulty the server worked out.
		activeRoll: (opts.rolled === false ? null : { id: 4, plan_id: 21, difficulty: 5, adjusted_difficulty: null, result: null }) as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(MakeDemandsPanel, { props: { ctx, plan: demand as never, mode: 'resolve' } }).body;
}

describe('Make Demands: the difficulty before the dice', () => {
	it('shows everyone how hard the roll is, and why: the target plan\'s difficulty plus the power gap', () => {
		for (const seat of [ADA, BEA, CYR]) {
			const t = textOf(renderMD({ seat }));
			expect(t).toContain('Difficulty 5');
			expect(t).toContain('Seek Answers is difficulty 2; bea outranks ada on power by 3, which adds 3.');
			expect(t).toContain('1 easy');
		}
	});

	it('is the roll\'s own value once it exists, not the client\'s sum', () => {
		// A roll made at 6 shows 6, whatever the rankings add up to.
		const t = textOf(renderMD({ seat: CYR }));
		expect(t).toContain('Difficulty 5');
	});

	it('is gone once the dice have landed', () => {
		for (const o of ['make', 'mar'] as const) expect(textOf(renderMD({ seat: CYR, outcome: o }))).not.toContain('1 easy');
	});

	it('says what the demand targets in the preparation line, not as a lead line in the open step', () => {
		const t = textOf(renderMD({ seat: CYR })).replace(/\s+'s/g, "'s");
		expect(t).not.toContain('Targeting');
		const at = t.search(/prepared Make Demands against bea's Seek Answers \(row \d+\)/);
		expect(at, 'the preparation line carries the target').toBeGreaterThan(-1);
		expect(at).toBeLessThan(t.indexOf('Difficulty 5'));
	});
});

describe('Make Demands: "What can happen"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen/;

	it('is a collapsed row for everyone before the dice land', () => {
		for (const seat of [ADA, BEA, CYR]) expect(renderMD({ seat })).toMatch(toggle);
	});

	it('is gone once the dice have landed: the draft\'s cards carry the same descriptions', () => {
		for (const o of ['make', 'mar'] as const) expect(renderMD({ seat: CYR, outcome: o })).not.toContain('What can happen');
	});
});

// ── The draft, as a watcher sees it ───────────────────────────────────────────────────
//
// The next player to pick saw four option cards; everyone else saw the heading and the
// finished picks. They now see the same board, greyed and inert, in the third person:
// the cards still on offer with what each does against this target, and the taken ones
// marked with whoever holds them. Everything on it is public (the draft's picks are in
// the plan's resolution_data, and the target plan is on the record).
//
// With ada the demander at power rank 4 and bea the target's preparer at rank 1, bea
// picks first, then ada, alternating.

describe('Make Demands: the draft, as a watcher sees it', () => {
	const make = (extra: Partial<Parameters<typeof renderMD>[0]> = {}) => ({ outcome: 'make' as const, ...extra });
	const PICK_OPTION = 'Pick option';

	it('shows a bystander all four cards, greyed, with what each does against this target', () => {
		const body = renderMD({ seat: CYR, ...make({ draft: [] }) });
		expect(body).toMatch(MIRROR);
		const t = textOf(mirrorOf(body));
		for (const title of ['Control leverage', 'Keep or change target', 'Keep assets', 'Perform steps']) expect(t).toContain(title);
		// The same per-target lines the actor's cards carry (and the "What can happen" before the dice).
		expect(t).toContain('Choose how many of bea’s assets go onto their roll.');
		expect(t).toContain('No effect here: Seek Answers isn’t aimed at anything a demand could redirect.');
		expect(t).toContain('No effect here: Seek Answers gives its preparer no assets to keep.');
		expect(t).toContain('Pick the make or mar options for bea once they have rolled.');
	});

	it('shows the other drafter the mirror too, when it is not their pick', () => {
		// bea picks first, so the demander ada is a watcher until bea has picked.
		expect(renderMD({ seat: ADA, ...make({ draft: [] }) })).toMatch(MIRROR);
		expect(renderMD({ seat: BEA, ...make({ draft: [] }) })).not.toMatch(MIRROR);
		const afterOne = [{ option: 'perform_steps', player_id: BEA }];
		expect(renderMD({ seat: BEA, ...make({ draft: afterOne }) })).toMatch(MIRROR);
		expect(renderMD({ seat: ADA, ...make({ draft: afterOne }) })).not.toMatch(MIRROR);
	});

	it('is inert: every control in it is disabled, and nothing in it can be submitted', () => {
		for (const seat of [CYR, ADA]) {
			const body = renderMD({ seat, ...make({ draft: [] }) });
			const mirror = mirrorOf(body);
			const buttons = [...mirror.matchAll(/<button\b[^>]*>/g)].map(m => m[0]);
			expect(buttons).toHaveLength(4);
			// A native `disabled` takes a card out of the tab order and out of a tap, and the
			// fieldset it sits in disables every control under it as well.
			for (const b of buttons) {
				expect(b).toMatch(/\bdisabled\b/);
				expect(b).not.toMatch(/tabindex="0"/);
			}
			expect(mirror).not.toMatch(/<(input|textarea|select|a)\b/);
			expect(body).not.toContain(PICK_OPTION);
		}
	});

	it('speaks in the third person: no "you" reaches a watcher', () => {
		for (const draft of [[], [{ option: 'perform_steps', player_id: BEA }]]) {
			for (const seat of [CYR, draft.length === 0 ? ADA : BEA]) {
				expect(textOf(renderMD({ seat, ...make({ draft }) }))).not.toMatch(/\byou(r)?\b/i);
			}
		}
	});

	it('marks a taken option with whoever holds it, and keeps the finished-pick line', () => {
		const body = renderMD({ seat: CYR, ...make({ draft: [{ option: 'perform_steps', player_id: BEA }] }) });
		const mirror = mirrorOf(body);
		const t = textOf(mirror);
		expect(t).toContain('Perform steps');
		expect(t).toContain('Taken by bea');
		// The other three are still on offer, with their lines.
		expect(t).toContain('Choose how many of bea’s assets go onto their roll.');
		expect(t).not.toContain('Pick the make or mar options');
		// The card says it is picked (the tick's own label), and only that one is.
		expect(mirror.match(/aria-pressed="true"/g)).toHaveLength(1);
		// The committed pick stays outside the mirror, in full colour, as it was.
		expect(textOf(body.replace(mirror, ''))).toContain('bea picked Perform steps');
	});

	it('says in neutral words that nothing left does anything, and does not tell a watcher to pick', () => {
		// Control leverage and Perform steps taken: Keep or change target and Keep assets
		// are all that is left, and both are duds against Seek Answers.
		const draft = [{ option: 'control_leverage', player_id: BEA }, { option: 'perform_steps', player_id: ADA }];
		const t = textOf(renderMD({ seat: CYR, ...make({ draft }) }));
		expect(t).toContain("None of what's left does anything against this plan.");
		expect(t).not.toContain('pick whichever');
		// The next picker is still told to take whichever.
		expect(textOf(renderMD({ seat: BEA, ...make({ draft }) }))).toContain('pick whichever');
	});

	it('is gone once the draft is complete: the committed winners are all there is', () => {
		const draft = [
			{ option: 'control_leverage', player_id: BEA }, { option: 'perform_steps', player_id: ADA },
			{ option: 'keep_assets', player_id: BEA }, { option: 'keep_or_change_target', player_id: ADA },
		];
		for (const seat of [ADA, BEA, CYR]) {
			const body = renderMD({ seat, ...make({ draft }) });
			expect(body).not.toMatch(MIRROR);
			expect(textOf(body)).toContain('Draft complete');
		}
	});

	it('is not drawn before the dice land, or on a mar', () => {
		expect(renderMD({ seat: CYR })).not.toMatch(MIRROR);
		expect(renderMD({ seat: CYR, outcome: 'mar' })).not.toMatch(MIRROR);
	});

	it('shows the same page to two watchers: nothing in it is per viewer', () => {
		const draft = [{ option: 'perform_steps', player_id: BEA }];
		const a = renderMD({ seat: CYR, ...make({ draft }) });
		// ada is next, so a third watcher is needed: render cyr's page against another bystander id.
		const b = renderMD({ seat: 99, ...make({ draft }) });
		expect(mirrorOf(a)).toBe(mirrorOf(b));
	});
});
