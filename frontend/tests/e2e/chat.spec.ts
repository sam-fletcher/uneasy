import { test, expect, request as pwRequest, type APIRequestContext } from '@playwright/test';

// End-to-end coverage for the multi-player chat path: two browser contexts
// (alice, bob), each with its own cookie jar; alice creates a table, bob
// joins, alice posts a chat message, and we assert bob's page picks it up
// via the WebSocket hub without a reload.
//
// Table creation and join are done via the HTTP API rather than the UI —
// the UI for those flows is exercised separately. This spec exists to pin
// the chat + WebSocket loop specifically. The Chat Overhaul Phase 2 tests
// below (catch-up, scroll-up pagination, no-yank, jump-to-history) extend
// it, and the Phase 3 test (hide-bookkeeping toggle) extends it further —
// see adr/CHAT_OVERHAUL_PLAN.md.

async function devLogin(api: APIRequestContext, username: string) {
  const res = await api.post(`/api/dev/login?username=${encodeURIComponent(username)}`);
  expect(res.ok(), `dev-login for ${username} failed`).toBeTruthy();
}

/** Posts `count` player messages via the API, sequentially, with bodies
 *  `${prefix}-0` .. `${prefix}-(count-1)`. Used to build up enough chat
 *  history that the windowed feed's pagination/context math kicks in. */
async function sendMessages(api: APIRequestContext, tableId: number, prefix: string, count: number) {
  for (let i = 0; i < count; i++) {
    const res = await api.post(`/api/tables/${tableId}/posts`, { data: { body: `${prefix}-${i}` } });
    expect(res.ok(), `post ${prefix}-${i} failed`).toBeTruthy();
  }
}

/** The newest post's id, as seen by `api`. */
async function newestPostID(api: APIRequestContext, tableId: number): Promise<number> {
  const res = await api.get(`/api/tables/${tableId}/posts`);
  expect(res.ok()).toBeTruthy();
  const { posts } = (await res.json()) as { posts: { id: number }[] };
  return posts[posts.length - 1].id;
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

test('catch-up: messages sent while away show the unread badge and the New messages divider', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });

  // Bob posts while alice is "away" — she hasn't opened the table page yet,
  // so her last_read_post_id is still 0 and every one of these is unread.
  const awayMessage = `away-msg-${Date.now()}`;
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: awayMessage } });

  // Mobile viewport: the unread badge only renders on the collapsed strip
  // (desktop's panel is always open, so it has no separate "closed" state
  // to badge). See feedback_mobile_first.
  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 });
  await alicePage.goto(`/table/${tableId}`);

  const strip = alicePage.locator('.strip');
  await expect(strip).toHaveClass(/has-unread/);
  await expect(strip.locator('.unread-badge')).toHaveText('1');

  // Opening the panel reveals the "New messages" divider right above the
  // message that arrived while she was away. Expanded-mobile sets
  // role="dialog" (see ChatPanel.svelte), so locate by aria-label rather
  // than the desktop-only "complementary" role.
  await strip.click();
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat.getByText('New messages')).toBeVisible();
  await expect(chat.getByText(awayMessage)).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test('scroll-up pagination: older posts outside the initial window load on demand', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });

  // Build up history in two batches with alice's read marker parked between
  // them, so a *fresh* load's "extend back 30 posts of read context" doesn't
  // simply swallow the whole thing (a brand-new marker of 0 would pull in
  // every unread post regardless of count, up to the 500 cap — see
  // buildInitialWindow in handler/posts.go). filler-* ends up outside the
  // initial window; newmsg-* stays inside it.
  await sendMessages(bobCtx.request, tableId, 'filler', 50);
  const markerID = await newestPostID(aliceCtx.request, tableId);
  const markRes = await aliceCtx.request.put(`/api/tables/${tableId}/read-marker`, {
    data: { last_read_post_id: markerID },
  });
  expect(markRes.ok()).toBeTruthy();
  await sendMessages(bobCtx.request, tableId, 'newmsg', 120);

  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${tableId}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();
  const feed = aliceChat.locator('.feed');

  await expect(feed.getByText('newmsg-0')).toBeVisible();
  await expect(feed.getByText('filler-0')).toHaveCount(0);

  await feed.evaluate((el) => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });

  await expect(feed.getByText('filler-0')).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test('scrolled away from the bottom is not yanked back when a new message arrives', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });

  // Enough history that the feed genuinely overflows and has something to
  // scroll away from.
  await sendMessages(bobCtx.request, tableId, 'seed', 40);

  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${tableId}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();
  const feed = aliceChat.locator('.feed');
  await expect(feed.getByText('seed-39')).toBeVisible();

  await feed.evaluate((el) => { el.scrollTop = 0; el.dispatchEvent(new Event('scroll')); });
  const scrollTopBefore = await feed.evaluate((el) => el.scrollTop);

  const message = `no-yank-${Date.now()}`;
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: message } });

  // The pill appears instead of the view snapping to the tail.
  await expect(aliceChat.locator('.new-pill')).toBeVisible();
  const scrollTopAfter = await feed.evaluate((el) => el.scrollTop);
  expect(scrollTopAfter).toBeLessThanOrEqual(scrollTopBefore + 5);

  // Tapping the pill catches her up.
  await aliceChat.locator('.new-pill').click();
  await expect(feed.getByText(message)).toBeVisible();
  await expect(aliceChat.locator('.new-pill')).toHaveCount(0);

  await aliceCtx.close();
  await bobCtx.close();
});

