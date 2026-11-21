import { afterEach, describe, expect, it, vi } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import { parse } from 'svelte/compiler';
import { BOUNDARY_ERROR_PREFIX, logBoundaryError } from './boundaryError';

afterEach(() => { vi.restoreAllMocks(); });

describe('logBoundaryError', () => {
	it('writes the caught error to the console under the recognisable prefix', () => {
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});
		const error = new Error('each_key_duplicate');

		logBoundaryError('table phase view (main_event)', error);

		expect(spy).toHaveBeenCalledTimes(1);
		const [label, logged] = spy.mock.calls[0];
		expect(label).toBe(`${BOUNDARY_ERROR_PREFIX} table phase view (main_event)`);
		// The Error itself, not its message: the console prints its stack.
		expect(logged).toBe(error);
	});

	it('also takes a thrown value that is not an Error', () => {
		const spy = vi.spyOn(console, 'error').mockImplementation(() => {});

		logBoundaryError('somewhere', 'a bare string');

		expect(spy.mock.calls[0][1]).toBe('a bare string');
	});
});

/*
 * A <svelte:boundary> with a `failed` snippet but no `onerror` swallows what it
 * catches. Reading each .svelte file's AST (as blockWhitespace.test.ts does)
 * makes that a failing test rather than something to remember: the table
 * page's boundary sat silent until the nightly e2e of 2026-11-08 had to be
 * debugged through CDP. Whoever rewrites or adds a boundary must give it an
 * `onerror` that calls logBoundaryError.
 *
 * Having an `onerror` is not enough. The e2e fixture (tests/e2e/fixtures.ts)
 * fails a test when a page logs BOUNDARY_ERROR_PREFIX, and only
 * logBoundaryError writes it. A boundary whose handler is `() => {}`, or that
 * logs some other way (`console.error(error)`), would show the player the
 * fallback while every spec passed over it. So the handler must call the
 * function this file exports, and the component must import it from here (a
 * local function of the same name that logs nothing would otherwise satisfy
 * the check).
 */
const SRC = join(__dirname, '..');

function svelteFiles(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...svelteFiles(p));
		else if (entry.name.endsWith('.svelte')) out.push(p);
	}
	return out;
}

type AstNode = { type: string; start: number; name?: string; [k: string]: unknown };

/** Every node of a given type under `node`, depth first. */
function nodesOf(node: unknown, type: string): AstNode[] {
	if (!node || typeof node !== 'object') return [];
	const n = node as AstNode;
	const found = n.type === type ? [n] : [];
	for (const key of Object.keys(n)) {
		const v = n[key];
		if (Array.isArray(v)) found.push(...v.flatMap(c => nodesOf(c, type)));
		else if (v && typeof v === 'object' && 'type' in v) found.push(...nodesOf(v, type));
	}
	return found;
}

/**
 * The name this component gives `logBoundaryError` (the import can be aliased),
 * or null when it doesn't import it from boundaryError.
 */
function logBoundaryErrorName(ast: ReturnType<typeof parse>): string | null {
	const imports = nodesOf(ast.instance, 'ImportDeclaration');
	for (const imp of imports) {
		const from = (imp.source as { value?: unknown }).value;
		if (typeof from !== 'string' || !/(^|\/)boundaryError$/.test(from)) continue;
		for (const spec of imp.specifiers as AstNode[]) {
			const imported = spec.imported as AstNode | undefined;
			if (spec.type === 'ImportSpecifier' && imported?.name === 'logBoundaryError') {
				return (spec.local as AstNode).name ?? null;
			}
		}
	}
	return null;
}

/**
 * Every <svelte:boundary> in a component's source, as "name:line", sorted by
 * what its `onerror` does: nothing (`silent`), something that never calls
 * logBoundaryError (`unlogged`), or calls it (`logged`).
 */
function boundariesIn(src: string, name: string) {
	const out = { silent: [] as string[], unlogged: [] as string[], logged: [] as string[] };
	const ast = parse(src, { modern: true });
	const logFn = logBoundaryErrorName(ast);
	for (const b of nodesOf(ast.fragment, 'SvelteBoundary')) {
		const at = `${name}:${src.slice(0, b.start).split('\n').length}`;
		const attrs = (b.attributes as AstNode[] | undefined) ?? [];
		const onerror = attrs.find(a => a.type === 'Attribute' && a.name === 'onerror');
		const callsIt =
			logFn !== null &&
			nodesOf(onerror?.value, 'CallExpression').some(c => (c.callee as AstNode).name === logFn);
		(onerror === undefined ? out.silent : callsIt ? out.logged : out.unlogged).push(at);
	}
	return out;
}

