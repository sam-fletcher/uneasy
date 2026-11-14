import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'svelte/server';
import BoutsPhase from './duel/BoutsPhase.svelte';
import GuestList from './festivity/GuestList.svelte';
import SocializingTurn from './festivity/SocializingTurn.svelte';
import MakeDemandsPanel from './MakeDemandsPanel.svelte';
import { declarations, styleOf } from '$lib/components/shared/cssTestKit';
import type { PlanContext } from './types';

/*
 * The redesign's rulings, held for the three plans with the longest sub-flows:
 * Make Demands, Host Festivity and Propose Duel. A box marks something you can
 * touch, a notice is a mark and coloured words, selected is the bright gold
 * border with a tick, a choice wears no meaning colour, and nothing is faded.
 * Each rule is written as a check on the files or on what they render, so a
 * panel that drifts back to the old look fails here and not in a screenshot.
 */

const PLANS = join(__dirname);
const read = (rel: string) => readFileSync(join(PLANS, rel), 'utf8');

/** Every .svelte file of the three plans (not their tests). */
function planFiles(): string[] {
	const out = ['MakeDemandsPanel.svelte', 'HostFestivityPanel.svelte', 'ProposeDuelPanel.svelte'];
	for (const dir of ['demand', 'festivity', 'duel']) {
		for (const f of readdirSync(join(PLANS, dir))) {
			if (f.endsWith('.svelte')) out.push(`${dir}/${f}`);
		}
	}
	return out;
}

const strip = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/\s+/g, ' ');
const cssOf = (rel: string) => declarations(styleOf(read(rel)));
const props = (css: ReturnType<typeof cssOf>, selector: string) =>
	css.filter((d) => d.selector === selector).map((d) => d.prop);

describe('the three plans are on the redesign (source rules)', () => {
	const files = planFiles();

	it('covers every file under the three folders', () => {
		expect(files.length).toBeGreaterThanOrEqual(15);
	});

	it.each(files)('%s uses the shared pieces, not the old class names', (rel) => {
		const src = read(rel);
		// A notice is <Notice>, a chip is <Chip>: the old markup names are gone.
		expect(src, 'a .res-warning paragraph').not.toMatch(/class="[^"]*\bres-warning\b/);
		expect(src, 'a .chip-btn chip').not.toMatch(/class="[^"]*\bchip-btn\b/);
		expect(src, 'a .player-chip chip').not.toMatch(/class="[^"]*\bplayer-chip\b/);
		// The file has been moved onto the scales, so it carries no marker.
		expect(src.startsWith('<!-- tokens: not migrated -->')).toBe(false);
	});

	it.each(files)('%s has no inline style and no typed glyphs', (rel) => {
		const src = read(rel).replace(/<!--[\s\S]*?-->/g, '');
		const markup = src.replace(/<script[\s\S]*?<\/script>/g, '').replace(/<style[\s\S]*?<\/style>/g, '');
		expect(markup, 'an inline style="" attribute').not.toMatch(/\sstyle="/);
		// A tick or a cross typed as a character comes from a different font on
		// every platform; the mark is drawn (ToneMark), and the word says the rest.
		expect(markup, 'a typed ✓ or ✗').not.toMatch(/[✓✗]/);
	});

	it.each(files)('%s styles use no retiring token and fade nothing', (rel) => {
		const css = styleOf(read(rel));
		for (const retired of [
			'--color-surface-active', '--color-surface-sunken', '--color-chip-', '--parchment-',
			'--color-warning-ink', '--color-warning-bg', '--color-warning-border', '--color-text-muted',
			'--color-text-faint-warm', '--color-border-warm)', '--color-border-warm-strong',
		]) {
			expect(css, `${retired} in ${rel}`).not.toContain(retired);
		}
		expect(css, 'a faded element (opacity)').not.toMatch(/\bopacity\s*:/);
	});
});

// ── Propose Duel ─────────────────────────────────────────────────────────────

const ALICE = 1; // prepared the duel
const BOB = 2; // challenged
const CAROL = 3; // watching
const players = [
	{ id: ALICE, display_name: 'alice', seat_order: 1 },
	{ id: BOB, display_name: 'bob', seat_order: 2 },
	{ id: CAROL, display_name: 'carol', seat_order: 3 },
] as never;
const assets = [
	{ id: 11, name: 'Lady Merrow' }, { id: 12, name: 'Captain Vell' },
	{ id: 13, name: 'The Orangery' }, { id: 14, name: 'Old Ferris' },
] as never;
const duelPlan = { id: 7, plan_type: 'propose_duel', preparer_id: ALICE, target_player_id: BOB } as never;

