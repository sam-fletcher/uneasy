import { test, expect, type Browser, type Page } from '@playwright/test';
import { cleanupGameAfterEach, openPane, openRecordTab, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// The Public Record pane's contents: the Events list (a line per plan or scene,
// the status word beside it, nothing else to tap) and the Laws and Rumors lists
// (plain entries, the "new" dot, Edit on your own). The pane's frame (tabs,
// chips, the dot on the tab) is table_shell.spec.ts. Everything here is checked
// at a phone's width first.

const PHONE = { width: 375, height: 812 };

const track = cleanupGameAfterEach();

type Seed = Record<string, unknown>;

/** Seeds a table, logs `as` in and opens it at a phone's width. */
async function openTable(browser: Browser, seed: Seed, as = 'alice') {
  const ctx = await browser.newContext({ baseURL: E2E, viewport: PHONE });
  expect((await ctx.request.post(`/api/dev/login?username=${as}`)).ok()).toBeTruthy();
  const res = await ctx.request.post('/api/dev/seed', { data: seed });
  expect(res.ok(), `seed failed: ${await res.text()}`).toBeTruthy();
  const body = await res.json();
  const tableId = body.game_id as number;
  track(tableId);
  const page = await ctx.newPage();
  await page.goto(`/table/${tableId}`);
  await expect(tableTitle(page)).not.toHaveText('');
  return { ctx, page, tableId, players: body.players as { id: number; display_name: string }[] };
}

const record = (page: Page) => page.getByRole('complementary', { name: 'Public Record' });
const lines = (page: Page) => record(page).locator('.row-list button');

async function planID(page: Page, tableId: number, type: string): Promise<number> {
  const res = await page.request.get(`/api/tables/${tableId}/plans`);
  const { plans } = (await res.json()) as { plans: { id: number; plan_type: string }[] };
  return plans.find((p) => p.plan_type === type)!.id;
}

test('Events: a plan is a 44px line that opens Chat; a row number and an empty row are inert', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, {
    phase: 'main_event',
    players: ['alice', 'bob'],
    current_row: 3,
    plans: [
      { preparer_idx: 0, plan_type: 'spread_rumors', category: 'knowledge', row: 2, row_order: 0 },
      { preparer_idx: 1, plan_type: 'exchange_courtiers', category: 'esteem', row: 5, row_order: 0 },
    ],
  });
  await openPane(page, 'Public Record');

  // One line per plan, and nothing else in the list is a button.
  await expect(lines(page)).toHaveCount(2);
  for (const line of await lines(page).all()) {
    const box = await line.boundingBox();
    expect(box!.height).toBeGreaterThanOrEqual(44);
    // The arrow says it opens somewhere else; there is no caret.
    await expect(line.locator('[data-kind="arrow"]')).toHaveCount(1);
  }
  await expect(lines(page).first()).toHaveAccessibleName(/^Spread Rumors by alice\. Open in Chat$/);

  // Both plans are still to come, so neither says anything.
  await expect(record(page).locator('.status-word')).toHaveCount(0);

  // The current row wears the bar and a deep gold number; its neighbours don't.
  const current = record(page).locator('.record-row[data-state="current"]');
  await expect(current).toHaveCount(1);
  await expect(current).toContainText('3');
  expect(await current.evaluate((el) => getComputedStyle(el).boxShadow)).not.toBe('none');
  const past = record(page).locator('.record-row[data-state="past"]').first();
  expect(await past.evaluate((el) => getComputedStyle(el).boxShadow)).toBe('none');

  // Tapping a number, or an empty row, does nothing: the pane stays where it was.
  await record(page).locator('.row-num', { hasText: /^Row\s*3$/ }).click();
  await record(page).locator('.row-num', { hasText: /^Row\s*8$/ }).click();
  await expect(page.getByRole('navigation', { name: 'Panes' }).locator('[aria-current="page"]'))
    .toContainText('Public Record');

  await ctx.close();
});

