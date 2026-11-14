import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import ClandestinelyLiaisePanel from './ClandestinelyLiaisePanel.svelte';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import MakeIntroductionsPanel from './MakeIntroductionsPanel.svelte';
import ProposeDecreePanel from './ProposeDecreePanel.svelte';
import type { PlanContext } from './types';

// The five panels of the second batch (Clandestinely Liaise, Propose Decree,
// Make Introductions, Exchange Courtiers, Re-aim) draw their chips with <Chip>
// and their warnings with <Notice>, not with the older class names that
// planPanel.css keeps only for panels that have not moved yet. These tests pin
// the pieces a player sees: which chip is selected and which is greyed out, which
// button is the main one, and where the step's own sentence sits.

const HERE = __dirname;
const PANELS = [
	'ClandestinelyLiaisePanel.svelte',
	'ProposeDecreePanel.svelte',
	'MakeIntroductionsPanel.svelte',
	'ExchangeCourtiersPanel.svelte',
	'ReAimPanel.svelte',
];

/** The markup of a .svelte file: no script, no style, no comments. */
function markupOf(file: string): string {
	return readFileSync(join(HERE, file), 'utf8')
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '')
		.replace(/<!--[\s\S]*?-->/g, '');
}

describe('the five panels draw no legacy chip or warning markup', () => {
	// .choice-card is not on this list on purpose: there is no shared option card
	// yet, so Re-aim's Make Demands cards keep the recipe in planPanel.css.
	it.each(PANELS)('%s', (file) => {
		const markup = markupOf(file);
		for (const legacy of ['chip-btn', 'player-chip', 'res-warning']) {
			expect(markup, `${file} still uses ${legacy}`).not.toContain(legacy);
		}
	});
});