test('hide bookkeeping toggle: hides a Trace rename post, keeps a Default take post visible', async ({ browser }) => {
  // A fresh lobby-phase table has no public-record row yet, and
  // EmitSystemPost silently drops any post whose rowNumber doesn't match an
  // existing row (see handler/posts.go) — so asset.renamed/asset.taken would
  // never actually land. Seed straight to main_event, like the jump-to-row
  // test below, so the row FK is satisfied.
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  // Alice creates a peer, then renames it — asset.renamed is Trace-tier
  // bookkeeping, hidden by default.
  const createAssetRes = await aliceCtx.request.post(`/api/tables/${tableId}/assets`, {
    data: { asset_type: 'peer', name: 'Original Name', is_main_character: false, marginalia: ['a trusted contact'] },
  });
  expect(createAssetRes.ok(), `create asset failed: ${await createAssetRes.text()}`).toBeTruthy();
  const assetId: number = (await createAssetRes.json()).asset.id;

  const renameRes = await aliceCtx.request.put(`/api/assets/${assetId}`, {
    data: { name: 'Renamed Peer' },
  });
  expect(renameRes.ok(), `rename failed: ${await renameRes.text()}`).toBeTruthy();

  // Bob takes it — asset.taken is Default-tier and always visible.
  const takeRes = await bobCtx.request.post(`/api/assets/${assetId}/take`);
  expect(takeRes.ok(), `take failed: ${await takeRes.text()}`).toBeTruthy();

  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${tableId}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();
  const feed = aliceChat.locator('.feed');

  // Default state: bookkeeping hidden — the rename post is gone, the take
  // post shows regardless. Display names default to the (lowercase)
  // dev-login username — see handler/tables.go's DisplayName: acct.Username.
  await expect(feed.getByText('bob took Renamed Peer', { exact: false })).toBeVisible();
  await expect(feed.getByText('renamed Original Name', { exact: false })).toHaveCount(0);

  // Unchecking "Hide bookkeeping" reveals the rename post too.
  await aliceChat.getByRole('checkbox', { name: 'Hide bookkeeping' }).uncheck();
  await expect(feed.getByText('renamed Original Name', { exact: false })).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test('jumping to a row outside the loaded window enters history mode; Return to now exits it', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: gameID } = await seedRes.json();
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice'); // alice is the seed's facilitator + focus player
  await devLogin(bobCtx.request, 'bob');

  // A real row advance (not the /api/dev/* shortcut, which writes current_row
  // directly and skips this) emits the row.advanced post the Public Record's
  // row jump anchors on.
  const advanceRes = await aliceCtx.request.post(`/api/tables/${gameID}/advance-row`);
  expect(advanceRes.ok(), `advance-row failed: ${await advanceRes.text()}`).toBeTruthy();

  // Same read-marker trick as the pagination test: push the row.advanced
  // anchor far enough behind alice's marker that a fresh load's window
  // starts after it.
  await sendMessages(bobCtx.request, gameID, 'filler', 40);
  const markerID = await newestPostID(aliceCtx.request, gameID);
  await aliceCtx.request.put(`/api/tables/${gameID}/read-marker`, {
    data: { last_read_post_id: markerID },
  });
  await sendMessages(bobCtx.request, gameID, 'newmsg', 120);

  // Default Playwright viewport (1280x720) is exactly at PublicRecord's
  // ≥1280px "permanent panel" breakpoint, so the row rail needs no tap to
  // expand.
  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${gameID}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();
  await expect(aliceChat.getByText('Row 2 begins')).toHaveCount(0);

  await alicePage.getByRole('button', { name: 'Jump to row 2' }).click();

  await expect(aliceChat.locator('.return-to-now')).toBeVisible();
  await expect(aliceChat.getByText('Row 2 begins')).toBeVisible();

  await aliceChat.locator('.return-to-now').click();
  await expect(aliceChat.locator('.return-to-now')).toHaveCount(0);
  await expect(aliceChat.locator('.feed').getByText('newmsg-119')).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

// Chat Overhaul Phase 4: scene containers. A turn scene's whole span —
// in-character lines and table-talk alike (single chronology) — renders
// inside one collapsible container, distinct from plain messages sent
// outside any scene. Mobile-first, per feedback_mobile_first: verify the
// narrow-viewport collapsed-strip path before the desktop column.
test('scene container: single chronology, collapses once ended and read, expands on tap', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId, players } = await seedRes.json();
  const alicePlayerID: number = players[0].id;
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice'); // alice is the seed's facilitator + focus player
  await devLogin(bobCtx.request, 'bob');

  // Alice (focus player) opens a scene.
  const sceneRes = await aliceCtx.request.post(`/api/tables/${tableId}/scenes`, {
    data: { location_custom: 'The Mill', time_elapsed: 'days', present_peer_ids: [] },
  });
  expect(sceneRes.ok(), `create scene failed: ${await sceneRes.text()}`).toBeTruthy();

  // Alice's main character (seeded by SeedMainEvent) to post in character;
  // the focus player's MC is implicitly present, no claim needed.
  const assetsRes = await aliceCtx.request.get(`/api/tables/${tableId}/assets`);
  const { assets } = (await assetsRes.json()) as { assets: { id: number; owner_id: number; is_main_character: boolean }[] };
  const aliceMC = assets.find((a) => a.owner_id === alicePlayerID && a.is_main_character);
  expect(aliceMC, 'alice main character not found').toBeTruthy();

  // One in-character line and one table-talk line — single chronology, both
  // inside the scene container.
  const icBody = 'The wind picks up.';
  const ttBody = 'nice weather huh';
  await aliceCtx.request.post(`/api/tables/${tableId}/posts`, {
    data: { body: icBody, speaking_as_asset_id: aliceMC!.id },
  });
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: ttBody } });

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 }); // mobile-first
  await alicePage.goto(`/table/${tableId}`);
  await alicePage.locator('.strip').click();
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat).toBeVisible();

  // Live scene: expanded, header shows the banner + count, and can't collapse.
  const header = chat.locator('.scene-header');
  await expect(header).toBeVisible();
  await expect(header).toContainText('The Mill');
  await expect(header).toContainText('2 messages');
  await expect(header).toBeDisabled();
  const box = await header.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44); // mobile-first tap target

  await expect(chat.getByText(icBody)).toBeVisible();
  await expect(chat.getByText(ttBody)).toBeVisible();
  // Registers: in-character gets the heavier treatment, table-talk the
  // lighter one — both only apply inside the scene container.
  await expect(chat.locator('.message.in-character', { hasText: icBody })).toBeVisible();
  await expect(chat.locator('.message.table-talk', { hasText: ttBody })).toBeVisible();

  // Alice ends the scene, then marks everything read so the next load's
  // default-collapse isn't overridden by an unread divider landing inside it.
  const endRes = await aliceCtx.request.post(`/api/tables/${tableId}/end-scene`);
  expect(endRes.ok(), `end-scene failed: ${await endRes.text()}`).toBeTruthy();
  const markerID = await newestPostID(aliceCtx.request, tableId);
  const markRes = await aliceCtx.request.put(`/api/tables/${tableId}/read-marker`, {
    data: { last_read_post_id: markerID },
  });
  expect(markRes.ok()).toBeTruthy();

  // A plain message sent after the scene ends lives outside any container
  // and never gets the in-scene register classes.
  const afterBody = 'the scene is over now';
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: afterBody } });

  await alicePage.reload();
  await alicePage.locator('.strip').click();
  const chat2 = alicePage.locator('[aria-label="Chat"]');
  const header2 = chat2.locator('.scene-header');
  await expect(header2).toBeVisible();
  await expect(header2).toContainText('2 messages');
  await expect(header2).not.toBeDisabled();

  // Collapsed by default (ended + fully read): inner posts aren't rendered.
  await expect(chat2.getByText(icBody)).toHaveCount(0);
  await expect(chat2.getByText(ttBody)).toHaveCount(0);
  await expect(chat2.getByText(afterBody)).toBeVisible();
  const afterMessage = chat2.locator('.message', { hasText: afterBody });
  await expect(afterMessage).not.toHaveClass(/in-character/);
  await expect(afterMessage).not.toHaveClass(/table-talk/);

  // Tap to expand reveals the single chronology again.
  await header2.click();
  await expect(chat2.getByText(icBody)).toBeVisible();
  await expect(chat2.getByText(ttBody)).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test('scene container renders in the always-open desktop chat column', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice');

  const sceneRes = await aliceCtx.request.post(`/api/tables/${tableId}/scenes`, {
    data: { location_custom: 'The Docks', time_elapsed: 'moments', present_peer_ids: [] },
  });
  expect(sceneRes.ok(), `create scene failed: ${await sceneRes.text()}`).toBeTruthy();
  await aliceCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: 'quiet down at the docks' } });

  // Default Playwright viewport (1280x720) is desktop-width; the chat panel
  // is a permanent column, no strip to tap.
  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${tableId}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();

  const header = aliceChat.locator('.scene-header');
  await expect(header).toBeVisible();
  await expect(header).toContainText('The Docks');
  await expect(header).toBeDisabled(); // still live

  await aliceCtx.request.post(`/api/tables/${tableId}/end-scene`);
  const markerID = await newestPostID(aliceCtx.request, tableId);
  await aliceCtx.request.put(`/api/tables/${tableId}/read-marker`, { data: { last_read_post_id: markerID } });

  await alicePage.reload();
  const header2 = aliceChat.locator('.scene-header');
  await expect(header2).not.toBeDisabled();
  await expect(aliceChat.getByText('quiet down at the docks')).toHaveCount(0);
  await header2.click();
  await expect(aliceChat.getByText('quiet down at the docks')).toBeVisible();

  await aliceCtx.close();
});

