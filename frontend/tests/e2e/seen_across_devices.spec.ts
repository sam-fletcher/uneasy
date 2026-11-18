import { test, expect, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach, openPane, openRecordTab, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// One player, two devices. Reading Laws, Rumors or chat on one of them moves a
// marker the server keeps for that player, and the pale "new" dot should go on
// the other one too. The marker is private to the player, so nothing is pushed:
// the other device asks for it when the player comes back to it (ws.ts's
// onForeground, wired in the table page).
//
// The "laptop" is a 1280px window where the Public Record and Chat are columns
// that are simply on screen; the "phone" is 375px, where each is a pane behind a
// tab. Both are logged in as alice, in separate browser contexts (separate
// cookie jars, so two sessions for the same seat, which is what two devices are).
//
// COMING BACK TO THE PHONE. Playwright's pages are all "visible", and nothing
// here can put a phone in a pocket. So `comeBack` does what the browser does when
// a tab is brought forward: the page says it is hidden, then visible again, and
// `visibilitychange` fires each time. That drives the real handler in ws.ts; it
// does not prove the browser raises the event (it does, on every browser the
// app supports). `focusWindow` is the same for a window that was already visible
// and gains focus.

const PHONE = { width: 375, height: 812 };
const LAPTOP = { width: 1280, height: 800 };

const track = cleanupGameAfterEach();
const opened: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of opened.splice(0)) await c.close().catch(() => {});
});

async function device(
	browser: Browser,
	who: string,
	viewport: { width: number; height: number },
): Promise<{ ctx: BrowserContext; page: Page }> {
	const ctx = await browser.newContext({ baseURL: E2E, viewport });
	opened.push(ctx);
	const login = await ctx.request.post(`/api/dev/login?username=${who}`);
	expect(login.ok(), `dev-login for ${who}`).toBeTruthy();
	return { ctx, page: await ctx.newPage() };
}

async function open(page: Page, gameID: number) {
	await page.goto(`/table/${gameID}`);
	await expect(tableTitle(page)).toHaveText(/^Row \d+/);
}

/** Opens the table on the phone and waits out the page's own one-shot look at
 *  the Laws and Rumors markers. A table with something new in either list asks
 *  the server for its markers once, half a second after loading, to catch up an
 *  author's own entry (the table page's `markerRereads`). Against a local server
 *  the other device can read the list inside that half second, and then that
 *  look, not the return under test, would clear the dot. So let it pass first. */
async function openPhone(page: Page, gameID: number) {
	const lists = ['laws', 'rumors'].map((list) =>
		page.waitForResponse((r) => r.url().endsWith(`/api/tables/${gameID}/${list}`)),
	);
	await open(page, gameID);
	await Promise.all(lists);
}

/** The page is hidden, then visible again, as when a phone is unlocked. */
async function comeBack(page: Page) {
	await page.evaluate(() => {
		const set = (state: string) =>
			Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state });
		set('hidden');
		document.dispatchEvent(new Event('visibilitychange'));
		set('visible');
		document.dispatchEvent(new Event('visibilitychange'));
	});
}

/** A window that was already visible gains focus: no visibility change at all. */
async function focusWindow(page: Page) {
	await page.evaluate(() => window.dispatchEvent(new Event('focus')));
}

/** Longer than a request takes, shorter than a player's patience. Used before
 *  asserting that a dot is STILL there: nothing may have pushed the news. */
const NOTHING_PUSHED_MS = 1500;
/** ws.ts folds returns closer together than this into one (FOREGROUND_MIN_GAP_MS). */
const BETWEEN_RETURNS_MS = 2100;

const chips = (page: Page) => page.getByRole('navigation', { name: 'Public Record' });
const lawsChip = (page: Page) => chips(page).getByRole('button', { name: /Laws/ });
const rumorsChip = (page: Page) => chips(page).getByRole('button', { name: /Rumors/ });
const recordTab = (page: Page) =>
	page.getByRole('navigation', { name: 'Panes' }).getByRole('button', { name: /Public Record/ });
