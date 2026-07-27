import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

/*
 * Layout width system guard (docs/STYLE_GUIDE.md "Layout widths";
 * derivations in adr/LAYOUT_WIDTHS_PLAN.md). The system has exactly two
 * viewport breakpoints — the chat dock (790) and the record dock (1040) —
 * and everything else adapts to its COLUMN via @container or fluid CSS.
 *
 * If this test fails you probably added a viewport media query to a
 * component. Ask "how wide is my column?" instead of "how wide is the
 * window?": use `@container column (…)` against the phase column, or make
 * the layout fluid. A genuinely new breakpoint or container threshold is a
 * STYLE_GUIDE + adr/LAYOUT_WIDTHS_PLAN.md change first, then this
 * allowlist.
 */

const SRC = join(__dirname, '..');

// Viewport width queries: only the dock literals (min-width form and the
// max-width complement), and only in the shell / modal-idiom files.
const ALLOWED_MEDIA = new Set(['789px', '790px', '1070px']);
// Container width queries: the documented thresholds only.
//   300 — the record-phase content minimum (a squeezed column)
//   400 — AssetCardSelectable's compact meta cluster
//   420 — the prologue tile grid's 2→3 flip (column at the 440-cap region)
const ALLOWED_CONTAINER = new Set(['300px', '400px', '420px']);

const MEDIA_WIDTH = /@media[^{]*\(\s*(?:min|max)-width:\s*([^)]+?)\s*\)/g;
const CONTAINER_WIDTH = /@container[^{]*\(\s*(?:min|max)-width:\s*([^)]+?)\s*\)/g;
// matchMedia calls with an inline width literal (anything not composed from
// lib/breakpoints.ts).
const MATCH_MEDIA_LITERAL = /matchMedia\(\s*['"`][^'"`]*width[^'"`]*['"`]\s*\)/g;

function* walk(dir: string): Generator<string> {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) yield* walk(p);
		else if (/\.(svelte|css|ts)$/.test(entry.name) && !/\.test\.ts$/.test(entry.name)) yield p;
	}
}

