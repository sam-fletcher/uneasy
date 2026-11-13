import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import AssetCardSelectable from './AssetCardSelectable.svelte';
import RetinueView from './RetinueView.svelte';
import Notice from './shared/Notice.svelte';
import StarMark from './shared/StarMark.svelte';
import ToneMark from './shared/ToneMark.svelte';
import { seatForColor } from '$lib/playerColor';
import { declarations, stripComments, styleOf } from './shared/cssTestKit';
import type { Asset, Marginalium, Player } from '$lib/api';

/*
 * The retinue and the game pieces: asset cards, marginalia tiles, the retinue
 * sheet. What a player sees is checked by what the components emit (rendered
 * through svelte/server, as the other component tests are; there is no DOM in
 * this suite). The rules that live only in a stylesheet are checked in the
 * stylesheet, read with comments stripped so prose that names a property can't
 * fail or hide anything.
 */

const read = (rel: string) => readFileSync(join(__dirname, rel), 'utf8');
const html = (component: unknown, props: Record<string, unknown> = {}) =>
	render(component as never, { props } as never).body;
/** Rendered text, with Svelte's hydration comments and the whitespace folded away. */
const text = (body: string) =>
	body.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, ' ').replace(/\s+/g, ' ').trim();

// ── fixtures ────────────────────────────────────────────────────────────────

function asset(id: number, over: Partial<Asset> = {}, notes: string[] = ['a note'], tornAt: number[] = []): Asset {
	const marginalia: Marginalium[] = notes.map((t, i) => ({
		id: id * 10 + i, asset_id: id, position: i + 1, text: t,
		is_torn: tornAt.includes(i + 1), torn_at: null, torn_by_id: null, title: null,
	}));
	return {
		id, game_id: 1, owner_id: 2, creator_id: 2, asset_type: 'holding', name: `Asset ${id}`,
		is_main_character: false, is_leveraged: false, is_destroyed: false,
		created_at: '', destroyed_at: null, linked_card_suit: null, linked_card_value: null,
		marginalia, secret_count: 0, ...over,
	};
}

const players = [
	{ id: 1, display_name: 'otto', seat_order: 1, token_color: null },
	{ id: 2, display_name: 'wren', seat_order: 2, token_color: null },
] as unknown as Player[];

function retinue(assets: Asset[], over: Record<string, unknown> = {}): string {
	return html(RetinueView, {
		playerId: 2, players, members: [], assets, viewerPlayerId: 1, ...over,
	});
}

const PERIDOT = '#8AD00B'; // seat 2
const UNKNOWN_GREY = '#8a8a8a';

// ── seat from colour ────────────────────────────────────────────────────────

describe('seatForColor', () => {
	it('finds the seat a palette colour belongs to, whatever its case', () => {
		expect(seatForColor('#C5AEFE')).toBe(1);
		expect(seatForColor('#8ad00b')).toBe(2);
		expect(seatForColor('#06AEAB')).toBe(5);
	});

	it('knows nothing about a colour that is not a seat colour, so no shape is drawn for nobody', () => {
		expect(seatForColor(UNKNOWN_GREY)).toBeNull();
		expect(seatForColor('')).toBeNull();
		expect(seatForColor(null)).toBeNull();
	});
});

// ── the asset card ──────────────────────────────────────────────────────────

