import { test, expect, type BrowserContext, type Page } from './fixtures';
import { createServer } from 'node:net';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// The table says so when its live connection drops, and takes the notice away
// when the board is current again (docs: ws.ts "Telling the player the table
// has gone quiet"; the strip is table/ConnectionNotice.svelte).
//
// HOW THE SOCKET IS CUT. Playwright's `context.setOffline(true)` does not do it:
// against this app, Chromium fired no `close` on the open WebSocket for 8s after
// going offline (ws.ts's retry loop runs only from `close`), so a spec built on
// it would not be exercising the reconnect code at all. Instead an init script
// wraps the page's `WebSocket`:
//
//   - it keeps every socket the page opens, so the spec can really close them
//     (a genuine close event reaches ws.ts, as a server dropping us would); and
//   - while "blocked", it points any NEW socket at a port nothing listens on,
//     so the reconnect attempts fail the way they do against an unreachable
//     server (refused, never opened), through the browser's own WebSocket
//     machinery. The dead URL is `wss://`: the page's CSP allows only
//     same-origin and `wss:` for connections, and a `ws://` one is blocked
//     before it is tried, which Chromium reports as `error` with no `close`
//     and so is not a failure ws.ts's retry loop ever sees.
//
// ws.ts itself, the page, and the real server are all untouched; only where the
// socket points changes. The notice has a 5s threshold, so these specs take a
// few real seconds each.

const track = cleanupGameAfterEach();
const opened: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of opened.splice(0)) await c.close().catch(() => {});
});

// What the init script puts on `window`, and the spec drives.
interface SocketControl {
	sockets: WebSocket[];
	blocked: boolean;
}
declare global {
	interface Window {
		__sockets: SocketControl;
	}
}

/** A port nothing listens on: bind to 0, read it, let go. */
async function deadPort(): Promise<number> {
	const server = createServer();
	await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
	const port = (server.address() as { port: number }).port;
	await new Promise<void>((resolve) => server.close(() => resolve()));
	return port;
}

/** Call before the page's first navigation. */
async function watchSockets(page: Page) {
	const port = await deadPort();
	await page.addInitScript((dead: number) => {
		const Real = window.WebSocket;
		const control: SocketControl = { sockets: [], blocked: false };
		window.__sockets = control;
		window.WebSocket = new Proxy(Real, {
			construct(target, args: [string | URL, (string | string[])?]) {
				const [url, protocols] = args;
				const to = control.blocked ? `wss://127.0.0.1:${dead}/` : url;
				const socket = new target(to, protocols);
				control.sockets.push(socket);
				return socket;
			},
		});
	}, port);
}

const openSockets = (page: Page) =>
	page.evaluate(() => window.__sockets.sockets.filter((s) => s.readyState === WebSocket.OPEN).length);

/** Cut the connection: close every socket and refuse the reconnects. */
const cut = (page: Page) =>
	page.evaluate(() => {
		window.__sockets.blocked = true;
		for (const s of window.__sockets.sockets) s.close();
	});

/** Let reconnects through again. The next retry (or "Try now") succeeds. */
const restore = (page: Page) =>
	page.evaluate(() => {
		window.__sockets.blocked = false;
	});

/** The notice. Filtered by its words: other status regions exist. */
const notice = (page: Page) => page.getByRole('status').filter({ hasText: 'Connection lost' });

/** A seeded main-event table, with bob (the viewer, cut off) and alice (who
 *  changes the board while he is away). Returns bob's page, already on it. */
async function setUp(browser: import('@playwright/test').Browser, width = 1280) {
	const alice = await browser.newContext({ baseURL: E2E });
	const bob = await browser.newContext({ baseURL: E2E, viewport: { width, height: 900 } });
	opened.push(alice, bob);
	await alice.request.post('/api/dev/login?username=alice');
	await bob.request.post('/api/dev/login?username=bob');
	const seed = await (
		await alice.request.post('/api/dev/seed', {
			data: { phase: 'main_event', players: ['alice', 'bob'] },
		})
	).json();
	track(seed.game_id);

	const page = await bob.newPage();
	await watchSockets(page);
	await page.goto(`/table/${seed.game_id}`);
	await expect(tableTitle(page)).toHaveText('Row 1');
	await expect.poll(() => openSockets(page)).toBe(1);
	return { page, alice, gameID: seed.game_id as number };
}

test('a drop that lasts shows the notice; coming back clears it with the board current', async ({ browser }) => {
	const { page, alice, gameID } = await setUp(browser);
	await expect(notice(page)).toHaveCount(0);

	await cut(page);

	// Not at once: a blip must not flash it. Two seconds in the retry loop has
	// tried (and failed) once, and the table is still quiet.
	await page.waitForTimeout(2000);
	await expect(notice(page)).toHaveCount(0);

	// Past the 5s threshold it is there, in words, with the way to act.
	await expect(notice(page)).toBeVisible({ timeout: 8000 });
	await expect(notice(page)).toContainText("This table isn't updating");
	await expect(page.getByRole('button', { name: 'Try now' })).toBeVisible();

	// The board really is stale: the game moves on while bob is cut off, and
	// nothing reaches him.
	const res = await alice.request.post('/api/dev/advance-row', {
		data: { game_id: gameID, advance_to: 2 },
	});
	expect(res.ok()).toBeTruthy();
	await page.waitForTimeout(500);
	await expect(tableTitle(page)).toHaveText('Row 1');
	await expect(notice(page)).toBeVisible();

	// The connection returns. No click: the retry loop finds it on its own.
	await restore(page);
	await expect(notice(page)).toHaveCount(0, { timeout: 20_000 });

	// The notice going away means the board is current, not merely that the
	// socket opened: the resync ran and bob has Row 2.
	await expect(tableTitle(page)).toHaveText('Row 2');
	await expect.poll(() => openSockets(page)).toBe(1);
});

