import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ProposeDecreePanel from './ProposeDecreePanel.svelte';
import type { PlanContext } from './types';
import { MIRROR, mirrorsOf } from './mirrorMatchers';

// Propose Decree showed no difficulty until the dice panel, and nothing said what a
// make or a mar of a law would do. Before the dice land (the council meeting, and the
// roll itself) everyone now sees the difficulty (the preparer's power rank) and a
// "What can happen" from the book's make and mar.

const PREPARER = 1;
const OTHER = 2;
const BYSTANDER = 3;
const textOf = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

function renderPD(opts: { seat: number; phase: 'council' | 'rolling' | 'passed'; outcome?: 'make' | 'mar' | null }): string {
	const pd: Record<string, unknown> = {
		signatory_id: PREPARER, signatory_player_ids: [PREPARER, OTHER], debate_started: true, law_text: 'No private armies.',
	};
	if (opts.phase === 'passed') pd.outcome = opts.outcome ?? 'make';
	const plan = {
		id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: PREPARER,
		preparation_notes: 'No private armies.', resolution_data: JSON.stringify({ propose_decree: pd }),
	};
	const ctx = {
		gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: [] as never,
		players: [{ id: PREPARER, display_name: 'ravi' }, { id: OTHER, display_name: 'sol' }, { id: BYSTANDER, display_name: 'mei' }] as never,
		rankings: [
			{ id: 1, game_id: 1, category: 'power', player_id: PREPARER, rank: 3 },
			{ id: 2, game_id: 1, category: 'power', player_id: OTHER, rank: 2 },
			{ id: 3, game_id: 1, category: 'power', player_id: BYSTANDER, rank: 4 },
		] as never,
		currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
		rollActive: opts.phase === 'rolling', rollOutcome: opts.phase === 'passed' ? (opts.outcome ?? 'make') : null,
		activeRoll: (opts.phase === 'rolling' ? { id: 4, plan_id: 9, difficulty: 3, adjusted_difficulty: null, result: null } : null) as never,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
	} as PlanContext;
	return render(ProposeDecreePanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
}

describe('Propose Decree: the difficulty before the dice', () => {
	it.each(['council', 'rolling'] as const)('shows everyone the preparer\'s power rank in the %s step', (phase) => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) {
			const t = textOf(renderPD({ seat, phase }));
			expect(t).toContain('Difficulty 3');
			expect(t).toContain('ravi’s rank on the power track: 3');
		}
	});

	it('is gone once the dice have landed and the law is being written', () => {
		for (const o of ['make', 'mar'] as const) {
			expect(textOf(renderPD({ seat: BYSTANDER, phase: 'passed', outcome: o }))).not.toContain('1 easy');
		}
	});
});

describe('Propose Decree: "What can happen"', () => {
	const toggle = /<button[^>]*aria-expanded="false"[^>]*>[\s\S]*?What can happen/;

	it.each(['council', 'rolling'] as const)('is a collapsed row for everyone in the %s step', (phase) => {
		for (const seat of [PREPARER, OTHER, BYSTANDER]) expect(renderPD({ seat, phase })).toMatch(toggle);
	});

	it('is gone once the dice have landed: the law-writing steps say what is happening', () => {
		for (const o of ['make', 'mar'] as const) expect(renderPD({ seat: OTHER, phase: 'passed', outcome: o })).not.toContain('What can happen');
	});
});

// ── Joining the council, as a watcher sees it ─────────────────────────────────────────
//
// Each player ranked below the preparer decides for themself: leverage one asset to join,
// or decline. Only they saw a picker; everyone else (the preparer who signs, the players
// already seated, a player who has decided) saw a line, or nothing. They now see each
// pending decider's picker, greyed out and inert, in the third person. The cards are that
// player's own un-leveraged assets, which every client already holds (existence and
// leverage are public; nothing per viewer is used). The amendment and the addendum stay
// lines: a watcher is shown no live draft of a resolution step, so a mirror of those text
// boxes could only ever be empty.