describe('AssetCardSelectable', () => {
	const card = (props: Record<string, unknown> = {}) =>
		html(AssetCardSelectable, { asset: asset(7), ownerColor: PERIDOT, ...props });

	it("marks whose it is with the owner's shape, not a coloured edge", () => {
		const body = card();
		expect(body).toContain('data-shape="triangle"');
		expect(body).toContain('fill="#8AD00B"');
	});

	it('draws no shape for an owner nobody can resolve (the grey fallback)', () => {
		expect(card({ ownerColor: UNKNOWN_GREY })).not.toContain('data-shape');
	});

	it('says "Leveraged" in words beside the die, and only when it is', () => {
		const on = card({ asset: asset(7, { is_leveraged: true }) });
		expect(text(on)).toContain('Leveraged');
		expect(on).toMatch(/class="[^"]*\bstatus-word\b[^"]*\blev-word\b/);
		expect(text(card())).not.toContain('Leveraged');
	});

	it('shows a tick, and the checkbox state, only when selected', () => {
		const on = card({ selectable: true, selected: true });
		expect(on).toContain('aria-checked="true"');
		expect(on).toMatch(/class="tone-mark[ "]/);
		expect(on).toMatch(/class="[^"]*\bselected\b/);
		const off = card({ selectable: true, selected: false });
		expect(off).toContain('aria-checked="false"');
		expect(off).not.toMatch(/class="tone-mark[ "]/);
		expect(off).not.toMatch(/class="card[^"]*\bselected\b/);
	});

	it('shows a torn note with the house tear mark and a label, not a typed cross', () => {
		const body = card({
			asset: asset(7, {}, ['kept', 'lost'], [2]),
			defaultExpanded: true,
		});
		expect(body).toContain('aria-label="torn"');
		expect(body).not.toContain('✗');
	});

	it('draws the leverage toggle as a chip with the house die, not the emoji', () => {
		const body = card({ leverageMode: true, leverageDrafted: true });
		expect(body).toMatch(/class="lev-tap chip[^"]*\bselected\b/);
		expect(body).toContain('aria-pressed="true"');
		expect(body).toMatch(/class="die-mark[ "]/);
		expect(body).not.toContain('🎲');
	});

	it('a drafted die is only a chip that is not selected until it is', () => {
		const body = card({ leverageMode: true });
		expect(body).toMatch(/class="lev-tap chip svelte-[\w-]+"/);
		expect(body).toContain('aria-pressed="false"');
	});

	it('carries no main-character star as a typed character', () => {
		const body = card({ asset: asset(7, { is_main_character: true, asset_type: 'peer' }) });
		expect(body).toContain('aria-label="Main character"');
		expect(body).not.toContain('★');
	});
});

describe('ToneMark', () => {
	const PATHS = {
		warning: 'M12 3.5L21.5 20H2.5zM12 10v4.5M12 17.5v0.01',
		danger: 'M8.5 2.5h7L21.5 8.5v7l-6 6h-7l-6-6v-7zM12 7.5v5.5M12 16.5v0.01',
		success: 'M5 12.5l4.5 4.5L19 7.5',
		info: 'M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM12 11v6M12 7.5v0.01',
	} as const;

	it('draws each tone once: a notice takes its default mark from here', () => {
		const text = createRawSnippet(() => ({ render: () => '<span>x</span>' }));
		for (const tone of ['warning', 'danger', 'success', 'info'] as const) {
			expect(html(ToneMark, { tone }), tone).toContain(PATHS[tone]);
			const notice = html(Notice, { tone, children: text });
			expect(notice, `notice ${tone}`).toContain(PATHS[tone]);
			expect(notice, `notice ${tone}`).toMatch(new RegExp(`class="tone-mark[ "][^>]*data-tone="${tone}"`));
		}
	});

	it('is the tick a selected card shows, the same drawing as a success notice', () => {
		const card = html(AssetCardSelectable, {
			asset: asset(7), ownerColor: PERIDOT, selectable: true, selected: true,
		});
		expect(card).toContain(PATHS.success);
		expect(card).toMatch(/class="tone-mark[ "][^>]*data-tone="success"/);
	});

	it('is decoration unless it is given a label, and takes its colour from the caller', () => {
		const body = html(ToneMark, { tone: 'success' });
		expect(body).toContain('aria-hidden="true"');
		expect(body).toContain('stroke="currentColor"');
		expect(html(ToneMark, { tone: 'success', label: 'picked' })).toContain('aria-label="picked"');
	});
});

describe('StarMark', () => {
	it('is solid for the main character and an outline for the control that makes one', () => {
		expect(html(StarMark)).toContain('fill="currentColor"');
		const outline = html(StarMark, { outline: true });
		expect(outline).toContain('fill="none"');
		expect(outline).toContain('stroke="currentColor"');
	});

	it('is decoration unless it is given a label', () => {
		expect(html(StarMark)).toContain('aria-hidden="true"');
		expect(html(StarMark, { label: 'Main character' })).toContain('aria-label="Main character"');
	});
});

// ── the retinue sheet ───────────────────────────────────────────────────────

