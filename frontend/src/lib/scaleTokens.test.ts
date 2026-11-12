import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

/*
 * Scale guard: font sizes, gaps and corner radii may only be the tokens in
 * app.css (docs/STYLE_GUIDE.md "Type, gaps and corners").
 *
 *   font-size, font          → var(--text-label|small|body|heading|title)
 *   padding*, margin*, gap   → var(--space-1 … --space-6)
 *   border-*radius*          → var(--radius-piece|control|chip|sheet|dot)
 *
 * 0 is always allowed. Widths and heights stay with layoutTokens.test.ts.
 *
 * If this test fails: use the step whose job fits (the comments in app.css
 * give each one). Never a literal here, and never a sixth size: a case that
 * "needs 13px" is a case for editing the copy or the layout.
 *
 * THE MARKER. The redesign migrates the app one file at a time, so a file that
 * still has literals starts its first line with a marker comment and is
 * skipped:
 *
 *     <!-- tokens: not migrated -->           in a .svelte file
 *     slash-star tokens: not migrated star-slash   in a .css file
 *
 * (The second is an ordinary CSS comment; it is spelled out here only because
 * writing it literally would end this comment.) The piece that migrates a file
 * deletes its marker. This test fails if a marked file has nothing left to
 * migrate, so a marker can't outlive its job and a migrating PR can't forget
 * it. There is deliberately no list of unmigrated files anywhere: every
 * migration PR would edit it and conflict.
 */

const SRC = join(__dirname, '..');

// ── Tokens ────────────────────────────────────────────────────────────────

const TEXT = ['label', 'small', 'body', 'heading', 'title'].map(n => `--text-${n}`);
// The landing page's cover title: the one documented exception to five sizes.
const TEXT_DISPLAY = '--text-display';
const SPACE = [1, 2, 3, 4, 5, 6].map(n => `--space-${n}`);
const RADIUS = ['piece', 'control', 'chip', 'sheet', 'dot'].map(n => `--radius-${n}`);
// Size-named radii from before the redesign. app.css still defines them so
// that unmigrated files don't change shape, but a migrated file must pick a
// radius by what the thing is, so they are not tokens here.
const RETIRED = ['--radius-sm', '--radius-md', '--radius-lg'];

type Kind = 'text' | 'space' | 'radius';
const FAMILY: Record<Kind, string[]> = {
	text: [...TEXT, TEXT_DISPLAY],
	space: SPACE,
	radius: RADIUS,
};
// A name starting with one of these is meant to be a scale token, so a typo
// like --space-7 is an error, not a harmless local variable. (An unknown var
// silently becomes the property's initial value: a 0 gap, an inherited size.)
const RESERVED_PREFIX = /^--(?:text|space|radius)-/;

function kindOf(prop: string): Kind | 'font' | null {
	if (prop === 'font') return 'font';
	if (prop === 'font-size') return 'text';
	if (/^(?:padding|margin|scroll-padding|scroll-margin)(?:-|$)/.test(prop)) return 'space';
	if (/^(?:gap|row-gap|column-gap|grid-gap|grid-row-gap|grid-column-gap)$/.test(prop)) return 'space';
	if (/^border(?:-[a-z]+)*-radius$/.test(prop)) return 'radius';
	return null;
}

// A custom property is a way round the scale if it carries a literal for a
// gap, so one named for a gap, gutter, padding, margin, radius or font size
// is held to the same rule as the property it will end up in.
function kindOfCustom(name: string): Kind | null {
	if (/(?:gap|gutter|padding|margin|spacing)$/.test(name)) return 'space';
	if (/radius$/.test(name)) return 'radius';
	if (/font-size$/.test(name)) return 'text';
	return null;
}

// ── Reading declarations ──────────────────────────────────────────────────

