import { test, expect, request as pwRequest } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Guards the stale-while-revalidate snapshots in $lib/pageCache.
//
// The failure this exists to catch is silent: if snapshot capture or seeding
// breaks, nothing errors — every page still loads, just slowly again, exactly
// as it did before. So the assertion is a time budget, made unambiguous by
// injecting latency far larger than the budget: a return visit that actually
// waited on the network could not possibly come in under it.

const track = cleanupGameAfterEach();

// Every /api call is delayed by this much. A cold table load needs three
// sequential rounds, so a genuine load cannot beat ~3x this.
const API_DELAY_MS = 400;
// A seeded paint does no network work at all, so it has the whole budget to
// itself. Comfortably under one round trip, comfortably over render time.
const RETURN_BUDGET_MS = 300;

test('returning to a page you just left paints from the snapshot, not the network', async ({ browser }) => {
  const fixture = await pwRequest.newContext({ baseURL: 'http://localhost:8090' });
  const seedRes = await fixture.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seedRes.ok(), 'seed call failed').toBeTruthy();
  const { game_id } = await seedRes.json();
  track(game_id);
  await fixture.dispose();

  const ctx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await ctx.request.post('/api/dev/login?username=alice');
  const page = await ctx.newPage();

  await page.route('**/api/**', async (route) => {
    await new Promise((r) => setTimeout(r, API_DELAY_MS));
    await route.continue();
  });

  // Clicking, not goto: a hard navigation re-executes the module and drops the
  // cache with it, which is expected and is why this must click through.
  await page.goto('/profile');
  await page.waitForSelector('a.table-card');

  const card = `a.table-card[href="/table/${game_id}"]`;

  // First visit pays full freight — asserted so the test fails loudly if the
  // latency injection ever stops working and makes the rest meaningless.
  const coldStart = Date.now();
  await page.click(card);
  await expect(page.locator('.main-event-view')).toBeVisible({ timeout: 20000 });
  const cold = Date.now() - coldStart;
  expect(cold, 'latency injection is not taking effect').toBeGreaterThan(API_DELAY_MS);
  await page.waitForLoadState('networkidle');

  // Back to the profile, which was itself snapshotted on the way through.
  const profileStart = Date.now();
  await page.click('a.home');
  await expect(page.locator('a.table-card').first()).toBeVisible({ timeout: 20000 });
  expect(Date.now() - profileStart).toBeLessThan(RETURN_BUDGET_MS);
  await page.waitForLoadState('networkidle');

  // And back to the same table.
  const returnStart = Date.now();
  await page.click(card);
  await expect(page.locator('.main-event-view')).toBeVisible({ timeout: 20000 });
  expect(Date.now() - returnStart).toBeLessThan(RETURN_BUDGET_MS);

  // The snapshot is a paint, not a substitute for loading: the revalidate must
  // still run and the socket must still come up, or the table would be frozen
  // at whatever it looked like a minute ago.
  await page.waitForLoadState('networkidle');
  await expect(page.locator('.phase-badge')).toHaveText('Main Event');
  await ctx.close();
});