describe('RetinueView: the at-risk count lives here', () => {
	const atRisk = (id: number) => asset(id, {}, ['one note']); // 1 intact, 3 free slots
	const safe = (id: number) => asset(id, {}, ['a', 'b', 'c', 'd']);

	it('says how many assets need marginalia, to their owner, in words', () => {
		// Fixture assets belong to player 2; make them player 1's and look as player 1.
		const mine = text(
			retinue([atRisk(1), atRisk(2), safe(3)].map((a) => ({ ...a, owner_id: 1 })), {
				playerId: 1, viewerPlayerId: 1,
			}),
		);
		expect(mine).toContain('2 assets need marginalia');
		expect(mine).toContain('one break could destroy them');
	});

	it("tells everyone else, naming the owner, and in the singular when it's one", () => {
		const body = text(retinue([atRisk(1), safe(2)]));
		expect(body).toContain('1 asset has too few marginalia');
		expect(body).toContain('wren can still shore it up');
	});

	it('is a danger notice with the tear mark, so red is never alone', () => {
		const body = retinue([atRisk(1)]);
		expect(body).toContain('data-tone="danger"');
		expect(text(body)).toContain('too few marginalia');
	});

	it('is absent when nothing is at risk, and ignores destroyed assets', () => {
		expect(text(retinue([safe(1)]))).not.toContain('too few marginalia');
		const dead = asset(2, { is_destroyed: true }, ['lost'], [1]);
		expect(text(retinue([safe(1), dead]))).not.toContain('too few marginalia');
	});
});

