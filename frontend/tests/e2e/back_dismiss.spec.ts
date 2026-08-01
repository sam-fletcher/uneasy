import { test, expect, type APIRequestContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Phone Back closes the overlay on top instead of leaving the table
// ($lib/overlayHistory). The three behaviours worth pinning are the ones that
// break quietly:
//
//   1. Back closes the overlay and stays on /table/[id].
//   2. Closing by the overlay's own control spends the history entry, so the
//      next Back leaves the table rather than being a dead press.
//   3. A hand-off — one surface closing as another opens in the same click —
//      adds no entry, so one Back still returns to the table instead of
//      reopening what was just dismissed.
//
// Driven at a phone viewport: the chat panel and the Public Record are only
// overlays below their dock breakpoints, and the sheets are the surfaces a
// player on a phone actually reaches for.

const PHONE = { width: 390, height: 844 };

async function devLogin(api: APIRequestContext, username: string) {
  const res = await api.post(`/api/dev/login?username=${encodeURIComponent(username)}`);
  expect(res.ok(), `dev-login for ${username} failed`).toBeTruthy();
}

/** The overlay depth the router is currently holding — the coordinator's one
 *  piece of observable state. 0 means no overlay owns a history entry. */
function overlayDepth(page: Page): Promise<number> {
  return page.evaluate(() => history.state?.['sveltekit:states']?.overlayDepth ?? 0);
}

const track = cleanupGameAfterEach();

test.use({ viewport: PHONE });

test('phone Back dismisses overlays instead of leaving the table', async ({ browser }) => {
  const ctx = await browser.newContext({ baseURL: 'http://localhost:8090', viewport: PHONE });
  await devLogin(ctx.request, 'alice');

  const seedRes = await ctx.request.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seedRes.ok()).toBeTruthy();
  const { game_id: tableId } = await seedRes.json();
  track(tableId);

  const page = await ctx.newPage();
  // Land on /profile first so the table has somewhere to go back *to* — that
  // navigation is exactly what these entries are protecting against.
  await page.goto('/profile');
  await page.goto(`/table/${tableId}`);
  await expect(page.locator('.phase-badge')).toHaveText('Main Event');
  expect(await overlayDepth(page)).toBe(0);

  // ── 1. Back closes the sheet, and the table survives ─────────────────────
  await page.getByRole('button', { name: 'Open tones' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(await overlayDepth(page)).toBe(1);

  await page.goBack();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/table/${tableId}$`));
  expect(await overlayDepth(page)).toBe(0);

  // ── 2. Closing by the × spends the entry ─────────────────────────────────
  await page.getByRole('button', { name: 'Open laws' }).click();
  await expect(page.getByRole('dialog')).toBeVisible();
  expect(await overlayDepth(page)).toBe(1);

  await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect.poll(() => overlayDepth(page)).toBe(0);

  // No stale entry left behind: Back now means what it always meant.
  await page.goBack();
  await expect(page).toHaveURL(/\/profile$/);
  await page.goForward();
  await expect(page.locator('.phase-badge')).toHaveText('Main Event');

  // ── 3. A hand-off costs no extra entry ───────────────────────────────────
  // Tapping a header panel while the chat sheet is up closes the chat and
  // opens the panel in one click (the table page keeps a single full-screen
  // surface on mobile).
  await page.getByRole('button', { name: /^Open chat/ }).click();
  await expect(page.locator('[aria-modal="true"]')).toBeVisible();
  expect(await overlayDepth(page)).toBe(1);

  await page.getByRole('button', { name: 'Open rumors' }).click();
  await expect(page.getByRole('dialog').getByRole('heading', { name: 'Rumors' })).toBeVisible();
  expect(await overlayDepth(page)).toBe(1);

  // One Back returns to the table — it must not reopen the chat it replaced.
  await page.goBack();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('[aria-modal="true"]')).toHaveCount(0);
  await expect(page).toHaveURL(new RegExp(`/table/${tableId}$`));
  expect(await overlayDepth(page)).toBe(0);

  await ctx.close();
});