// Replace a comment by spaces, keeping its newlines, so line numbers survive.
const blank = (s: string) => s.replace(/[^\n]/g, ' ');
const stripCssComments = (css: string) => css.replace(/\/\*[\s\S]*?\*\//g, blank);

interface Decl {
	prop: string;
	value: string;
	line: number;
}

// Split CSS into `prop: value` declarations. Parentheses and quotes are
// respected so `var(--x, 1px)`, `url(data:…;base64,…)` and `content: ';'` don't
// end a declaration early. Anything ending in `{` is a selector or at-rule
// prelude, not a declaration.
function* declarations(css: string, firstLine: number): Generator<Decl> {
	let depth = 0;
	let quote = '';
	let start = 0;
	for (let i = 0; i <= css.length; i++) {
		const ch = css[i];
		if (quote) {
			if (ch === quote && css[i - 1] !== '\\') quote = '';
			continue;
		}
		if (ch === '"' || ch === "'") quote = ch;
		else if (ch === '(') depth++;
		else if (ch === ')') depth = Math.max(0, depth - 1);
		else if (depth === 0 && (ch === ';' || ch === '}' || ch === '{' || i === css.length)) {
			if (ch !== '{') {
				const chunk = css.slice(start, i);
				const m = chunk.match(/^\s*(--[\w-]+|[a-zA-Z-]+)\s*:([\s\S]*)$/);
				if (m) {
					const lead = chunk.length - chunk.trimStart().length;
					const line = firstLine + (css.slice(0, start + lead).match(/\n/g)?.length ?? 0);
					yield { prop: m[1].startsWith('--') ? m[1] : m[1].toLowerCase(), value: m[2], line };
				}
			}
			start = i + 1;
		}
	}
}

// The string and template literals inside a JS expression, with `${…}`
// interpolations swallowed. A literal in a style expression is CSS the
// browser will apply, so it gets judged like any other.
function stringLiterals(expr: string): string[] {
	const out: string[] = [];
	for (let i = 0; i < expr.length; i++) {
		const q = expr[i];
		if (q !== '"' && q !== "'" && q !== '`') continue;
		let body = '';
		let j = i + 1;
		for (; j < expr.length && expr[j] !== q; j++) {
			if (expr[j] === '\\') {
				body += expr.slice(j, j + 2);
				j++;
			} else if (q === '`' && expr[j] === '$' && expr[j + 1] === '{') {
				let depth = 0;
				for (; j < expr.length; j++) {
					if (expr[j] === '{') depth++;
					else if (expr[j] === '}' && --depth === 0) break;
				}
				body += '__DYN__';
			} else body += expr[j];
		}
		out.push(body);
		i = j;
	}
	return out;
}

// Index of the `}` closing the Svelte {expression} that opens at text[open],
// skipping braces inside string literals.
function expressionEnd(text: string, open: number): number {
	let depth = 0;
	let quote = '';
	for (let i = open; i < text.length; i++) {
		const ch = text[i];
		if (quote) {
			if (ch === '\\') i++;
			else if (ch === quote) quote = '';
		} else if (ch === '"' || ch === "'" || ch === '`') quote = ch;
		else if (ch === '{') depth++;
		else if (ch === '}' && --depth === 0) return i;
	}
	return text.length - 1;
}

// The declarations in one style attribute's value (`raw`, with its {expressions}
// still in it). `directiveProp` is set for a style:prop directive, whose whole
// value belongs to that property.
//
// An {expression} stands for one opaque value, so its braces can't end a
// declaration early. But a string or template literal inside it is CSS the
// browser will apply, so each is judged: as a value for the property it sits
// in (`padding: {'7px'}`, `style:padding={'7px'}`), or, where it opens a
// declaration (`style={`padding:7px`}`), as a list of declarations.
function* styleAttribute(directiveProp: string | undefined, raw: string, line: number): Generator<Decl> {
	let text = '';
	const literals: { prop: string | null; value: string }[] = [];
	for (let i = 0; i < raw.length; ) {
		if (raw[i] !== '{') {
			text += raw[i++];
			continue;
		}
		const end = expressionEnd(raw, i);
		const pending = text.slice(text.lastIndexOf(';') + 1).match(/^\s*(--[\w-]+|[a-zA-Z-]+)\s*:/);
		const prop = directiveProp ?? (pending ? (pending[1].startsWith('--') ? pending[1] : pending[1].toLowerCase()) : null);
		for (const value of stringLiterals(raw.slice(i + 1, end))) literals.push({ prop, value });
		text += '__DYN__';
		i = end + 1;
	}
	if (directiveProp) yield { prop: directiveProp, value: text, line };
	else yield* declarations(text, line);
	for (const { prop, value } of literals) {
		if (prop) yield { prop, value, line };
		else yield* declarations(value, line);
	}
}

// All declarations in a .svelte or .css file, with 1-based line numbers: the
// <style> blocks, and style="…", style={…} and style:prop attributes. Script
// blocks are skipped (TypeScript has `gap: number`), and so is the prose in
// the markup.
function declarationsIn(file: string, text: string): Decl[] {
	if (file.endsWith('.css')) return [...declarations(stripCssComments(text), 1)];

	const out: Decl[] = [];
	const lineAt = (offset: number) => 1 + (text.slice(0, offset).match(/\n/g)?.length ?? 0);

	const withoutScripts = text.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, blank);
	for (const m of withoutScripts.matchAll(/<style\b[^>]*>([\s\S]*?)<\/style>/gi)) {
		const bodyStart = m.index + m[0].indexOf('>') + 1;
		out.push(...declarations(stripCssComments(m[1]), lineAt(bodyStart)));
	}

	const markup = withoutScripts
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, blank)
		.replace(/<!--[\s\S]*?-->/g, blank);
	for (const m of markup.matchAll(/\sstyle(?::(--[\w-]+|[a-z-]+)(?:\|important)?)?\s*=\s*/g)) {
		const open = m.index + m[0].length;
		const q = markup[open];
		let raw: string;
		if (q === '"' || q === "'") {
			let j = open + 1;
			while (j < markup.length && markup[j] !== q) j = markup[j] === '{' ? expressionEnd(markup, j) + 1 : j + 1;
			raw = markup.slice(open + 1, j);
		} else if (q === '{') {
			raw = markup.slice(open, expressionEnd(markup, open) + 1);
		} else continue;
		out.push(...styleAttribute(m[1], raw, lineAt(m.index)));
	}
	return out;
}

