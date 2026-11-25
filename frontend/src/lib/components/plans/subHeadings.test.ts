import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';
import { render } from 'svelte/server';
import type { Player } from '$lib/api';
import { declarations, stripComments } from '$lib/components/shared/cssTestKit';
import SetPiece from './shared/SetPiece.svelte';
import QuotedText from './shared/QuotedText.svelte';
import { createRawSnippet } from 'svelte';

/*
 * Round 6, "Inside a step": a sub-heading is a small deep-gold label that names one
 * part of the step, once. These hold the plan panels to it, in the shape of
 * headingSaysIt.test.ts: a source scan that names the file when a panel writes a
 * sub-heading the old way (a colon, a question mark, a count), plus the look itself.
 */

const HERE = __dirname;
function svelteFiles(dir: string): string[] {
	return readdirSync(dir).flatMap((name) => {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) return svelteFiles(path);
		return name.endsWith('.svelte') ? [path] : [];
	});
}
const markupOf = (path: string) =>
	stripComments(readFileSync(path, 'utf8'))
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '');

describe('a sub-heading is a small deep-gold label', () => {
	const css = declarations(readFileSync(join(HERE, 'planPanel.css'), 'utf8'));
	const of = (selector: string) => Object.fromEntries(css.filter((d) => d.selector === selector).map((d) => [d.prop, d.value]));

	it('is 12px, uppercase, letter-spaced and deep gold', () => {
		expect(of('.choices-header')).toMatchObject({
			'font-size': 'var(--text-label)',
			'text-transform': 'uppercase',
			'letter-spacing': '0.1em',
			color: 'var(--color-accent)',
		});
	});

	it('leaves a field\'s prompt a 14px secondary label, not a heading', () => {
		expect(of('.picker-label')).toMatchObject({
			'font-size': 'var(--text-small)',
			color: 'var(--color-text-secondary)',
		});
	});

	it('draws no hairline between the sub-steps of an open step', () => {
		// The step's groups are told apart by space (the group gap) and these labels.
		expect(of('.step-groups')).toMatchObject({ gap: 'var(--space-5)' });
		for (const d of css.filter((d) => /^\.(step-groups|choices-section|plan-form|complete-section)\b/.test(d.selector))) {
			expect(d.prop, `${d.selector} must not draw a rule`).not.toMatch(/^border-(top|bottom)/);
		}
	});
});

describe('no plan panel writes a sub-heading the old way', () => {
	const files = svelteFiles(HERE);
	// <p class="choices-header">…</p>: the text between, tags dropped and each {expression}
	// reduced to {} (a ternary's ? is not a question mark).
	const headings = (file: string) =>
		[...markupOf(file).matchAll(/<(p|span)\b[^>]*class="[^"]*\bchoices-header\b[^"]*"[^>]*>([\s\S]*?)<\/\1>/g)]
			.map((m) => m[2].replace(/<[^>]+>/g, '').replace(/\{[^}]*\}/g, '{}').replace(/\s+/g, ' ').trim());

	it('finds the sub-headings (the guard is not vacuous)', () => {
		const all = files.flatMap(headings);
		expect(all.length).toBeGreaterThan(40);
		expect(all).toContain('The council');
	});

	it.each([
		['a colon at the end', /:$/],
		['a question mark', /\?/],
		['a count in parentheses', /\([^)]*(?:\{\}|\d)[^)]*\)/],
		['a separator dot or dash', /·| — /],
	])('has no heading with %s', (_what, bad) => {
		const offenders = files.flatMap((f) => headings(f).filter((h) => bad.test(h)).map((h) => `${relative(HERE, f)}: ${h}`));
		expect(offenders).toEqual([]);
	});
});

