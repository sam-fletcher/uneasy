import { test, expect, type Page, type CDPSession } from './fixtures';
import { cleanupGameAfterEach, openTones, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// Swipe a bottom sheet down to dismiss it ($lib/sheetSwipe, drawn by
// RetinueSheet). Driven with real touch events — Chrome's own gesture pipeline
// through CDP `Input.dispatchTouchEvent` — at 375px, because the whole point is
// how a finger and a scrolling body share a gesture. What this can't cover is
// real iOS Safari and Android Chrome; the owner tries those by hand.
//
//   1. A short sheet (Help) closes when swiped down.
//   2. A long sheet (Tones), scrolled partway, scrolls back up on a downward
//      swipe instead of closing, and closes only once it is back at the top.
//   3. A short, slow drag springs back.
//   4. A drag that starts in a text field (the feedback form's) doesn't move
//      the sheet.
//   +  While a sheet is open the page's own overscroll is shut (the
//      pull-to-refresh guard), and opens again afterwards.

const PHONE = { width: 375, height: 700 };

const track = cleanupGameAfterEach();

test.use({ viewport: PHONE, hasTouch: true, isMobile: true });

type Pt = { x: number; y: number };

/** One finger down at `from`, moving to `to` in `steps` moves `stepMs` apart,
 *  then up (unless `hold`). */
async function drag(
  cdp: CDPSession,
  from: Pt,
  to: Pt,
  { steps = 10, stepMs = 16, hold = false } = {},
) {
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [from] });
  for (let i = 1; i <= steps; i++) {
    await new Promise((r) => setTimeout(r, stepMs));
    await cdp.send('Input.dispatchTouchEvent', {
      type: 'touchMove',
      touchPoints: [{ x: from.x + ((to.x - from.x) * i) / steps, y: from.y + ((to.y - from.y) * i) / steps }],
    });
  }
  if (!hold) await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
}

const sheetTop = (page: Page) =>
  page.locator('.sheet').evaluate((el) => el.getBoundingClientRect().top);

async function openTable(page: Page, ctxRequest: Page['request']) {
  const login = await ctxRequest.post('/api/dev/login?username=alice');
  expect(login.ok()).toBeTruthy();
  const seed = await ctxRequest.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seed.ok()).toBeTruthy();
  const { game_id } = await seed.json();
  track(game_id);
  await page.goto(`/table/${game_id}`);
  await expect(tableTitle(page)).toHaveText(/^Row \d+/);
}

test('a sheet closes when swiped down, and the touch rules hold', async ({ browser }) => {
  const ctx = await browser.newContext({ baseURL: E2E, viewport: PHONE, hasTouch: true, isMobile: true });
  const page = await ctx.newPage();
  await openTable(page, ctx.request);
  const cdp = await ctx.newCDPSession(page);
  const dialog = page.getByRole('dialog');

  // ── 1. A short sheet closes on a swipe down ──────────────────────────────
  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300); // entrance animation
  const top = await sheetTop(page);
  // The page's own overscroll is shut while the sheet is open.
  expect(await page.evaluate(() => document.documentElement.style.overscrollBehaviorY)).toBe('none');
  await drag(cdp, { x: 187, y: top + 30 }, { x: 187, y: top + 330 });
  await expect(dialog).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.style.overscrollBehaviorY)).toBe('');
  await expect(page).toHaveURL(new RegExp(`/table/\\d+$`)); // Back entry spent, still on the table

  // ── 3. A short, slow drag springs back ───────────────────────────────────
  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300);
  const restTop = await sheetTop(page);
  await drag(cdp, { x: 187, y: restTop + 30 }, { x: 187, y: restTop + 90 }, { stepMs: 40, hold: true });
  expect(await sheetTop(page)).toBeGreaterThan(restTop + 40); // it followed the finger
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  await expect.poll(() => sheetTop(page)).toBeCloseTo(restTop, 0);
  await expect(dialog).toBeVisible();

  // ── 4. A drag that starts in a text field doesn't move the sheet ─────────
  // The feedback form is its own sheet, opened from the foot of Help.
  await page.getByRole('dialog').getByRole('button', { name: 'Send feedback' }).click();
  await page.waitForTimeout(300);
  const field = dialog.locator('textarea').first();
  await expect(field).toBeVisible();
  const box = (await field.boundingBox())!;
  const before = await sheetTop(page);
  await drag(cdp, { x: box.x + box.width / 2, y: box.y + 10 }, { x: box.x + box.width / 2, y: box.y + 10 + 260 });
  await page.waitForTimeout(300);
  expect(await sheetTop(page)).toBeCloseTo(before, 0);
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await ctx.close();
});

