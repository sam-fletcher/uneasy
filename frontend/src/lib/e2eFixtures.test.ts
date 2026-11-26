import { describe, expect, it } from 'vitest';
import { existsSync, readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';
import {
	ALLOWED_CONSOLE_ERRORS,
	isAllowedConsoleError,
	type AllowedConsoleError,
} from '../../tests/e2e/allowedConsoleErrors';
import { failureMessage, uncaughtText } from '../../tests/e2e/guardReport';

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

	it('names, for every entry, a spec that exists (and always a spec: with none, a line is allowed in every spec)', () => {
		const bad = ALLOWED_CONSOLE_ERRORS.filter(a => !a.spec || !existsSync(join(E2E, a.spec)));
		expect(bad.map(a => `${a.spec}: ${a.match}`), 'a renamed or deleted spec leaves its entry allowing nothing').toEqual([]);
	});

	it('names, where an entry is scoped to one test, a test the spec still has', () => {
		const gone = ALLOWED_CONSOLE_ERRORS.filter(
			a => a.test !== undefined && !readFileSync(join(E2E, a.spec), 'utf8').includes(a.test),
		);
		expect(gone.map(a => `${a.spec}: "${a.test}"`), 'a renamed test leaves its entry allowing nothing').toEqual([]);
	});

	it('points every bug entry at a tracker item that still exists', () => {
		const gone = ALLOWED_CONSOLE_ERRORS.filter(a => a.item !== undefined && !existsSync(join(TRACKER_NEXT, `${a.item}.md`)));
		expect(
			gone.map(a => `${a.item}: ${a.match}`),
			'the item is finished or renamed: delete the entry (the bug is fixed) or fix its `item`',
		).toEqual([]);
	});

	it('keeps every pattern narrow: none matches a generic line, however it is scoped', () => {
		// What a catch-all would catch. Each entry names one URL or one message, so none of
		// these (a bare failed fetch, a script error, a 4xx or 5xx on some other route) matches.
		const URL_LINE = (status: string, path: string) =>
			`Failed to load resource: the server responded with a status of ${status} @ http://localhost:1${path}`;
		const GENERIC = [
			'',
			'some unrelated console error',
			'Failed to fetch',
			'TypeError: Cannot read properties of undefined (reading \'id\')',
			'Failed to load resource: net::ERR_FAILED',
			URL_LINE('500 (Internal Server Error)', '/api/anything'),
			URL_LINE('403 (Forbidden)', '/api/tables/1/laws'),
			URL_LINE('404 (Not Found)', '/api/plans/1'),
			URL_LINE('400 (Bad Request)', '/api/plans/1'),
		];
		const wide = ALLOWED_CONSOLE_ERRORS.filter(a => GENERIC.some(line => a.match.test(line)));
		expect(wide.map(a => String(a.match))).toEqual([]);
	});

	it("allows no test to see the Make War war-state 404: the panel no longer asks before its war exists", () => {
		const LINE = 'Failed to load resource: the server responded with a status of 404 (Not Found) @ http://localhost:8090/api/plans/222/war-state';
		// public_record.spec.ts had an entry for this until MakeWarPanel stopped fetching early.
		const test = { spec: 'public_record.spec.ts', title: 'Events: a war the server stored as "make" is not shown as a Make' };
		expect(isAllowedConsoleError(LINE, test)).toBe(false);
		expect(isAllowedConsoleError(LINE, { spec: 'make_war.spec.ts', title: 'any Make War test' })).toBe(false);
	});

	it("pins the cross-spec canary's line: allowed for chat's failed-send test, and nowhere else", () => {
		// tests/e2e/fixtures.spec.ts logs this line and requires it to fail there. Without this,
		// that canary would pass for the wrong reason if the entry's pattern stopped matching it.
		const LINE = 'Failed to load resource: the server responded with a status of 500 (Internal Server Error) @ http://canary.test/api/tables/999/posts';
		const chatTest = { spec: 'chat.spec.ts', title: 'a failed send restores the text, but never clobbers a draft typed while in flight' };
		expect(isAllowedConsoleError(LINE, chatTest)).toBe(true);
		expect(isAllowedConsoleError(LINE, { ...chatTest, title: 'any other chat test' })).toBe(false);
		expect(isAllowedConsoleError(LINE, { spec: 'fixtures.spec.ts', title: chatTest.title })).toBe(false);
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
	const chat = { spec: 'chat.spec.ts', title: 'a failed send restores the text' };

	it('lets a line through for the spec that is named, and only that spec', () => {
		expect(isAllowedConsoleError(LINE, chat, [entry()])).toBe(true);
		expect(isAllowedConsoleError(LINE, { ...chat, spec: 'make_war.spec.ts' }, [entry()])).toBe(false);
	});

	it('lets an entry through only for the test it names, when it names one', () => {
		const list = [entry({ test: 'a failed send' })];
		expect(isAllowedConsoleError(LINE, chat, list)).toBe(true);
		expect(isAllowedConsoleError(LINE, { ...chat, title: 'scroll-up pagination' }, list)).toBe(false);
	});

	it('is as narrow as its pattern: another status, another route and a trailing path all fail', () => {
		const list = [entry()];
		expect(isAllowedConsoleError(LINE.replace('500 (Internal Server Error)', '403 (Forbidden)'), chat, list)).toBe(false);
		expect(isAllowedConsoleError(LINE.replace('/posts', '/wars'), chat, list)).toBe(false);
		expect(isAllowedConsoleError(`${LINE}/extra`, chat, list)).toBe(false);
		expect(isAllowedConsoleError('some other error', chat, list)).toBe(false);
	});

	it('is closed by default: an empty list allows nothing', () => {
		expect(isAllowedConsoleError(LINE, chat, [])).toBe(false);
	});
});

describe('what the guard says when it fails a test', () => {
	it('shows a thrown value that is not an Error: Playwright gives it an empty stack and the value as the message', () => {
		// What `throw 'x'` arrives as (checked against Playwright 1.60): name "", stack "", message "x".
		expect(uncaughtText({ message: 'a string', stack: '' })).toContain('a string');
		expect(uncaughtText({ message: 'a string', stack: '' }).trim()).not.toBe('');
		// A real Error keeps its stack, which already starts with its message.
		expect(uncaughtText({ message: 'real', stack: 'Error: real\n    at f (x.js:1:1)' })).toBe('Error: real\n    at f (x.js:1:1)');
	});

	it('lists each different throw on a page once, instead of folding them into one', () => {
		const at = (text: string) => ({ kind: 'uncaught' as const, url: 'http://localhost/table/1', text });
		const msg = failureMessage([
			at(uncaughtText({ message: 'first', stack: '' })),
			at(uncaughtText({ message: 'second', stack: '' })),
			at(uncaughtText({ message: 'first', stack: '' })),
		]);
		expect(msg).toContain('first');
		expect(msg).toContain('second');
		expect(msg.match(/page: /g)).toHaveLength(2);
	});

	it('has one section for each kind that happened, and none for the others', () => {
		const page = 'http://localhost/table/1';
		const one = failureMessage([{ kind: 'console.error', url: page, text: 'oops' }]);
		expect(one).toContain('allowedConsoleErrors.ts');
		expect(one).not.toContain('nothing caught');
		expect(one).not.toContain('[uneasy:boundary]');
		const all = failureMessage([
			{ kind: 'boundary', url: page, text: '[uneasy:boundary] x' },
			{ kind: 'uncaught', url: page, text: 'Error: y' },
			{ kind: 'console.error', url: page, text: 'z' },
		]);
		expect(all.match(/page: /g)).toHaveLength(3);
	});
});