const stake = (id: number, owner: number, asset: number, o: Record<string, unknown> = {}) =>
	({ id, plan_id: 7, player_id: owner, asset_id: asset, is_resolved: false, is_winner: null, hidden_die: null, ...o });
const bout = (o: Record<string, unknown>) => ({
	id: 1, plan_id: 7, bout_number: 1, declarer_id: ALICE, declarer_stake_id: 101, responder_id: BOB,
	responder_stake_id: null, declaration: 'high', declarer_die: null, responder_die: null,
	winner_id: null, is_match: false, created_at: '', resolved_at: null, ...o,
});

function bouts(o: {
	viewer: number;
	prep: unknown[]; targ: unknown[]; list: unknown[]; mine?: unknown[];
	current?: number;
}): string {
	return strip(render(BoutsPhase, {
		props: {
			plan: duelPlan, players, assets, currentPlayerID: o.viewer,
			duelRes: { duelType: 'arms', phase: 'bouts', initiativeID: ALICE, prepChampID: null, targChampID: null,
				prepChampDeclared: true, targChampDeclared: true, currentBout: o.current ?? 0 },
			amParticipant: o.viewer === ALICE || o.viewer === BOB,
			preparerStakes: o.prep as never, targetStakes: o.targ as never, bouts: o.list as never,
			myUnresolvedStakes: (o.mine ?? []) as never,
			onPlansChanged: () => {}, onRefresh: () => {},
		},
	}).body);
}

// The die tile's face is its own element; a die nobody may read draws the mark and no number.
const FACE = /class="face[^"]*">(\d)</g;
const faces = (html: string) => [...html.matchAll(FACE)].map((m) => m[1]);