test('a blip that heals inside the threshold never shows the notice', async ({ browser }) => {
	const { page } = await setUp(browser);

	await cut(page);
	await restore(page); // the first retry (1s) finds the server
	await expect.poll(() => openSockets(page), { timeout: 5000 }).toBe(1);

	// Wait out the whole threshold and then some. If reconnecting did not cancel
	// the drop's timer, the notice would turn up at 5s for a connection that is
	// fine.
	await page.waitForTimeout(6500);
	await expect(notice(page)).toHaveCount(0);
	await expect(tableTitle(page)).toHaveText('Row 1');
});

test('"Try now" retries at once and clears the notice when the connection is back', async ({ browser }) => {
	const { page, alice, gameID } = await setUp(browser);

	await cut(page);
	await expect(notice(page)).toBeVisible({ timeout: 8000 });
	const tryNow = page.getByRole('button', { name: 'Try now' });

	// Still unreachable: the attempt fails, the strip stays, and the button is
	// usable again. (A dead port fails in milliseconds, too fast to catch the
	// "Trying…" label here; connectionNotice.render.test.ts pins that state.)
	await tryNow.click();
	await expect(notice(page)).toBeVisible();
	await expect(page.getByRole('button', { name: 'Try now' })).toBeVisible();

	// Reachable again, and the next retry could be many seconds off (the backoff
	// has grown): the button does not wait for it.
	await alice.request.post('/api/dev/advance-row', { data: { game_id: gameID, advance_to: 2 } });
	await restore(page);
	await page.getByRole('button', { name: 'Try now' }).click();
	await expect(notice(page)).toHaveCount(0, { timeout: 4000 });
	await expect(tableTitle(page)).toHaveText('Row 2');
});

test('a reconnect whose fetch fails keeps the notice up, and clears it when a fetch lands', async ({ browser }) => {
	// The case a tab waking from sleep or a redeploy hits: the socket reconnects
	// fine, but the first fetch over a dead pooled connection fails. The page's
	// own load swallows that failure (a background refresh must not blank the
	// board), so the socket alone would say "live" over a board that never
	// refreshed, and with the socket staying open nothing would fetch again.
	const { page, alice, gameID } = await setUp(browser);

	let stateFails = true;
	let stateAttempts = 0;
	await page.route(new RegExp(`/api/tables/${gameID}/state(\\?|$)`), (route) => {
		stateAttempts += 1;
		return stateFails ? route.abort('failed') : route.continue();
	});

	await cut(page);
	await expect(notice(page)).toBeVisible({ timeout: 8000 });
	const res = await alice.request.post('/api/dev/advance-row', {
		data: { game_id: gameID, advance_to: 2 },
	});
	expect(res.ok()).toBeTruthy();

	// The server is reachable again: the socket will open, but its fetch fails.
	await restore(page);
	await page.getByRole('button', { name: 'Try now' }).click();
	await expect.poll(() => stateAttempts, { timeout: 10_000 }).toBeGreaterThan(0);
	await page.waitForTimeout(1500);
	await expect(notice(page), 'a failed resync is not a live table').toBeVisible();
	await expect(tableTitle(page)).toHaveText('Row 1');

	// A fetch gets through: the notice goes, and with it the stale board.
	stateFails = false;
	await expect(notice(page)).toHaveCount(0, { timeout: 20_000 });
	await expect(tableTitle(page)).toHaveText('Row 2');
});

test('at 375px the notice and its 44px button fit the frame without scrolling sideways', async ({ browser }) => {
	const { page } = await setUp(browser, 375);

	await cut(page);
	await expect(notice(page)).toBeVisible({ timeout: 8000 });

	const button = page.getByRole('button', { name: 'Try now' });
	const box = await button.boundingBox();
	expect(box, 'the button is on screen').not.toBeNull();
	expect(box!.height, 'a 44px tap target').toBeGreaterThanOrEqual(44);
	expect(box!.x + box!.width, 'inside the viewport').toBeLessThanOrEqual(375);

	const overflow = await page.evaluate(
		() => document.documentElement.scrollWidth - document.documentElement.clientWidth,
	);
	expect(overflow, 'no horizontal scroll').toBeLessThanOrEqual(0);

	// The notice sits above the panes, below the player row: it does not cover
	// the table, it moves it down.
	const noticeBox = await notice(page).boundingBox();
	const panes = await page.locator('.panes').boundingBox();
	expect(noticeBox!.y + noticeBox!.height).toBeLessThanOrEqual(panes!.y);
});
