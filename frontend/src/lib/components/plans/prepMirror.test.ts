import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import ExchangeCourtiersPanel from './ExchangeCourtiersPanel.svelte';
import ClandestinelyLiaisePanel from './ClandestinelyLiaisePanel.svelte';
import DuelPrepForm from './duel/PrepForm.svelte';
import WarPrepForm from './war/PrepForm.svelte';
import type { PlanContext } from './types';

// While the focus player prepares a plan, everyone else sees a read-only mirror of
// their form. That mirror is drawn from the preparer, not from the viewer: its
// partner chips are everyone but the preparer, and its "meeting peer" list is the
// preparer's own peers, named as theirs. Propose Duel's challenger chips and Make
// War's enemy chips work the same way. The preparer's own form is unchanged.
//
// svelte/server does not run $effect, so the chip that is picked (copied from the
// draft in an effect) is not in these renders. What is pinned here is the list
// each viewer is given and the words around it; the picked chip is checked in the
// browser.

const PREPARER = 1;
const TARGET = 2;
const BYSTANDER = 3;

const PLAYERS = [
	{ id: PREPARER, display_name: 'alice' },
	{ id: TARGET, display_name: 'bob' },
	{ id: BYSTANDER, display_name: 'carol' },
];
const ASSETS = [
	{ id: 10, name: 'Mara', owner_id: PREPARER, asset_type: 'peer', is_destroyed: false, marginalia: [] },
	{ id: 11, name: 'Tibor', owner_id: TARGET, asset_type: 'peer', is_destroyed: false, marginalia: [] },
	{ id: 12, name: 'Ines', owner_id: BYSTANDER, asset_type: 'peer', is_destroyed: false, marginalia: [] },
];

function ctxFor(seat: number, prepDraft: Record<string, unknown> | null, assets = ASSETS): PlanContext {
	return {
		gameID: 1, currentRow: 5, endingMode: null, plans: [] as never,
		assets: assets as never, players: PLAYERS as never, rankings: [],
		currentPlayerID: seat, focusPlayerID: PREPARER, isFocusPlayer: seat === PREPARER,
		rollActive: false, rollOutcome: null, activeRoll: null,
		onRollCreated: () => {}, onPlansChanged: () => {}, onPlanPrepared: () => {},
		readOnly: seat !== PREPARER, prepDraft: seat === PREPARER ? null : prepDraft,
		emitPrepDraft: () => {},
	} as PlanContext;
}

const text = (html: string) => html
	.replace(/<!--.*?-->/g, '')
	.replace(/<[^>]+>/g, '')
	.replace(/&#39;|&#x27;/g, "'")
	.replace(/\s+/g, ' ')
	.trim();

/** The player chips only: the house chip, one per player the form offers. */
const chipNames = (html: string) =>
	(html.match(/<button\b[^>]*class="[^"]*\bchip\b[\s\S]*?<\/button>/g) ?? []).map(text);

/** The player chips of a form that has other chips too (Duel's Arms and Wits): only the player-chips row. */
const playerChips = (html: string) => {
	const row = html.split('player-chips')[1]?.split('</div>')[0] ?? '';
	return (row.match(/<button\b[\s\S]*?<\/button>/g) ?? []).map(text);
};

describe('Exchange Courtiers: the target chips', () => {
	const DRAFT = { target_player_id: TARGET, target_asset_id: 11, notes: 'x' };
	const render_ = (seat: number) =>
		render(ExchangeCourtiersPanel, { props: { ctx: ctxFor(seat, DRAFT), plan: null, mode: 'prep' } }).body;

	it("the preparer's own chips are everyone but the preparer", () => {
		expect(chipNames(render_(PREPARER))).toEqual(['bob', 'carol']);
	});

	it.each([
		['the target (bob)', TARGET],
		['a bystander (carol)', BYSTANDER],
	])('a watcher, %s, is shown the preparer\'s chips, not their own', (_, seat) => {
		const names = chipNames(render_(seat));
		expect(names).toEqual(['bob', 'carol']);
		expect(names).not.toContain('alice');
	});
});

describe('Clandestinely Liaise: the partner chips and the meeting-peer list', () => {
	const DRAFT = { partner_id: TARGET, preparer_peer_id: 10, partner_peer_id: 11, notes: 'x' };
	const render_ = (seat: number, assets = ASSETS) =>
		render(ClandestinelyLiaisePanel, { props: { ctx: ctxFor(seat, DRAFT, assets), plan: null, mode: 'prep' } }).body;

	it("the preparer's own form: everyone but themself, and their own peers as 'Your meeting peer'", () => {
		const html = render_(PREPARER);
		expect(chipNames(html)).toEqual(['bob', 'carol']);
		const t = text(html);
		expect(t).toContain('Your meeting peer');
		expect(t).toContain('Mara');
		expect(t).not.toContain('Tibor');
	});

	it.each([
		['the partner (bob)', TARGET, 'Tibor'],
		['a bystander (carol)', BYSTANDER, 'Ines'],
	])("a watcher, %s, sees the preparer's chips and the preparer's peers, named as alice's", (_, seat, ownPeer) => {
		const html = render_(seat);
		expect(chipNames(html)).toEqual(['bob', 'carol']);
		const t = text(html);
		expect(t).toContain("alice's meeting peer");
		expect(t).not.toContain('Your meeting peer');
		expect(t).toContain('Mara');
		expect(t).not.toContain(ownPeer);
	});

	it('the empty meeting-peer list names whose peers they are', () => {
		const noAlicePeers = ASSETS.filter(a => a.owner_id !== PREPARER);
		expect(text(render_(TARGET, noAlicePeers))).toContain('alice has no peer to bring to the meeting.');
		expect(text(render_(PREPARER, noAlicePeers))).toContain('You have no peer to bring to the meeting.');
	});
});

describe('Propose Duel: the challenger chips', () => {
	const DRAFT = { target_player_id: TARGET, duel_type: 'arms', notes: 'the square' };
	const render_ = (seat: number) =>
		render(DuelPrepForm, { props: { ctx: ctxFor(seat, DRAFT) } }).body;

	it("the preparer's own chips are everyone but the preparer", () => {
		expect(playerChips(render_(PREPARER))).toEqual(['bob', 'carol']);
	});

	it.each([
		['the challenger (bob)', TARGET],
		['a bystander (carol)', BYSTANDER],
	])("a watcher, %s, is shown the preparer's list: everyone but alice", (_, seat) => {
		const names = playerChips(render_(seat));
		expect(names).toEqual(['bob', 'carol']);
		expect(names).not.toContain('alice');
	});
});

describe('Make War: the enemy chips', () => {
	const DRAFT = { enemy_ids: [TARGET], notes: 'the border' };
	const render_ = (seat: number) =>
		render(WarPrepForm, { props: { ctx: ctxFor(seat, DRAFT) } }).body;

	it("the preparer's own chips are everyone but the preparer", () => {
		expect(playerChips(render_(PREPARER))).toEqual(['bob', 'carol']);
	});

	it.each([
		['the enemy (bob)', TARGET],
		['a bystander (carol)', BYSTANDER],
	])("a watcher, %s, is shown the preparer's list: everyone but alice", (_, seat) => {
		const names = playerChips(render_(seat));
		expect(names).toEqual(['bob', 'carol']);
		expect(names).not.toContain('alice');
	});
});