describe('error boundaries', () => {
	const all = svelteFiles(SRC).map(f => boundariesIn(readFileSync(f, 'utf8'), relative(SRC, f)));

	it('every <svelte:boundary> has an onerror, so what it catches is not silent', () => {
		expect(all.flatMap(b => b.silent)).toEqual([]);
	});

	it("every <svelte:boundary>'s onerror calls logBoundaryError, the line the e2e fixture listens for", () => {
		expect(
			all.flatMap(b => b.unlogged),
			"call logBoundaryError(where, error) from the boundary's onerror, importing it from $lib/boundaryError. " +
			'Any other handler is invisible to tests/e2e/fixtures.ts: a spec would pass over the fallback.',
		).toEqual([]);
	});

	it('finds the boundary it is guarding (a rename of the node type would pass the checks above for nothing)', () => {
		expect(all.flatMap(b => b.logged).length).toBeGreaterThan(0);
	});
});

describe('the boundary matcher', () => {
	const IMPORT = `<script>import { logBoundaryError } from '$lib/boundaryError';</script>`;
	const boundary = (attrs: string) => `<svelte:boundary ${attrs}>x{#snippet failed()}y{/snippet}</svelte:boundary>`;
	const sort = (src: string) => boundariesIn(src, 'T.svelte');

	it('accepts a handler that calls it', () => {
		const b = sort(IMPORT + boundary('onerror={(e) => logBoundaryError("where", e)}'));
		expect(b).toEqual({ silent: [], unlogged: [], logged: ['T.svelte:1'] });
	});

	it('accepts a handler that calls it among other statements, and an aliased import', () => {
		const body = boundary('onerror={(e, reset) => { track(e); log("where", e); }}');
		const aliased = `<script>import { logBoundaryError as log } from '$lib/boundaryError';</script>`;
		expect(sort(aliased + body).logged).toHaveLength(1);
	});

	it('sorts a boundary with no onerror as silent', () => {
		expect(sort(IMPORT + boundary('')).silent).toEqual(['T.svelte:1']);
	});

	it('sorts an empty handler as unlogged', () => {
		expect(sort(IMPORT + boundary('onerror={() => {}}')).unlogged).toEqual(['T.svelte:1']);
	});

	it('sorts a handler that logs some other way as unlogged', () => {
		expect(sort(IMPORT + boundary('onerror={(e) => console.error(e)}')).unlogged).toHaveLength(1);
	});

	it('sorts a handler that only passes logBoundaryError along, or names it without calling it, as unlogged', () => {
		expect(sort(IMPORT + boundary('onerror={logBoundaryError}')).unlogged).toHaveLength(1);
		expect(sort(IMPORT + boundary('onerror={(e) => { const f = logBoundaryError; }}')).unlogged).toHaveLength(1);
	});

	it('sorts a handler defined elsewhere in the component as unlogged: the call must be in the attribute', () => {
		const src = `<script>import { logBoundaryError } from '$lib/boundaryError'; const h = (e) => logBoundaryError('w', e);</script>` + boundary('onerror={h}');
		expect(sort(src).unlogged).toHaveLength(1);
	});

	it('sorts a call to a lookalike that is not imported from boundaryError as unlogged', () => {
		const local = `<script>function logBoundaryError() {}</script>`;
		const elsewhere = `<script>import { logBoundaryError } from '$lib/somewhereElse';</script>`;
		const call = boundary('onerror={(e) => logBoundaryError("w", e)}');
		expect(sort(local + call).unlogged).toHaveLength(1);
		expect(sort(elsewhere + call).unlogged).toHaveLength(1);
		expect(sort(call).unlogged).toHaveLength(1);
	});

	it('sorts each boundary on its own, nested ones included', () => {
		const good = 'onerror={(e) => logBoundaryError("w", e)}';
		const src = IMPORT + `<svelte:boundary ${good}><svelte:boundary onerror={() => {}}>x</svelte:boundary></svelte:boundary>`;
		const b = sort(src);
		expect(b.logged).toHaveLength(1);
		expect(b.unlogged).toHaveLength(1);
	});
});
