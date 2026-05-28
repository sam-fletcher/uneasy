import { test, expect, request as pwRequest, type APIRequestContext } from '@playwright/test';

// End-to-end coverage for the multi-player chat path: two browser contexts
// (alice, bob), each with its own cookie jar; alice creates a table, bob
// joins, alice posts a chat message, and we assert bob's page picks it up
// via the WebSocket hub without a reload.
//
// Table creation and join are done via the HTTP API rather than the UI —
// the UI for those flows is exercised separately. This spec exists to pin
// the chat + WebSocket loop specifically.

async function devLogin(api: APIRequestContext, username: string) {
  const res = await api.post(`/api/dev/login?username=${encodeURIComponent(username)}`);
  expect(res.ok(), `dev-login for ${username} failed`).toBeTruthy();
}

test('alice sends a chat message; bob sees it live', async ({ browser, playwright }) => {
  // Global reset so we know the test starts from a clean uneasy_test.
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });

  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  // Alice creates the table via API; capture the id and join code.
  const createRes = await aliceCtx.request.post('/api/tables');
  expect(createRes.ok()).toBeTruthy();
  const { game } = await createRes.json();
  const tableId: number = game.id;
  const joinCode: string = game.join_code;

  // Bob joins by code.
  const joinRes = await bobCtx.request.post('/api/tables/join', {
    data: { join_code: joinCode },
  });
  expect(joinRes.ok(), `bob could not join ${joinCode}`).toBeTruthy();

  // Both open the table page. Pages are created from the per-user contexts
  // so each carries the right session cookie.
  const alicePage = await aliceCtx.newPage();
  const bobPage = await bobCtx.newPage();
  await Promise.all([
    alicePage.goto(`/table/${tableId}`),
    bobPage.goto(`/table/${tableId}`),
  ]);

  // The chat panel is rendered with aria-label="Chat" on desktop. Wait for
  // bob's panel to mount and his WebSocket subscription to be live before
  // alice sends.
  const bobChat = bobPage.getByRole('complementary', { name: 'Chat' });
  await expect(bobChat).toBeVisible();
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();

  const message = `hello-from-alice-${Date.now()}`;
  await aliceChat.getByPlaceholder('Write a message…').fill(message);
  await aliceChat.getByRole('button', { name: 'Send' }).click();

  // Alice should see her own message echo back.
  await expect(aliceChat.getByText(message)).toBeVisible();

  // The real assertion: bob's page picks it up via the WebSocket broadcast
  // without any navigation or reload.
  await expect(bobChat.getByText(message)).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});