// ── Judging a value ───────────────────────────────────────────────────────

// Find every var(…) in `value`, replacing each with V, and report the ones
// that aren't allowed. A var of a local custom property is fine (a column
// passes `--chat-strip-height` down, say), but its fallback is judged too.
function judgeVars(value: string, kind: Kind, why: string[]): string {
	let out = '';
	for (let i = 0; i < value.length; ) {
		const rest = value.slice(i);
		const m = rest.match(/^var\(/i);
		if (!m) {
			out += value[i++];
			continue;
		}
		let depth = 1;
		let j = i + m[0].length;
		for (; j < value.length && depth > 0; j++) {
			if (value[j] === '(') depth++;
			else if (value[j] === ')') depth--;
		}
		const inner = value.slice(i + m[0].length, j - 1);
		const comma = inner.indexOf(',');
		const name = (comma < 0 ? inner : inner.slice(0, comma)).trim();
		const fallback = comma < 0 ? '' : inner.slice(comma + 1);
		if (name.includes('__DYN__')) {
			// var(--space-{n}): the step is picked at runtime, which this can't judge.
		} else if (RETIRED.includes(name)) {
			why.push(`${name} is retired: pick the radius for the kind of thing (--radius-piece, -control, -chip, -sheet, -dot)`);
		} else if (RESERVED_PREFIX.test(name) && !FAMILY[kind].includes(name)) {
			why.push(
				[...TEXT, TEXT_DISPLAY, ...SPACE, ...RADIUS].includes(name)
					? `${name} is not a ${kind === 'space' ? 'gap' : kind === 'text' ? 'text size' : 'radius'} token`
					: `${name} is not a token (typo? it would silently fall back to the property's default)`
			);
		}
		if (fallback.trim()) why.push(...judge(kind, fallback));
		out += 'V';
		i = j;
	}
	return out;
}

const KEYWORDS = new Set([
	'auto', 'normal', 'inherit', 'initial', 'unset', 'revert', 'revert-layer',
	'calc', 'min', 'max', 'clamp', '+', '-',
]);
const UNITLESS = /^[+-]?(?:\d+\.?\d*|\.\d+)$/;
const ZERO = /^[+-]?0*\.?0+[a-z%]*$/i;
const DIMENSION = /^[+-]?(?:\d+\.?\d*|\.\d+)[a-z%]+$/i;

// Why `value` isn't made only of tokens, zero and keywords; [] if it is.
//
// What counts as fine, deliberately:
//   0 (with or without a unit)     inherit, initial, unset, revert    auto, normal
//   calc()/min()/max()/clamp() whose operands are all tokens, 0 or a bare
//   multiplier: `calc(-1 * var(--space-2))` is the way to write a negative
//   margin, and `calc(var(--space-3) + env(safe-area-inset-bottom))` pads
//   around a notch.
// And what isn't: any other number with a unit (including 1em, 50% and vw,
// even inside calc), a keyword size such as `small`, and a signed var like
// `-var(--x)`, which is invalid CSS anyway.
function judge(kind: Kind | 'font', value: string): string[] {
	const why: string[] = [];
	let v = value.replace(/!\s*important/gi, '').trim();
	v = judgeVars(v, kind === 'font' ? 'text' : kind, why);
	v = v.replace(/\benv\([^()]*\)/gi, 'V').replace(/__DYN__/g, 'V');
	for (const tok of v.split(/[\s(),*/]+/).filter(Boolean)) {
		if (tok === 'V' || KEYWORDS.has(tok.toLowerCase()) || UNITLESS.test(tok) || ZERO.test(tok)) continue;
		// `font` is a shorthand full of families and weights; only the sizes in it
		// are ours to judge.
		if (kind === 'font' && !DIMENSION.test(tok)) continue;
		why.push(`${tok}`);
	}
	return why;
}

interface Violation {
	line: number;
	prop: string;
	value: string;
	why: string;
}

function violationsIn(file: string, text: string): Violation[] {
	const out: Violation[] = [];
	for (const d of declarationsIn(file, text)) {
		const kind = d.prop.startsWith('--') ? kindOfCustom(d.prop) : kindOf(d.prop);
		if (!kind) continue;
		const why = judge(kind, d.value);
		if (why.length) {
			out.push({ line: d.line, prop: d.prop, value: d.value.trim().replace(/\s+/g, ' '), why: why.join('; ') });
		}
	}
	return out;
}

// ── The marker ────────────────────────────────────────────────────────────

const MARKER_TEXT = /tokens:\s*not migrated/i;
const MARKER_LINE = {
	svelte: /^<!--\s*tokens: not migrated\s*-->\s*$/,
	css: /^\/\*\s*tokens: not migrated\s*\*\/\s*$/,
};

type Verdict =
	| { ok: true; marked: boolean }
	| { ok: false; reason: 'literals'; violations: Violation[] }
	| { ok: false; reason: 'stale-marker' }
	| { ok: false; reason: 'misplaced-marker'; line: number };

// One file's verdict. A marked file is skipped, unless the marker is stale.
function evaluate(file: string, text: string): Verdict {
	const lines = text.split('\n');
	const style = file.endsWith('.css') ? 'css' : 'svelte';
	const marked = MARKER_LINE[style].test(lines[0] ?? '');

	// The marker text anywhere but line 1 is a mistake: it does nothing, and
	// the author likely thinks it does.
	const stray = lines.findIndex((l, i) => (i > 0 || !marked) && MARKER_TEXT.test(l));
	if (stray >= 0) return { ok: false, reason: 'misplaced-marker', line: stray + 1 };

	const violations = violationsIn(file, text);
	if (marked) return violations.length ? { ok: true, marked } : { ok: false, reason: 'stale-marker' };
	return violations.length ? { ok: false, reason: 'literals', violations } : { ok: true, marked };
}

// ── The suite ─────────────────────────────────────────────────────────────

function* walk(dir: string): Generator<string> {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) yield* walk(p);
		else if (/\.(svelte|css)$/.test(entry.name)) yield p;
	}
}