describe('RetinueView: status is words, not badges', () => {
	it('says the type as a status word, and no longer draws it as a chip', () => {
		const body = retinue([asset(1, { asset_type: 'artifact' })]);
		expect(body).toMatch(/class="status-word"[^>]*>Artifact</);
		expect(body).not.toContain('class="asset-type"');
	});

	it('says "Leveraged" under a leveraged asset and counts them in the header', () => {
		const body = retinue([asset(1, { is_leveraged: true }), asset(2, { is_leveraged: true }), asset(3)]);
		expect(text(body)).toContain('2 leveraged assets');
		expect(body.match(/>\s*(?:<[^>]+>\s*)*Leveraged</g)?.length).toBeGreaterThanOrEqual(2);
	});

	it('says "Destroyed" in red on a tombstone, and draws no cross over it', () => {
		const body = retinue([asset(1, { is_destroyed: true, destroyed_at: '2026-11-01' }, ['lost'], [1])]);
		expect(body).toMatch(/class="status-word danger"[^>]*>Destroyed</);
		expect(body).toMatch(/class="[^"]*\basset-tile\b[^"]*\bdestroyed\b/);
		expect(body).not.toContain('tombstone-x');
	});

	it('says "Sets next scene" for the focus player, as a word', () => {
		expect(text(retinue([asset(1)], { focusPlayerId: 2 }))).toContain('Sets next scene');
		expect(text(retinue([asset(1)], { focusPlayerId: 1 }))).not.toContain('Sets next scene');
	});

	it("wears the owner's shape beside the title, once", () => {
		const body = retinue([asset(1), asset(2)]);
		expect(body).toContain('data-shape="triangle"');
		expect(body.match(/data-shape=/g)).toHaveLength(1);
		expect(text(body)).toContain("wren's Retinue");
	});
});

// ── the stylesheets ─────────────────────────────────────────────────────────

describe('the pieces are drawn from the piece tokens', () => {
	const tile = declarations(read('shared/marginaliaTile.css'));
	const card = declarations(styleOf(read('AssetCardSelectable.svelte')));
	const retinueCss = declarations(styleOf(read('RetinueView.svelte')));
	const value = (decls: ReturnType<typeof declarations>, selector: string, prop: string) =>
		decls.find((d) => d.selector === selector && d.prop === prop)?.value;

	it('a marginalia tile is a piece: warm border, piece corner, raised surface', () => {
		expect(value(tile, '.m-tile', 'border')).toBe('1px solid var(--color-border-piece)');
		expect(value(tile, '.m-tile', 'border-radius')).toBe('var(--radius-piece)');
		expect(value(tile, '.m-tile', 'background')).toBe('var(--color-surface)');
	});

	it('an asset card and an asset tile are pieces too', () => {
		expect(value(card, '.card', 'border')).toBe('1px solid var(--color-border-piece)');
		expect(value(card, '.card', 'border-radius')).toBe('var(--radius-piece)');
		expect(value(card, '.card', 'background')).toBe('var(--color-surface)');
		expect(value(retinueCss, '.asset-tile', 'border')).toBe('1px solid var(--color-border-piece)');
		expect(value(retinueCss, '.asset-tile', 'border-radius')).toBe('var(--radius-piece)');
		expect(value(retinueCss, '.asset-tile', 'background')).toBe('var(--color-surface)');
	});

	it('selected is the bright gold border and nothing is filled', () => {
		const selectors = card.filter((d) => d.selector.includes('.selected')).map((d) => d.selector);
		expect(selectors.length).toBeGreaterThan(0);
		for (const sel of new Set(selectors)) {
			expect(value(card, sel, 'border-color'), sel).toBe('var(--color-you)');
			expect(value(card, sel, 'background'), `${sel} must not fill`).toBeUndefined();
		}
		// The picked marginalia line in pick mode has its own selected rule: the
		// card holding it takes the same border.
		expect(card.some((d) => d.selector.includes(':has(.marginalia li.picked)') && d.prop === 'border-color')).toBe(true);
	});

	it("the owner's colour never reaches a card's border or its selected state", () => {
		const source = stripComments(read('AssetCardSelectable.svelte'));
		expect(source).not.toContain('--owner-color');
	});

	it('nothing is faded with opacity or desaturated: that takes text under the contrast floor', () => {
		for (const [name, decls] of [
			['marginaliaTile.css', tile],
			['AssetCardSelectable', card],
			['RetinueView', retinueCss],
		] as const) {
			expect(decls.filter((d) => d.prop === 'opacity' || (d.prop === 'filter' && /grayscale/.test(d.value))), name).toEqual([]);
		}
	});

	it('a torn tile is struck through in faint words, with the line in red', () => {
		expect(value(tile, '.m-tile.torn', 'text-decoration')).toBe('line-through');
		expect(value(tile, '.m-tile.torn', 'color')).toBe('var(--color-text-faint)');
		expect(value(tile, '.m-tile.torn', 'text-decoration-color')).toBe('var(--color-danger-muted)');
	});

	it('none of these files reaches for a retiring token', () => {
		const retiring = [
			'--color-surface-sunken', '--color-surface-active', '--color-border-warm', '--color-border-strong',
			'--color-border-warm-hover', '--color-border-warm-strong', '--color-text-muted',
			'--color-text-tertiary-warm', '--color-text-faint-warm', '--color-accent-bright',
			'--radius-sm', '--radius-md', '--radius-lg',
		];
		const files = [
			'shared/marginaliaTile.css', 'AssetCardSelectable.svelte', 'RetinueView.svelte',
			'AssetCreationForm.svelte', 'MainCharacterChoicePanel.svelte', 'SuggestionPicker.svelte',
		];
		for (const f of files) {
			const source = stripComments(read(f));
			for (const token of retiring) {
				expect(source.includes(`${token})`) || source.includes(`${token},`), `${f} uses ${token}`).toBe(false);
			}
		}
	});

	it('marks are drawn from paths: no typed star, caret or die in these files', () => {
		for (const f of ['AssetCardSelectable.svelte', 'RetinueView.svelte']) {
			// Markup and styles only: a `//` comment in a script may name a glyph.
			const source = stripComments(read(f)).replace(/<script[\s\S]*?<\/script>/g, '');
			for (const glyph of ['★', '☆', '▾', '▸', '🎲', '✗']) {
				expect(source.includes(glyph), `${f} contains ${glyph}`).toBe(false);
			}
		}
	});

	it('the checkbox is a control, so it takes the control corner; the card itself keeps the piece corner', () => {
		expect(value(card, '.select-tap', 'border-radius')).toBe('var(--radius-control)');
		expect(value(card, '.card', 'border-radius')).toBe('var(--radius-piece)');
	});

	it('the placeholder prose class is .empty-note, never a bare .empty that collides with the tile slot', () => {
		expect(card.filter((d) => /(^|[\s.])\.empty(\s|\{|$)/.test(d.selector))).toEqual([]);
		expect(html(AssetCardSelectable, { asset: asset(7, {}, []), ownerColor: PERIDOT, defaultExpanded: true }))
			.toMatch(/class="empty-note[ "]/);
	});

	it('the retinue counts at-risk assets with the helper the player row uses, so they cannot disagree', () => {
		const source = stripComments(read('RetinueView.svelte'));
		expect(source).toContain("from '$lib/tableHeader'");
		expect(source).toContain('atRiskCountByPlayer(');
		expect(source).not.toMatch(/filter\([^)]*isNeedlesslyAtRisk[^)]*\)\.length/);
	});

	it('every pressable thing on the tile head is at the touch minimum', () => {
		expect(value(retinueCss, '.ctl', 'min-width')).toBe('var(--size-tap)');
		expect(value(retinueCss, '.ctl', 'min-height')).toBe('var(--size-tap)');
	});
});