test('a scrolled long sheet scrolls back up first, and closes only from the top', async ({ browser }) => {
  // A short window so Tones overflows its 90% sheet.
  const small = { width: 375, height: 520 };
  const ctx = await browser.newContext({ baseURL: E2E, viewport: small, hasTouch: true, isMobile: true });
  // A real lobby table, not a seeded board: Tones is long while tone topics are
  // still editable, and empty once the Main Event has locked them.
  await ctx.request.post('/api/dev/login?username=alice');
  const { game } = await (await ctx.request.post('/api/tables')).json();
  track(game.id);
  const page = await ctx.newPage();
  await page.goto(`/table/${game.id}`);
  const cdp = await ctx.newCDPSession(page);
  const dialog = page.getByRole('dialog');

  await openTones(page);
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300);
  const body = page.locator('.sheet-body');
  const overflow = await body.evaluate((el) => el.scrollHeight - el.clientHeight);
  expect(overflow, 'Tones must overflow for this to mean anything').toBeGreaterThan(60);

  const top = await sheetTop(page);
  const half = Math.floor(overflow / 2);
  await body.evaluate((el, y) => (el.scrollTop = y), half);

  // Swipe down on a half-scrolled body: it scrolls, the sheet stays put.
  await drag(cdp, { x: 187, y: top + 80 }, { x: 187, y: top + 80 + 120 }, { stepMs: 30 });
  expect(await sheetTop(page)).toBeCloseTo(top, 0);
  await expect(dialog).toBeVisible();
  const scrolled = await body.evaluate((el) => el.scrollTop);
  expect(scrolled).toBeLessThan(half);

  // Back at the top, the same swipe closes it.
  await body.evaluate((el) => (el.scrollTop = 0));
  await drag(cdp, { x: 187, y: top + 80 }, { x: 187, y: top + 80 + 260 });
  await expect(dialog).toHaveCount(0);
  await ctx.close();
});

test('reduced motion: the sheet follows the finger, then jumps to the end state', async ({ browser }) => {
  const ctx = await browser.newContext({
    baseURL: E2E, viewport: PHONE, hasTouch: true, isMobile: true, reducedMotion: 'reduce',
  });
  const page = await ctx.newPage();
  await openTable(page, ctx.request);
  const cdp = await ctx.newCDPSession(page);
  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  const top = await sheetTop(page);
  // Spring-back is immediate: no transition is ever set.
  await drag(cdp, { x: 187, y: top + 30 }, { x: 187, y: top + 90 }, { stepMs: 40, hold: true });
  expect(await sheetTop(page)).toBeGreaterThan(top + 40);
  await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
  expect(await sheetTop(page)).toBeCloseTo(top, 0);
  // And a closing drag closes at once.
  await drag(cdp, { x: 187, y: top + 30 }, { x: 187, y: top + 330 });
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await ctx.close();
});

test('only a bottom sheet drags: hosted over a column yes, the centred dialog no', async ({ browser }) => {
  const wide = { width: 900, height: 700 };
  const ctx = await browser.newContext({ baseURL: E2E, viewport: wide, hasTouch: true });
  const page = await ctx.newPage();
  await openTable(page, ctx.request);
  const cdp = await ctx.newCDPSession(page);
  const dialog = page.getByRole('dialog');

  // On the table, Help opens over a column: a bottom sheet.
  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300);
  const sheetBox = (await page.locator('.sheet').boundingBox())!;
  await drag(cdp, { x: sheetBox.x + sheetBox.width / 2, y: sheetBox.y + 30 },
    { x: sheetBox.x + sheetBox.width / 2, y: sheetBox.y + 330 });
  await expect(dialog).toHaveCount(0);

  // On the profile page there is no column to cover: a centred dialog, left alone.
  await page.goto('/profile');
  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300);
  const b = (await page.locator('.sheet').boundingBox())!;
  await drag(cdp, { x: b.x + b.width / 2, y: b.y + 30 }, { x: b.x + b.width / 2, y: b.y + 330 });
  await page.waitForTimeout(300);
  await expect(dialog).toBeVisible();
  expect((await page.locator('.sheet').boundingBox())!.y).toBeCloseTo(b.y, 0);
  await ctx.close();
});

test("Help's own scrolling tab panel is protected like the sheet body", async ({ browser }) => {
  // On a phone Help gives its tab panel a fixed height and scrolls *that*, so
  // the sheet body's own scrollTop stays 0. The drag has to look at the
  // scroller under the finger, not just the body.
  const ctx = await browser.newContext({ baseURL: E2E, viewport: PHONE, hasTouch: true, isMobile: true });
  const page = await ctx.newPage();
  await openTable(page, ctx.request);
  const cdp = await ctx.newCDPSession(page);
  const dialog = page.getByRole('dialog');

  await page.getByRole('button', { name: /How to play/ }).click();
  await expect(dialog).toBeVisible();
  await page.waitForTimeout(300);
  const inner = dialog.locator('.help.panel .body');
  const overflow = await inner.evaluate((el) => el.scrollHeight - el.clientHeight);
  expect(overflow, "Help's panel body must overflow for this to mean anything").toBeGreaterThan(100);
  expect(await page.locator('.sheet-body').evaluate((el) => el.scrollTop)).toBe(0);

  const half = Math.floor(overflow / 2);
  await inner.evaluate((el, y) => (el.scrollTop = y), half);
  const box = (await inner.boundingBox())!;
  const top = await sheetTop(page);

  // A downward swipe on the half-scrolled inner body scrolls it back up.
  await drag(cdp, { x: 187, y: box.y + 40 }, { x: 187, y: box.y + 40 + 120 }, { stepMs: 30 });
  expect(await sheetTop(page)).toBeCloseTo(top, 0);
  await expect(dialog).toBeVisible();
  expect(await inner.evaluate((el) => el.scrollTop)).toBeLessThan(half);

  // At the top of the inner body, the same swipe closes the sheet.
  await inner.evaluate((el) => (el.scrollTop = 0));
  await drag(cdp, { x: 187, y: box.y + 40 }, { x: 187, y: box.y + 40 + 300 });
  await expect(dialog).toHaveCount(0);
  await ctx.close();
});