function sourceFiles() {
	// app.css is where the tokens are defined, so its literals are the point.
	return [...walk(SRC)]
		.map(p => ({ rel: relative(SRC, p), text: readFileSync(p, 'utf8') }))
		.filter(f => f.rel !== 'app.css');
}

describe('scale tokens (type, gaps, corners)', () => {
	it('unmigrated files carry the marker, and migrated files use only tokens', () => {
		const offenders: string[] = [];
		for (const f of sourceFiles()) {
			const v = evaluate(f.rel, f.text);
			if (!v.ok && v.reason === 'literals') {
				for (const x of v.violations) offenders.push(`${f.rel}:${x.line}  ${x.prop}: ${x.value}   (${x.why})`);
			}
		}
		expect(
			offenders,
			`font sizes, gaps and radii that aren't tokens (see this file's header):\n${offenders.join('\n')}`
		).toEqual([]);
	});

	it('a marker comes off the file that no longer needs it', () => {
		const stale = sourceFiles()
			.filter(f => {
				const v = evaluate(f.rel, f.text);
				return !v.ok && v.reason === 'stale-marker';
			})
			.map(f => f.rel);
		expect(
			stale,
			`these files have no literals left: delete the "tokens: not migrated" first line:\n${stale.join('\n')}`
		).toEqual([]);
	});

	it('the marker is only ever the first line, in the comment style of its file type', () => {
		const misplaced = sourceFiles()
			.map(f => ({ rel: f.rel, v: evaluate(f.rel, f.text) }))
			.filter(x => !x.v.ok && x.v.reason === 'misplaced-marker')
			.map(x => `${x.rel}:${(x.v as { line: number }).line}`);
		expect(
			misplaced,
			`"tokens: not migrated" must be the whole of line 1 (<!-- … --> in .svelte, /* … */ in .css):\n${misplaced.join('\n')}`
		).toEqual([]);
	});

	describe('app.css defines exactly the scales that were ruled', () => {
		const appCss = readFileSync(join(SRC, 'app.css'), 'utf8');
		const defined = (prefix: string) =>
			[...appCss.matchAll(new RegExp(`^\\s*(--${prefix}-[\\w-]+):\\s*([^;]+);`, 'gm'))].map(m => ({
				name: m[1],
				value: m[2].trim(),
			}));
		const rem = (v: string) => {
			const m = v.match(/^([\d.]+)rem$/);
			return m ? parseFloat(m[1]) : NaN;
		};

		// Names, order and units are pinned; the values themselves are not, on
		// purpose: the tokens are named by job so that tuning a value is a
		// one-line change in app.css, not an edit here. What is held is the
		// invariant that matters: steps ascend, and nothing is under 12px.
		it('five text sizes, in rem, none under 12px, each larger than the last', () => {
			const sizes = defined('text').filter(t => t.name !== TEXT_DISPLAY);
			expect(sizes.map(t => t.name)).toEqual(TEXT);
			const rems = sizes.map(t => rem(t.value));
			expect(rems.every(Number.isFinite), 'text sizes must be plain rem values').toBe(true);
			expect(Math.min(...rems) * 16).toBeGreaterThanOrEqual(12);
			expect(rems).toEqual([...rems].sort((a, b) => a - b));
			expect(new Set(rems).size).toBe(rems.length);
			expect(defined('text').map(t => t.name)).toContain(TEXT_DISPLAY);
		});

		it('six gaps, in rem, each larger than the last', () => {
			const gaps = defined('space');
			expect(gaps.map(g => g.name)).toEqual(SPACE);
			const rems = gaps.map(g => rem(g.value));
			expect(rems.every(Number.isFinite), 'gaps must be plain rem values').toBe(true);
			expect(rems).toEqual([...rems].sort((a, b) => a - b));
			expect(new Set(rems).size).toBe(rems.length);
		});

		it('five radii by kind of thing, with the old size-named three left until the cleanup', () => {
			expect(defined('radius').map(r => r.name)).toEqual([...RADIUS, ...RETIRED]);
		});
	});
});