describe('Propose Decree: joining the council, as a watcher sees it', () => {
	const RAVI = 1; // preparer, signs
	const SOL = 2;
	const MEI = 3;
	const rowsOf = (html: string) => [...html.matchAll(/<[^>]*role="checkbox"[^>]*>/g)].map(m => m[0]);
	const asset = (id: number, name: string, owner_id: number, extra: Record<string, unknown> = {}) => ({
		id, name, owner_id, asset_type: 'resource', is_destroyed: false, is_main_character: false,
		is_leveraged: false, secret_count: 0, marginalia: [] as unknown[], ...extra,
	});
	const ASSETS = [
		asset(10, 'The Old Charter', RAVI),
		asset(20, 'Sol Hold', SOL), asset(21, 'Sol Seal', SOL, { is_leveraged: true }),
		asset(30, 'Mei Ledger', MEI), asset(31, 'Mei Cloak', MEI), asset(32, 'Mei Wreck', MEI, { is_destroyed: true }),
	];

	function renderCouncil(opts: {
		seat: number | null;
		declined?: number[];
		council?: number[];
		debateStarted?: boolean;
		rolling?: boolean;
		assets?: unknown[];
	}): string {
		const pd = {
			signatory_id: RAVI, signatory_player_ids: opts.council ?? [RAVI], declined_player_ids: opts.declined ?? [],
			debate_started: opts.debateStarted ?? true, law_text: 'No private armies.',
		};
		const plan = {
			id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: RAVI,
			preparation_notes: 'No private armies.', resolution_data: JSON.stringify({ propose_decree: pd }),
		};
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: (opts.assets ?? ASSETS) as never,
			players: [{ id: RAVI, display_name: 'ravi' }, { id: SOL, display_name: 'sol' }, { id: MEI, display_name: 'mei' }] as never,
			rankings: [
				{ id: 1, game_id: 1, category: 'power', player_id: RAVI, rank: 1 },
				{ id: 2, game_id: 1, category: 'power', player_id: SOL, rank: 2 },
				{ id: 3, game_id: 1, category: 'power', player_id: MEI, rank: 3 },
			] as never,
			currentPlayerID: opts.seat, focusPlayerID: null, isFocusPlayer: false,
			rollActive: opts.rolling ?? false, rollOutcome: null,
			activeRoll: (opts.rolling ? { id: 4, plan_id: 9, difficulty: 1, adjusted_difficulty: null, result: null } : null) as never,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		return render(ProposeDecreePanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
	}

	it('shows the preparer each pending decider\'s picker, greyed, with only that player\'s un-leveraged assets', () => {
		const body = renderCouncil({ seat: RAVI });
		expect(mirrorsOf(body)).toHaveLength(2);
		const [sol, mei] = mirrorsOf(body).map(textOf);
		expect(sol).toContain('Sol Hold');
		// Leveraged already: not on offer.
		expect(sol).not.toContain('Sol Seal');
		expect(mei).toContain('Mei Ledger');
		expect(mei).toContain('Mei Cloak');
		// Destroyed, and another player's: not on offer.
		expect(mei).not.toContain('Mei Wreck');
		expect(mei).not.toContain('Sol Hold');
		expect(mei).not.toContain('The Old Charter');
	});

	it('names whose picker each one is, and what they are choosing, in the third person', () => {
		const t = textOf(renderCouncil({ seat: RAVI }));
		expect(t).toContain('sol may join');
		expect(t).toContain('mei may join');
		expect(t).toContain('They may leverage one of their assets to join the debate, or decline.');
		expect(t.match(/Their un-leveraged assets/g)).toHaveLength(2);
	});

	it('is inert: every card says it is disabled, and there is no Join or Decline button', () => {
		for (const seat of [RAVI, null]) {
			const body = renderCouncil({ seat });
			expect(mirrorsOf(body)).toHaveLength(2);
			for (const m of mirrorsOf(body)) {
				const rows = rowsOf(m);
				expect(rows.length).toBeGreaterThan(0);
				for (const r of rows) {
					expect(r).toContain('aria-disabled="true"');
					expect(r).toContain('tabindex="-1"');
				}
				// The card's own header button sits inside a disabled fieldset, which disables it;
				// nothing in the mirror is in the tab order, and nothing in it can be submitted.
				expect(m).toMatch(/^<fieldset[^>]*\bdisabled\b/);
				expect(m).not.toMatch(/\btabindex="0"/);
				expect(m).not.toMatch(/<(input|textarea|select|a)\b/);
				expect(m).not.toMatch(/type="submit"/);
			}
			expect(body).not.toMatch(/<button[^>]*>\s*Join\s*<\/button>/);
			expect(body).not.toMatch(/<button[^>]*>\s*Decline\s*<\/button>/);
		}
	});

	it('addresses a watcher as no one: no second-person word', () => {
		for (const seat of [RAVI, null]) {
			expect(textOf(mirrorsOf(renderCouncil({ seat })).join(' '))).not.toMatch(/\byou(r)?\b/i);
		}
		// The whole page for the preparer too, but for the signatory's own prompt and the
		// difficulty line, which are not about the join.
		const t = textOf(renderCouncil({ seat: RAVI }));
		expect(t).not.toContain('Leverage one asset to join the debate — it becomes a die you can use');
	});

	it('leaves the player who decides the live picker, with its buttons, and no mirror of their own', () => {
		const body = renderCouncil({ seat: MEI, council: [RAVI], declined: [SOL] });
		// sol has declined, so mei is the only one left: the live picker, not a mirror of it.
		expect(mirrorsOf(body)).toHaveLength(0);
		expect(body).toMatch(/<button[^>]*>\s*Join\s*<\/button>/);
		expect(body).toMatch(/<button[^>]*>\s*Decline\s*<\/button>/);
		expect(textOf(body)).toContain('you can use');
	});

	it('does not draw a decider\'s own picker twice, nor a co-decider\'s to someone who still has to decide', () => {
		// sol and mei both owe a decision: each has the live picker and is given no mirror.
		for (const seat of [SOL, MEI]) expect(mirrorsOf(renderCouncil({ seat }))).toHaveLength(0);
	});

	it('shows a player who has decided, or who is already seated, the pickers still open', () => {
		const sol = renderCouncil({ seat: SOL, council: [RAVI, SOL] });
		expect(mirrorsOf(sol)).toHaveLength(1);
		expect(textOf(mirrorsOf(sol)[0]!)).toContain('Mei Ledger');
		const declined = renderCouncil({ seat: SOL, declined: [SOL] });
		expect(mirrorsOf(declined)).toHaveLength(1);
		expect(textOf(declined)).toContain('You declined to join the council.');
	});

	it('is gone once nobody is left to decide', () => {
		for (const seat of [RAVI, SOL, MEI]) {
			const body = renderCouncil({ seat, council: [RAVI, SOL], declined: [MEI] });
			expect(body).not.toMatch(MIRROR);
		}
	});

	it('is gone once the roll is called', () => {
		expect(renderCouncil({ seat: RAVI, rolling: true })).not.toMatch(MIRROR);
	});

	it('is there before the debate opens too: joining is open from the start of the council', () => {
		expect(mirrorsOf(renderCouncil({ seat: RAVI, debateStarted: false }))).toHaveLength(2);
	});

	it('says when a player has nothing to leverage, so can only decline', () => {
		const t = textOf(renderCouncil({ seat: RAVI, assets: ASSETS.filter(a => a.owner_id !== MEI) }));
		expect(t).toContain('mei has no un-leveraged assets to offer, so can only decline.');
	});

	it('shows two watchers the same page: nothing in it is per viewer', () => {
		const a = mirrorsOf(renderCouncil({ seat: RAVI, council: [RAVI, SOL], declined: [] }));
		const b = mirrorsOf(renderCouncil({ seat: SOL, council: [RAVI, SOL], declined: [] }));
		expect(a).toEqual(b);
	});

	it('leaves the amendment and the addendum as lines: no mirror of a text step', () => {
		const pd = {
			signatory_id: RAVI, signatory_player_ids: [RAVI, SOL, MEI], outcome: 'mar', law_text: 'No private armies.',
			amendment_order: [SOL, MEI], amended_by: [],
		};
		const plan = { id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: RAVI,
			preparation_notes: 'x', resolution_data: JSON.stringify({ propose_decree: pd }) };
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [plan] as never, assets: ASSETS as never,
			players: [{ id: RAVI, display_name: 'ravi' }, { id: SOL, display_name: 'sol' }, { id: MEI, display_name: 'mei' }] as never,
			rankings: [] as never, currentPlayerID: RAVI, focusPlayerID: null, isFocusPlayer: false,
			rollActive: false, rollOutcome: 'mar', activeRoll: null as never,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		const body = render(ProposeDecreePanel, { props: { ctx, plan: plan as never, mode: 'resolve' } }).body;
		expect(body).not.toMatch(MIRROR);
		expect(body).not.toContain('<textarea');
		expect(textOf(body)).toContain('The council is amending the law (2 to go).');
	});
});