test('Events: a war the server stored as "make" is not shown as a Make', async ({ browser }) => {
  const { ctx, page, tableId } = await openTable(browser, {
    phase: 'main_event',
    players: ['alice', 'bob', 'carol'],
    current_row: 3,
    plans: [{ preparer_idx: 0, plan_type: 'make_war', category: 'power', row: 3, row_order: 0 }],
  });
  const war = await planID(page, tableId, 'make_war');

  // Make War closes in one step, and the server writes result = "make" when it does.
  expect((await page.request.post(`/api/plans/${war}/resolve`)).ok()).toBeTruthy();
  const stored = await (await page.request.get(`/api/tables/${tableId}/plans`)).json();
  const row = stored.plans.find((p: { id: number }) => p.id === war);
  expect(row.status).toBe('resolved');
  expect(row.result).toBe('make');

  // The record shows the war and nothing about how it fared.
  await page.reload();
  await openPane(page, 'Public Record');
  const line = lines(page).filter({ hasText: 'Make War' });
  await expect(line).toHaveCount(1);
  await expect(line.locator('.status-word')).toHaveCount(0);
  await expect(line).not.toContainText(/\bMake\b(?! War)/);

  await ctx.close();
});

test('Events: the plan being played says "Resolving", and no other line says anything', async ({ browser }) => {
  const { ctx, page, tableId } = await openTable(browser, {
    phase: 'main_event',
    players: ['alice', 'bob', 'carol'],
    current_row: 5,
    rankings: { knowledge: [1, 2, 0] },
    plans: [
      { preparer_idx: 0, plan_type: 'seek_answers', category: 'knowledge', row: 5, row_order: 0 },
      { preparer_idx: 1, plan_type: 'exchange_courtiers', category: 'esteem', row: 9, row_order: 0 },
    ],
  });
  const seek = await planID(page, tableId, 'seek_answers');
  expect((await page.request.post(`/api/plans/${seek}/resolve`)).ok()).toBeTruthy();

  await page.reload();
  await openPane(page, 'Public Record');
  const words = record(page).locator('.status-word');
  await expect(words).toHaveCount(1);
  await expect(words).toHaveText('Resolving');
  await expect(lines(page).filter({ hasText: 'Seek Answers' }).locator('.status-word')).toHaveText('Resolving');
  // Resolving takes no colour: it is where the table is, not how it fared.
  await expect(words).not.toHaveClass(/success|danger/);

  await ctx.close();
});

test('Events: in the Prologue the record shows thirteen blank rows and says when play starts', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, {
    phase: 'prologue_closing',
    players: ['alice', 'bob'],
  });
  await openPane(page, 'Public Record');

  await expect(record(page).getByText('Play starts when the Prologue ends')).toBeVisible();
  await expect(record(page).locator('.record-row')).toHaveCount(13);
  await expect(lines(page)).toHaveCount(0);
  // Nothing is current before play starts, and the Shake-Up row is drawn with the
  // house burst, not a typed star.
  await expect(record(page).locator('.record-row[data-state="current"]')).toHaveCount(0);
  await expect(record(page).locator('.shakeup-row .burst svg')).toHaveCount(1);
  await expect(record(page)).not.toContainText('✶');

  await ctx.close();
});

test('Events: once play has started the note is gone', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, { phase: 'main_event', players: ['alice', 'bob'] });
  await openPane(page, 'Public Record');
  await expect(record(page).locator('.record-row')).toHaveCount(13);
  await expect(record(page).getByText('Play starts')).toHaveCount(0);
  await ctx.close();
});