// Chat Overhaul Phase 5: plan-scenes. Host Festivity has no location/time
// setup step — the plan-scene opens automatically the moment resolution
// kicks off, with every player already a participant (no join step; see
// hfRoster in handler/plan_host_festivity.go) — so both host and guest can
// speak in character immediately, rendering inside the SAME scene-container
// UI Phase 4 already built (no ChatPanel/chatFeed changes needed for this).
test('plan-scene: a festivity renders both players\' in-character posts inside one container', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId, players } = await seedRes.json();
  const alicePlayerID: number = players[0].id;
  const bobPlayerID: number = players[1].id;
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await devLogin(aliceCtx.request, 'alice'); // alice is the seed's facilitator + focus player
  await devLogin(bobCtx.request, 'bob');

  // Alice hosts a festivity.
  const prepRes = await aliceCtx.request.post(`/api/tables/${tableId}/prepare-plan`, {
    data: { plan_type: 'host_festivity', preparation_notes: 'a grand ball' },
  });
  expect(prepRes.ok(), `prepare-plan failed: ${await prepRes.text()}`).toBeTruthy();
  const { plan } = await prepRes.json();

  // Jump straight to the festivity's row — advance-row's row-state broadcast
  // auto-kicks off resolution for a pending plan on the new current row (no
  // separate "click resolve" step; see broadcastRowState in handler/row_state.go).
  const jumpRes = await aliceCtx.request.post('/api/dev/advance-row', { data: { plan_id: plan.id } });
  expect(jumpRes.ok(), `advance-row failed: ${await jumpRes.text()}`).toBeTruthy();

  const assetsRes = await aliceCtx.request.get(`/api/tables/${tableId}/assets`);
  const { assets } = (await assetsRes.json()) as {
    assets: { id: number; owner_id: number; is_main_character: boolean }[];
  };
  const aliceMC = assets.find((a) => a.owner_id === alicePlayerID && a.is_main_character);
  const bobMC = assets.find((a) => a.owner_id === bobPlayerID && a.is_main_character);
  expect(aliceMC, 'alice main character not found').toBeTruthy();
  expect(bobMC, 'bob main character not found').toBeTruthy();

  const hostLine = 'Welcome, honored guests!';
  const guestLine = 'A pleasure to be here.';
  const hostPost = await aliceCtx.request.post(`/api/tables/${tableId}/posts`, {
    data: { body: hostLine, speaking_as_asset_id: aliceMC!.id },
  });
  expect(hostPost.ok(), `host in-character post failed: ${await hostPost.text()}`).toBeTruthy();
  const guestPost = await bobCtx.request.post(`/api/tables/${tableId}/posts`, {
    data: { body: guestLine, speaking_as_asset_id: bobMC!.id },
  });
  expect(guestPost.ok(), `guest in-character post failed: ${await guestPost.text()}`).toBeTruthy();

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 }); // mobile-first
  await alicePage.goto(`/table/${tableId}`);
  await alicePage.locator('.strip').click();
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat).toBeVisible();

  // Both in-character lines render inside ONE scene container — the plan
  // itself, not a location/time turn-scene.
  const header = chat.locator('.scene-header');
  await expect(header).toBeVisible();
  await expect(header).toContainText('Host Festivity');
  await expect(header).toBeDisabled(); // live

  await expect(chat.getByText(hostLine)).toBeVisible();
  await expect(chat.getByText(guestLine)).toBeVisible();
  await expect(chat.locator('.message.in-character', { hasText: hostLine })).toBeVisible();
  await expect(chat.locator('.message.in-character', { hasText: guestLine })).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});
