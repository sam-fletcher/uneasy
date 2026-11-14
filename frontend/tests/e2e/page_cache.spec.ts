import { test, expect, request as pwRequest, type Page, type Request } from '@playwright/test';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// Guards the stale-while-revalidate snapshots in $lib/pageCache.
//
// The failure this exists to catch is silent: if snapshot capture or seeding
// breaks, nothing errors — every page still loads, just slowly again, exactly
// as it did before. So the assertion is a time budget, made unambiguous by
// injecting latency far larger than the budget: a return visit that actually
// waited on the network could not possibly come in under it.
//
// The budget must time the PAINT and nothing else. Two things used to leak
// into it (the nightly of 2026-11-14 failed on the first, at 1984ms):
//   - `page.click()` waits for its target to be "stable", which needs
//     animation frames; a stalled runner held it for ~1.9s before the click
//     even landed, and the table then painted 26ms after. `timedClick` runs
//     those checks first, untimed, and starts the clock only at the click.
//   - `waitForLoadState('networkidle')` is not a reliable "the page has
//     settled" in a client-routed app: it resolved at once while the profile's
//     revalidate was still in flight, so the next click raced it. `settled`
//     counts the page's own /api requests instead.

const track = cleanupGameAfterEach();

// Every /api call is delayed by this much. A cold table load needs three
// sequential rounds, so a genuine load cannot beat ~3x this.
const API_DELAY_MS = 400;
// A seeded paint does no network work at all, so it has the whole budget to
// itself. Comfortably under one round trip, comfortably over render time.
const RETURN_BUDGET_MS = 300;
// How long /api must stay quiet to count as settled. Rounds of one load follow
// each other within milliseconds of a response landing, so this only has to
// bridge that gap; it is not a guess at how long loading takes.
const QUIET_MS = 250;

/** Counts the page's in-flight /api requests (the page's own, so a delayed one
 *  counts from the moment it is issued, not when the route lets it through).
 *  Call before the first navigation. Returns `settled()`: resolves once none
 *  has been in flight for QUIET_MS. */
function trackApi(page: Page) {
  let inFlight = 0;
  const isApi = (r: Request) => new URL(r.url()).pathname.startsWith('/api/');
  const done = (r: Request) => { if (isApi(r)) inFlight -= 1; };
  page.on('request', (r) => { if (isApi(r)) inFlight += 1; });
  page.on('requestfinished', done);
  page.on('requestfailed', done);

  return async function settled() {
    const deadline = Date.now() + 20_000;
    let quietSince: number | null = null;
    while (Date.now() < deadline) {
      if (inFlight === 0) {
        quietSince ??= Date.now();
        if (Date.now() - quietSince >= QUIET_MS) return;
      } else {
        quietSince = null;
      }
      await page.waitForTimeout(25);
    }
    throw new Error(`/api never went quiet (${inFlight} requests still in flight)`);
  };
}

/** Clicks `selector` and returns the moment the click was dispatched. The trial
 *  click does Playwright's actionability waiting (visible, enabled, stable)
 *  without clicking, so none of it lands inside the caller's timed window. */
async function timedClick(page: Page, selector: string): Promise<number> {
  const target = page.locator(selector);
  await target.click({ trial: true });
  const start = Date.now();
  await target.click({ force: true });
  return start;
}

test('returning to a page you just left paints from the snapshot, not the network', async ({ browser }) => {
  const fixture = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await fixture.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seedRes.ok(), 'seed call failed').toBeTruthy();
  const { game_id } = await seedRes.json();
  track(game_id);
  await fixture.dispose();

  const ctx = await browser.newContext({ baseURL: E2E });
  await ctx.request.post('/api/dev/login?username=alice');
  const page = await ctx.newPage();
  const settled = trackApi(page);

  await page.route('**/api/**', async (route) => {
    await new Promise((r) => setTimeout(r, API_DELAY_MS));
    await route.continue();
  });

  // Clicking, not goto: a hard navigation re-executes the module and drops the
  // cache with it, which is expected and is why this must click through.
  await page.goto('/profile');
  await page.waitForSelector('a.table-link');

  const card = `a.table-link[href="/table/${game_id}"]`;

  // First visit pays full freight — asserted so the test fails loudly if the
  // latency injection ever stops working and makes the rest meaningless.
  const coldStart = Date.now();
  await page.click(card);
  await expect(page.locator('.main-event-view')).toBeVisible({ timeout: 20000 });
  const cold = Date.now() - coldStart;
  expect(cold, 'latency injection is not taking effect').toBeGreaterThan(API_DELAY_MS);
  // The snapshot is written when the load finishes: leave only after it has.
  await settled();

  // Back to the profile, which was itself snapshotted on the way through.
  const profileStart = await timedClick(page, 'a.home');
  await expect(page.locator('a.table-link').first()).toBeVisible({ timeout: 20000 });
  expect(Date.now() - profileStart).toBeLessThan(RETURN_BUDGET_MS);
  // Let the profile's own revalidate land, so it can't re-render the card
  // under the next click.
  await settled();

  // And back to the same table.
  const returnStart = await timedClick(page, card);
  await expect(page.locator('.main-event-view')).toBeVisible({ timeout: 20000 });
  expect(Date.now() - returnStart).toBeLessThan(RETURN_BUDGET_MS);

  // The snapshot is a paint, not a substitute for loading: the revalidate must
  // still run and the socket must still come up, or the table would be frozen
  // at whatever it looked like a minute ago.
  await settled();
  await expect(tableTitle(page)).toHaveText(/^Row \d+/);
  await ctx.close();
});
