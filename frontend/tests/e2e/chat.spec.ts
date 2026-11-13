import { test, expect, request as pwRequest, type APIRequestContext } from '@playwright/test';
import { cleanupGameAfterEach, openPane } from './helpers';
import { E2E } from './baseUrl';

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

const track = cleanupGameAfterEach();

test('alice sends a chat message; bob sees it live', async ({ browser, playwright }) => {
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });

  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  // Alice creates the table via API; capture the id and join code.
  const createRes = await aliceCtx.request.post('/api/tables');
  expect(createRes.ok()).toBeTruthy();
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
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

test('catch-up: messages sent while away light the Chat tab and show the "N new" divider', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });

  // Bob posts while alice is "away" — she hasn't opened the table page yet,
  // so her last_read_post_id is still 0 and every one of these is unread.
  const awayMessage = `away-msg-${Date.now()}`;
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: awayMessage } });

  // Mobile viewport: below 744px the chat is a tab, and the tab is where the
  // pale "new" dot shows. From two columns up the chat is docked and always on
  // screen, so there is no closed state to put a dot on. See feedback_mobile_first.
  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 });
  await alicePage.goto(`/table/${tableId}`);

  const chatTab = alicePage.getByRole('navigation', { name: 'Panes' }).getByRole('button', { name: 'Chat' });
  await expect(chatTab.getByRole('img', { name: 'new messages' })).toBeVisible();
  // A dot, never a count.
  await expect(chatTab).not.toContainText(/\d/);

  // Opening the pane reveals the unread divider right above the message that
  // arrived while she was away. The count lives in the divider ("1 new"), not
  // on the tab.
  await chatTab.click();
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat.getByRole('separator', { name: '1 new message' })).toBeVisible();
  await expect(chat.getByText('1 new', { exact: true })).toBeVisible();
  await expect(chat.getByText(awayMessage)).toBeVisible();

  // Having had the new messages on screen, the dot clears.
  await expect(chatTab.getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });

  await aliceCtx.close();
  await bobCtx.close();
});

test('scroll-up pagination: older posts outside the initial window load on demand', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
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
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
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
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId, players } = await seedRes.json();
  const bobPlayerID: number = players[1].id;
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
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

  // Bob takes it (dev shortcut — players have no raw take route) — asset.taken
  // is Default-tier and always visible.
  const takeRes = await bobCtx.request.post('/api/dev/take-asset', {
    data: { asset_id: assetId, player_id: bobPlayerID },
  });
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
  await aliceChat.getByRole('switch', { name: 'Hide bookkeeping' }).uncheck();
  await expect(feed.getByText('renamed Original Name', { exact: false })).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});

