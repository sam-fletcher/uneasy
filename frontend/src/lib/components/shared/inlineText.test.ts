import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'svelte/server';
import AssetName from './AssetName.svelte';
import PlayerName from './PlayerName.svelte';
import PlayerNames from './PlayerNames.svelte';
import Quote from './Quote.svelte';
import { declarations, stripComments } from './cssTestKit';
import { renderLogBody } from '$lib/logMarkup';

/*
 * Inline text has three kinds, and each has ONE component that owns its look:
 * a player's name (<PlayerName>), an asset or peer's name (<AssetName>) and a
 * player's own words (<Quote>). This holds the app to that, in the shape of
 * plans/sharedRecipes.test.ts: a source scan that fails a file that writes one
 * by hand, so a panel can't quietly drift back to its own look.
 */

const SRC = join(__dirname, '..', '..', '..'); // frontend/src
function walk(dir: string, out: string[] = []): string[] {
	for (const name of readdirSync(dir)) {
		const full = join(dir, name);
		if (statSync(full).isDirectory()) walk(full, out);
		else if (/\.(svelte|css)$/.test(name)) out.push(full);
	}
	return out;
}
const FILES = walk(SRC);
const rel = (f: string) => f.slice(SRC.length + 1);

/** The markup of a .svelte file: no script, no style, no comments. */
function markupOf(file: string): string {
	return stripComments(readFileSync(file, 'utf8'))
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '');
}
function cssOf(file: string): string {
	const src = readFileSync(file, 'utf8');
	if (file.endsWith('.css')) return src;
	return [...stripComments(src).matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)].map((m) => m[1]).join('\n');
}
const SVELTE = FILES.filter((f) => f.endsWith('.svelte') && !f.endsWith('.test.ts'));

describe('asset names are written by <AssetName>, never by hand', () => {
	// The one `<em>` allowed is the book's title in Help: titles of works are
	// italic as they always are. Everything else that was an `<em>` was either an
	// asset (now <AssetName>) or emphasis, and italic means an asset and nothing
	// else.
	it('has no hand-written <em> outside <AssetName> and the book title', () => {
		const offenders: string[] = [];
		for (const f of SVELTE) {
			if (rel(f) === 'lib/components/shared/AssetName.svelte') continue;
			const ems = [...markupOf(f).matchAll(/<em\b[^>]*>([\s\S]*?)<\/em>/g)].map((m) => m[1].trim());
			const stray = ems.filter(
				(inner) => !(rel(f) === 'lib/components/HelpContent.svelte' && inner === 'Uneasy Lies the Head'),
			);
			if (stray.length) offenders.push(`${rel(f)}: <em>${stray[0]}</em>`);
		}
		expect(offenders, 'use <AssetName> for an asset, plain text for anything else').toEqual([]);
	});

	it('italic is declared only where an asset name is', () => {
		// `font-style: italic` and the `font: italic …` shorthand both count.
		const italic = (d: { prop: string; value: string }) =>
			(d.prop === 'font-style' && /\b(italic|oblique)\b/.test(d.value)) ||
			(d.prop === 'font' && /\b(italic|oblique)\b/.test(d.value));
		const declaring = FILES.filter((f) => declarations(cssOf(f)).some(italic)).map(rel).sort();
		expect(declaring).toEqual([
			// the font-face for italic Spectral
			'app.css',
			// .asset-name itself
			'lib/components/shared/inlineText.css',
		]);
	});
});

