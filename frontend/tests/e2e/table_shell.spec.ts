import { test, expect, type Browser, type Page } from '@playwright/test';
import { cleanupGameAfterEach, openPane, tableTitle } from './helpers';
import { E2E } from './baseUrl';
import {
  seedWarGame, runScene, declareWar, revealFaces, closeContextsAfterEach,
} from './helpers/make_war';

// The table page's frame: a title bar and a player row over three panes, Public
// Record, Play and Chat. A wide window shows them as columns and a narrow one
// swaps them with tabs, so the same table is checked at the three widths the
// layout has:
//
//   375   one pane at a time, three tabs at the foot
//   800   Chat docked on the right; the left column has two tabs (744 and up)
//   1280  three columns, no tabs (1112 and up)
//
// The marks are the other half of the contract: a gold clock on Play only when
// it is YOUR move, and a pale dot for "new since you looked" on Chat and on the
// Public Record (whenever Laws or Rumors has one). Never a number.

const PHONE = { width: 375, height: 812 };
const TWO = { width: 800, height: 1000 };
const THREE = { width: 1280, height: 800 };

const track = cleanupGameAfterEach();
closeContextsAfterEach();

/** Seeds a main-event table (alice is on the move, with one sample law and one
 *  rumor she has not seen) and opens it as alice at the given size. */
async function openTable(browser: Browser, viewport: { width: number; height: number }) {
  const ctx = await browser.newContext({ baseURL: E2E, viewport });
  const login = await ctx.request.post('/api/dev/login?username=alice');
  expect(login.ok()).toBeTruthy();
  const seed = await ctx.request.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seed.ok()).toBeTruthy();
  const { game_id: tableId } = await seed.json();
  track(tableId);
  const page = await ctx.newPage();
  await page.goto(`/table/${tableId}`);
  await expect(tableTitle(page)).toHaveText(/^Row \d+/);
  return { ctx, page, tableId: tableId as number };
}

const paneTabs = (page: Page) => page.getByRole('navigation', { name: 'Panes' });
const recordChips = (page: Page) => page.getByRole('navigation', { name: 'Public Record' });
const chatColumn = (page: Page) => page.getByRole('complementary', { name: 'Chat' });
const recordColumn = (page: Page) => page.getByRole('complementary', { name: 'Public Record' });
const playColumn = (page: Page) => page.getByRole('region', { name: 'Play' });

test('phone: opens on Play, with Home, the title and Help above three tabs', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, PHONE);

  // The header is Home, the title and Help: no phase badge, and the buttons for
  // Tones, Laws, Rumors and War are gone from it.
  await expect(page.getByRole('link', { name: 'Home' })).toBeVisible();
  await expect(page.getByRole('button', { name: /How to play/ })).toBeVisible();
  await expect(page.locator('.phase-badge')).toHaveCount(0);
  for (const name of ['Open tones', 'Open laws', 'Open rumors', 'Open wars']) {
    await expect(page.getByRole('button', { name })).toHaveCount(0);
  }

  // The table always opens on Play.
  const tabs = paneTabs(page).getByRole('button');
  await expect(tabs).toHaveCount(3);
  await expect(tabs.nth(0)).toContainText('Public Record');
  await expect(tabs.nth(1)).toContainText('Play');
  await expect(tabs.nth(2)).toContainText('Chat');
  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Play');
  await expect(playColumn(page)).toBeVisible();
  await expect(chatColumn(page)).toBeHidden();
  await expect(recordColumn(page)).toBeHidden();
  // The record's chips only show while the Public Record does.
  await expect(recordChips(page)).toHaveCount(0);

  // One pane at a time; the tab bar follows the pane.
  await openPane(page, 'Chat');
  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Chat');
  // The bar is redrawn in the Chat pane's foot; focus follows the tab you chose.
  await expect(paneTabs(page).getByRole('button', { name: /Chat/ })).toBeFocused();
  await expect(chatColumn(page).getByPlaceholder('Write a message…')).toBeVisible();
  await expect(playColumn(page)).toBeHidden();

  await openPane(page, 'Public Record');
  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Public Record');
  await expect(recordChips(page).getByRole('button')).toHaveCount(3);
  await expect(recordChips(page).locator('[aria-current="page"]')).toContainText('Events');

  // Which pane you picked is only kept for the visit: a reload starts on Play.
  await page.reload();
  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Play');

  await ctx.close();
});

