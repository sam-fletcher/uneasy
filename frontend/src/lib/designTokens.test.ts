import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

/*
 * ADR-009 guard: colour literals may only appear in app.css (the Tier-1
 * primitive block). Everywhere else, components must reference a semantic
 * --color-* token or, for one-offs with no role, a primitive step.
 *
 * If this test fails: find the right token in src/app.css (primitives are
 * listed as ramps; adr/009-design-tokens.md has the full map). A genuinely
 * new colour means adding a primitive step there — never a literal here.
 */

const SRC = join(__dirname, '..');

// Hex colours, and rgb()/hsl() with literal channel values. rgba(0,0,0,…)
// shadow/scrim washes are allowed: they're opacity effects, not palette.
const HEX = /#[0-9a-fA-F]{3,8}\b/g;
const FUNC = /\b(?:rgb|hsl)a?\(\s*\d/g;
const ALLOWED_FUNC = /\brgba?\(\s*(?:0\s*,\s*0\s*,\s*0|255\s*,\s*255\s*,\s*255)\s*,/;

function* walk(dir: string): Generator<string> {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) yield* walk(p);
		else if (/\.(svelte|css)$/.test(entry.name)) yield p;
	}
}

describe('design tokens (ADR-009)', () => {
	it('no colour literals outside app.css', () => {
		const offenders: string[] = [];
		for (const file of walk(SRC)) {
			const rel = relative(SRC, file);
			if (rel === 'app.css') continue;
			const lines = readFileSync(file, 'utf8').split('\n');
			lines.forEach((line, i) => {
				const hex = line.match(HEX);
				if (hex) offenders.push(`${rel}:${i + 1}  ${hex.join(' ')}`);
				const fn = line.match(FUNC);
				if (fn && !ALLOWED_FUNC.test(line)) offenders.push(`${rel}:${i + 1}  ${fn.join(' ')}`);
			});
		}
		expect(offenders, `colour literals found outside app.css:\n${offenders.join('\n')}`).toEqual([]);
	});

	// Player identity colours (COLOR_ROLES_PLAN.md follow-up) are categorical,
	// not ramped, so they live in lib/playerColor.ts rather than app.css's
	// --<family>-<step> tiers — the guard above doesn't (and shouldn't) cover
	// that file. Instead app.css carries a reference-only --player-* block
	// (never consumed via var()) so a colour audit sees every hex in one
	// place; this test just asserts the two files haven't drifted apart.
	it('playerColor.ts matches app.css\'s --player-* reference block', () => {
		const appCss = readFileSync(join(SRC, 'app.css'), 'utf8');
		const playerColorTs = readFileSync(join(SRC, 'lib/playerColor.ts'), 'utf8');

		const cssPlayers: Record<string, string> = {};
		for (const m of appCss.matchAll(/--player-(\w+):\s*(#[0-9a-fA-F]{6})/g)) {
			cssPlayers[m[1]] = m[2].toLowerCase();
		}

		const paletteMatch = playerColorTs.match(/FALLBACK_PALETTE = \[([\s\S]*?)\];/);
		const tsPalette = paletteMatch
			? [...paletteMatch[1].matchAll(/#[0-9a-fA-F]{6}/g)].map(m => m[0].toLowerCase())
			: [];
		const oocMatch = playerColorTs.match(/OOC_COLOR = '(#[0-9a-fA-F]{6})'/);
		const tsOoc = oocMatch ? oocMatch[1].toLowerCase() : null;

		const cssPalette = ['1', '2', '3', '4', '5'].map(n => cssPlayers[n]);
		expect(tsPalette, 'playerColor.ts FALLBACK_PALETTE vs app.css --player-1..5').toEqual(cssPalette);
		expect(tsOoc, 'playerColor.ts OOC_COLOR vs app.css --player-ooc').toEqual(cssPlayers['ooc']);
	});
});