describe('player colour on a name is written by <PlayerName>, never by hand', () => {
	// PlayerChips still names its chips through <PlayerName fixed>; the carve-out
	// is for the old inline style, so it is listed here only to say it is allowed.
	const ALLOWED = new Set([
		'lib/components/shared/PlayerName.svelte',
		'lib/components/plans/PlayerChips.svelte',
		// Chat colours bylines and the persona picker its own way: an in-character
		// speaker wears the muted mask-cast of the player's colour, which no
		// component here draws (the style guide's "Player colours").
		'lib/components/ChatPanel.svelte',
	]);
	it('has no inline style:color from playerColor outside the components and PlayerChips', () => {
		const offenders: string[] = [];
		for (const f of SVELTE) {
			if (ALLOWED.has(rel(f))) continue;
			// `style:color=`, `style:--anything=` (the custom-property route round the
			// same thing) and a `style="…"` string, each fed from a player colour.
			const m = markupOf(f).match(
				/style:(?:color|--[\w-]+)=\{[^}]*playerColor(?:ByID)?\(|style="[^"]*(?:\bcolor|--[\w-]+):[^"]*playerColor/,
			);
			if (m) offenders.push(`${rel(f)}: ${m[0]}`);
		}
		expect(offenders, 'a name in a player colour is <PlayerName>').toEqual([]);
	});
});

describe('one definition each', () => {
	it.each(['player-name', 'asset-name', 'quote'])('.%s is styled only in inlineText.css', (name) => {
		const re = new RegExp(`(^|[^\\w-])\\.${name}(?![\\w-])`);
		const declaredIn = FILES.filter((f) => declarations(cssOf(f)).some((d) => re.test(d.selector))).map(rel);
		expect(declaredIn).toEqual(['lib/components/shared/inlineText.css']);
	});

	it('names inside a control take the control colour; a player chip and a disabled control are the exceptions', () => {
		const sheet = declarations(readFileSync(join(__dirname, 'inlineText.css'), 'utf8'));
		const inherit = sheet.filter((d) => d.prop === 'color' && d.value === 'inherit').map((d) => d.selector).join(' ');
		expect(inherit).toContain('.action-btn .player-name');
		expect(inherit).toContain('.chip .player-name:not(.fixed)');
		// fixed (a player chip) keeps its colour... until the chip is unavailable
		expect(inherit).toContain('.chip.unavailable .player-name');
	});

	it('faintness is keyed on .unavailable, never on bare :disabled, which a read-only mirror also gets', () => {
		// A mirror's chips are `disabled` for the browser (and so is every control in a
		// disabled fieldset around one), and a mirror isn't faded. So no rule here may
		// reach a name or a shape through :disabled / [disabled] / [aria-disabled].
		const sheet = declarations(readFileSync(join(__dirname, 'inlineText.css'), 'utf8'));
		const touching = sheet
			.filter((d) => /player-name|player-shape/.test(d.selector))
			.filter((d) => /:disabled|\[disabled|\[aria-disabled/.test(d.selector));
		expect(touching, 'a faint rule keyed on :disabled would fade a read-only mirror').toEqual([]);
		// The shape of an unavailable chip is faint too: it takes the chip's colour.
		expect(sheet).toContainEqual({
			selector: '.chip.unavailable .player-shape path',
			prop: 'fill',
			value: 'currentColor',
		});
		// And an unknown name is grey on its own account, not by inheritance.
		expect(sheet).toContainEqual({
			selector: '.player-name.unknown',
			prop: 'color',
			value: 'var(--player-unknown)',
		});
	});

	it('the log marks use the same classes as the components', () => {
		const html = renderLogBody('**Old Keep** goes to @@1|alice@@', [
			{ id: 1, token_color: null, seat_order: 1 },
		]);
		expect(html).toContain('class="asset-name"');
		expect(html).toContain('class="player-name"');
		expect(html).toContain('--player-name-color: #C5AEFE');
	});
});

describe('the components', () => {
	const alice = { id: 1, display_name: 'alice', token_color: null, seat_order: 1 };
	const bob = { id: 2, display_name: 'bob', token_color: null, seat_order: 2 };

	it('PlayerName is the name in the player colour, with no shape in a sentence', () => {
		const html = render(PlayerName, { props: { player: alice } }).body;
		expect(html).toContain('alice');
		expect(html).toContain('--player-name-color: #C5AEFE');
		expect(html).not.toContain('<svg');
		expect(html).not.toContain('italic');
	});

	it('PlayerName adds the shape only as a label', () => {
		const html = render(PlayerName, { props: { player: bob, label: true } }).body;
		expect(html).toContain('<svg');
		expect(html).toContain('labelled');
		expect(html).toContain('data-shape="triangle"');
	});

	it('PlayerName looks a player up by id', () => {
		const html = render(PlayerName, { props: { players: [alice, bob], id: 2 } }).body;
		expect(html).toContain('bob');
		expect(html).toContain('--player-name-color: #8AD00B');
	});

	it('a player you cannot find is the unknown grey, upright, with no shape, even as a label', () => {
		const html = render(PlayerName, { props: { player: null, name: 'Unknown', label: true } }).body;
		expect(html).toContain('Unknown');
		expect(html).toContain('unknown');
		expect(html).not.toContain('--player-name-color');
		expect(html).not.toContain('<svg');
		expect(html).not.toContain('labelled');
	});

	it('PlayerName keeps "fixed" for a player chip', () => {
		expect(render(PlayerName, { props: { player: alice, fixed: true } }).body).toContain('fixed');
	});

	it('PlayerNames lists names, each in its own colour', () => {
		const html = render(PlayerNames, { props: { players: [alice, bob], ids: [1, 2] } }).body;
		expect(html).toContain('alice');
		expect(html).toContain('bob');
		expect(html.match(/class="player-name/g)).toHaveLength(2);
		expect(html).toContain(', ');
	});

	it('AssetName is an <em> and takes the surrounding colour', () => {
		const html = render(AssetName, { props: { name: 'Old Keep' } }).body;
		expect(html).toMatch(/<em class="asset-name">Old Keep<\/em>/);
		expect(html).not.toContain('--player-name-color');
		expect(render(AssetName, { props: { assets: [{ id: 3, name: 'The Crown' }], id: 3 } }).body).toContain('The Crown');
		expect(render(AssetName, { props: { assets: [], id: 3 } }).body).toContain('>?<');
	});

	it('Quote keeps curly marks and parses nothing', () => {
		const html = render(Quote, { props: { text: '**Old Keep** and @@1|alice@@ <b>x</b>' } }).body;
		expect(html).toContain('“**Old Keep** and @@1|alice@@ &lt;b>x&lt;/b>”');
		expect(html).not.toContain('<em');
		expect(html).not.toContain('player-name');
	});
});
