/**
 * The `test` every e2e spec uses, in place of `@playwright/test`'s own.
 *
 * It fails any test whose page showed the table's error-boundary fallback.
 *
 * The table page wraps its phase views in a `<svelte:boundary>`. When a view
 * throws while rendering, the player sees "something went wrong" and the
 * boundary logs the error to the console under `[uneasy:boundary]`
 * (`logBoundaryError`, src/lib/boundaryError.ts). Nothing else changes: the
 * spec's own assertions may well still pass over a half-drawn table, which is
 * how a crash passed the nightly run of 2026-11-08.
 *
 * So this watches every page's console, and fails the test when that line
 * appears. A listener sees the line the moment it is logged, even when the
 * player dismisses the fallback with "Try again" or the spec closes the page
 * afterwards, where a scan of the DOM at the end of the test would miss both.
 *
 * Pages come from two places, and both are watched:
 *  - the `page` / `context` fixtures, and
 *  - `browser.newContext()` / `browser.newPage()`, which specs and helpers
 *    call for every extra player (24 files do). The guard listens to the
 *    browser's own `context` event for the length of one test. Playwright
 *    emits it while `newContext()` is still running (checked against 1.60:
 *    the server announces the context before it answers the call), so the
 *    console listener is on before anything can open a page in it. The
 *    alternative, wrapping `newContext`/`newPage` on the worker-wide `browser`,
 *    made every such call appear in traces and reports at this file instead of
 *    at the spec line that made it.
 * A page opened in `beforeAll`/`afterAll` is not watched: those run outside any
 * one test, and no spec uses them.
 *
 * Specs and helpers import everything from here (`import { test, expect } from
 * './fixtures'`); `export *` below passes the rest of `@playwright/test`
 * through, so each file's change is the module path on its import line.
 * src/lib/e2eFixtures.test.ts, which `make check-fast` runs, fails a file that
 * imports `test` straight from `@playwright/test`, so a new spec can't skip
 * the check.
 */
import { test as base, type BrowserContext } from '@playwright/test';
import { BOUNDARY_ERROR_PREFIX } from '../../src/lib/boundaryError';

export * from '@playwright/test';

/** One boundary line a page logged: where the page was, and what it said. */
type Caught = { url: string; text: string };

/** How many distinct lines the failure message spells out (the rest are counted). */
const SHOWN = 3;

/** The failure message: the page, and the logged error with its stack. */
function failureMessage(caught: Caught[]): string {
	const distinct = new Map<string, Caught>();
	for (const c of caught) if (!distinct.has(`${c.url}\n${c.text}`)) distinct.set(`${c.url}\n${c.text}`, c);
	const all = [...distinct.values()];
	const lines = all.slice(0, SHOWN).map(c => `  page: ${c.url}\n  ${c.text.replaceAll('\n', '\n  ')}`);
	if (all.length > SHOWN) lines.push(`  … and ${all.length - SHOWN} more`);
	return [
		`The table's error boundary caught a render crash during this test (${BOUNDARY_ERROR_PREFIX}).`,
		'The player saw the fallback instead of the table, whatever the assertions above say.',
		'',
		...lines,
	].join('\n');
}

export const test = base.extend<{ boundaryGuard: { watch(context: BrowserContext): void } }>({
	boundaryGuard: [
		async ({ browser }, use) => {
			const caught: Caught[] = [];
			const watched = new WeakSet<BrowserContext>();
			const watch = (context: BrowserContext) => {
				if (watched.has(context)) return;
				watched.add(context);
				// A context-level listener hears every page in it, including popups.
				context.on('console', msg => {
					const text = msg.text();
					if (text.includes(BOUNDARY_ERROR_PREFIX)) caught.push({ url: msg.page()?.url() ?? '(no page)', text });
				});
			};

			// The `browser` is shared by every test in the worker: listen for this
			// test's contexts only, and stop afterwards.
			browser.on('context', watch);
			try {
				await use({ watch });
			} finally {
				browser.off('context', watch);
			}

			// Throwing here, after the test body and its afterEach hooks, fails the
			// test and is reported next to whatever the body itself ran into. The
			// message is the whole report: its own stack would only repeat the
			// logged one (and point at this file), as globalSetup.ts's does.
			if (caught.length > 0) {
				const failure = new Error(failureMessage(caught));
				failure.stack = failure.message;
				throw failure;
			}
		},
		// Runs for every test without the spec asking, so no spec can forget it.
		{ auto: true },
	],

	// The `page` fixture's own context. The `context` event above would hear it too, as
	// long as the listener is on before the context is made; depending on boundaryGuard
	// makes that so, whatever order Playwright sets fixtures up in.
	// (tests/e2e/fixtures.spec.ts checks the outcome for each way of getting a page.)
	context: async ({ context, boundaryGuard }, use) => {
		boundaryGuard.watch(context);
		await use(context);
	},
});