// The guard's own behaviour. These are the rulings about calc(), negative
// margins, inherit and auto, written down as cases, plus proof that it bites
// in both directions: a literal in an unmarked file fails, and a marker on a
// file with nothing left to migrate fails.
describe('the guard itself', () => {
	const css = (body: string) => `.a {\n${body}\n}\n`;
	const svelte = (body: string, marker = false) =>
		`${marker ? '<!-- tokens: not migrated -->\n' : ''}<div class="a">x</div>\n<style>\n${css(body)}</style>\n`;
	const verdict = (body: string, marker = false) => evaluate('X.svelte', svelte(body, marker));

	describe('accepts', () => {
		for (const body of [
			'font-size: var(--text-body);',
			'padding: var(--space-2) var(--space-4);',
			'padding: 0;',
			'padding: 0px;',
			'margin: 0 auto;',
			'margin-inline: auto;',
			'font-size: inherit;',
			'font: inherit;',
			'gap: normal;',
			'border-radius: var(--radius-control);',
			'border-radius: var(--radius-sheet) var(--radius-sheet) 0 0;',
			'border-radius: var(--radius-dot);',
			'margin-left: calc(-1 * var(--space-2));',
			'margin: calc(var(--space-2) * -1) 0;',
			'padding-bottom: calc(var(--space-3) + env(safe-area-inset-bottom));',
			'padding-bottom: calc(var(--chat-strip-height) + var(--space-2));',
			'padding: max(var(--space-4), env(safe-area-inset-left));',
			'gap: var(--chip-gap);',
			'gap: var(--chip-gap, var(--space-2));',
			'font-size: calc(var(--text-display) / 2);',
			'font: 600 var(--text-small)/1.2 var(--font-serif);',
			'font-size: var(--text-display);',
			'row-gap: var(--space-1) !important;',
			'/* padding: 4px; margin: 3px */ margin: 0;',
			'width: 300px; height: 44px; max-width: 12rem;', // widths and heights belong to layoutTokens.test.ts
			'line-height: 1.5; letter-spacing: 0.05em;',
		]) {
			it(body, () => expect(verdict(body)).toEqual({ ok: true, marked: false }));
		}
	});

	describe('rejects', () => {
		for (const [body, why] of [
			['font-size: 0.8rem;', 'a literal size'],
			['font-size: 13px;', 'a literal size'],
			['font-size: 1em;', 'a relative size: say inherit, or use a token'],
			['font-size: small;', 'a keyword size'],
			['font-size: clamp(1rem, 4vw, 2rem);', 'fluid sizes with literals'],
			['font: 14px serif;', 'a size inside the font shorthand'],
			['padding: 0.5rem;', 'a literal gap'],
			['padding: 0 12px;', 'one literal among tokens and zero'],
			['margin-top: 4px;', 'a literal margin'],
			['margin-left: -0.5rem;', 'a negative literal'],
			['margin-left: -var(--space-2);', 'a signed var'],
			['margin: calc(0.5rem + 2px);', 'a literal inside calc'],
			['margin: calc(100% - var(--space-4));', 'a percentage inside calc'],
			['gap: 8px;', 'a literal gap'],
			['column-gap: 1em;', 'a literal gap'],
			['padding: 50%;', 'a percentage'],
			['border-radius: 6px;', 'a literal radius'],
			['border-radius: 50%;', 'a circle by percentage: use --radius-dot'],
			['border-bottom-left-radius: 4px;', 'a corner radius'],
			['border-radius: var(--radius-md);', 'the retired size-named radius'],
			['border-radius: var(--radius-sm);', 'the retired size-named radius'],
			['padding: var(--space-7);', 'a step that does not exist'],
			['font-size: var(--text-huge);', 'a step that does not exist'],
			['padding: var(--text-body);', 'the wrong family'],
			['font-size: var(--space-2);', 'the wrong family'],
			['border-radius: var(--space-1);', 'the wrong family'],
			['gap: var(--chip-gap, 0.4rem);', 'a literal hidden in a fallback'],
			['--chip-gap: 0.4rem;', 'a gap smuggled through a custom property'],
			['--board-gutter: 15px;', 'a gutter smuggled through a custom property'],
			['--card-radius: 6px;', 'a radius smuggled through a custom property'],
			['padding: 7px !important;', 'a literal wearing !important'],
			['padding: 7px ! important;', 'a literal wearing a spaced !important'],
			['margin: 3px !important; padding: var(--space-1) !important;', 'one literal among important declarations'],
		] as const) {
			it(`${body}   (${why})`, () => {
				const v = verdict(body);
				expect(v.ok).toBe(false);
				expect(v.ok === false && v.reason).toBe('literals');
			});
		}
	});

	describe('reads declarations wherever they hide', () => {
		it('in a .css file', () => {
			expect(evaluate('x.css', '.a { padding: 3px; }').ok).toBe(false);
			expect(evaluate('x.css', '.a { padding: var(--space-2); }').ok).toBe(true);
		});
		it('in an inline style attribute', () => {
			const v = evaluate('X.svelte', '<p style="margin:0;padding-left:1.25rem;">hi</p>\n');
			expect(v.ok === false && v.reason === 'literals' && v.violations.map(x => x.line)).toEqual([1]);
		});
		it('in a style: directive, but not when the value is an expression', () => {
			expect(evaluate('X.svelte', '<p style:gap="0.5rem">hi</p>\n').ok).toBe(false);
			expect(evaluate('X.svelte', '<p style:gap={g}>hi</p>\n').ok).toBe(true);
		});
		it('in a string or template literal inside a style expression', () => {
			for (const src of [
				"<p style:padding={'7px'}>x</p>",
				'<p style:padding={`7px`}>x</p>',
				"<p style:gap={big ? '12px' : 'var(--space-2)'}>x</p>",
				'<p style:padding|important="7px">x</p>',
				'<p style={`padding:7px`}>x</p>',
				"<p style={wide ? 'margin-left: -0.5rem' : ''}>x</p>",
				'<p style={`gap: ${n}px`}>x</p>',
				"<p style=\"padding: {wide ? '7px' : 'var(--space-2)'}\">x</p>",
				"<p style='padding: {wide ? \"7px\" : \"0\"}'>x</p>",
				'<p style="gap: 4px; margin: {m}">x</p>',
			]) {
				expect(evaluate('X.svelte', src + '\n').ok, src).toBe(false);
			}
		});
		it('but lets token strings, runtime picks and non-guarded properties through', () => {
			for (const src of [
				"<p style:padding={'var(--space-2)'}>x</p>",
				"<p style:gap={big ? 'var(--space-3)' : 'var(--space-2)'}>x</p>",
				'<p style={`padding: var(--space-${n})`}>x</p>',
				"<p style={hidden ? 'display: none' : ''}>x</p>",
				"<p style:color={dark ? 'red' : 'blue'}>x</p>",
				'<p style:--accent={color}>x</p>',
				'<p style={styleFor(row)}>x</p>',
			]) {
				expect(evaluate('X.svelte', src + '\n'), src).toEqual({ ok: true, marked: false });
			}
		});
		it('reports a literal inside a style expression on the attribute\'s line', () => {
			const v = evaluate('X.svelte', '<div>\n<p\n\tstyle:gap={big ? \'12px\' : \'0\'}\n>x</p></div>\n');
			expect(v.ok === false && v.reason === 'literals' && v.violations.map(x => x.line)).toEqual([3]);
		});
		it('after an {expression} that gets a unit stuck on it', () => {
			expect(evaluate('X.svelte', '<p style="gap: {n}px">hi</p>\n').ok).toBe(false);
			expect(evaluate('X.svelte', '<p style="gap: {g}">hi</p>\n').ok).toBe(true);
		});
		it('inside nested rules and at-rules', () => {
			const body = '@media (min-width: 790px) { .b { & .c { gap: 5px; } } }';
			expect(verdict(body).ok).toBe(false);
		});
		it('after a data URI with its own semicolons', () => {
			const body = "background: url('data:image/svg+xml;utf8,<svg/>'); padding: 3px;";
			expect(verdict(body).ok).toBe(false);
		});
		it('on the right line', () => {
			const v = evaluate('X.svelte', '<div>\n</div>\n<style>\n.a {\n\tcolor: red;\n\tgap: 7px;\n}\n</style>\n');
			expect(v.ok === false && v.reason === 'literals' && v.violations[0].line).toBe(6);
		});
		it('but not in a script block, a comment, or prose', () => {
			const src = [
				'<script lang="ts">',
				'	let box: { gap: number; padding: string } = { gap: 4, padding: "4px" };',
				'	const css = "font-size: 13px";',
				'</script>',
				'<!-- <p style="padding: 3px"> -->',
				'<p>Set padding: 3px for a tight fit.</p>',
				'<style>/* gap: 5px */ .a { gap: var(--space-1); }</style>',
			].join('\n');
			expect(evaluate('X.svelte', src)).toEqual({ ok: true, marked: false });
		});
	});

	describe('bites in both directions', () => {
		it('a literal in an unmarked file fails', () => {
			expect(verdict('padding: 0.6rem;')).toMatchObject({ ok: false, reason: 'literals' });
		});
		it('the same literal passes once the file carries the marker', () => {
			expect(verdict('padding: 0.6rem;', true)).toEqual({ ok: true, marked: true });
		});
		it('a marker on a file with nothing left to migrate fails', () => {
			expect(verdict('padding: var(--space-2);', true)).toEqual({ ok: false, reason: 'stale-marker' });
			expect(evaluate('x.css', '/* tokens: not migrated */\n.a { padding: 0; }\n')).toEqual({
				ok: false,
				reason: 'stale-marker',
			});
		});
		it('a marker that is not the first line does nothing, and is an error', () => {
			const src = '<script></script>\n<!-- tokens: not migrated -->\n<style>.a { gap: 3px; }</style>\n';
			expect(evaluate('X.svelte', src)).toEqual({ ok: false, reason: 'misplaced-marker', line: 2 });
		});
		it('a marker in the wrong comment style for its file type is an error', () => {
			expect(evaluate('X.svelte', '/* tokens: not migrated */\n<style>.a { gap: 3px; }</style>\n').ok).toBe(false);
			expect(evaluate('x.css', '<!-- tokens: not migrated -->\n.a { gap: 3px; }\n').ok).toBe(false);
		});
	});
});
