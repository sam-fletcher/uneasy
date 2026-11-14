import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';

/*
 * The four panels of plan panels A (Spread Rumors, Seek Answers, Spread
 * Propaganda, Chronicle Histories). These are source guards, not renders: the
 * rules are about which markup a panel draws and which boxes its own CSS adds,
 * and a render would only show the result of those choices, not the choice.
 *
 *   - a warning is a <Notice> (a mark and coloured words), never the old
 *     .res-warning paragraph, which had no mark;
 *   - a chip is the house <Chip>, never the old .chip-btn / .active markup;
 *   - a panel draws no inline style attribute: its literals belong in its
 *     own <style> block, where the scale guard reads them;
 *   - a panel's own CSS adds no box: a box marks a thing you can touch, and a
 *     group is heading and space, so no background and no framed border;
 *   - the "tokens: not migrated" marker is gone from all four.
 */

const PANELS = [
	'SpreadRumorsPanel.svelte',
	'SeekAnswersPanel.svelte',
	'SpreadPropagandaPanel.svelte',
	'ChronicleHistoriesPanel.svelte',
] as const;

const source = (name: string) => readFileSync(join(__dirname, name), 'utf8');
const ownStyle = (s: string) => s.match(/<style>([\s\S]*?)<\/style>/)?.[1] ?? '';

describe.each(PANELS)('%s', (name) => {
	const s = source(name);

	it('has no tokens: not migrated marker', () => {
		expect(s).not.toContain('tokens: not migrated');
	});

	it('draws its warnings as a Notice, never the old .res-warning paragraph', () => {
		expect(s).not.toMatch(/res-warning/);
		expect(s).toMatch(/import Notice from '\$lib\/components\/shared\/Notice\.svelte';/);
		expect(s).toMatch(/<Notice>/);
	});

	it('draws no inline style attribute', () => {
		expect(s).not.toMatch(/\sstyle="/);
	});

	it('adds no box in its own CSS: no background, no framed border', () => {
		// The rules for a control's own states (hover, focus) may paint; the
		// resting look may not. A box at rest is the thing this guards against.
		const css = ownStyle(s).replace(/[^{}]*:(?:hover|focus[^{]*?)\s*\{[^}]*\}/g, '');
		expect(css).not.toMatch(/background/);
		// `border:` on its own is a frame; border-left / border-bottom are rules.
		expect(css).not.toMatch(/(^|[\s;{])border\s*:/m);
	});
});

describe('the house chip', () => {
	it('Chronicle Histories draws its options as <Chip>, not .chip-btn', () => {
		const s = source('ChronicleHistoriesPanel.svelte');
		expect(s).toMatch(/import Chip from '\$lib\/components\/shared\/Chip\.svelte';/);
		expect(s).not.toMatch(/chip-btn/);
		expect(s).not.toMatch(/class:active=/);
		expect(s.match(/<Chip/g)?.length).toBe(2);
	});

	it('no panel of this piece draws the old chip markup', () => {
		for (const name of PANELS) {
			expect(source(name), name).not.toMatch(/class="chip-btn"/);
		}
	});
});

describe('Spread Propaganda and Spread Rumors keep their secret rumor back', () => {
	it.each(['SpreadRumorsPanel.svelte', 'SpreadPropagandaPanel.svelte'])(
		'%s passes showNotes={false} to the plan frame',
		(name) => {
			expect(source(name)).toMatch(/showNotes=\{false\}/);
		},
	);
});
