import type { Page } from '@playwright/test';
import { BOUNDARY_ERROR_PREFIX } from '../../src/lib/boundaryError';
import { test, expect, tableDeleted } from './fixtures';

// Canaries for fixtures.ts: each way a spec can get a page must be one the
// guard hears, and each thing it fails on (the boundary line, an uncaught
// exception, a console.error that is not on the allow-list) must fail. They need
// no game and no server (about:blank logs to the console like any page), so
// they cost milliseconds.
//
// `test.fail()` flips the verdict: the test passes only if it failed. If the
// guard goes blind (a Playwright upgrade changes how contexts are made, a
// refactor drops a listener), the test would pass, and Playwright reports
// "expected to fail, but passed".
//
// That flip can't tell the guard's failure from any other, so each body runs
// inside `unlessGuarded`, which swallows the body's own errors. A body that
// breaks for its own reasons (no page, a changed API) then ends without
// failing, and reads "expected to fail, but passed". Only the guard can fail
// the test, so only the guard can make a canary pass.

/** What the table page's boundary logs: the prefix, where, and the Error. */
async function logBoundary(page: Page) {
	await page.evaluate(
		prefix => console.error(`${prefix} table phase view (canary)`, new Error('canary')),
		BOUNDARY_ERROR_PREFIX,
	);
}

/** Throws from a timer, so that nothing catches it: Chrome reports it as an uncaught exception. */
async function throwUncaught(page: Page) {
	await page.evaluate(
		() =>
			new Promise<void>(resolve => {
				setTimeout(() => {
					throw new Error('canary: uncaught exception');
				}, 0);
				setTimeout(resolve, 50);
			}),
	);
}

/** Runs a canary's body so that nothing it throws fails the test; says what it swallowed. */
async function unlessGuarded(body: () => Promise<void>) {
	try {
		await body();
	} catch (err) {
		console.warn('canary body failed for its own reasons (so the guard was never tested):', err);
	}
}

test('a page from the `page` fixture that logs the boundary line fails the test', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		await logBoundary(page);
	});
});

test('a page from browser.newContext() that logs the boundary line fails the test', async ({ browser }) => {
	test.fail();
	await unlessGuarded(async () => {
		const ctx = await browser.newContext();
		const page = await ctx.newPage();
		await logBoundary(page);
		await ctx.close();
	});
});

test('a page from browser.newPage() that logs the boundary line fails the test', async ({ browser }) => {
	test.fail();
	await unlessGuarded(async () => {
		const page = await browser.newPage();
		await logBoundary(page);
	});
});

test('a second page in the same context (a popup, a new tab) that logs it fails the test', async ({ page, context }) => {
	test.fail();
	await unlessGuarded(async () => {
		const second = await context.newPage();
		await logBoundary(second);
		await page.goto('about:blank');
	});
});

test('a page closed before the test ends still fails it (a DOM scan at the end would miss it)', async ({ browser }) => {
	test.fail();
	await unlessGuarded(async () => {
		const ctx = await browser.newContext();
		const page = await ctx.newPage();
		await logBoundary(page);
		await page.close();
		await ctx.close();
	});
});

test('an uncaught exception in a page from the `page` fixture fails the test', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		await throwUncaught(page);
	});
});

test('an uncaught exception in a page from browser.newContext() fails the test', async ({ browser }) => {
	test.fail();
	await unlessGuarded(async () => {
		const ctx = await browser.newContext();
		const page = await ctx.newPage();
		await throwUncaught(page);
		await ctx.close();
	});
});

test('a promise rejection nobody handles fails the test', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		await page.evaluate(
			() =>
				new Promise<void>(resolve => {
					Promise.reject(new Error('canary: unhandled rejection'));
					setTimeout(resolve, 50);
				}),
		);
	});
});

test('a console.error that is not on the allow-list fails the test', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		await page.evaluate(() => console.error('canary: nothing allows this line', new Error('x')));
	});
});

test('a console.error from a page opened with browser.newContext() fails the test too', async ({ browser }) => {
	test.fail();
	await unlessGuarded(async () => {
		const ctx = await browser.newContext();
		const page = await ctx.newPage();
		await page.evaluate(() => console.error('canary: nothing allows this line either'));
		await ctx.close();
	});
});

test('a console.error the allow-list names, for this spec, does not fail the test', async ({ browser }) => {
	const ctx = await browser.newContext();
	const page = await ctx.newPage();
	await page.evaluate(() => console.error('canary: an allow-listed console.error is let through'));
	expect(await page.evaluate(() => 1 + 1)).toBe(2);
	await ctx.close();
});

test('once the test has deleted its game, console errors stop counting (the pages are talking to nothing)', async ({ page }) => {
	tableDeleted();
	await page.evaluate(() => console.error('canary: a 403 from a table that is gone'));
	expect(await page.evaluate(() => 1 + 1)).toBe(2);
});

test('...but a boundary line still fails the test after the game is deleted', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		tableDeleted();
		await logBoundary(page);
	});
});

test('...and so does an uncaught exception', async ({ page }) => {
	test.fail();
	await unlessGuarded(async () => {
		tableDeleted();
		await throwUncaught(page);
	});
});
