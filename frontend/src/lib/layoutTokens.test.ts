import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

/*
 * Layout width system guard (docs/STYLE_GUIDE.md "Layout widths"). The system
 * has exactly two viewport breakpoints, where two and then three phone-width
 * columns fit (744 and 1112), and everything else adapts to its COLUMN via
 * @container or fluid CSS.
 *
 * If this test fails you probably added a viewport media query to a
 * component. Ask "how wide is my column?" instead of "how wide is the
 * window?": use `@container column (…)` against the play column, or make
 * the layout fluid. A genuinely new breakpoint or container threshold is a
 * STYLE_GUIDE change first, then this allowlist.
 */

const SRC = join(__dirname, '..');

// Viewport width queries: only the two breakpoints (the min-width form and the
// max-width complement), and only in the shell / modal-idiom files.
const ALLOWED_MEDIA = new Set(['743px', '744px', '1111px', '1112px']);
// Old literals that a screen's own redesign piece still has to retire. Each
// entry is one file and the queries it may keep until then; the piece that
// rewrites the file deletes its entry. The profile page's two-up grid was keyed
// to the old chat dock (790).
const PENDING_MIGRATION: Record<string, string[]> = {
	'routes/profile/+page.svelte': ['790px'],
};
// Container width queries: the documented thresholds only.
//   300 — SceneSetupForm's squeezed-column layout
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
	it('viewport width queries use only the two breakpoint literals', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			const lines = readFileSync(file, 'utf8').split('\n');
			lines.forEach((line, i) => {
				for (const m of line.matchAll(MEDIA_WIDTH)) {
					if (!ALLOWED_MEDIA.has(m[1]) && !PENDING_MIGRATION[rel]?.includes(m[1])) {
						offenders.push(`${rel}:${i + 1}  @media ${m[1]}`);
					}
				}
			});
		}
		expect(offenders, `viewport width queries outside the breakpoint literals:\n${offenders.join('\n')}`).toEqual([]);
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

	it('CSS breakpoint literals agree with lib/breakpoints.ts', () => {
		const bp = readFileSync(join(SRC, 'lib/breakpoints.ts'), 'utf8');
		const two = Number(bp.match(/TWO_COLUMN_PX = (\d+)/)?.[1]);
		const three = Number(bp.match(/THREE_COLUMN_PX = (\d+)/)?.[1]);
		// The allowlist above IS the CSS-side contract; assert the constants
		// line up with it (min-width uses the breakpoint, max-width its complement).
		expect(new Set([`${two - 1}px`, `${two}px`, `${three - 1}px`, `${three}px`])).toEqual(ALLOWED_MEDIA);
	});

	// The breakpoints are derived, not chosen: a breakpoint is where that many
	// columns fit at the narrowest they are drawn, with a gutter between each
	// and beside the outer two. Retuning the column band means changing the
	// constants and app.css together; this points at whichever is stale.
	it('the breakpoints are the widths where two and three minimum columns fit', () => {
		const bp = readFileSync(join(SRC, 'lib/breakpoints.ts'), 'utf8');
		const num = (name: string) => Number(bp.match(new RegExp(`${name} = (\\d+)`))?.[1]);
		const min = num('COLUMN_MIN_PX');
		const gutter = num('GUTTER_PX');
		expect(num('TWO_COLUMN_PX'), 'TWO_COLUMN_PX = gutter, column, gutter, column, gutter').toBe(
			3 * gutter + 2 * min
		);
		expect(num('THREE_COLUMN_PX'), 'THREE_COLUMN_PX = four gutters and three columns').toBe(
			4 * gutter + 3 * min
		);
	});

	it('app.css carries the same column band and gutter as lib/breakpoints.ts', () => {
		const bp = readFileSync(join(SRC, 'lib/breakpoints.ts'), 'utf8');
		const css = readFileSync(join(SRC, 'app.css'), 'utf8');
		const fromTs = (name: string) => Number(bp.match(new RegExp(`${name} = (\\d+)`))?.[1]);
		const fromCss = (name: string) => Number(css.match(new RegExp(`${name}:\\s*(\\d+)px`))?.[1]);
		expect(fromCss('--layout-column-min'), '--layout-column-min vs COLUMN_MIN_PX').toBe(fromTs('COLUMN_MIN_PX'));
		expect(fromCss('--layout-column-max'), '--layout-column-max vs COLUMN_MAX_PX').toBe(fromTs('COLUMN_MAX_PX'));
		// The gutter is --space-2, in rem (0.5rem = 8px at the default size).
		expect(css.match(/--space-2:\s*([\d.]+)rem/)?.[1], '--space-2 vs GUTTER_PX').toBe(
			String(fromTs('GUTTER_PX') / 16)
		);
	});

	/*
	 * The table shell's box IS the viewport as far as the width system is
	 * concerned: every derivation in the STYLE_GUIDE table measures from the
	 * raw viewport (the 744 breakpoint = 8+360+8+360+8). main.full-bleed wraps
	 * the whole table route, so anything it takes horizontally comes straight
	 * out of the columns — and it takes it silently, because the shell's own
	 * `overflow-x: clip` hides the resulting overflow instead of showing a
	 * scrollbar.
	 *
	 * Two ways to lose the width, both of which have happened:
	 *   padding — a 0.2rem gutter left the play content at 293.6px on a 360
	 *     phone (under the 300 floor SceneSetupForm's @container query keys
	 *     off) and put both breakpoints ~4px short of their tracks.
	 *   auto inline margins — body is a flex column on this route, and auto
	 *     cross-axis margins suppress a flex item's default `stretch`, so
	 *     `margin: 0 auto` inherited from the base `main` rule collapsed the
	 *     whole page to shrink-to-fit (~302px on a 412px phone).
	 *
	 * main.flush is the same contract for pages that own their gutter
	 * (currently /profile): with main's gutter outside the capped box the
	 * profile column measured 408 at a 440 viewport while every other column
	 * measured 440. Pad inside the column instead; the table's columns and
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
});
