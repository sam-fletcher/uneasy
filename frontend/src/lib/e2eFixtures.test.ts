import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join, relative } from 'node:path';

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

	it('the fixtures module itself watches for the boundary line, from the app\'s own constant', () => {
		const src = readFileSync(FIXTURES, 'utf8');
		expect(src).toMatch(/import\s*\{\s*BOUNDARY_ERROR_PREFIX\s*\}\s*from\s*['"]\.\.\/\.\.\/src\/lib\/boundaryError['"]/);
		expect(src).toMatch(/msg\.text\(\)[\s\S]*includes\(BOUNDARY_ERROR_PREFIX\)/);
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