// One mechanism puts a title close to its sentence: wrap the two in a block with a
// small gap. A negative margin pulling a sentence up closed the gap by a different
// amount depending on what else set one (a wrapper's own gap made it overlap), so
// none of the five uses one.
describe('the five panels do not close a gap with a negative margin', () => {
	function styleOf(file: string): string {
		const src = readFileSync(join(HERE, file), 'utf8');
		const blocks = [...src.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/gi)].map(m => m[1]);
		const inline = [...src.matchAll(/\sstyle="([^"]*)"/g)].map(m => m[1]);
		return [...blocks, ...inline].join('\n').replace(/\/\*[\s\S]*?\*\//g, '');
	}
	it.each(PANELS)('%s', (file) => {
		expect(styleOf(file), `${file} has a negative margin`).not.toMatch(/margin[a-z-]*\s*:[^;}]*(?:calc\(\s*-|:\s*-\s*\d|\s-\s*\d|\(\s*-\s*1\s*\*)/i);
	});
});

// ── Reading rendered HTML ────────────────────────────────────────────────

interface Btn { open: string; text: string }
const text = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim();
function buttons(html: string): Btn[] {
	return [...html.matchAll(/<button\b([^>]*)>([\s\S]*?)<\/button>/g)].map(m => ({ open: m[1], text: text(m[2]) }));
}
const byLabel = (html: string, label: string) => {
	const found = buttons(html).filter(b => b.text === label);
	expect(found, `one button labelled "${label}"`).toHaveLength(1);
	return found[0];
};
const hasClass = (b: Btn, cls: string) => new RegExp(`class="[^"]*\\b${cls}\\b`).test(b.open);
const pressed = (b: Btn) => /aria-pressed="true"/.test(b.open);
const ariaDisabled = (b: Btn) => /aria-disabled="true"/.test(b.open);

/** The HTML with the framework's comment markers dropped, so adjacent elements read as adjacent. */
const clean = (html: string) => html.replace(/<!--.*?-->/g, '').replace(/>\s+</g, '><');
/** A title and the sentence under it, in one `.titled` block. */
function expectTitled(html: string, title: string) {
	const escaped = title.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	expect(clean(html), `"${title}" sits in a titled block with its sentence`).toMatch(
		new RegExp(`<div class="titled[^"]*"><p class="choices-header">${escaped}</p><p class="ft-prompt`),
	);
}

const PREPARER = 1;
const OTHER = 2;
const THIRD = 3;
const PLAYERS = [
	{ id: PREPARER, display_name: 'ravi' },
	{ id: OTHER, display_name: 'sol' },
	{ id: THIRD, display_name: 'mel' },
];

function ctxFor(over: Partial<PlanContext> & { plans: never; currentPlayerID: number }): PlanContext {
	return {
		gameID: 1, currentRow: 5, endingMode: null, assets: [], players: PLAYERS as never,
		rankings: [], isFocusPlayer: false, rollActive: false, rollOutcome: null, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		...over,
	} as PlanContext;
}

// ── Make Introductions ───────────────────────────────────────────────────

describe('Make Introductions chips', () => {
	it('the preparation form picks the peer count with chips, one selected', () => {
		const body = render(MakeIntroductionsPanel, {
			props: { ctx: ctxFor({ plans: [] as never, currentPlayerID: PREPARER }), plan: null, mode: 'prep' },
		}).body;
		const counts = ['1', '2', '3', '4'].map(n => byLabel(body, n));
		for (const c of counts) expect(hasClass(c, 'chip')).toBe(true);
		expect(counts.map(pressed)).toEqual([true, false, false, false]);
		expect(hasClass(counts[0], 'selected')).toBe(true);
	});

	function marAs(player: number, outcomes: unknown[]) {
		const plan = {
			id: 9, plan_type: 'make_introductions', status: 'resolving', preparer_id: PREPARER,
			preparation_notes: 'x',
			resolution_data: JSON.stringify({
				make_introductions: {
					peer_count: 3, mar_pending: true,
					drafts: [{ id: 'd1', name: 'Lady Vane' }, { id: 'd2', name: 'Sir Pell' }, { id: 'd3', name: 'Dame Orr' }],
					mar_outcomes: outcomes,
				},
				make_mar_choices: [],
				choice_auto_applied: true,
			}),
		};
		return render(MakeIntroductionsPanel, {
			props: { ctx: ctxFor({ plans: [plan] as never, currentPlayerID: player, rollOutcome: 'mar' }), plan: plan as never, mode: 'resolve' },
		}).body;
	}

	it('an outcome already handed out is a greyed, struck-through chip; the others stay live', () => {
		const body = marAs(PREPARER, [{ draft_id: 'd1', outcome: 'other_retinue', author_player_id: OTHER, done: false }]);
		const taken = byLabel(body, 'Another retinue');
		expect(hasClass(taken, 'chip')).toBe(true);
		expect(ariaDisabled(taken)).toBe(true);
		expect(body).toMatch(/aria-disabled="true"><span class="[^"]*\bstruck\b[^"]*">Another retinue</);
		for (const free of ['Not who you thought', 'Delayed', 'Broken journey']) {
			expect(ariaDisabled(byLabel(body, free)), free).toBe(false);
		}
		// The default pick moves off the used one, onto the first still free.
		expect(pressed(byLabel(body, 'Not who you thought'))).toBe(true);
		expect(pressed(taken)).toBe(false);
	});

	it('the settled peers are one tight list above the next form', () => {
		const body = clean(marAs(PREPARER, [
			{ draft_id: 'd1', outcome: 'other_retinue', author_player_id: OTHER, done: false },
			{ draft_id: 'd2', outcome: 'delayed', done: true },
		]));
		const recap = body.match(/<div class="recap[^"]*">(.*?)<\/div>/)?.[1] ?? '';
		expect(recap.match(/class="choices-applied"/g)).toHaveLength(2);
		expect(text(recap)).toBe('Lady Vane — Another retinue (awaiting marginalia)Sir Pell — Delayed');
		// And with nothing settled there is no empty block to add a gap of its own.
		expect(clean(marAs(PREPARER, []))).not.toContain('class="recap');
	});

	it('the recap of a settled peer carries no typed check mark', () => {
		const body = marAs(PREPARER, [{ draft_id: 'd1', outcome: 'delayed', done: true }]);
		expect(text(body)).toContain('Lady Vane — Delayed');
		expect(body).not.toContain('✓');
	});
});

// ── Propose Decree ───────────────────────────────────────────────────────

describe('Propose Decree', () => {
	const decree = (over: Record<string, unknown>, currentPlayerID = PREPARER, rollOutcome: 'make' | 'mar' | null = 'make') => {
		const plan = {
			id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'x',
			resolution_data: JSON.stringify({
				propose_decree: {
					signatory_id: PREPARER, signatory_player_ids: [PREPARER, OTHER],
					debate_started: true, outcome: rollOutcome, law_text: 'No private armies.', ...over,
				},
			}),
		};
		return render(ProposeDecreePanel, {
			props: { ctx: ctxFor({ plans: [plan] as never, currentPlayerID, rollOutcome }), plan: plan as never, mode: 'resolve' },
		}).body;
	};

	it('the addendum connector is a pair of chips, "and" selected to begin with', () => {
		const body = decree({});
		const and = byLabel(body, 'and');
		const but = byLabel(body, 'but');
		expect(hasClass(and, 'chip') && hasClass(but, 'chip')).toBe(true);
		expect(pressed(and)).toBe(true);
		expect(pressed(but)).toBe(false);
	});

	it('the result reads "Make" or "Mar" in words, with no typed check or cross', () => {
		for (const outcome of ['make', 'mar'] as const) {
			const body = decree({ amendment_order: [], addendum_placed: false }, PREPARER, outcome);
			const word = outcome === 'make' ? 'Make' : 'Mar';
			expect(body).toMatch(new RegExp(`Result: <span class="outcome-${outcome}[^"]*">${word}</span>`));
			expect(body).not.toMatch(/[✓✗]/);
		}
	});

	it('each step\'s title and its sentence share one block', () => {
		const RANKS = [
			{ category: 'power', player_id: PREPARER, rank: 1 },
			{ category: 'power', player_id: OTHER, rank: 2 },
		];
		const as = (over: Record<string, unknown>, seat: number, rollOutcome: 'make' | 'mar' | null, extra: { rankings?: never; assets?: never } = {}) => {
			const plan = {
				id: 9, plan_type: 'propose_decree', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'x',
				resolution_data: JSON.stringify({
					propose_decree: { signatory_id: PREPARER, signatory_player_ids: [PREPARER], debate_started: true, law_text: 'No private armies.', ...over },
				}),
			};
			return render(ProposeDecreePanel, {
				props: { ctx: ctxFor({ plans: [plan] as never, currentPlayerID: seat, rollOutcome, ...extra }), plan: plan as never, mode: 'resolve' },
			}).body;
		};
		expectTitled(as({ debate_started: false }, PREPARER, null), 'Finalize your decree');
		expectTitled(as({}, OTHER, null, { rankings: RANKS as never, assets: [{ id: 1, owner_id: OTHER, name: 'Ring', asset_type: 'artifact', is_destroyed: false, is_leveraged: false, marginalia: [] }] as never }), 'Join the council?');
		expectTitled(as({ outcome: 'mar', amendment_order: [OTHER], amended_by: [] }, OTHER, 'mar'), 'Amend the law (your turn)');
		expectTitled(as({ outcome: 'make' }, PREPARER, 'make'), "Signatory's Addendum");
		expectTitled(as({ outcome: 'make', addendum_placed: true }, PREPARER, 'make'), 'Enact the law');
		expectTitled(as({}, OTHER, null), 'Proposed decree being debated:');
	});

	it('the addendum is plain text after the law, not italic', () => {
		const body = clean(decree({ addendum_placed: true, addendum: 'salt is exempt', addendum_connector: 'and' }));
		expect(body).toMatch(/<p class="ft-prompt[^"]*">Law: No private armies\. — and salt is exempt<\/p>/);
	});

	it('the law under debate is step text, and the council is a plain list', () => {
		const body = decree({ outcome: '' }, OTHER, null);
		expect(body).toMatch(/<p class="ft-prompt[^"]*">“No private armies\.”<\/p>/);
		expect(body).toContain('council-list');
		expect(body).not.toMatch(/style="[^"]*margin/);
	});
});