describe('the duel tracker', () => {
	// Bob is to answer alice's open call. He can read his own dice, never hers.
	const openCall = {
		viewer: BOB,
		prep: [stake(101, ALICE, 11), stake(103, ALICE, 13)],
		targ: [stake(102, BOB, 12, { hidden_die: 3 }), stake(104, BOB, 14, { hidden_die: 5 })],
		list: [bout({})],
		mine: [stake(102, BOB, 12, { hidden_die: 3 }), stake(104, BOB, 14, { hidden_die: 5 })],
	};

	it('shows a player their own dice and nobody else\'s, in the DOM', () => {
		const html = bouts(openCall);
		// Bob's own two faces are on the page; alice's two stakes carry no number at all.
		expect(faces(html).sort()).toEqual(['3', '5']);
		expect(html).toContain('aria-label="alice\'s hidden die under Lady Merrow"');
		expect(html).toContain('aria-label="alice\'s hidden die under The Orangery"');
	});

	it('shows a bystander no die value at all', () => {
		const html = bouts({ ...openCall, viewer: CAROL, mine: [],
			targ: [stake(102, BOB, 12), stake(104, BOB, 14)] });
		expect(faces(html)).toEqual([]);
		// And no stake is pressable for someone who is not a duellist.
		expect(html).not.toContain('stake-line pick');
	});

	it('lets the player whose turn it is pick one of their own stakes, and only those', () => {
		const html = bouts(openCall);
		const picks = html.match(/<button[^>]*stake-line pick[^>]*>/g) ?? [];
		expect(picks).toHaveLength(2);
		// A pick is a control: the option card, pressed or not. Alice's stakes are plain lines.
		for (const p of picks) {
			expect(p).toContain('choice-card');
			expect(p).toContain('aria-pressed="false"');
		}
		expect(html).toContain('Lady Merrow');
		expect(html).not.toMatch(/<button[^>]*>[^<]*Lady Merrow/);
	});

	it('shows nobody a pick when it is not their turn', () => {
		// Alice declared; she waits for bob, so her own stakes are lines, not controls.
		const html = bouts({ ...openCall, viewer: ALICE,
			prep: [stake(101, ALICE, 11, { hidden_die: 4 }), stake(103, ALICE, 13, { hidden_die: 2 })],
			targ: [stake(102, BOB, 12), stake(104, BOB, 14)], mine: [
				stake(101, ALICE, 11, { hidden_die: 4 }), stake(103, ALICE, 13, { hidden_die: 2 })] });
		expect(html).not.toContain('stake-line pick');
		expect(html).toContain('Waiting for bob to respond');
	});

	it('states the public call as an information notice, with the stake it was made on', () => {
		const html = bouts(openCall);
		expect(html).toMatch(/<p class="notice[^"]*" data-tone="info"/);
		expect(html).toContain('opens with <em>Lady Merrow</em>, calling higher.');
		expect(html).toContain('called higher');
	});

	it('marks a bout\'s result with a word beside its colour, and a tied die as dashed', () => {
		const resolved = bout({ resolved_at: 'x', responder_stake_id: 102, declarer_die: 6, responder_die: 2, winner_id: ALICE });
		const html = bouts({
			viewer: CAROL, current: 1,
			prep: [stake(101, ALICE, 11, { is_resolved: true, is_winner: true }), stake(103, ALICE, 13)],
			targ: [stake(102, BOB, 12, { is_resolved: true, is_winner: false }), stake(104, BOB, 14)],
			list: [resolved],
		});
		expect(html).toMatch(/status-word success[^>]*>won</);
		expect(html).toMatch(/status-word danger[^>]*>lost</);
		// The dice a side has won are tiles, 6 and 2 for alice's pool.
		expect(faces(html).sort()).toEqual(['2', '2', '6', '6']);

		const tied = bout({ resolved_at: 'x', responder_stake_id: 102, declarer_die: 4, responder_die: 4, is_match: true });
		const tiedHtml = bouts({
			viewer: CAROL, current: 1,
			prep: [stake(101, ALICE, 11, { is_resolved: true }), stake(103, ALICE, 13)],
			targ: [stake(102, BOB, 12, { is_resolved: true }), stake(104, BOB, 14)],
			list: [tied],
		});
		// The two tied stakes show a dashed tile each, and the two dice waiting in the
		// tray are dashed too: four in all, never faded.
		expect((tiedHtml.match(/die-tile[^"]*\bduplicate\b/g) ?? []).length).toBe(4);
		expect(tiedHtml).toContain('tied');
	});

	it('says "hidden" beside every die the viewer cannot read, so it is not mistaken for an unrolled one', () => {
		const hidden = (html: string) => (html.match(/status-word stake-tag[^"]*">hidden</g) ?? []).length;
		// Bob reads his own two dice; alice's two are hidden from him.
		expect(hidden(bouts(openCall))).toBe(2);
		// A bystander can read none of the four.
		expect(hidden(bouts({ ...openCall, viewer: CAROL, mine: [],
			targ: [stake(102, BOB, 12), stake(104, BOB, 14)] }))).toBe(4);
		// A die that has been played is shown, not hidden.
		const played = bout({ resolved_at: 'x', responder_stake_id: 102, declarer_die: 6, responder_die: 2, winner_id: ALICE });
		expect(hidden(bouts({
			viewer: CAROL, current: 1,
			prep: [stake(101, ALICE, 11, { is_resolved: true, is_winner: true }), stake(103, ALICE, 13)],
			targ: [stake(102, BOB, 12, { is_resolved: true, is_winner: false }), stake(104, BOB, 14)],
			list: [played],
		}))).toBe(2);
	});

	it('separates two status words on one stake, so "called higher" and "hidden" do not read as a phrase', () => {
		const css = cssOf('duel/BoutsPhase.svelte');
		const sep = css.filter((d) => d.selector === '.stake-tag + .stake-tag::before');
		expect(sep.find((d) => d.prop === 'content')?.value).toBe("'·'");
		// And the called stake does carry both words, in that order.
		const html = bouts(openCall);
		expect(html).toMatch(/>called higher<\/span> <span class="status-word stake-tag[^"]*">hidden</);
	});

	it('draws each tied die in the tray with its owner\'s colour on the edge', () => {
		const tied = bout({ resolved_at: 'x', responder_stake_id: 102, declarer_die: 4, responder_die: 4, is_match: true });
		const html = bouts({
			viewer: CAROL, current: 1,
			prep: [stake(101, ALICE, 11, { is_resolved: true }), stake(103, ALICE, 13)],
			targ: [stake(102, BOB, 12, { is_resolved: true }), stake(104, BOB, 14)],
			list: [tied],
		});
		const tray = [...html.matchAll(/aria-label="(\w+)'s tied die, set aside"[^>]*style="border-color: (#[0-9A-Fa-f]+);"/g)]
			.map((m) => [m[1], m[2].toUpperCase()]);
		// Alice declared, so her die comes first: Lavender, then bob's Peridot. Never a control's border.
		expect(tray).toEqual([['alice', '#C5AEFE'], ['bob', '#8AD00B']]);
		expect(html).not.toContain('border-color: var(--color-border-control)');
	});

	it('draws the sides as sections and the tray as the one box, without a fill', () => {
		const css = cssOf('duel/BoutsPhase.svelte');
		for (const forbidden of ['background', 'border', 'box-shadow', 'padding']) {
			expect(props(css, '.duel-side'), `.duel-side ${forbidden}`).not.toContain(forbidden);
		}
		// The tray keeps the antique gold as a dashed rim; it takes no fill.
		const tray = css.filter((d) => d.selector === '.set-aside');
		expect(tray.find((d) => d.prop === 'border')?.value).toBe('1px dashed var(--color-border-warm-antique)');
		expect(tray.map((d) => d.prop)).not.toContain('background');
		// A stake is told apart by its name, never by a coloured edge on the side.
		expect(styleOf(read('duel/BoutsPhase.svelte'))).not.toContain('--color-accent');
	});
});

// ── Host Festivity ───────────────────────────────────────────────────────────

const fest = (o: Record<string, unknown> = {}) => ({
	guests: [ALICE, BOB, CAROL], outcomes: {}, guestMakes: {}, guestMars: {}, hostMakesTaken: [],
	guestRollIDs: {}, guestIOUs: [], hostMarInsists: [], unchosenHostMars: [], pendingHostMars: [],
	acceptDuels: [], pendingDuelPlanID: null, pendingChallenge: null, centeredAssetIDs: [], centeredDrafts: [], ...o,
});
const festPlan = { id: 5, game_id: 1, plan_type: 'host_festivity', preparer_id: ALICE } as never;

describe('the festivity guest list', () => {
	const html = (f: unknown, viewer = BOB) => strip(render(GuestList, {
		props: { plan: festPlan, fest: f as never, players, currentPlayerID: viewer },
	}).body);

	it('has one row per guest, each with the player\'s shape and name', () => {
		const body = html(fest());
		expect(body.match(/<li[ >]/g)).toHaveLength(3);
		for (const name of ['alice', 'bob', 'carol']) expect(body).toContain(`>${name}</span>`);
		// A shape in front of each name (the key a colour-blind player reads).
		expect(body.match(/class="player-shape/g)).toHaveLength(3);
		expect(body).toContain('(you)');
		expect(body).toContain('(host)');
	});

	it('colours a verdict only where it is a result, beside its words', () => {
		const body = html(fest({
			outcomes: { [BOB]: 'make', [CAROL]: 'mar' },
			guestMakes: { [BOB]: 'spread_rumor' }, guestMars: { [CAROL]: 'break_self' },
		}));
		expect(body).toMatch(/outcome-make[^>]*>Made a good impression</);
		expect(body).toMatch(/outcome-mar[^>]*>Slipped up</);
		// A guest who has not played yet reads in plain words.
		const pending = html(fest());
		expect(pending).toContain('Yet to make their mark');
		expect(pending).not.toContain('outcome-make');
		expect(pending).not.toContain('outcome-mar');
	});

	it('puts nothing in italics: that is for the names of assets', () => {
		expect(styleOf(read('festivity/GuestList.svelte'))).not.toContain('italic');
	});
});

describe('a guest\'s turn at the festivity', () => {
	const turn = (o: { myRollID: number | null; outcome: 'make' | 'mar' | null; blocked?: boolean }) => strip(render(SocializingTurn, {
		props: {
			plan: festPlan, fest: fest() as never, players, assets: [], currentPlayerID: BOB,
			myRollID: o.myRollID, myEffectiveOutcome: o.outcome, difficulty: 2,
			blockedByOtherRoll: o.blocked ?? false, activeRollerID: null, onPlansChanged: () => {},
		},
	}).body);

	it('offers Roll as the main button and Opt out as a secondary one beside it', () => {
		const body = turn({ myRollID: null, outcome: null });
		expect(body).toMatch(/class="action-btn primary[^"]*"[^>]*>Roll · difficulty 2</);
		// Not a bare .action-btn, which draws as the browser's own white button.
		expect(body).toMatch(/class="action-btn secondary[^"]*"[^>]*>Opt out</);
	});

	it('names the result with its word and gives each option as a house chip', () => {
		const body = turn({ myRollID: 3, outcome: 'make' });
		expect(body).toMatch(/outcome-make[^>]*>Make</);
		expect(body).not.toMatch(/[✓✗]/);
		// A choice among a few things is a chip, a toggle with aria-pressed.
		const chips = body.match(/<button[^>]*class="chip[^"]*"[^>]*aria-pressed="false"/g) ?? [];
		expect(chips.length).toBeGreaterThanOrEqual(4);
		expect(body).toContain('Spread a new rumor');
	});

	it('gives a mar its own word', () => {
		expect(turn({ myRollID: 3, outcome: 'mar' })).toMatch(/outcome-mar[^>]*>Mar</);
	});
});

// ── Make Demands ─────────────────────────────────────────────────────────────

describe('the Make Demands draft', () => {
	function draft(viewer: number): string {
		const target = { id: 9, plan_type: 'seek_answers', status: 'pending', preparer_id: ALICE, row_number: 5 };
		const demand = {
			id: 12, plan_type: 'make_demands', status: 'resolving', preparer_id: BOB, targeted_plan_id: 9,
			preparation_notes: 'Hand it over.',
			resolution_data: JSON.stringify({ make_demands: { draft_choices: [{ option: 'control_leverage', player_id: ALICE }] } }),
		};
		const ctx = {
			gameID: 1, currentRow: 5, endingMode: null, plans: [target, demand] as never, assets: [], players,
			rankings: [
				{ category: 'power', player_id: ALICE, rank: 1 }, { category: 'power', player_id: BOB, rank: 2 },
			] as never,
			currentPlayerID: viewer, isFocusPlayer: false, rollActive: false, rollOutcome: 'make', activeRoll: null,
			onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
			readOnly: false, prepDraft: null, emitPrepDraft: () => {},
		} as PlanContext;
		return strip(render(MakeDemandsPanel, { props: { ctx, plan: demand as never, mode: 'resolve' } }).body);
	}

	it('shows each pick so far as a finished line with its picker\'s shape', () => {
		const body = draft(BOB);
		// The pick is a finished step of its own (not just text under a .done-steps
		// wrapper), with its picker's shape drawn in front of the words.
		const line = body.split('class="done-step"').find((chunk) => chunk.includes('alice picked'));
		expect(line, 'a finished step holding the pick').toBeDefined();
		expect(line).toContain('class="player-shape');
		// One line, the short name of the option: the long description is on the cards.
		expect(body).toContain('alice picked Control leverage');
		expect(body).not.toContain('picked Control leverage —');
	});

	it('gives an option card a title and its effect on separate lines', () => {
		const body = draft(BOB);
		expect(body).toMatch(/choice-title[^>]*>Perform steps<\/span>/);
		expect(body).toMatch(/choice-desc[^>]*>Submit the target plan’s make\/mar choice in their place</);
		// Control leverage is already taken, so it is not offered again.
		expect(body).not.toMatch(/choice-title[^>]*>Control leverage</);
	});

	it('shows a card that would do nothing as locked, with the reason in words', () => {
		const body = draft(BOB);
		const keepAssets = body.match(/<button[^>]*choice-card[^>]*>(?:(?!<\/button>).)*Keep assets(?:(?!<\/button>).)*<\/button>/)?.[0] ?? '';
		expect(keepAssets).toContain('aria-disabled="true"');
		expect(keepAssets).toContain('gives its preparer no assets to keep');
		// A live card is not locked.
		const perform = body.match(/<button[^>]*choice-card[^>]*>(?:(?!<\/button>).)*Perform steps(?:(?!<\/button>).)*<\/button>/)?.[0] ?? '';
		expect(perform).toContain('aria-disabled="false"');
	});

	it('shows the player who is not up a wait and no option cards', () => {
		const body = draft(CAROL);
		expect(body).toContain('Waiting for bob to pick');
		expect(body).not.toContain('choice-card');
	});
});

describe('the demand overlay', () => {
	const css = cssOf('demand/TargetPlanDemandOverlay.svelte');

	it('is a group on the page: no box, no fill, no ink of its own', () => {
		for (const sel of ['.demand-banner', '.demand-summary', '.demand-form']) {
			for (const forbidden of ['background', 'border', 'border-top', 'padding', 'color', 'box-shadow']) {
				expect(props(css, sel), `${sel} ${forbidden}`).not.toContain(forbidden);
			}
		}
		// It was the app's one light panel; nothing on it is inked for a light ground.
		expect(styleOf(read('demand/TargetPlanDemandOverlay.svelte'))).not.toContain('--color-bg');
	});

	it('sets what the demand does as plain secondary text, with no colour of its own', () => {
		const winners = css.filter((d) => d.selector === '.demand-winners');
		expect(winners.find((d) => d.prop === 'color')?.value).toBe('var(--color-text-secondary)');
	});

	it('stacks its two buttons full width in a phone column', () => {
		const grow = css.filter((d) => d.selector.startsWith('.demand-actions >') && d.prop === 'flex');
		// Each is 12rem at least, so two do not fit beside each other in 344px.
		expect(grow.map((d) => d.value).sort()).toEqual(['1 1 12rem', '2 1 12rem']);
	});
});
