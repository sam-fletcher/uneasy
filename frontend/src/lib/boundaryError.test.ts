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
 * `onerror` — logBoundaryError is the one-line way.
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

/** Every <svelte:boundary> in the file, as "path:line", split by whether it has `onerror`. */
function boundaries(file: string) {
	const src = readFileSync(file, 'utf8');
	const silent: string[] = [];
	const logged: string[] = [];
	const walk = (node: unknown) => {
		if (!node || typeof node !== 'object') return;
		const n = node as AstNode;
		if (n.type === 'SvelteBoundary') {
			const at = `${relative(SRC, file)}:${src.slice(0, n.start).split('\n').length}`;
			const attrs = (n.attributes as AstNode[] | undefined) ?? [];
			(attrs.some(a => a.type === 'Attribute' && a.name === 'onerror') ? logged : silent).push(at);
		}
		for (const key of Object.keys(n)) {
			const v = n[key];
			if (Array.isArray(v)) v.forEach(walk);
			else if (v && typeof v === 'object' && 'type' in v) walk(v);
		}
	};
	walk(parse(src, { modern: true }).fragment);
	return { silent, logged };
}

describe('error boundaries', () => {
	const all = svelteFiles(SRC).map(boundaries);

	it('every <svelte:boundary> has an onerror, so what it catches is not silent', () => {
		expect(all.flatMap(b => b.silent)).toEqual([]);
	});

	it('finds the boundary it is guarding (a rename of the node type would pass the check above for nothing)', () => {
		expect(all.flatMap(b => b.logged).length).toBeGreaterThan(0);
	});
});
