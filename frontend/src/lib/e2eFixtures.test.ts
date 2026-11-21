import { describe, expect, it } from 'vitest';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import {
	ALLOWED_CONSOLE_ERRORS,
	isAllowedConsoleError,
	type AllowedConsoleError,
} from '../../tests/e2e/allowedConsoleErrors';

/*
 * Every e2e spec takes `test` from tests/e2e/fixtures.ts, not from
 * `@playwright/test`. The fixture fails any test whose page logs the table's
 * error-boundary line (`[uneasy:boundary]`), so a render crash turns a spec red
 * instead of passing over a half-drawn table, as it did in the nightly run of
 * 2026-11-08. A spec that imports `test` straight from Playwright runs without
 * that check, and nothing would say so. This makes it a failing test, so
 * `make check-fast` catches it on the PR that adds the spec.
 *
 * Only `test` matters: `expect`, `request` and the types are the same from
 * either module, and `import type` costs nothing at run time. "Playwright's
 * own" means both names the same module goes by: `@playwright/test`, and
 * `playwright/test`, which it re-exports and which is installed beside it.
 */
const E2E = join(__dirname, '../../tests/e2e');
const FIXTURES = join(E2E, 'fixtures.ts');
const TRACKER_NEXT = join(__dirname, '../../../docs/work/next');

function tsFiles(dir: string): string[] {
	const out: string[] = [];
	for (const entry of readdirSync(dir, { withFileTypes: true })) {
		const p = join(dir, entry.name);
		if (entry.isDirectory()) out.push(...tsFiles(p));
		else if (entry.name.endsWith('.ts')) out.push(p);
	}
	return out;
}

/**
 * Whether a module's import list reaches the value `test`: by name (`test`,
 * `test as t`) or as the module's default export, which is the same object
 * (`default as t`, or `export { default }`).
 */
function bindsTest(list: string): boolean {
	return list
		.replace(/\/\*[\s\S]*?\*\/|\/\/.*$/gm, '')
		.split(',')
		.map(s => s.trim())
		.filter(s => s !== '' && !/^type\s/.test(s))
		.some(s => ['test', 'default'].includes(s.split(/\s+as\s+/)[0]));
}

/**
 * How the file reaches Playwright's own `test`, one string per offence ("" when it doesn't):
 * a named import of `test`; a namespace or default import (either can say
 * `pw.test`); a re-export; any of these from `playwright/test` too. Not
 * `import('@playwright/test').Browser` in a type position, which several specs
 * use and which never touches `test`.
 */
function directTestImports(src: string): string[] {
	const found: string[] = [];
	const FROM = String.raw`\s*from\s*['"](?:@playwright|playwright)/test['"]`;

	// import { test, … } from …   /   export { test, … } from …   (not `import type`)
	for (const m of src.matchAll(new RegExp(String.raw`\b(import|export)\s*\{([^}]*)\}${FROM}`, 'g'))) {
		if (bindsTest(m[2])) found.push(m[0].replace(/\s+/g, ' '));
	}
	// import * as pw from …   /   import pw from …   /   import pw, { … } from …   /   export * from …
	for (const m of src.matchAll(new RegExp(String.raw`\b(import|export)\s*(\*(?:\s+as\s+\w+)?|\w+\s*(?:,\s*\{[^}]*\})?)${FROM}`, 'g'))) {
		if (!/^type\b/.test(m[2])) found.push(m[0].replace(/\s+/g, ' '));
	}
	return found;
}