test('jumping to a plan outside the loaded window enters history mode; Return to now exits it', async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: gameID } = await seedRes.json();
  track(gameID);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice'); // alice is the seed's facilitator + focus player
  await devLogin(bobCtx.request, 'bob');

  // The jump the Public Record offers is a plan line (row numbers are not
  // tappable). A prepared plan has a plan.prepared post to land on, written now,
  // so the filler below pushes it out of the window.
  const prepRes = await aliceCtx.request.post(`/api/tables/${gameID}/prepare-plan`, {
    data: { plan_type: 'host_festivity', preparation_notes: 'a grand ball' },
  });
  expect(prepRes.ok(), `prepare-plan failed: ${await prepRes.text()}`).toBeTruthy();

  // Same read-marker trick as the pagination test: push the plan.prepared
  // anchor far enough behind alice's marker that a fresh load's window
  // starts after it.
  await sendMessages(bobCtx.request, gameID, 'filler', 40);
  const markerID = await newestPostID(aliceCtx.request, gameID);
  await aliceCtx.request.put(`/api/tables/${gameID}/read-marker`, {
    data: { last_read_post_id: markerID },
  });
  await sendMessages(bobCtx.request, gameID, 'newmsg', 120);

  // Default Playwright viewport (1280x720) is three columns (from 1112px), so
  // the Public Record is a column beside the chat and needs no tap to open.
  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${gameID}`);
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await expect(aliceChat).toBeVisible();
  await expect(aliceChat.getByText('alice prepared Host Festivity')).toHaveCount(0);

  await alicePage.getByRole('complementary', { name: 'Public Record' })
    .getByRole('button', { name: /^Host Festivity by alice/ }).click();

  await expect(aliceChat.locator('.return-to-now')).toBeVisible();
  await expect(aliceChat.getByText('alice prepared Host Festivity')).toBeVisible();

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
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId, players } = await seedRes.json();
  track(tableId);
  const alicePlayerID: number = players[0].id;
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
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
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat).toBeVisible();

  // Live scene: expanded, header shows the banner and says it is live, and
  // can't collapse. (No message count: totals belong in lists, not in chrome.)
  const header = chat.locator('.scene-header');
  await expect(header).toBeVisible();
  await expect(header).toContainText('The Mill');
  await expect(header).toContainText('Live');
  await expect(header).not.toContainText('messages');
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
  await openPane(alicePage, 'Chat');
  const chat2 = alicePage.locator('[aria-label="Chat"]');
  const header2 = chat2.locator('.scene-header');
  await expect(header2).toBeVisible();
  await expect(header2).toContainText('The Mill');
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
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');

  const sceneRes = await aliceCtx.request.post(`/api/tables/${tableId}/scenes`, {
    data: { location_custom: 'The Docks', time_elapsed: 'moments', present_peer_ids: [] },
  });
  expect(sceneRes.ok(), `create scene failed: ${await sceneRes.text()}`).toBeTruthy();
  await aliceCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: 'quiet down at the docks' } });

  // Default Playwright viewport (1280x720) is three columns; the chat is
  // docked on the right, with no tab to tap.
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
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId, players } = await seedRes.json();
  track(tableId);
  const alicePlayerID: number = players[0].id;
  const bobPlayerID: number = players[1].id;
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
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
  await openPane(alicePage, 'Chat');
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

/**
 * Takes a seeded Seek Answers plan from pending to resolved over the API, the
 * way seek_answers.spec.ts does (the plan flow isn't the subject here; how the
 * feed draws it is). Every pick is "declare a truth", so nothing is left for a
 * panel to do. A make is forced; the dice are genuinely random otherwise.
 */
async function resolveSeekAnswers(api: APIRequestContext, planID: number): Promise<void> {
  const resolve = await api.post(`/api/plans/${planID}/resolve`);
  expect(resolve.ok(), `resolve failed: ${await resolve.text()}`).toBeTruthy();
  const cast = await api.post(`/api/plans/${planID}/seek-cast-roll`, {
    data: { narration: 'I read the harbour ledgers by candlelight.' },
  });
  expect(cast.ok(), `cast failed: ${await cast.text()}`).toBeTruthy();
  const { roll } = (await cast.json()) as { roll: { id: number } };
  const forced = await api.post('/api/dev/force-roll', { data: { roll_id: roll.id, outcome: 'make' } });
  expect(forced.ok(), `force-roll failed: ${await forced.text()}`).toBeTruthy();
  const { roll: forcedRoll } = (await forced.json()) as { roll: { result: number } };
  const result = forcedRoll.result;
  const choices = Array(result).fill('declare_truth');
  const choice = await api.post(`/api/plans/${planID}/make-choice`, { data: { result: 'make', choices } });
  expect(choice.ok(), `make-choice failed: ${await choice.text()}`).toBeTruthy();
  for (let i = 0; i < result; i++) {
    const truth = await api.post(`/api/plans/${planID}/declare-truth`, {
      data: { text: `A truth the ledgers kept (${i + 1}).` },
    });
    expect(truth.ok(), `declare-truth failed: ${await truth.text()}`).toBeTruthy();
  }
}

// A plan's resolution in the feed is one header row (the plan's mark, its name, the
// preparer's shape, a status word, a caret) with its lines behind a rule. The
// header is the only thing that is pressed, and it folds.
test("a plan's resolution is a header row that folds: name, preparer's shape, status", async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: {
      phase: 'main_event',
      players: ['alice', 'bob'],
      current_row: 5,
      plans: [{ preparer_idx: 0, plan_type: 'seek_answers', category: 'knowledge', row: 5, row_order: 0 }],
    },
  });
  expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  const plansRes = await aliceCtx.request.get(`/api/tables/${tableId}/plans`);
  const { plans } = (await plansRes.json()) as { plans: { id: number; plan_type: string }[] };
  const planID = plans.find((p) => p.plan_type === 'seek_answers')!.id;
  await resolveSeekAnswers(aliceCtx.request, planID);

  // Everything has been seen, so the resolved span starts folded.
  const markerID = await newestPostID(aliceCtx.request, tableId);
  const mark = await aliceCtx.request.put(`/api/tables/${tableId}/read-marker`, {
    data: { last_read_post_id: markerID },
  });
  expect(mark.ok()).toBeTruthy();

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 375, height: 812 }); // mobile-first
  await alicePage.goto(`/table/${tableId}`);
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');

  const header = chat.locator(`.plan-header[data-plan-id="${planID}"]`);
  await expect(header).toBeVisible();
  // Name, then the preparer's shape (named for a screen reader), then the status.
  await expect(header).toContainText('Seek Answers');
  await expect(header.getByRole('img', { name: 'alice' })).toBeVisible();
  await expect(header.locator('.status-word')).toHaveText('Make');
  // The header is a tappable row, never a card: 44px tall at least, no box.
  const box = await header.boundingBox();
  expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
  expect(await header.evaluate((el) => getComputedStyle(el).borderTopWidth)).toBe('0px');
  expect(await header.evaluate((el) => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');

  // Folded: the lines inside are not rendered, but the sentence behind the status
  // word stays, so a shut span still says how the plan landed.
  await expect(header).toHaveAttribute('aria-expanded', 'false');
  const group = chat
    .locator('.plan-group')
    .filter({ has: alicePage.locator(`.plan-header[data-plan-id="${planID}"]`) });
  await expect(group.getByText('succeeded', { exact: false })).toBeVisible();
  await expect(group.getByText('rolled', { exact: false })).toHaveCount(0);

  // Tapping the header unfolds it: the lines come back, behind a thin rule.
  await header.click();
  await expect(header).toHaveAttribute('aria-expanded', 'true');
  await expect(group.getByText('rolled', { exact: false }).first()).toBeVisible();
  const body = group.locator('.span-body');
  expect(await body.evaluate((el) => getComputedStyle(el).borderLeftWidth)).toBe('1px');
  expect(await body.evaluate((el) => getComputedStyle(el).backgroundColor)).toBe('rgba(0, 0, 0, 0)');

  await aliceCtx.close();
});

// The dot on Chat clears once the new messages have been on screen: on the tab
// on a phone (covered above), and on the column's name from three columns up,
// where there is no tab and the chat is always visible.
test('the dot on the Chat column name clears once new messages have been seen', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');
  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: 'waiting for you' } });

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 1280, height: 800 }); // three columns
  await alicePage.goto(`/table/${tableId}`);
  const chat = alicePage.getByRole('complementary', { name: 'Chat' });
  // The column name carries the same pale dot as the tab would, and no number.
  const name = chat.locator('.pane-name');
  await expect(name).toContainText('Chat');
  await expect(chat.getByText('waiting for you')).toBeVisible();
  await expect(name).not.toContainText(/\d/);
  // It was on screen at the bottom of the feed, so the dot goes by itself.
  await expect(name.getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });

  // A message that arrives while the column is on screen and at the bottom is
  // seen at once, so the dot does not linger.
  await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: 'and another' } });
  await expect(chat.getByText('and another')).toBeVisible();
  await expect(name.getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });

  await aliceCtx.close();
  await bobCtx.close();
});

// Leaving the Chat pane must never mark what the reader has not seen as read. A tab
// switch is one tap, so it is easy to hit: parked on the "N new" divider with a long
// unread run below the fold, tapping Play used to flush a report of the NEWEST post
// and clear the dot for messages nobody had looked at.
test('switching tabs does not mark unread chat as read; reaching the bottom does', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  await devLogin(bobCtx.request, 'bob');
  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });
  // Enough long messages that the unread run is several screens tall at 375px.
  const filler = 'a long enough line to wrap across a phone screen and take up real room ';
  for (let i = 0; i < 30; i++) {
    const res = await bobCtx.request.post(`/api/tables/${tableId}/posts`, { data: { body: `unread-${i}: ${filler}${filler}` } });
    expect(res.ok()).toBeTruthy();
  }
  const markerOf = async () => {
    const res = await aliceCtx.request.get(`/api/tables/${tableId}/posts`);
    return ((await res.json()) as { last_read_post_id: number }).last_read_post_id;
  };
  expect(await markerOf()).toBe(0);

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 375, height: 812 });
  await alicePage.goto(`/table/${tableId}`);
  const tabs = alicePage.getByRole('navigation', { name: 'Panes' });
  const chatTab = tabs.getByRole('button', { name: 'Chat' });
  await expect(chatTab.getByRole('img', { name: 'new messages' })).toBeVisible();
  await chatTab.click();
  const chat = alicePage.locator('[aria-label="Chat"]');
  const feed = chat.locator('.feed');
  const divider = chat.getByRole('separator', { name: /new messages$/ });
  await expect(divider).toBeVisible();

  // Arriving lands on the divider, not at the bottom: the reader starts where the
  // new messages start and the newest ones are off screen.
  await expect(divider).toBeInViewport();

  // Parked at the divider, a long way from the bottom. Wait out the debounce: nothing
  // may be reported, because nothing past the fold was on screen.
  const away = await feed.evaluate((el) => el.scrollHeight - el.scrollTop - el.clientHeight);
  expect(away, 'the unread run must be longer than the screen for this test to mean anything').toBeGreaterThan(400);
  await alicePage.waitForTimeout(3000);
  expect(await markerOf()).toBe(0);

  // Tap Play. This is the regression: the pane going away flushed the newest post.
  await tabs.getByRole('button', { name: 'Play' }).click();
  await alicePage.waitForTimeout(3000);
  expect(await markerOf(), 'a tab switch marked unread messages as read').toBe(0);
  await expect(chatTab.getByRole('img', { name: 'new messages' })).toBeVisible();

  // Back in Chat, reaching the bottom is what clears it.
  await chatTab.click();
  await feed.evaluate((el) => { el.scrollTop = el.scrollHeight; });
  await expect.poll(markerOf, { timeout: 10_000 }).toBeGreaterThan(0);
  await expect(chatTab.getByRole('img', { name: 'new messages' })).toHaveCount(0, { timeout: 10_000 });

  await aliceCtx.close();
  await bobCtx.close();
});

// "Speaking as" is a popup, so it closes like one: choosing, Escape, or a tap outside.
test('the "Speaking as" list closes on Escape and on a tap outside', async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();
  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  // A scene is what gives alice a character to speak as.
  const sceneRes = await aliceCtx.request.post(`/api/tables/${tableId}/scenes`, {
    data: { location_custom: 'The Mill', time_elapsed: 'days', present_peer_ids: [] },
  });
  expect(sceneRes.ok(), `create scene failed: ${await sceneRes.text()}`).toBeTruthy();

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 375, height: 812 });
  await alicePage.goto(`/table/${tableId}`);
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');
  const picker = chat.getByRole('button', { name: /Speaking as/ });
  const list = chat.getByRole('listbox');
  await expect(picker).toBeVisible();

  await picker.click();
  await expect(list).toBeVisible();
  await alicePage.keyboard.press('Escape');
  await expect(list).toHaveCount(0);
  // Focus went back to the control, not the message box, so no soft keyboard.
  await expect(picker).toBeFocused();
  await expect(chat.getByPlaceholder('Write a message…')).not.toBeFocused();

  await picker.click();
  await expect(list).toBeVisible();
  await chat.locator('.feed').click({ position: { x: 20, y: 20 } });
  await expect(list).toHaveCount(0);

  // Tapping the control itself still toggles it, and a choice still closes it.
  await picker.click();
  await expect(list).toBeVisible();
  await picker.click();
  await expect(list).toHaveCount(0);
  await picker.click();
  await list.getByRole('option').first().click();
  await expect(list).toHaveCount(0);

  await aliceCtx.close();
});

// The composer. Send is greyed out until there is something to send, and the
// keyboard opens only when the box is tapped, never because the chat arrived.
test('Send is greyed out until there is text, and the keyboard never opens on its own', async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');
  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 375, height: 812 }); // mobile-first
  await alicePage.goto(`/table/${tableId}`);
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');
  const box = chat.getByPlaceholder('Write a message…');
  const send = chat.getByRole('button', { name: 'Send' });
  await expect(box).toBeVisible();

  // Arriving on the chat opens no keyboard: nothing in it has focus.
  await expect(box).not.toBeFocused();

  // Greyed out: the house button's disabled look (raised-twice fill, faint text,
  // no border), not the main button's deep gold, and not faded by opacity.
  await expect(send).toBeDisabled();
  const greyed = await send.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { opacity: cs.opacity, border: cs.borderTopColor, bg: cs.backgroundColor };
  });
  expect(greyed.opacity).toBe('1');
  await box.fill('   ');
  await expect(send).toBeDisabled(); // whitespace is not a message
  await box.fill('hello');
  await expect(send).toBeEnabled();
  const live = await send.evaluate((el) => getComputedStyle(el).backgroundColor);
  expect(live).not.toBe(greyed.bg); // the main button is a different fill than the greyed one

  // The box is a 16px input at least 44px tall: iOS zooms the page for anything smaller.
  const size = await box.evaluate((el) => {
    const cs = getComputedStyle(el);
    return { font: cs.fontSize, height: el.getBoundingClientRect().height };
  });
  expect(size.font).toBe('16px');
  expect(size.height).toBeGreaterThanOrEqual(44);

  await aliceCtx.close();
});

// Composer focus retention. Sending must not blur the compose box — people
// fire off several messages in a row, and on mobile a blur closes the soft
// keyboard that a later programmatic .focus() can't reopen (it happens past
// the user gesture). Two specific regressions are pinned here: `disabled` on
// the textarea while the POST is in flight (disabling a focused element
// blurs it), and the Send button taking focus on click and then going
// disabled as the box empties (dropping focus to <body>).
test('composer keeps focus after sending, via Enter and via the Send button', async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 }); // mobile-first
  await alicePage.goto(`/table/${tableId}`);
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat).toBeVisible();

  const box = chat.getByPlaceholder('Write a message…');
  await box.click();
  await expect(box).toBeFocused();

  await box.fill('first message');
  await box.press('Enter');
  await expect(chat.getByText('first message')).toBeVisible();
  await expect(box).toHaveValue('');
  await expect(box).toBeFocused();

  await box.fill('second message');
  await chat.getByRole('button', { name: 'Send' }).click();
  await expect(chat.getByText('second message')).toBeVisible();
  await expect(box).toHaveValue('');
  await expect(box).toBeFocused();

  await aliceCtx.close();
});

// The flip side of never disabling the textarea: send() clears the box up
// front so a message typed while the POST is in flight isn't eaten by the
// clear-on-success. That makes the failure path load-bearing — the text has
// to come back, without trampling a draft the user has already started.
test('a failed send restores the text, but never clobbers a draft typed while in flight', async ({ browser }) => {
  const reset = await pwRequest.newContext({ baseURL: E2E });
  const seedRes = await reset.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  const { game_id: tableId } = await seedRes.json();
  track(tableId);
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(aliceCtx.request, 'alice');

  const alicePage = await aliceCtx.newPage();
  await alicePage.setViewportSize({ width: 390, height: 844 }); // mobile-first
  // Fail every chat POST, slowly enough that there's a real in-flight window
  // to type into.
  await alicePage.route('**/api/tables/*/posts', async (route) => {
    if (route.request().method() !== 'POST') return route.continue();
    await new Promise((r) => setTimeout(r, 1500));
    await route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'simulated send failure' }),
    });
  });

  await alicePage.goto(`/table/${tableId}`);
  await openPane(alicePage, 'Chat');
  const chat = alicePage.locator('[aria-label="Chat"]');
  await expect(chat).toBeVisible();

  const box = chat.getByPlaceholder('Write a message…');

  // Nothing typed since: the failed text comes straight back, ready to retry.
  await box.fill('doomed message');
  await box.press('Enter');
  await expect(chat.locator('.error-text')).toContainText('simulated send failure');
  await expect(box).toHaveValue('doomed message');
  await expect(box).toBeFocused();

  // A new draft started during the in-flight window wins; the failed message
  // is surfaced in the error line rather than pasted over the draft.
  await box.fill('another doomed message');
  await box.press('Enter');
  await box.fill('a draft typed while in flight');
  await expect(chat.locator('.error-text')).toContainText('Unsent message: another doomed message');
  await expect(box).toHaveValue('a draft typed while in flight');
  await expect(box).toBeFocused();

  await aliceCtx.close();
});
