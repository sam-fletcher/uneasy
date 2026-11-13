import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import LawsRumors from './LawsRumors.svelte';
import { declarations, styleOf } from './shared/cssTestKit';
import type { Asset, Law, Plan, Player, Rumor } from '$lib/api';

/*
 * The Laws and Rumors lists, rendered through svelte/server (no jsdom). Pinned:
 *   - the "new" dot starts exactly the entries above the marker it was opened with;
 *   - a hidden source reads "Unknown", faint and italic, with no shape, while every
 *     known name carries its player's shape;
 *   - only the writer gets the Edit button;
 *   - the entries are plain text between lines, not cards.
 * What a render can't show, the dot's staying while the page moves the marker,
 * belongs to the e2e spec (public_record.spec.ts).
 */

const SOURCE = readFileSync(join(__dirname, 'LawsRumors.svelte'), 'utf8');

function player(id: number, name: string, seat: number): Player {
	return {
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: '2026-08-16T00:00:00Z',
		is_facilitator: false,
		token_color: null,
		seat_order: seat,
	};
}
const ash = player(1, 'Ash', 1); // diamond
const wren = player(2, 'Wren', 2); // triangle
const otto = player(3, 'otto', 3); // square
const players = [ash, wren, otto];
const playerNames = new Map(players.map((p) => [p.id, p.display_name]));

const law = (id: number, rev: number, over: Partial<Law> = {}): Law => ({
	id,
	game_id: 1,
	text: `Law number ${id}`,
	addendum: null,
	origin_plan_id: null,
	signatory_id: 3,
	created_at: '',
	is_active: true,
	display_order: id,
	rev,
	...over,
});
const rumor = (id: number, rev: number, over: Partial<Rumor> = {}): Rumor => ({
	id,
	game_id: 1,
	text: `Rumor number ${id}`,
	target_asset_id: null,
	origin_plan_id: null,
	source_player_id: 2,
	is_active: true,
	created_at: '',
	display_order: id,
	rev,
	...over,
});
const plan = (id: number, preparer_id: number): Plan =>
	({ id, preparer_id, plan_type: 'propose_decree' }) as Plan;
const asset = (id: number, owner_id: number, name: string): Asset =>
	({ id, owner_id, name, is_destroyed: false }) as Asset;

function draw(over: Record<string, unknown> = {}) {
	const { body } = render(LawsRumors, {
		props: {
			kind: 'laws',
			laws: [],
			rumors: [],
			plans: [],
			players,
			playerNames,
			currentPlayerID: 1,
			...over,
		},
	});
	return body;
}

const count = (html: string, cls: string) =>
	html.match(new RegExp(`class="${cls}(?: [^"]*)?"`, 'g'))?.length ?? 0;