describe('e2e specs get `test` from the fixtures module', () => {
	const files = tsFiles(E2E).filter(f => f !== FIXTURES);
	const sources = files.map(f => [relative(E2E, f), readFileSync(f, 'utf8')] as const);

	it("no file under tests/e2e imports Playwright's own `test`", () => {
		const offenders = sources.flatMap(([path, src]) => directTestImports(src).map(what => `${path}: ${what}`));

		expect(
			offenders,
			"import `test` from the fixtures module instead: `import { test } from './fixtures'` (`'../fixtures'` in helpers/). " +
			'It fails a test whose page shows the table error boundary. See tests/e2e/fixtures.ts.',
		).toEqual([]);
	});

	it('sees the specs it is guarding (a changed import style would pass the check above for nothing)', () => {
		const specs = sources.filter(([path]) => path.endsWith('.spec.ts'));
		expect(specs.length).toBeGreaterThan(20);
		const usingFixtures = specs.filter(([, src]) => /from\s*['"]\.\/fixtures['"]/.test(src));
		expect(usingFixtures.length).toBeGreaterThan(20);
	});

	it('only deleteGame deletes a game, so the guard hears about it (a raw POST would leave the teardown\'s 403s counting)', () => {
		const HELPERS = join(E2E, 'helpers.ts');
		const offenders = sources
			.filter(([path]) => join(E2E, path) !== HELPERS)
			.filter(([, src]) => src.includes('/api/dev/delete-game'))
			.map(([path]) => path);

		expect(
			offenders,
			"call deleteGame(request, gameId) from tests/e2e/helpers.ts instead of posting to /api/dev/delete-game. " +
			"It tells the fixtures guard the table is gone, so the 403s and 404s its still-open pages get are not counted against the app.",
		).toEqual([]);
	});

	it('the fixtures module itself watches for the boundary line, from the app\'s own constant', () => {
		const src = readFileSync(FIXTURES, 'utf8');
		expect(src).toMatch(/import\s*\{\s*BOUNDARY_ERROR_PREFIX\s*\}\s*from\s*['"]\.\.\/\.\.\/src\/lib\/boundaryError['"]/);
		expect(src).toMatch(/msg\.text\(\)[\s\S]*includes\(BOUNDARY_ERROR_PREFIX\)/);
	});

	it('the fixtures module also listens for uncaught exceptions and console errors, so a rewrite can\'t drop either', () => {
		const src = readFileSync(FIXTURES, 'utf8');
		expect(src).toMatch(/context\.on\('weberror'/);
		expect(src).toMatch(/msg\.type\(\) === 'error'/);
		expect(src).toMatch(/isAllowedConsoleError\(/);
	});
});

/*
 * The console.error allow-list (tests/e2e/allowedConsoleErrors.ts) is the one
 * place the e2e guard agrees to stay blind, so its entries are checked here, in
 * `make check-fast`, rather than trusted: each names a spec that still exists,
 * says why, and if it stands for an app bug, points at the tracker item that
 * holds the bug. Finish the item and delete its file, and the entry has to go
 * with it, or the guard would go on hiding a bug that no longer exists.
 */
describe('the console.error allow-list', () => {
	// Longer than this and the list costs more than it buys (see allowedConsoleErrors.ts).
	const MAX_ENTRIES = 10;

	it('stays short: a long list means the harness should change instead', () => {
		expect(
			ALLOWED_CONSOLE_ERRORS.length,
			`more than ${MAX_ENTRIES} allowed console errors: fix what logs them, or change the harness (as deleteGame does for a deleted table) instead of adding another`,
		).toBeLessThanOrEqual(MAX_ENTRIES);
	});

	it('gives every entry a reason', () => {
		expect(ALLOWED_CONSOLE_ERRORS.filter(a => a.reason.trim().length < 15).map(a => String(a.match))).toEqual([]);
	});

	it('names, for every entry, a spec that exists', () => {
		const missing = ALLOWED_CONSOLE_ERRORS.filter(a => a.spec !== undefined && !existsSync(join(E2E, a.spec)));
		expect(missing.map(a => `${a.spec}: ${a.match}`), 'a renamed or deleted spec leaves its entry allowing nothing').toEqual([]);
	});

	it('points every bug entry at a tracker item that still exists', () => {
		const gone = ALLOWED_CONSOLE_ERRORS.filter(a => a.item !== undefined && !existsSync(join(TRACKER_NEXT, `${a.item}.md`)));
		expect(
			gone.map(a => `${a.item}: ${a.match}`),
			'the item is finished or renamed: delete the entry (the bug is fixed) or fix its `item`',
		).toEqual([]);
	});

	it('keeps every pattern narrow enough to reject an unrelated line', () => {
		const wide = ALLOWED_CONSOLE_ERRORS.filter(a => a.match.test('') || a.match.test('some unrelated console error'));
		expect(wide.map(a => String(a.match))).toEqual([]);
	});
});

describe('the allow-list matcher', () => {
	const entry = (over: Partial<AllowedConsoleError> = {}): AllowedConsoleError => ({
		match: /status of 500 .* @ \S+\/api\/tables\/\d+\/posts$/,
		spec: 'chat.spec.ts',
		reason: 'a reason long enough',
		...over,
	});
	const LINE = 'Failed to load resource: the server responded with a status of 500 (Internal Server Error) @ http://localhost:8090/api/tables/17/posts';

	it('lets a line through for the spec that is named, and only that spec', () => {
		expect(isAllowedConsoleError(LINE, 'chat.spec.ts', [entry()])).toBe(true);
		expect(isAllowedConsoleError(LINE, 'make_war.spec.ts', [entry()])).toBe(false);
	});

	it('lets an entry with no spec through for any spec', () => {
		expect(isAllowedConsoleError(LINE, 'anything.spec.ts', [entry({ spec: undefined })])).toBe(true);
	});

	it('is as narrow as its pattern: another status, another route and a trailing path all fail', () => {
		const list = [entry()];
		expect(isAllowedConsoleError(LINE.replace('500 (Internal Server Error)', '403 (Forbidden)'), 'chat.spec.ts', list)).toBe(false);
		expect(isAllowedConsoleError(LINE.replace('/posts', '/wars'), 'chat.spec.ts', list)).toBe(false);
		expect(isAllowedConsoleError(`${LINE}/extra`, 'chat.spec.ts', list)).toBe(false);
		expect(isAllowedConsoleError('some other error', 'chat.spec.ts', list)).toBe(false);
	});

	it('is closed by default: an empty list allows nothing', () => {
		expect(isAllowedConsoleError(LINE, 'chat.spec.ts', [])).toBe(false);
	});
});

describe('the import matcher', () => {
	// The matcher is the whole guard: pin what it reads, so it can't quietly go blind.
	const hits = (src: string) => directTestImports(src).length > 0;

	it('flags Playwright imports that reach `test`', () => {
		expect(hits(`import { test, expect } from '@playwright/test';`)).toBe(true);
		expect(hits(`import { expect, test as t, type Page } from "@playwright/test";`)).toBe(true);
		expect(hits(`import {\n\texpect,\n\ttest,\n} from '@playwright/test';`)).toBe(true);
		expect(hits(`import * as pw from '@playwright/test';`)).toBe(true);
		expect(hits(`import pw from '@playwright/test';`)).toBe(true);
		expect(hits(`export { test } from '@playwright/test';`)).toBe(true);
		expect(hits(`export * from '@playwright/test';`)).toBe(true);
		// The same module under its other name, and `test` as the default export.
		expect(hits(`import { test } from 'playwright/test';`)).toBe(true);
		expect(hits(`import { expect, test as t } from "playwright/test";`)).toBe(true);
		expect(hits(`import * as pw from 'playwright/test';`)).toBe(true);
		expect(hits(`import { default as t } from '@playwright/test';`)).toBe(true);
		expect(hits(`import { type Page, default as t } from 'playwright/test';`)).toBe(true);
		expect(hits(`export { default } from '@playwright/test';`)).toBe(true);
		expect(hits(`export { default as test } from 'playwright/test';`)).toBe(true);
	});

	it('lets through what is the same from either module', () => {
		expect(hits(`import { expect, request as pwRequest, type Page } from '@playwright/test';`)).toBe(false);
		expect(hits(`import { type test } from '@playwright/test';`)).toBe(false);
		expect(hits(`import type { test } from '@playwright/test';`)).toBe(false);
		expect(hits(`import {\n\texpect, // not test\n\ttype Page,\n} from '@playwright/test';`)).toBe(false);
		expect(hits(`import { test, expect } from './fixtures';`)).toBe(false);
		expect(hits(`import { expect, type Page } from 'playwright/test';`)).toBe(false);
		expect(hits(`import { type default as t } from '@playwright/test';`)).toBe(false);
		expect(hits(`import { testInfo } from '@playwright/test';`)).toBe(false);
		expect(hits(`async function f(browser: import('@playwright/test').Browser) {}`)).toBe(false);
	});
});
