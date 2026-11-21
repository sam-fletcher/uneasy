/**
 * The `test` every e2e spec uses, in place of `@playwright/test`'s own.
 *
 * It fails a test whose page broke without the spec noticing. Three things
 * count, and each has passed a nightly run before:
 *
 *  1. **The error boundary's fallback.** The table page wraps its phase views
 *     in a `<svelte:boundary>`. When a view throws while rendering, the player
 *     sees "something went wrong" and the boundary logs the error to the
 *     console under `[uneasy:boundary]` (`logBoundaryError`,
 *     src/lib/boundaryError.ts). Nothing else changes: the spec's own
 *     assertions may well still pass over a half-drawn table, which is how a
 *     crash passed the nightly run of 2026-11-08.
 *  2. **An exception nothing caught** (`weberror`: what Playwright's
 *     `pageerror` is for a whole context). A throw in an event handler, an
 *     `$effect` or a promise nobody awaits escapes every boundary, so there is
 *     no fallback and no boundary line, only a half-working page. Always a
 *     failure; there is no allow-list for it.
 *  3. **A `console.error` that tests/e2e/allowedConsoleErrors.ts does not
 *     name.** Chrome logs one for every 4xx/5xx a page's own fetch gets, so a
 *     handful of specs log on purpose (they fake a server failure, cut the
 *     socket); each is on that list with its spec and a reason. Anything else
 *     is new, and someone should look at it before it is allowed.
 *
 * So this watches every page's console and exceptions, and fails the test in
 * teardown. A listener sees each one the moment it happens, even when the
 * player dismisses the fallback with "Try again" or the spec closes the page
 * afterwards, where a scan of the DOM at the end of the test would miss both.
 *
 * One quiet period: once a test deletes its game (`tableDeleted()`, which
 * `deleteGame` in helpers.ts calls), `console.error` lines stop counting. The
 * pages are still open for a moment, their socket drops, and the refetch they
 * make on reconnecting gets a 403 or 404 from a table that no longer exists.
 * That is the teardown, not the app. Boundary lines and exceptions still count.
 * A delete made in the middle of a test, with its pages already closed (a helper
 * that reseeds a game), passes `pagesOpen: false` and does not start the quiet
 * period, so the rest of that test is still checked.
 *
 * Pages come from two places, and both are watched:
 *  - the `page` / `context` fixtures, and
 *  - `browser.newContext()` / `browser.newPage()`, which specs and helpers
 *    call for every extra player (24 files do). The guard listens to the
 *    browser's own `context` event for the length of one test. Playwright
 *    emits it while `newContext()` is still running (checked against 1.60:
 *    the server announces the context before it answers the call), so the
 *    listeners are on before anything can open a page in it. The
 *    alternative, wrapping `newContext`/`newPage` on the worker-wide `browser`,
 *    made every such call appear in traces and reports at this file instead of
 *    at the spec line that made it.
 * A page opened in `beforeAll`/`afterAll` is not watched: those run outside any
 * one test, and no spec uses them. If one ever does, give the guard a
 * worker-scoped listener for it, or open its pages inside a test.
 *
 * Specs and helpers import everything from here (`import { test, expect } from
 * './fixtures'`); `export *` below passes the rest of `@playwright/test`
 * through, so each file's change is the module path on its import line.
 * src/lib/e2eFixtures.test.ts, which `make check-fast` runs, fails a file that
 * imports `test` straight from `@playwright/test`, so a new spec can't skip
 * the check.
 */
import { basename } from 'node:path';
import { test as base, type BrowserContext } from '@playwright/test';
import { BOUNDARY_ERROR_PREFIX } from '../../src/lib/boundaryError';
import { isAllowedConsoleError } from './allowedConsoleErrors';
import { failureMessage, uncaughtText, type Caught } from './guardReport';

export * from '@playwright/test';

/**
 * Whether the running test has deleted its game. The worker runs one test at a
 * time and the guard clears this as each test starts, so one flag is enough.
 */
let tableGone = false;

/**
 * Call when a test deletes the game its pages are on. From here a page's
 * failed fetches are the table being gone, not the app, so `console.error`
 * stops counting for this test (see the header).
 */
export function tableDeleted(): void {
	tableGone = true;
}

export const test = base.extend<{ boundaryGuard: { watch(context: BrowserContext): void } }>({
	boundaryGuard: [
		async ({ browser }, use, testInfo) => {
			const caught: Caught[] = [];
			const origin = { spec: basename(testInfo.file), title: testInfo.title };
			tableGone = false;
			const watched = new WeakSet<BrowserContext>();
			const watch = (context: BrowserContext) => {
				if (watched.has(context)) return;
				watched.add(context);
				// A context-level listener hears every page in it, including popups.
				context.on('console', msg => {
					const text = msg.text();
					const url = msg.page()?.url() ?? '(no page)';
					if (text.includes(BOUNDARY_ERROR_PREFIX)) {
						caught.push({ kind: 'boundary', url, text });
					} else if (msg.type() === 'error' && !tableGone) {
						// Chrome's own network lines carry no URL in the text; it is in the location.
						const where = msg.location().url;
						const line = where ? `${text} @ ${where}` : text;
						if (!isAllowedConsoleError(line, origin)) caught.push({ kind: 'console.error', url, text: line });
					}
				});
				// An exception nothing caught: in a handler, an $effect, a promise.
				context.on('weberror', webError => {
					caught.push({ kind: 'uncaught', url: webError.page()?.url() ?? '(no page)', text: uncaughtText(webError.error()) });
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
			// message is the whole report: this file's own stack would be noise, and
			// so would any stack at all, since Playwright prints every frame it finds
			// in the message a second time at the end. The pages' own frames are
			// already in the message, under the line that logged them.
			if (caught.length > 0) {
				const failure = new Error(failureMessage(caught));
				failure.stack = '';
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