/** The entries (`<li class="lr-item">`) of the markup. */
const items = (html: string) => html.match(/<li class="lr-item[\s\S]*?<\/li>/g) ?? [];

describe('the "new" dot', () => {
	const laws = [law(1, 3), law(2, 7), law(3, 9)];

	it('starts exactly the entries above the marker the list opened with', () => {
		const html = draw({ laws, seenLaws: 3 });
		const [a, b, c] = items(html);
		expect(a).not.toContain('new-dot');
		expect(b).toContain('new-dot');
		expect(c).toContain('new-dot');
	});

	it('is not drawn at all on a list that is up to date', () => {
		expect(count(draw({ laws, seenLaws: 9 }), 'new-dot')).toBe(0);
	});

	it('is not drawn before the marker has loaded, rather than lighting every entry', () => {
		expect(count(draw({ laws, seenLaws: null }), 'new-dot')).toBe(0);
		expect(count(draw({ laws }), 'new-dot')).toBe(0);
	});

	it('counts an edit as new: a revision above the marker is new, whoever it was first', () => {
		// Law 1 was seen at rev 3; an edit gives it rev 12, above a marker of 9.
		const html = draw({ laws: [law(1, 12)], seenLaws: 9 });
		expect(count(html, 'new-dot')).toBe(1);
	});

	it('takes the marker of the list on screen, never the other one', () => {
		const html = draw({ kind: 'rumors', rumors: [rumor(1, 5)], seenLaws: 0, seenRumors: 5 });
		expect(count(html, 'new-dot')).toBe(0);
		const html2 = draw({ kind: 'rumors', rumors: [rumor(1, 5)], seenLaws: 99, seenRumors: 4 });
		expect(count(html2, 'new-dot')).toBe(1);
	});

	it('is the shared pale dot, which says "new" to a screen reader', () => {
		const html = draw({ laws: [law(1, 7)], seenLaws: 0 });
		expect(html).toContain('role="img" aria-label="new"');
	});
});

describe('a byline', () => {
	it('carries each name\'s shape: the proposer and the signatory on a law', () => {
		const html = draw({
			laws: [law(1, 1, { origin_plan_id: 10, signatory_id: 3 })],
			plans: [plan(10, 2)],
		});
		const byline = html.match(/<div class="lr-bylines[\s\S]*?<\/div>/)?.[0] ?? '';
		expect(byline).toContain('Proposed by');
		expect(byline).toContain('Signed by');
		expect(byline).toContain('data-shape="triangle"'); // Wren proposed
		expect(byline).toContain('data-shape="square"'); // otto signed
		expect(byline).toContain('>Wren<');
		expect(byline).toContain('>otto<');
	});

	it('leaves out "Proposed by" for a law with no origin plan', () => {
		const html = draw({ laws: [law(1, 1, { origin_plan_id: null, signatory_id: 3 })] });
		expect(html).not.toContain('Proposed by');
		expect(html).toContain('Signed by');
	});

	it('names a rumor\'s known source with their shape', () => {
		const html = draw({ kind: 'rumors', rumors: [rumor(1, 1, { source_player_id: 2 })] });
		expect(html).toContain('Spread by');
		expect(html).toContain('data-shape="triangle"');
		expect(html).not.toContain('Unknown');
	});

	it('reads "Unknown" for a hidden source: faint italics, with no shape', () => {
		const html = draw({ kind: 'rumors', rumors: [rumor(1, 1, { source_player_id: null })] });
		const byline = html.match(/<div class="lr-bylines[\s\S]*?<\/div>/)?.[0] ?? '';
		expect(byline).toContain('Spread by');
		expect(byline).toMatch(/class="lr-unknown[^"]*">Unknown</);
		expect(byline).not.toContain('data-shape');
		// The style is faint and italic, so a hidden source reads as an absence.
		const decls = declarations(styleOf(SOURCE)).filter((d) => d.selector === '.lr-unknown');
		expect(decls).toContainEqual({ selector: '.lr-unknown', prop: 'color', value: 'var(--color-text-faint)' });
		expect(decls).toContainEqual({ selector: '.lr-unknown', prop: 'font-style', value: 'italic' });
	});
});

describe('a rumor\'s "About" line', () => {
	it('names the asset with the shape of the player who owns it', () => {
		const html = draw({
			kind: 'rumors',
			rumors: [rumor(1, 1, { target_asset_id: 50 })],
			assets: [asset(50, 3, 'Lady Merrow')],
		});
		const about = html.match(/<div class="lr-about[\s\S]*?<\/div>/)?.[0] ?? '';
		expect(about).toContain('About');
		expect(about).toContain('Lady Merrow');
		expect(about).toContain('data-shape="square"'); // otto owns it
		// The shape stands alone, so it speaks for a screen reader.
		expect(about).toContain('aria-label="otto’s"');
	});

	it('is left out when the rumor is about nothing, or its asset is not known', () => {
		expect(draw({ kind: 'rumors', rumors: [rumor(1, 1)], assets: [asset(50, 3, 'x')] })).not.toContain('About');
		expect(
			draw({ kind: 'rumors', rumors: [rumor(1, 1, { target_asset_id: 99 })], assets: [asset(50, 3, 'x')] })
		).not.toContain('About');
	});
});

describe('Edit', () => {
	it('is a bordered 44px button (the house secondary) on your own law', () => {
		const html = draw({ laws: [law(1, 1, { signatory_id: 1 })], currentPlayerID: 1 });
		expect(html).toMatch(/<button class="action-btn secondary lr-edit[^"]*"[^>]*>Edit<\/button>/);
	});

	it('is on a law you proposed (its origin plan\'s preparer) as well as one you signed', () => {
		const html = draw({
			laws: [law(1, 1, { origin_plan_id: 10, signatory_id: 3 })],
			plans: [plan(10, 1)],
			currentPlayerID: 1,
		});
		expect(html).toContain('>Edit<');
	});

	it('is not on a law you neither proposed nor signed', () => {
		const html = draw({
			laws: [law(1, 1, { origin_plan_id: 10, signatory_id: 3 })],
			plans: [plan(10, 2)],
			currentPlayerID: 1,
		});
		expect(html).not.toContain('>Edit<');
	});

	it('is on a rumor you spread, not on one someone else did', () => {
		const mine = draw({ kind: 'rumors', rumors: [rumor(1, 1, { source_player_id: 1 })], currentPlayerID: 1 });
		const theirs = draw({ kind: 'rumors', rumors: [rumor(1, 1, { source_player_id: 2 })], currentPlayerID: 1 });
		expect(mine).toContain('>Edit<');
		expect(theirs).not.toContain('>Edit<');
	});

	it('is on a rumor whose source is hidden only for the player who prepared it', () => {
		const rumors = [rumor(1, 1, { source_player_id: null, origin_plan_id: 10 })];
		const preparer = draw({ kind: 'rumors', rumors, plans: [plan(10, 1)], currentPlayerID: 1 });
		const bystander = draw({ kind: 'rumors', rumors, plans: [plan(10, 1)], currentPlayerID: 3 });
		expect(preparer).toContain('>Edit<');
		expect(bystander).not.toContain('>Edit<');
	});

	it('is never offered to anyone when there is no current player', () => {
		const html = draw({ laws: [law(1, 1, { signatory_id: 1 })], currentPlayerID: null });
		expect(html).not.toContain('>Edit<');
	});

	it('is no longer the small "edit" text link', () => {
		const html = draw({ laws: [law(1, 1, { signatory_id: 1 })], currentPlayerID: 1 });
		expect(html).not.toContain('>edit<');
		const decls = declarations(styleOf(SOURCE));
		expect(decls.some((d) => d.selector === '.lr-edit' && d.prop === 'text-decoration')).toBe(false);
	});
});

describe('what the list says', () => {
	it('shows its own total, in words, and only for the list on screen', () => {
		const html = draw({ laws: [law(1, 1), law(2, 2)], rumors: [rumor(1, 1)] });
		expect(html).toContain('2 laws');
		expect(html).toContain('Law number 1');
		expect(html).not.toContain('Rumor number 1');
		expect(draw({ kind: 'rumors', laws: [law(1, 1)], rumors: [rumor(1, 1)] })).toContain('1 rumor<');
		expect(draw({ laws: [law(1, 1)] })).toContain('1 law<');
	});

	it('says so when there is nothing yet', () => {
		expect(draw()).toContain('No laws yet.');
		expect(draw({ kind: 'rumors' })).toContain('No rumors yet.');
	});

	it('puts a law\'s addendum under its text, in italics, after an ellipsis', () => {
		const html = draw({ laws: [law(1, 1, { addendum: 'unless it flies the crown’s colours' })] });
		expect(html).toMatch(/class="lr-addendum[^"]*">…unless it flies the crown’s colours</);
	});
});

describe('the stylesheet', () => {
	const css = styleOf(SOURCE);
	const decls = declarations(css);

	// Boxes are for controls and these entries are only read, so the only boxed
	// thing in a list is the Edit button, which comes from the shared recipe.
	it('draws an entry as plain text between lines: no fill, no box, no tint', () => {
		const item = decls.filter((d) => /^\.lr-item\b/.test(d.selector));
		const props = item.map((d) => d.prop);
		expect(props).not.toContain('background');
		expect(props).not.toContain('background-color');
		expect(props).not.toContain('border-radius');
		expect(props).not.toContain('border-left');
		expect(item).toContainEqual({
			selector: '.lr-item',
			prop: 'border-bottom',
			value: '1px solid var(--color-border)',
		});
	});

	it('keeps each byline row at the touch floor, by token', () => {
		expect(decls).toContainEqual({ selector: '.lr-bylines', prop: 'min-height', value: 'var(--size-tap)' });
	});

	it('never takes a card colour from the author', () => {
		expect(css).not.toContain('--accent');
		expect(css).not.toContain('color-mix');
	});

	it('writes no colour literal', () => {
		expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
		expect(css).not.toMatch(/\brgba?\(/);
	});
});