// ── Exchange Courtiers ───────────────────────────────────────────────────

describe('Exchange Courtiers buttons', () => {
	const PEER_ASSETS = [
		{ id: 5, name: 'Lord Vane', owner_id: OTHER, asset_type: 'peer', is_destroyed: false },
		{ id: 6, name: 'Dame Orr', owner_id: PREPARER, asset_type: 'peer', is_destroyed: false },
	];
	function ec(resolution: Record<string, unknown>, seat: number, top: Record<string, unknown> = {}, assets = PEER_ASSETS, rollOutcome: 'make' | 'mar' | null = null) {
		const plan = {
			id: 9, plan_type: 'exchange_courtiers', status: 'resolving', preparer_id: PREPARER,
			target_player_id: OTHER, target_asset_id: 5, preparation_notes: 'x',
			resolution_data: JSON.stringify({ exchange_courtiers: resolution, ...top }),
		};
		return render(ExchangeCourtiersPanel, {
			props: {
				ctx: ctxFor({ plans: [plan] as never, currentPlayerID: seat, assets: assets as never, rollOutcome }),
				plan: plan as never, mode: 'resolve',
			},
		}).body;
	}

	it('a riposte asks for a main "Break, then surrender", with "Surrender intact" beside it', () => {
		const body = ec(
			{ fair_trade_asset_id: 6, fair_trade_accepted: false, riposte_allowed: true },
			PREPARER, { choice_submitted: true }, PEER_ASSETS, 'mar',
		);
		const brk = byLabel(body, 'Break, then surrender');
		const intact = byLabel(body, 'Surrender intact');
		expect(hasClass(brk, 'primary')).toBe(true);
		expect(hasClass(intact, 'secondary')).toBe(true);
		expect(hasClass(intact, 'primary')).toBe(false);
	});

	it('a preparer with no peer to trade has one way on, and it is the main button', () => {
		const body = ec({}, PREPARER, {}, PEER_ASSETS.filter(a => a.owner_id !== PREPARER));
		expect(hasClass(byLabel(body, 'Proceed to dice roll'), 'primary')).toBe(true);
	});

	it('an offer waiting for the preparer keeps one main button, Accept, with Decline beside it', () => {
		const body = ec({ fair_trade_asset_id: 6 }, PREPARER);
		expect(hasClass(byLabel(body, 'Accept — exchange without rolling'), 'primary')).toBe(true);
		expect(hasClass(byLabel(body, 'Decline — proceed to dice roll'), 'secondary')).toBe(true);
	});
});