describe('a field\'s label carries no trailing colon', () => {
	// The label over a field names what it takes; punctuation is for sentences. This reads
	// <FormField label="…"> and the first line of a <label class="form-label">.
	const files = svelteFiles(HERE);
	const labels = (file: string) => {
		const src = markupOf(file);
		const fromField = [...src.matchAll(/<FormField label="([^"]*)"/g)].map((m) => m[1]);
		const fromLabel = [...src.matchAll(/<label class="form-label[^"]*"[^>]*>\s*(?:<span class="form-label-text">)?([^<{]*)/g)].map((m) => m[1].trim());
		return [...fromField, ...fromLabel].filter(Boolean);
	};

	it('finds the labels (the guard is not vacuous)', () => {
		expect(files.flatMap(labels).length).toBeGreaterThan(40);
	});

	it.each([['a colon', /:$/], ['a question mark', /\?$/]])('has none ending with %s', (_what, bad) => {
		const offenders = files.flatMap((f) => labels(f).filter((l) => bad.test(l)).map((l) => `${relative(HERE, f)}: ${l}`));
		expect(offenders).toEqual([]);
	});
});

const players = [
	{ id: 1, display_name: 'alice', token_color: null, seat_order: 1 },
	{ id: 2, display_name: 'bob', token_color: null, seat_order: 2 },
] as Player[];
const text = (html: string) => html.replace(/<!--[\s\S]*?-->/g, '').replace(/<[^>]+>/g, '').replace(/\s+/g, ' ').trim();

describe('SetPiece', () => {
	const piece = (props: Record<string, unknown> = {}) =>
		render(SetPiece, {
			props: {
				players, a: { playerID: 1, peer: 'Lady Vey' }, b: { playerID: 2, peer: 'Ser Ott' }, connector: 'meets', ...props,
			},
		}).body;

	it('puts each player and their peer face to face, the peers in the set-piece look', () => {
		const html = piece({ scene: 'After vespers.' });
		expect(text(html)).toBe("“After vespers.” alice's Lady Vey meets bob's Ser Ott");
		expect(html.match(/class="asset-name set-piece-peer"/g)).toHaveLength(2);
		// Names go through <PlayerName>, in their colour.
		expect(html.match(/class="player-name"/g)).toHaveLength(2);
	});

	it('carries no difficulty line, and no scene paragraph when there is no scene', () => {
		const html = piece();
		expect(text(html)).not.toMatch(/difficulty/i);
		expect(html).not.toContain('set-piece-scene');
	});

	it('stands a player with no peer on their own name', () => {
		expect(text(piece({ b: { playerID: 2, peer: null }, connector: 'faces' }))).toBe("alice's Lady Vey faces bob");
	});

	it('is centred, with the connector a small faint uppercase label', () => {
		const css = declarations(stripComments(readFileSync(join(HERE, 'shared', 'SetPiece.svelte'), 'utf8').match(/<style>([\s\S]*)<\/style>/)![1]));
		const of = (selector: string) => Object.fromEntries(css.filter((d) => d.selector === selector).map((d) => [d.prop, d.value]));
		expect(of('.set-piece')).toMatchObject({ 'align-items': 'center', 'text-align': 'center' });
		expect(of('.set-piece-connector')).toMatchObject({
			'font-size': 'var(--text-label)',
			'text-transform': 'uppercase',
			color: 'var(--color-text-faint)',
		});
		const inline = declarations(readFileSync(join(HERE, '..', 'shared', 'inlineText.css'), 'utf8'));
		expect(inline).toContainEqual({ selector: '.asset-name.set-piece-peer', prop: 'color', value: 'var(--color-accent)' });
		expect(inline).toContainEqual({ selector: '.asset-name.set-piece-peer', prop: 'font-size', value: 'var(--text-heading)' });
	});
});

describe('QuotedText', () => {
	const quoted = (props: Record<string, unknown> = {}) =>
		render(QuotedText, {
			props: {
				text: 'No private armies.',
				label: createRawSnippet(() => ({ render: () => '<span>alice proposes</span>' })),
				...props,
			},
		}).body;

	it('sets the label over the player\'s words, in their own quotation marks', () => {
		const html = quoted();
		expect(html).toMatch(/<p class="choices-header">[\s\S]*alice proposes[\s\S]*<\/p>/);
		expect(html).toContain('<span class="quote">“No private armies.”</span>');
	});

	it('is 16px, at most 320px wide and centred; a note under it is optional', () => {
		const src = readFileSync(join(HERE, 'shared', 'QuotedText.svelte'), 'utf8').match(/<style>([\s\S]*)<\/style>/)![1];
		const css = declarations(stripComments(src));
		const of = (selector: string) => Object.fromEntries(css.filter((d) => d.selector === selector).map((d) => [d.prop, d.value]));
		expect(of('.quoted-text')).toMatchObject({ 'align-items': 'center', 'text-align': 'center' });
		expect(of('.quoted-body')).toMatchObject({ 'max-width': '320px', 'font-size': 'var(--text-body)' });
		expect(quoted()).not.toContain('quoted-note');
		expect(quoted({ note: 'How it spreads.' })).toContain('quoted-note');
	});
});
