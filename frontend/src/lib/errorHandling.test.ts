import { describe, it, expect } from 'vitest';
import { readFileSync, readdirSync } from 'node:fs';
import { join, relative } from 'node:path';

// Guard test in the shape of designTokens.test.ts / layoutTokens.test.ts: the
// conventions in docs/STYLE_GUIDE.md "Errors" that a reviewer would otherwise
// have to catch by eye. Source-text checks, deliberately — a parser would be a
// dependency for very little extra precision. See adr/ERROR_HANDLING_PLAN.md.

const SRC = new URL('..', import.meta.url).pathname;

function walk(dir: string, out: string[] = []): string[] {
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const full = join(dir, entry.name);
		if (entry.isDirectory()) walk(full, out);
		else if (/\.(svelte|ts)$/.test(entry.name) && !entry.name.endsWith('.test.ts')) {
			out.push(full);
		}
	}
	return out;
}

const files = walk(SRC).map((f) => [relative(SRC, f), readFileSync(f, 'utf8')] as const);

describe('errors render through ErrorText', () => {
	// Rule 3. role="alert" cannot come from CSS, so the markup has to be shared;
	// a raw <p class="error-text"> is silent to a screen reader.
	it('has no raw error-text / res-error elements outside ErrorText itself', () => {
		const offenders = files
			.filter(([path]) => !path.endsWith('shared/ErrorText.svelte'))
			.filter(([, src]) => /class="(error-text|res-error)[^"]*"/.test(src))
			.map(([path]) => path);

		expect(offenders, 'render these through <ErrorText> — see STYLE_GUIDE "Errors"').toEqual([]);
	});
});

describe('silent catches are explained', () => {
	// Rule 4. Swallowing is often correct; an unexplained swallow is
	// indistinguishable from an oversight.
	const EMPTY_CATCH = /catch\s*(?:\([^)]*\)\s*)?\{([^{}]*)\}/g;

	it('has no empty catch block without a comment saying why', () => {
		const offenders: string[] = [];

		for (const [path, src] of files) {
			for (const m of src.matchAll(EMPTY_CATCH)) {
				const body = m[1];
				if (body.trim() === '') {
					// `catch {}` with nothing at all, not even a comment.
					const line = src.slice(0, m.index).split('\n').length;
					offenders.push(`${path}:${line}`);
					continue;
				}
				// Anything else either handles the failure or explains itself
				// with a comment; both are fine.
			}
		}

		expect(offenders, 'add a comment saying why the failure is safe to swallow').toEqual([]);
	});
});

describe('status is read from ApiError, not from message text', () => {
	// Server prose is not an API. There were already two wordings for the
	// "no war yet" 404 when MakeWarPanel was matching /no war/i against it.
	it('does not branch on HTTP status embedded in an error message', () => {
		const offenders = files
			.filter(([, src]) => /\.message\s*\)?\s*\.(includes|startsWith|match)\(|test\(\s*(?:msg|message)\b/.test(src))
			.map(([path]) => path);

		expect(offenders, 'branch on `err instanceof ApiError && err.status === …` instead').toEqual([]);
	});
});
