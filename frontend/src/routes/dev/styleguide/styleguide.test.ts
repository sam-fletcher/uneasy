import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { render } from 'svelte/server';
import Page from './+page.svelte';
import { PLAYER_SHAPES } from '$lib/playerShape';
import {
	BORDERS,
	GAPS,
	GOLDS,
	MEANING,
	RADII,
	SURFACES,
	TEXT_DISPLAY,
	TEXT_SHADES,
	TEXT_SIZES,
	TOUCH,
} from './tokens';

/*
 * /dev/styleguide shows every token and every shared piece, drawn by the real
 * components, with values read from the browser. What can still drift is the
 * LIST of tokens it knows to show, and whether the page stays hidden from the
 * app and from crawlers; those are what these tests pin.
 */

const SRC = join(__dirname, '..', '..', '..');
const appCss = readFileSync(join(SRC, 'app.css'), 'utf8');

/** Every custom property DEFINED in app.css (declared, not just used). */
const defined = new Set([...appCss.matchAll(/^\s*(--[a-z0-9-]+)\s*:/gm)].map((m) => m[1]));

describe('the styleguide covers every scale token', () => {
	// The three scales are closed: five sizes, six gaps, five radii (the three
	// size-named radii are retiring and not shown).
	const scale = (prefix: string, skip: string[] = []) =>
		[...defined].filter((n) => n.startsWith(prefix) && !skip.includes(n)).sort();

	it('text sizes', () => {
		const shown = [...TEXT_SIZES, TEXT_DISPLAY].map((t) => t.name).sort();
		expect(shown).toEqual(scale('--text-'));
	});

	it('gaps', () => {
		expect(GAPS.map((t) => t.name).sort()).toEqual(scale('--space-'));
	});

	it('radii', () => {
		expect(RADII.map((t) => t.name).sort()).toEqual(
			scale('--radius-', ['--radius-sm', '--radius-md', '--radius-lg'])
		);
	});

	it('the touch floor', () => {
		expect([TOUCH.name]).toEqual(scale('--size-'));
	});

	it('shows only tokens that exist (a typo would silently measure as nothing)', () => {
		const all = [...SURFACES, ...TEXT_SHADES, ...BORDERS, ...GOLDS, ...MEANING];
		const missing = all.map((t) => t.name).filter((n) => !defined.has(n));
		expect(missing).toEqual([]);
	});
});

describe('the page', () => {
	const { body, head } = render(Page, { props: {} });

	it('tells crawlers to stay away', () => {
		expect(head).toContain('name="robots"');
		expect(head).toContain('noindex');
	});

	it('draws every player shape from the real component', () => {
		for (const shape of PLAYER_SHAPES) {
			expect(body).toContain(`data-shape="${shape.name}"`);
		}
	});

	it('has a section for each thing the brief lists', () => {
		for (const id of [
			'type', 'gaps', 'corners', 'surfaces', 'text', 'borders', 'gold', 'meaning', 'players',
			'marks', 'buttons', 'chips', 'rows', 'status', 'notices', 'sheets', 'pieces',
		]) {
			expect(body, `section #${id}`).toContain(`id="${id}"`);
		}
	});

	it('renders the real pieces, not look-alikes', () => {
		// Each of these class names is emitted by a shared component, so seeing
		// it here means the page imported the component rather than restyling.
		for (const marker of [
			'class="clock-mark', 'class="new-dot', 'class="die-mark', 'class="barbute-mark',
			'class="player-shape', 'class="row-mark', 'class="notice', 'checklist-row',
			'table-roster', 'class="chip', 'class="star-mark', 'class="tone-mark', 'class="m-tile', 'class="card ',
		]) {
			expect(body, marker).toContain(marker);
		}
	});

	it('holds no game data or rule text: no hex colour literal anywhere in the page source', () => {
		const sources = [
			'+page.svelte', 'Section.svelte', 'Cell.svelte', 'tokens.ts',
		].map((f) => readFileSync(join(__dirname, f), 'utf8'));
		for (const s of sources) expect(s).not.toMatch(/#[0-9a-fA-F]{6}\b/);
	});
});

describe('nothing in the app links to the styleguide', () => {
	function* walk(dir: string): Generator<string> {
		for (const e of readdirSync(dir, { withFileTypes: true })) {
			const p = join(dir, e.name);
			if (e.isDirectory()) yield* walk(p);
			else if (/\.(svelte|ts|css|html)$/.test(e.name)) yield p;
		}
	}

	it('no other source file names the route', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			if (rel.startsWith(join('routes', 'dev', 'styleguide'))) continue;
			// Tests may name the route (this one does); the app may not.
			if (/\.test\.ts$/.test(rel)) continue;
			if (readFileSync(file, 'utf8').includes('dev/styleguide')) offenders.push(rel);
		}
		expect(offenders, 'the styleguide ships but must not be linked from the app').toEqual([]);
	});
});
