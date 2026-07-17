import { test, expect, type Browser, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Resilience specs for the asynchronous play-by-post model: players close
// tabs, lose connections, and come back later. Two scenarios:
//
//   1. Reload — bob refreshes the page after seeing a message. Prior
//      history must rehydrate from the server, and a new live message
//      from alice must still arrive without further interaction (proves
//      the WebSocket re-subscribes cleanly on a fresh page load).
//
//   2. Offline blip — bob's connection drops mid-session, then comes
//      back. ws.ts has an exponential-backoff reconnect (1s..30s);
//      messages alice sends during the outage should arrive once bob is
//      back online. Validates the reconnect path in ws.ts:295.

async function setupAliceAndBob(browser: Browser) {
  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await aliceCtx.request.post('/api/dev/login?username=alice');
  await bobCtx.request.post('/api/dev/login?username=bob');

  const { game } = await (await aliceCtx.request.post('/api/tables')).json();
  await bobCtx.request.post('/api/tables/join', {
    data: { join_code: game.join_code },
  });

  const alicePage = await aliceCtx.newPage();
  const bobPage = await bobCtx.newPage();
  await Promise.all([
    alicePage.goto(`/table/${game.id}`),
    bobPage.goto(`/table/${game.id}`),
  ]);
  return { aliceCtx, bobCtx, alicePage, bobPage, gameID: game.id };
}

async function sendChat(page: Page, body: string) {
  const chat = page.getByRole('complementary', { name: 'Chat' });
  await chat.getByPlaceholder('Write a message…').fill(body);
  await chat.getByRole('button', { name: 'Send' }).click();
}

const track = cleanupGameAfterEach();

test('reload preserves chat history and re-subscribes to live updates', async ({ browser }) => {
  const { aliceCtx, bobCtx, alicePage, bobPage, gameID } = await setupAliceAndBob(browser);
  track(gameID);

  const before = `before-reload-${Date.now()}`;
  await sendChat(alicePage, before);
  await expect(bobPage.getByRole('complementary', { name: 'Chat' }).getByText(before))
    .toBeVisible();

  // Bob refreshes — full page nav, fresh WebSocket subscription, fresh
  // history fetch from /api/tables/{id}/posts.
  await bobPage.goto(`/table/${gameID}`);

  const bobChat = bobPage.getByRole('complementary', { name: 'Chat' });
  await expect(bobChat).toBeVisible();
  // Persistence: prior message is still there after reload.
  await expect(bobChat.getByText(before)).toBeVisible();

  // Re-subscription: a new message from alice still arrives live.
  const after = `after-reload-${Date.now()}`;
  await sendChat(alicePage, after);
  await expect(bobChat.getByText(after)).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test("bob's WebSocket auto-reconnects after a network blip", async ({ browser }) => {
  const { aliceCtx, bobCtx, alicePage, bobPage, gameID } = await setupAliceAndBob(browser);
  track(gameID);

  // Sanity: live path works before the blip.
  const baseline = `baseline-${Date.now()}`;
  await sendChat(alicePage, baseline);
  await expect(bobPage.getByRole('complementary', { name: 'Chat' }).getByText(baseline))
    .toBeVisible();

  // Knock bob offline. This severs the WebSocket; ws.ts:295 will start
  // its exponential backoff.
  await bobCtx.setOffline(true);

  // While bob is offline, alice posts a message. Bob's UI cannot have
  // received it yet — wait long enough for any pending in-flight frame
  // to settle, then confirm it really isn't there.
  const duringOutage = `during-outage-${Date.now()}`;
  await sendChat(alicePage, duringOutage);
  await bobPage.waitForTimeout(500);
  await expect(bobPage.getByRole('complementary', { name: 'Chat' }).getByText(duringOutage))
    .toHaveCount(0);

  // Bring bob back online. ws.ts resyncs on (re)connect, so the missed
  // message should arrive without any user action. Backoff starts at 1s
  // but doubles on failed attempts, so give the assertion generous time.
  await bobCtx.setOffline(false);
  await expect(bobPage.getByRole('complementary', { name: 'Chat' }).getByText(duringOutage))
    .toBeVisible({ timeout: 15_000 });

  await aliceCtx.close();
  await bobCtx.close();
});