describe('layout width tokens', () => {
	it('viewport width queries use only the dock literals', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			const lines = readFileSync(file, 'utf8').split('\n');
			lines.forEach((line, i) => {
				for (const m of line.matchAll(MEDIA_WIDTH)) {
					if (!ALLOWED_MEDIA.has(m[1])) offenders.push(`${rel}:${i + 1}  @media ${m[1]}`);
				}
			});
		}
		expect(offenders, `viewport width queries outside the dock literals:\n${offenders.join('\n')}`).toEqual([]);
	});

	it('container width queries use only the documented thresholds', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			const lines = readFileSync(file, 'utf8').split('\n');
			lines.forEach((line, i) => {
				for (const m of line.matchAll(CONTAINER_WIDTH)) {
					if (!ALLOWED_CONTAINER.has(m[1])) offenders.push(`${rel}:${i + 1}  @container ${m[1]}`);
				}
			});
		}
		expect(offenders, `container width queries outside the allowlist:\n${offenders.join('\n')}`).toEqual([]);
	});

	it('no inline width matchMedia — compose from lib/breakpoints.ts', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			if (rel === 'lib/breakpoints.ts') continue;
			const lines = readFileSync(file, 'utf8').split('\n');
			lines.forEach((line, i) => {
				for (const m of line.matchAll(MATCH_MEDIA_LITERAL)) {
					offenders.push(`${rel}:${i + 1}  ${m[0]}`);
				}
			});
		}
		expect(offenders, `matchMedia with an inline width literal:\n${offenders.join('\n')}`).toEqual([]);
	});

	it('CSS dock literals agree with lib/breakpoints.ts', () => {
		const bp = readFileSync(join(SRC, 'lib/breakpoints.ts'), 'utf8');
		const chat = Number(bp.match(/CHAT_DOCK_PX = (\d+)/)?.[1]);
		const record = Number(bp.match(/RECORD_DOCK_PX = (\d+)/)?.[1]);
		// The allowlist above IS the CSS-side contract; assert the constants
		// line up with it (min-width uses the dock, max-width its complement).
		expect(new Set([`${chat - 1}px`, `${chat}px`, `${record}px`])).toEqual(ALLOWED_MEDIA);
	});

	/*
	 * The table shell's box IS the viewport as far as the width system is
	 * concerned: every derivation in the STYLE_GUIDE table measures from the
	 * raw viewport (300 = 360 − 44 rail − 2×8; the 790 dock = 44+8+360+8+
	 * 360+8). main.full-bleed wraps the whole table route, so anything it
	 * takes horizontally comes straight out of the phase column — and it
	 * takes it silently, because the shell's own `overflow-x: clip` hides
	 * the resulting overflow instead of showing a scrollbar.
	 *
	 * Two ways to lose the width, both of which have happened:
	 *   padding — a 0.2rem gutter left the record-phase content at 293.6px
	 *     on a 360 phone (under the 300 floor SceneSetupForm's @container
	 *     query keys off) and put both docks ~4px short of their tracks.
	 *   auto inline margins — body is a flex column on this route, and auto
	 *     cross-axis margins suppress a flex item's default `stretch`, so
	 *     `margin: 0 auto` inherited from the base `main` rule collapsed the
	 *     whole page to shrink-to-fit (~302px on a 412px phone).
	 *
	 * main.flush is the same contract for pages that own their gutter
	 * (currently /profile): with main's gutter outside the capped box the
	 * profile column measured 408 at a 440 viewport while every other column
	 * measured 440. Pad inside the column instead; the phase views and
	 * .profile already do.
	 */
	it.each(['full-bleed', 'flush'])('main.%s adds no horizontal gutter', (modifier) => {
		const layout = readFileSync(join(SRC, 'routes/+layout.svelte'), 'utf8');
		const block = layout.match(new RegExp(`main\\.${modifier}\\s*\\{([^}]*)\\}`))?.[1];
		expect(block, `main.${modifier} rule not found in routes/+layout.svelte`).toBeDefined();

		// full-bleed's comment quotes `margin: 0 auto` as the thing being
		// undone — scan declarations only.
		const decls = block!.replace(/\/\*[\s\S]*?\*\//g, '');

		const offenders: string[] = [];
		for (const m of decls.matchAll(/(margin|padding)(-inline|-left|-right)?\s*:\s*([^;]+)/g)) {
			const [, prop, side, raw] = m;
			const parts = raw.trim().split(/\s+/);
			// Pull out just the inline-axis values: the shorthand's 2nd slot
			// (and 4th, when all four sides are listed), both values of the
			// -inline pair, or the single value of -left / -right.
			const inline = !side
				? (parts.length === 1 ? [parts[0]] : parts.length === 4 ? [parts[1], parts[3]] : [parts[1]])
				: side === '-inline' ? parts
				: [parts[0]];
			for (const v of inline) {
				if (v !== '0' && v !== '0px') offenders.push(`${prop}${side ?? ''}: ${raw.trim()}  →  ${v}`);
			}
		}
		expect(
			offenders,
			`main.${modifier} must not take horizontal space from the column below it:\n${offenders.join('\n')}`
		).toEqual([]);
	});

	// The record width (RECORD_WIDTH_PX) can't be read by CSS, so it has two
	// mirrors: PublicRecord's overlay width and the table grid's record
	// column. Retuning it means changing the constant and both mirrors —
	// these assertions point at whichever is stale. The dock check keeps the
	// derived breakpoint honest: dock ≥ 8+W+8+360+8+360+8.
	it('record width CSS mirrors agree with lib/breakpoints.ts', () => {
		const bp = readFileSync(join(SRC, 'lib/breakpoints.ts'), 'utf8');
		const w = Number(bp.match(/RECORD_WIDTH_PX = (\d+)/)?.[1]);
		const dock = Number(bp.match(/RECORD_DOCK_PX = (\d+)/)?.[1]);
		const publicRecord = readFileSync(join(SRC, 'lib/components/PublicRecord.svelte'), 'utf8');
		expect(publicRecord, `PublicRecord .expanded should be width: ${w}px`).toContain(`width: ${w}px`);
		const tablePage = readFileSync(join(SRC, 'routes/table/[id]/+page.svelte'), 'utf8');
		expect(tablePage, `record grid column should be ${w}px`).toContain(`grid-template-columns: ${w}px`);
		expect(dock, 'RECORD_DOCK_PX must cover 8+W+8+360+8+360+8').toBeGreaterThanOrEqual(w + 752);
	});
});