const chatTab = (page: Page) =>
	page.getByRole('navigation', { name: 'Panes' }).getByRole('button', { name: 'Chat' });

/** A seeded main-event table: alice and bob, with one law and one rumor that
 *  neither has seen. */
async function seed(browser: Browser) {
	const first = await device(browser, 'alice', PHONE);
	const res = await first.ctx.request.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob'] },
	});
	expect(res.ok()).toBeTruthy();
	const { game_id: gameID } = (await res.json()) as { game_id: number };
	track(gameID);
	return { phone: first, gameID };
}

test('Laws and Rumors: reading them on the laptop clears their dots on the phone when you pick it up', async ({ browser }) => {
	const { phone, gameID } = await seed(browser);
	await openPhone(phone.page, gameID);

	// The phone is on its Public Record, looking at Events, with a dot on each of
	// the two lists it has not opened and on the tab that carries them.
	await openPane(phone.page, 'Public Record');
	await expect(lawsChip(phone.page).getByRole('img', { name: 'new laws' })).toBeVisible();
	await expect(rumorsChip(phone.page).getByRole('img', { name: 'new rumors' })).toBeVisible();

	// The same player, on the laptop, reads Laws.
	const laptop = await device(browser, 'alice', LAPTOP);
	await open(laptop.page, gameID);
	await openRecordTab(laptop.page, 'Laws');
	await expect(laptop.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await expect(lawsChip(laptop.page).getByRole('img')).toHaveCount(0);
	// The server has it. (Asked through the list endpoint, which is not what the
	// phone will use.)
	await expect
		.poll(async () => ((await (await laptop.ctx.request.get(`/api/tables/${gameID}/laws`)).json()) as { seen_rev: number }).seen_rev)
		.toBeGreaterThan(0);

	// Nothing tells the phone: it is not told, it has to look.
	await phone.page.waitForTimeout(NOTHING_PUSHED_MS);
	await expect(lawsChip(phone.page).getByRole('img', { name: 'new laws' })).toBeVisible();

	// She picks the phone up. Laws loses its dot; Rumors, which she has not read,
	// keeps its own, and so does the tab (it carries the dot for either list).
	await comeBack(phone.page);
	await expect(lawsChip(phone.page).getByRole('img')).toHaveCount(0, { timeout: 3000 });
	await expect(rumorsChip(phone.page).getByRole('img', { name: 'new rumors' })).toBeVisible();
	await expect(recordTab(phone.page).getByRole('img', { name: 'new laws or rumors' })).toBeVisible();

	// Rumors on the laptop, and the phone again: now the tab's dot goes as well.
	await openRecordTab(laptop.page, 'Rumors');
	await expect(laptop.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await expect
		.poll(async () => ((await (await laptop.ctx.request.get(`/api/tables/${gameID}/rumors`)).json()) as { seen_rev: number }).seen_rev)
		.toBeGreaterThan(0);
	await phone.page.waitForTimeout(BETWEEN_RETURNS_MS);
	await comeBack(phone.page);
	await expect(rumorsChip(phone.page).getByRole('img')).toHaveCount(0, { timeout: 3000 });
	await expect(recordTab(phone.page).getByRole('img')).toHaveCount(0);
});

test('Laws: a second window that was already visible catches up when it gains focus', async ({ browser }) => {
	// Two windows side by side never change visibility; the dot should still go
	// when the player clicks into the other one.
	const { phone, gameID } = await seed(browser);
	const wide = await device(browser, 'alice', LAPTOP);
	await open(wide.page, gameID);
	await openPhone(phone.page, gameID);
	await openPane(phone.page, 'Public Record');
	await expect(lawsChip(phone.page).getByRole('img', { name: 'new laws' })).toBeVisible();

	await openRecordTab(wide.page, 'Laws');
	await expect(wide.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await expect
		.poll(async () => ((await (await wide.ctx.request.get(`/api/tables/${gameID}/laws`)).json()) as { seen_rev: number }).seen_rev)
		.toBeGreaterThan(0);

	await focusWindow(phone.page);
	await expect(lawsChip(phone.page).getByRole('img')).toHaveCount(0, { timeout: 3000 });
});

test("someone else's reading never clears my dot", async ({ browser }) => {
	// The markers are per player. Bob opens Laws on his phone; alice's phone, which
	// comes back to the foreground and asks the server, keeps her dot.
	const { phone, gameID } = await seed(browser);
	await openPhone(phone.page, gameID);
	await openPane(phone.page, 'Public Record');
	await expect(lawsChip(phone.page).getByRole('img', { name: 'new laws' })).toBeVisible();

	const bob = await device(browser, 'bob', PHONE);
	await open(bob.page, gameID);
	await openRecordTab(bob.page, 'Laws');
	await expect(bob.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await expect(lawsChip(bob.page).getByRole('img')).toHaveCount(0);
	await expect
		.poll(async () => ((await (await bob.ctx.request.get(`/api/tables/${gameID}/laws`)).json()) as { seen_rev: number }).seen_rev)
		.toBeGreaterThan(0);

	const asked = phone.page.waitForResponse((r) => r.url().endsWith(`/api/tables/${gameID}/seen`) && r.ok());
	await comeBack(phone.page);
	const markers = (await (await asked).json()) as Record<string, number>;
	expect(markers.laws, "bob's marker must not reach alice").toBe(0);
	await expect(lawsChip(phone.page).getByRole('img', { name: 'new laws' })).toBeVisible();
});

test('Chat: reading on the laptop clears the dot on the Chat tab on the phone when you pick it up', async ({ browser }) => {
	const { phone, gameID } = await seed(browser);
	const bob = await device(browser, 'bob', PHONE);

	// The phone is open on Play. Bob writes; the Chat tab gets its pale dot.
	await open(phone.page, gameID);
	const said = await bob.ctx.request.post(`/api/tables/${gameID}/posts`, { data: { body: 'are you there?' } });
	expect(said.ok()).toBeTruthy();
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();

	// The laptop shows Chat as a column, at the bottom, so reading is automatic:
	// its dot goes and the marker is reported to the server.
	const laptop = await device(browser, 'alice', LAPTOP);
	await open(laptop.page, gameID);
	const column = laptop.page.getByRole('complementary', { name: 'Chat' });
	await expect(column.getByText('are you there?')).toBeVisible();
	await expect(column.locator('.pane-name').getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });
	await expect
		.poll(async () => ((await (await laptop.ctx.request.get(`/api/tables/${gameID}/posts`)).json()) as { last_read_post_id: number }).last_read_post_id)
		.toBeGreaterThan(0);

	// Nothing tells the phone.
	await phone.page.waitForTimeout(NOTHING_PUSHED_MS);
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();

	// She picks it up.
	await comeBack(phone.page);
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 3000 });

	// The dot was not merely hidden: opening Chat on the phone shows the message
	// with no "N new" divider above it, because she has read it.
	await chatTab(phone.page).click();
	const chat = phone.page.locator('[aria-label="Chat"]');
	await expect(chat.getByText('are you there?')).toBeVisible();
	await expect(chat.getByRole('separator', { name: /new message/ })).toHaveCount(0);
});

test('a message that arrives after the laptop read still lights the phone: only what was read goes', async ({ browser }) => {
	const { phone, gameID } = await seed(browser);
	const bob = await device(browser, 'bob', PHONE);
	await open(phone.page, gameID);
	await bob.ctx.request.post(`/api/tables/${gameID}/posts`, { data: { body: 'first' } });
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();

	const laptop = await device(browser, 'alice', LAPTOP);
	await open(laptop.page, gameID);
	await expect(laptop.page.getByRole('complementary', { name: 'Chat' }).getByText('first')).toBeVisible();
	await expect
		.poll(async () => ((await (await laptop.ctx.request.get(`/api/tables/${gameID}/posts`)).json()) as { last_read_post_id: number }).last_read_post_id)
		.toBeGreaterThan(0);

	// Bob writes again, and only the phone is open to hear it (the laptop is
	// closed). The phone then comes back: the marker it learns covers "first", not
	// "second", so the dot must stay.
	await laptop.page.close();
	await bob.ctx.request.post(`/api/tables/${gameID}/posts`, { data: { body: 'second' } });
	await phone.page.waitForTimeout(NOTHING_PUSHED_MS);
	await comeBack(phone.page);
	await phone.page.waitForTimeout(NOTHING_PUSHED_MS);
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();
});

test('a return with no dot lit makes no request; one with a dot lit does', async ({ browser }) => {
	// A read can only clear dots, so with none lit it is skipped: most returns to
	// a phone are free.
	const { phone, gameID } = await seed(browser);
	await openPhone(phone.page, gameID);

	// Read everything on the phone itself, until no dot is left.
	await openRecordTab(phone.page, 'Laws');
	await expect(phone.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await openRecordTab(phone.page, 'Rumors');
	await expect(phone.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await chatTab(phone.page).click();
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });
	await expect(recordTab(phone.page).getByRole('img')).toHaveCount(0);
	await openPane(phone.page, 'Play');

	let asked = 0;
	phone.page.on('request', (r) => {
		if (r.method() === 'GET' && r.url().endsWith(`/api/tables/${gameID}/seen`)) asked++;
	});
	await comeBack(phone.page);
	await phone.page.waitForTimeout(NOTHING_PUSHED_MS);
	expect(asked, 'nothing is lit, so there is nothing a read could clear').toBe(0);

	// The control: with a dot lit the same return does ask, so the zero above is
	// the skip and not a trigger that never fires.
	const bob = await device(browser, 'bob', PHONE);
	await bob.ctx.request.post(`/api/tables/${gameID}/posts`, { data: { body: 'now there is a dot' } });
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();
	await phone.page.waitForTimeout(BETWEEN_RETURNS_MS);
	await comeBack(phone.page);
	await expect.poll(() => asked).toBe(1);
});

test("a divider this device's own reading left behind survives a return that only moved Laws", async ({ browser }) => {
	// The phone reads a message itself (so its chat marker is current and its
	// "1 new message" divider stays where the window loaded). The laptop then reads
	// Laws, and the phone returns: only Laws moved elsewhere, so chat's divider must
	// not move. The server's chat marker is no further than the phone's own.
	const { phone, gameID } = await seed(browser);
	const bob = await device(browser, 'bob', PHONE);
	await openPhone(phone.page, gameID);
	await bob.ctx.request.post(`/api/tables/${gameID}/posts`, { data: { body: 'read on the phone' } });
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toBeVisible();

	await chatTab(phone.page).click();
	const chat = phone.page.locator('[aria-label="Chat"]');
	await expect(chat.getByRole('separator', { name: /new message/ })).toBeVisible();
	await expect(chatTab(phone.page).getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });
	await openPane(phone.page, 'Play');

	const laptop = await device(browser, 'alice', LAPTOP);
	await open(laptop.page, gameID);
	await openRecordTab(laptop.page, 'Laws');
	await expect(laptop.page.locator('.laws-rumors .lr-item').first()).toBeVisible();
	await expect
		.poll(async () => ((await (await laptop.ctx.request.get(`/api/tables/${gameID}/laws`)).json()) as { seen_rev: number }).seen_rev)
		.toBeGreaterThan(0);

	await phone.page.waitForTimeout(BETWEEN_RETURNS_MS);
	const asked = phone.page.waitForResponse((r) => r.url().endsWith(`/api/tables/${gameID}/seen`) && r.ok());
	await comeBack(phone.page);
	await asked;
	await openPane(phone.page, 'Public Record');
	await expect(lawsChip(phone.page).getByRole('img')).toHaveCount(0, { timeout: 3000 });

	await chatTab(phone.page).click();
	await expect(chat.getByText('read on the phone')).toBeVisible();
	await expect(chat.getByRole('separator', { name: /new message/ })).toBeVisible();
});