test('the clock sits on whoever the game waits on, gold on you, and marks Play only for you', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, PHONE);

  // alice is on the move in the seeded table: her own chip says so, bob's does
  // not, and so does Play's tab.
  const alice = page.getByRole('button', { name: /View alice's retinue/ });
  const bob = page.getByRole('button', { name: /View bob's retinue/ });
  await expect(alice).toHaveAccessibleName(/your move/);
  await expect(bob).not.toHaveAccessibleName(/move/);
  await expect(paneTabs(page).getByRole('img', { name: 'your move' })).toBeVisible();

  // The mark on a chip is the clock in "you" gold; nobody's chip has a number.
  await expect(alice.locator('[data-tone="you"]')).toBeVisible();
  await expect(alice).not.toContainText(/\d/);
  await expect(bob).not.toContainText(/\d/);

  // Each player is a shape beside a name, and your own chip reads "You".
  await expect(alice).toContainText('You');
  await expect(alice.locator('[data-shape]')).toHaveCount(1);
  await expect(bob).toContainText('bob');
  await expect(bob.locator('[data-shape]')).toHaveCount(1);

  // Bob's view: the game is waiting on alice, so there is no clock on Play.
  const bobCtx = await browser.newContext({ baseURL: E2E, viewport: PHONE });
  await bobCtx.request.post('/api/dev/login?username=bob');
  const bobPage = await bobCtx.newPage();
  await bobPage.goto(page.url());
  await expect(tableTitle(bobPage)).toHaveText(/^Row \d+/);
  await expect(bobPage.getByRole('button', { name: /View alice's retinue/ })).toHaveAccessibleName(/their move/);
  await expect(paneTabs(bobPage).getByRole('img', { name: 'your move' })).toHaveCount(0);

  await bobCtx.close();
  await ctx.close();
});

test('Laws and Rumors: the dot lights the chip and the tab above it, and opening a list clears it for good', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, PHONE);

  // The seeded law and rumor are new to alice, and the Public Record tab says so
  // from any pane.
  const recordTab = paneTabs(page).getByRole('button', { name: /Public Record/ });
  await expect(recordTab.getByRole('img', { name: 'new laws or rumors' })).toBeVisible();

  await openPane(page, 'Public Record');
  const chips = recordChips(page);
  await expect(chips.getByRole('button', { name: /Laws/ }).getByRole('img', { name: 'new laws' })).toBeVisible();
  await expect(chips.getByRole('button', { name: /Rumors/ }).getByRole('img', { name: 'new rumors' })).toBeVisible();
  await expect(chips.getByRole('button', { name: /Events/ }).getByRole('img')).toHaveCount(0);

  // Opening Laws marks that list seen; Rumors keeps its dot, and so does the tab
  // above them while either does (the tab carries the dot for either list).
  await chips.getByRole('button', { name: /Laws/ }).click();
  await expect(page.locator('.laws-rumors .lr-item').first()).toBeVisible();
  await expect(chips.getByRole('button', { name: /Laws/ }).getByRole('img')).toHaveCount(0);
  await expect(chips.getByRole('button', { name: /Rumors/ }).getByRole('img', { name: 'new rumors' })).toBeVisible();
  await expect(recordTab.getByRole('img', { name: 'new laws or rumors' })).toBeVisible();

  await chips.getByRole('button', { name: /Rumors/ }).click();
  await expect(chips.getByRole('button', { name: /Rumors/ }).getByRole('img')).toHaveCount(0);
  await expect(recordTab.getByRole('img')).toHaveCount(0);

  // It is the server that remembers, not this tab.
  await page.reload();
  await expect(paneTabs(page).getByRole('button', { name: /Public Record/ })).toBeVisible();
  await expect(paneTabs(page).getByRole('img', { name: 'new laws or rumors' })).toHaveCount(0);

  await ctx.close();
});

test('Laws: an edit is new to everyone but the editor, live', async ({ browser }) => {
  const { ctx, page, tableId } = await openTable(browser, PHONE);

  // bob has seen the law (he opened it), so it carries no dot for him.
  const bobCtx = await browser.newContext({ baseURL: E2E, viewport: PHONE });
  await bobCtx.request.post('/api/dev/login?username=bob');
  const bobPage = await bobCtx.newPage();
  await bobPage.goto(`/table/${tableId}`);
  await expect(tableTitle(bobPage)).toHaveText(/^Row \d+/);
  await openPane(bobPage, 'Public Record');
  await recordChips(bobPage).getByRole('button', { name: /Laws/ }).click();
  await expect(recordChips(bobPage).getByRole('button', { name: /Laws/ }).getByRole('img')).toHaveCount(0);
  // He goes back to Events, so a change to the list is not on his screen when it
  // happens (a list he is looking at is seen as it changes).
  await recordChips(bobPage).getByRole('button', { name: /Events/ }).click();

  // alice is the law's signatory. She has not looked at the Laws list either,
  // and the seeded law is still new to her.
  await openPane(page, 'Public Record');
  const aliceLaws = recordChips(page).getByRole('button', { name: /Laws/ });
  await expect(aliceLaws.getByRole('img', { name: 'new laws' })).toBeVisible();

  // She edits it. For bob an edit counts as new, and the dot comes back on his
  // open page with no reload.
  const { laws } = await (await ctx.request.get(`/api/tables/${tableId}/laws`)).json();
  const edit = await ctx.request.patch(`/api/laws/${laws[0].id}`, { data: { text: 'No private armies in the capital.' } });
  expect(edit.ok()).toBeTruthy();
  await expect(recordChips(bobPage).getByRole('button', { name: /Laws/ }).getByRole('img', { name: 'new laws' })).toBeVisible();

  // Your own writing is not new to you: the edit caught alice's marker up with
  // it, so her chip has lost the dot she had, without her opening the list.
  await expect(aliceLaws.getByRole('img')).toHaveCount(0, { timeout: 10_000 });
  // The rumor is still new to her, and the tab above keeps its dot for it.
  await expect(recordChips(page).getByRole('button', { name: /Rumors/ }).getByRole('img', { name: 'new rumors' })).toBeVisible();

  await bobCtx.close();
  await ctx.close();
});

test('phone: a plan line in the Public Record opens Chat at that point', async ({ browser }) => {
  const { ctx, page, tableId } = await openTable(browser, PHONE);
  // The feed has to have something to land on: a prepared plan writes its
  // plan.prepared post, which is where its line in the record opens Chat. (A
  // row's number does nothing, so a plan or a scene is the only way in.)
  const prep = await ctx.request.post(`/api/tables/${tableId}/prepare-plan`, {
    data: { plan_type: 'host_festivity', preparation_notes: 'a grand ball' },
  });
  expect(prep.ok(), `prepare-plan failed: ${await prep.text()}`).toBeTruthy();
  await page.reload();
  await expect(tableTitle(page)).toHaveText(/^Row \d+/);

  await openPane(page, 'Public Record');
  await page.getByRole('button', { name: /^Host Festivity by alice/ }).click();

  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Chat');
  await expect(chatColumn(page).getByPlaceholder('Write a message…')).toBeVisible();
  await expect(chatColumn(page).getByText('alice prepared Host Festivity')).toBeVisible();

  await ctx.close();
});

test('two columns: Chat is docked, the left column has two tabs, and Help covers only that column', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, TWO);

  // Chat needs no tab: it is always there.
  await expect(chatColumn(page)).toBeVisible();
  await expect(paneTabs(page).getByRole('button')).toHaveCount(2);
  await expect(paneTabs(page).getByRole('button', { name: 'Chat' })).toHaveCount(0);
  await expect(paneTabs(page).locator('[aria-current="page"]')).toContainText('Play');
  await expect(playColumn(page)).toBeVisible();

  // Only Chat's header starts with its name; the tabs name the left column.
  await expect(chatColumn(page).getByRole('heading', { name: 'Chat' })).toBeVisible();
  await expect(playColumn(page).getByRole('heading', { name: 'Play' })).toHaveCount(0);

  // The left column switches between Public Record and Play.
  await openPane(page, 'Public Record');
  await expect(recordColumn(page)).toBeVisible();
  await expect(playColumn(page)).toBeHidden();
  await expect(recordChips(page).getByRole('button')).toHaveCount(3);
  await openPane(page, 'Play');

  // A sheet from the title bar rises over the left column only: the chat stays
  // readable, and live, beside it.
  await page.getByRole('button', { name: /How to play/ }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  const sheet = (await dialog.boundingBox())!;
  const chat = (await chatColumn(page).boundingBox())!;
  expect(sheet.x + sheet.width, 'the sheet stays out of the chat column').toBeLessThanOrEqual(chat.x + 1);
  await chatColumn(page).getByPlaceholder('Write a message…').click({ trial: true });

  await ctx.close();
});

test('three columns: every pane is a column with its name, and a sheet covers one column', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, THREE);

  // No tab bar: all three panes are on screen, each starting with its name.
  await expect(paneTabs(page)).toHaveCount(0);
  await expect(recordColumn(page)).toBeVisible();
  await expect(playColumn(page)).toBeVisible();
  await expect(chatColumn(page)).toBeVisible();
  await expect(recordColumn(page).getByRole('heading', { name: 'Public Record' })).toBeVisible();
  await expect(playColumn(page).getByRole('heading', { name: 'Play' })).toBeVisible();
  await expect(chatColumn(page).getByRole('heading', { name: 'Chat' })).toBeVisible();

  // The column names carry the same marks as the tabs do on a phone: the
  // gold clock on Play (it is alice's move) and the dot on the Public Record.
  await expect(playColumn(page).getByRole('img', { name: 'your move' })).toBeVisible();
  await expect(recordColumn(page).getByRole('img', { name: 'new laws or rumors' })).toBeVisible();

  // The record's chips stay at the foot of its column.
  await expect(recordChips(page).getByRole('button')).toHaveCount(3);

  // Tones rise over Chat only.
  await page.getByRole('button', { name: /^Tones/ }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog).toBeVisible();
  const sheet = (await dialog.boundingBox())!;
  const chat = (await chatColumn(page).boundingBox())!;
  const play = (await playColumn(page).boundingBox())!;
  expect(sheet.x, 'Tones starts at the chat column').toBeGreaterThanOrEqual(chat.x - 1);
  expect(sheet.x + sheet.width).toBeLessThanOrEqual(chat.x + chat.width + 1);
  expect(sheet.x, 'and leaves Play alone').toBeGreaterThanOrEqual(play.x + play.width);
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);

  // A sheet from the player row covers Play.
  await page.getByRole('button', { name: /View bob's retinue/ }).click();
  await expect(dialog).toBeVisible();
  const retinue = (await dialog.boundingBox())!;
  expect(retinue.x).toBeGreaterThanOrEqual(play.x - 1);
  expect(retinue.x + retinue.width).toBeLessThanOrEqual(play.x + play.width + 1);

  await ctx.close();
});

test('phone: a sheet opens even when the pane it belongs to is another tab', async ({ browser }) => {
  // The lobby's "Adjust the tone of the game" row is in Play, but Tones belong to
  // Chat, which is hidden while Play shows. The sheet must still appear.
  const ctx = await browser.newContext({ baseURL: E2E, viewport: PHONE });
  await ctx.request.post('/api/dev/login?username=alice');
  const { game } = await (await ctx.request.post('/api/tables')).json();
  track(game.id);
  const page = await ctx.newPage();
  await page.goto(`/table/${game.id}`);
  await expect(tableTitle(page)).toHaveText('Lobby');
  await expect(chatColumn(page)).toBeHidden();

  await page.getByRole('button', { name: /Adjust the tone of the game/ }).click();
  const dialog = page.getByRole('dialog');
  await expect(dialog.getByRole('heading', { name: 'Tones' })).toBeVisible();
  // One column on a phone, so the sheet is the width of the screen.
  expect((await dialog.boundingBox())!.width).toBeGreaterThanOrEqual(PHONE.width - 1);

  await ctx.close();
});

// The War button left the header with Tones, Laws and Rumors. Each live war has a
// one-line band at the top of Play, which opens the war's panel as a sheet over Play.
for (const [name, viewport] of [['phone', PHONE], ['three columns', THREE]] as const) {
  test(`${name}: each war has a band at the top of Play, and it opens the war's panel over Play`, async ({ browser }) => {
    const g = await seedWarGame(browser, viewport);
    track(g.gameID);
    await runScene(g);
    await declareWar(g, ['bob']);
    await revealFaces(g, { alice: 2, bob: 3 });

    const page = await g.ctx.alice.newPage();
    await page.goto(`/table/${g.gameID}`);
    await expect(tableTitle(page)).toHaveText(/^Row \d+/);

    // The old header button is gone; the band is inside Play, above the plan.
    const play = playColumn(page);
    const wars = play.getByRole('button', { name: /alice’s war on bob/ });
    await expect(wars).toBeVisible();
    await expect(page.getByRole('banner').getByRole('button', { name: /war on/ })).toHaveCount(0);

    await wars.click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByRole('heading', { name: 'alice’s war on bob' })).toBeVisible();

    const sheet = (await dialog.boundingBox())!;
    if (viewport === THREE) {
      // A column's sheet: inside Play's box, with the other two untouched.
      const box = (await play.boundingBox())!;
      expect(sheet.x).toBeGreaterThanOrEqual(box.x - 1);
      expect(sheet.x + sheet.width).toBeLessThanOrEqual(box.x + box.width + 1);
      await chatColumn(page).getByPlaceholder('Write a message…').click({ trial: true });
    } else {
      // One column on a phone, so the sheet is the width of the screen.
      expect(sheet.width).toBeGreaterThanOrEqual(viewport.width - 1);
    }
  });
}
