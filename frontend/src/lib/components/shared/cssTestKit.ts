// A small kit for tests that have to check a stylesheet's RULES, in a suite
// with no DOM (so no getComputedStyle). Two jobs:
//
//   1. Read CSS without being fooled by comments: `stripComments`, `styleOf`,
//      `declarations`. A test that greps raw source fails when a comment
//      merely mentions a property, and passes when a comment hides nothing.
//
//   2. Answer "which rule wins?": `winning` runs the cascade (specificity,
//      then source order) for an element described by its classes, pseudo
//      states and attributes. It exists because the bug it catches is a
//      cascade bug: a hover rule out-ranking `.selected` so the selected
//      chip's border turned grey on hover. A grep can't see that.
//
// It understands only what the shared recipes use: compound selectors (no
// combinators), classes, attributes, pseudo-classes and :not(). It is not a
// CSS engine, and it throws on a selector it can't read rather than guess.

/** Remove /* … *\/ and <!-- … --> comments, keeping newlines so line numbers hold. */
export function stripComments(source: string): string {
	return source
		.replace(/\/\*[\s\S]*?\*\//g, (m) => m.replace(/[^\n]/g, ' '))
		.replace(/<!--[\s\S]*?-->/g, (m) => m.replace(/[^\n]/g, ' '));
}

/** The CSS of a .svelte file: its <style> blocks, comments removed. */
export function styleOf(svelteSource: string): string {
	const blocks = [...stripComments(svelteSource).matchAll(/<style[^>]*>([\s\S]*?)<\/style>/g)];
	return blocks.map((m) => m[1]).join('\n');
}

export interface Declaration {
	selector: string;
	prop: string;
	value: string;
}

export interface Rule {
	selector: string;
	body: string;
	order: number;
}

/** Flat rules (no nesting; @keyframes and @media bodies are skipped). */
export function parseRules(css: string): Rule[] {
	const flat = stripComments(css);
	const rules: Rule[] = [];
	let order = 0;
	for (const m of flat.matchAll(/([^{}@]+)\{([^{}]*)\}/g)) {
		const selector = m[1].trim();
		if (!selector || /^\d+%|^from\b|^to\b/.test(selector)) continue;
		rules.push({ selector, body: m[2], order: order++ });
	}
	return rules;
}

/** Every `prop: value` in every rule, with the rule's selector. */
export function declarations(css: string): Declaration[] {
	const out: Declaration[] = [];
	for (const r of parseRules(css)) {
		for (const d of r.body.split(';')) {
			const i = d.indexOf(':');
			if (i < 0) continue;
			const prop = d.slice(0, i).trim();
			if (prop) out.push({ selector: r.selector, prop, value: d.slice(i + 1).trim() });
		}
	}
	return out;
}

// ── Cascade ────────────────────────────────────────────────────────────────

export interface El {
	classes?: string[];
	/** Pseudo-classes that currently apply: 'hover', 'disabled', 'focus-visible'. */
	pseudos?: string[];
	attrs?: Record<string, string>;
}

type Spec = [number, number, number];
const TOKEN = /(\.[\w-]+|:not\((?:[^()]|\([^()]*\))*\)|:[\w-]+|\[[^\]]+\]|[a-zA-Z][\w-]*)/g;

function tokens(compound: string): string[] {
	const found = compound.match(TOKEN) ?? [];
	if (found.join('') !== compound.replace(/\s+/g, '')) {
		throw new Error(`cssTestKit can't read selector "${compound}" (compound selectors only)`);
	}
	return found;
}

function tokenMatches(tok: string, el: El): boolean {
	if (tok.startsWith('.')) return (el.classes ?? []).includes(tok.slice(1));
	if (tok.startsWith(':not(')) return !compoundMatches(tok.slice(5, -1), el);
	if (tok.startsWith(':')) return (el.pseudos ?? []).includes(tok.slice(1));
	if (tok.startsWith('[')) {
		const m = tok.match(/^\[([\w-]+)(?:=['"]?([^'"\]]*)['"]?)?\]$/);
		if (!m) throw new Error(`cssTestKit can't read attribute selector ${tok}`);
		const have = el.attrs?.[m[1]];
		return m[2] === undefined ? have !== undefined : have === m[2];
	}
	return true; // an element name: every specimen is the element the file styles
}

function compoundMatches(compound: string, el: El): boolean {
	return tokens(compound).every((t) => tokenMatches(t, el));
}

function specOf(compound: string): Spec {
	let a = 0, b = 0, c = 0;
	for (const t of tokens(compound)) {
		if (t.startsWith(':not(')) {
			const [x, y, z] = specOf(t.slice(5, -1));
			a += x; b += y; c += z;
		} else if (t.startsWith('.') || t.startsWith('[') || t.startsWith(':')) b += 1;
		else c += 1;
	}
	return [a, b, c];
}

const higher = (x: Spec, y: Spec) => x[0] - y[0] || x[1] - y[1] || x[2] - y[2];

/** The value `prop` ends up with on `el`, or null if no rule sets it. */
export function winning(css: string, prop: string, el: El): string | null {
	let best: { spec: Spec; order: number; value: string } | null = null;
	for (const rule of parseRules(css)) {
		for (const sel of rule.selector.split(',').map((s) => s.trim())) {
			if (!compoundMatches(sel, el)) continue;
			const decl = rule.body
				.split(';')
				.map((d) => d.split(/:(.*)/s))
				.find(([p]) => p && p.trim() === prop);
			if (!decl) continue;
			const spec = specOf(sel);
			if (!best || higher(spec, best.spec) > 0 || (higher(spec, best.spec) === 0 && rule.order > best.order)) {
				best = { spec, order: rule.order, value: decl[1].trim() };
			}
		}
	}
	return best ? best.value : null;
}