test('Laws: an entry you have not seen starts with the dot, which stays while you read and is gone when you return', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, { phase: 'main_event', players: ['alice', 'bob'] });
  // The seeded law and rumor are new to alice, and the chip says so.
  await openRecordTab(page, 'Laws');
  const entry = page.locator('.laws-rumors .lr-item').first();
  await expect(entry).toBeVisible();
  const dot = entry.getByRole('img', { name: 'new' });
  await expect(dot).toBeVisible();

  // Opening the list tells the server it is seen, and the chip's dot goes (the
  // table page does that), but the entry keeps its dot while you are reading it:
  // it would otherwise disappear the moment it was drawn.
  const lawsChip = page.getByRole('navigation', { name: 'Public Record' }).getByRole('button', { name: /Laws/ });
  await expect(lawsChip.getByRole('img')).toHaveCount(0);
  await expect(dot).toBeVisible();

  // Flip to Rumors and back (the same list, mounted once): the law has been seen.
  await openRecordTab(page, 'Rumors');
  await expect(page.locator('.laws-rumors .lr-item').first().getByRole('img', { name: 'new' })).toBeVisible();
  await openRecordTab(page, 'Laws');
  await expect(page.locator('.laws-rumors .lr-item').first()).toBeVisible();
  await expect(page.locator('.laws-rumors .lr-item').first().getByRole('img', { name: 'new' })).toHaveCount(0);

  // It is the server that remembers: a reload comes back with no dot anywhere.
  await page.reload();
  await openRecordTab(page, 'Laws');
  await expect(page.locator('.laws-rumors .lr-item').first()).toBeVisible();
  await expect(page.locator('.laws-rumors').getByRole('img', { name: 'new' })).toHaveCount(0);

  await ctx.close();
});

test('Laws: your own edit is not new to you, and only the writer gets Edit', async ({ browser }) => {
  const { ctx, page, tableId } = await openTable(browser, {
    phase: 'main_event',
    players: ['alice', 'bob'],
  });
  await openRecordTab(page, 'Laws');
  const entry = page.locator('.laws-rumors .lr-item').first();

  // alice is the seeded law's signatory, so the law carries a bordered, 44px Edit
  // button in place of the old small "edit" link.
  const edit = entry.getByRole('button', { name: 'Edit' });
  await expect(edit).toBeVisible();
  expect((await edit.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  expect(await edit.evaluate((el) => getComputedStyle(el).borderTopWidth)).toBe('1px');

  await edit.click();
  await page.getByLabel('Law text').fill('No private armies in the capital.');
  await page.getByRole('button', { name: 'Save' }).click();
  await expect(page.locator('.laws-rumors .lr-item').first()).toContainText('No private armies in the capital.');
  // The edit drew a new revision, which is above the marker the list opened with,
  // and would read as new to the person who just wrote it. It doesn't.
  await expect(page.locator('.laws-rumors').getByRole('img', { name: 'new' })).toHaveCount(0);

  // bob did not write it: no Edit for him, and it is new to him (he hasn't looked).
  const bobCtx = await browser.newContext({ baseURL: E2E, viewport: PHONE });
  expect((await bobCtx.request.post('/api/dev/login?username=bob')).ok()).toBeTruthy();
  const bobPage = await bobCtx.newPage();
  await bobPage.goto(`/table/${tableId}`);
  await expect(tableTitle(bobPage)).not.toHaveText('');
  await openRecordTab(bobPage, 'Laws');
  const bobEntry = bobPage.locator('.laws-rumors .lr-item').first();
  await expect(bobEntry).toContainText('No private armies in the capital.');
  await expect(bobEntry.getByRole('button', { name: 'Edit' })).toHaveCount(0);
  await expect(bobEntry.getByRole('img', { name: 'new' })).toBeVisible();

  await bobCtx.close();
  await ctx.close();
});

test('Rumors: plain entries, a name beside its shape, and no cards', async ({ browser }) => {
  const { ctx, page } = await openTable(browser, {
    phase: 'main_event',
    players: ['alice', 'bob'],
    rumors: ['She reads letters that were never meant for her.'],
  });
  await openRecordTab(page, 'Rumors');
  const entry = page.locator('.laws-rumors .lr-item').first();
  await expect(entry).toContainText('She reads letters');
  await expect(entry).toContainText('Spread by');

  // A plain entry: no fill and no box of its own, a line under it.
  const style = await entry.evaluate((el) => {
    const s = getComputedStyle(el);
    return { bg: s.backgroundColor, radius: s.borderTopLeftRadius, bottom: s.borderBottomWidth, left: s.borderLeftWidth };
  });
  expect(style.bg).toBe('rgba(0, 0, 0, 0)');
  expect(style.radius).toBe('0px');
  expect(style.left).toBe('0px');
  expect(style.bottom).toBe('1px');

  // The list's own total is in words.
  await expect(page.locator('.laws-rumors')).toContainText('1 rumor');

  await ctx.close();
});