// ── Clandestinely Liaise ─────────────────────────────────────────────────

describe('Clandestinely Liaise delay reveal', () => {
	const plan = {
		id: 9, plan_type: 'clandestinely_liaise', status: 'pending', preparer_id: PREPARER,
		preparation_notes: 'Under the old bridge.',
		resolution_data: JSON.stringify({
			liaise: { partner_id: OTHER, preparer_peer_id: 20, partner_peer_id: 21, delay_reveal_id: 3 },
		}),
	};
	const assets = [
		{ id: 20, name: 'Dame Orr', owner_id: PREPARER, asset_type: 'peer', is_destroyed: false },
		{ id: 21, name: 'Lord Vane', owner_id: OTHER, asset_type: 'peer', is_destroyed: false },
	];
	const body = render(ClandestinelyLiaisePanel, {
		props: {
			ctx: ctxFor({ plans: [plan] as never, currentPlayerID: THIRD, assets: assets as never }),
			plan: plan as never, mode: 'delayReveal',
		},
	}).body;

	it('says who meets whom as one line of step text', () => {
		expect(body).toMatch(/<p class="ft-prompt cl-meeting-peers[^"]*">/);
		expect(text(body)).toContain("ravi's Dame Orr meets sol's Lord Vane");
	});

	it('quotes the note in typographic quotes, left-aligned like every other step line', () => {
		expect(text(body)).toContain('“Under the old bridge.”');
		expect(body).not.toContain('class="cl-peer');
	});
});


describe('Clandestinely Liaise phases', () => {
	const PHASES: [string, string][] = [
		['together_at_last', 'Together at Last'],
		['secrets_we_keep', 'Secrets We Keep'],
		['things_we_share', 'Things We Share'],
		['when_will_i_see_you_again', 'When will I see you again?'],
	];
	it.each(PHASES)('%s: the title and its sentence share one block', (phase, title) => {
		const plan = {
			id: 9, plan_type: 'clandestinely_liaise', status: 'resolving', preparer_id: PREPARER, preparation_notes: 'x',
			resolution_data: JSON.stringify({ liaise: { phase, partner_id: OTHER, preparer_peer_id: 20, partner_peer_id: 21 } }),
		};
		const body = render(ClandestinelyLiaisePanel, {
			props: { ctx: ctxFor({ plans: [plan] as never, currentPlayerID: PREPARER }), plan: plan as never, mode: 'resolve' },
		}).body;
		expectTitled(body, title);
	});
});
