import { test, expect, request as pwRequest, type APIRequestContext, type Page } from '@playwright/test';
import { deleteGame } from './helpers';
import { E2E } from './baseUrl';

// The profile page's table list: three groups under small headings, each table
// a tappable row that says the table's own title and who is there. What the
// rows show (titles, clocks, "N new") is pinned for the markup by
// TableRow.test.ts; this spec checks the same page against a real server, with
// real games in each state, and that the row still behaves as a row (the whole
// of it opens the table, the bell beside it does not).
//
// Every run uses brand-new accounts, so the profile lists exactly these games
// and nothing left over from another spec.

test.use({ viewport: { width: 375, height: 812 } });

const created: number[] = [];
test.afterEach(async ({ request }) => {
  while (created.length) await deleteGame(request, created.pop()!);
});

async function devLogin(api: APIRequestContext, username: string) {
  const res = await api.post(`/api/dev/login?username=${encodeURIComponent(username)}`);
  expect(res.ok(), `dev-login for ${username} failed`).toBeTruthy();
}

/** A Main Event table at row 6 with Seek Answers prepared by the first player,
 *  so the game is waiting on whoever sits first. */
async function seedRow6(players: string[]): Promise<{ id: number; joinCode: string }> {
  const fixture = await pwRequest.newContext({ baseURL: E2E });
  const res = await fixture.post('/api/dev/seed', {
    data: {
      phase: 'main_event',
      players,
      current_row: 6,
      plans: [{ preparer_idx: 0, plan_type: 'seek_answers', category: 'knowledge', row: 6, row_order: 0 }],
    },
  });
  expect(res.ok(), 'seed call failed').toBeTruthy();
  const body = await res.json();
  await fixture.dispose();
  created.push(body.game_id);
  return { id: body.game_id, joinCode: body.join_code };
}

/** Three tables for `me`: the game waits on me in one, on `other` in the
 *  second, and the third is finished. There is no way to finish a game from the
 *  dev API, so the third is a live table whose list entry is rewritten to
 *  "ended" on its way to the page: the real response, with one phase changed. */
async function openProfile(page: Page, me: string, other: string) {
  const mine = await seedRow6([me, other]);
  const theirs = await seedRow6([other, me]);
  const done = await seedRow6([me, other]);

  await page.route('**/api/accounts/me/tables', async (route) => {
    const res = await route.fetch();
    const json = await res.json();
    for (const t of json.tables) {
      if (t.game_id === done.id) {
        t.phase = 'ended';
        t.waiting_on_player_ids = [];
      }
    }
    await route.fulfill({ response: res, json });
  });

  await devLogin(page.context().request, me);
  await page.goto('/profile');
  await expect(page.getByRole('heading', { name: 'Your tables' })).toBeVisible();
  return { mine, theirs, done };
}

const names = () => {
  const tag = Math.random().toString(36).slice(2, 6);
  return { me: `pt${tag}`, other: `po${tag}` };
};

test('tables sit under the heading that says whose move it is', async ({ page }) => {
  const { me, other } = names();
  const { mine, theirs, done } = await openProfile(page, me, other);

  const you = page.getByRole('list', { name: 'Waiting on you' });
  const others = page.getByRole('list', { name: 'Waiting on others' });
  const finished = page.getByRole('list', { name: 'Finished' });

  await expect(you.locator(`a[href="/table/${mine.id}"]`)).toBeVisible();
  await expect(others.locator(`a[href="/table/${theirs.id}"]`)).toBeVisible();
  await expect(finished.locator(`a[href="/table/${done.id}"]`)).toBeVisible();
  // One row each: nothing sits under two headings.
  await expect(page.locator('a[href^="/table/"]')).toHaveCount(3);
});

test('a group with no tables shows no heading', async ({ page }) => {
  const { me, other } = names();
  const theirs = await seedRow6([other, me]);
  await devLogin(page.context().request, me);
  await page.goto('/profile');
  await expect(page.locator(`a[href="/table/${theirs.id}"]`)).toBeVisible();

  await expect(page.getByRole('heading', { name: 'Waiting on others' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Waiting on you' })).toHaveCount(0);
  await expect(page.getByRole('heading', { name: 'Finished' })).toHaveCount(0);
});

test('a row says the table\'s title and its players, and shows no join code', async ({ page }) => {
  const { me, other } = names();
  const { mine, done } = await openProfile(page, me, other);

  const row = page.locator(`a[href="/table/${mine.id}"]`);
  await expect(row).toContainText('Row 6 · Seek Answers');
  await expect(row).toContainText(me);
  await expect(row).toContainText(other);
  // The game is waiting on me here, and the row says so in words as well as the clock.
  await expect(row).toContainText('the game is waiting on you');

  // A finished table reads "Game over" and waits on no one.
  const finishedRow = page.locator(`a[href="/table/${done.id}"]`);
  await expect(finishedRow).toContainText('Game over');
  await expect(finishedRow).not.toContainText('waiting on');

  // The code is for inviting someone, which is done from the table, not here.
  await expect(page.getByText(mine.joinCode)).toHaveCount(0);
  await expect(page.getByText(done.joinCode)).toHaveCount(0);
});

test('unread chat shows as "N new" on the row it belongs to, and no other', async ({ page, browser }) => {
  const { me, other } = names();
  const { mine, theirs } = await openProfile(page, me, other);

  const otherCtx = await browser.newContext({ baseURL: E2E });
  await devLogin(otherCtx.request, other);
  for (const body of ['first', 'second']) {
    const res = await otherCtx.request.post(`/api/tables/${mine.id}/posts`, { data: { body } });
    expect(res.ok(), 'chat post failed').toBeTruthy();
  }
  await otherCtx.close();

  await page.reload();
  await expect(page.locator(`a[href="/table/${mine.id}"]`)).toContainText('2 new');
  await expect(page.locator(`a[href="/table/${theirs.id}"]`)).not.toContainText(/\d+ new/);
});

test('the whole row opens the table, but its bell only silences reminders', async ({ page }) => {
  const { me, other } = names();
  const { mine, theirs } = await openProfile(page, me, other);

  // The bell exists only on a table that is waiting on me.
  const bell = page.getByRole('button', { name: /Stop reminding me about Row 6 · Seek Answers/ });
  await expect(bell).toHaveCount(1);
  await expect(page.locator(`a[href="/table/${theirs.id}"]`)).toBeVisible();
  const box = await bell.boundingBox();
  expect(box!.width).toBeGreaterThanOrEqual(44);
  expect(box!.height).toBeGreaterThanOrEqual(44);

  // Tapping the bell asks the server and stays here.
  const muted = page.waitForResponse((r) => r.url().includes(`/api/tables/${mine.id}/reminder-mute`));
  await bell.click();
  await muted;
  await expect(page).toHaveURL(/\/profile$/);

  // Tapping anywhere else on the row, here its far right edge (the arrow), opens the table.
  const row = page.locator(`a[href="/table/${mine.id}"]`);
  const rowBox = (await row.locator('xpath=..').boundingBox())!;
  await page.mouse.click(rowBox.x + rowBox.width - 4, rowBox.y + rowBox.height / 2);
  await expect(page).toHaveURL(new RegExp(`/table/${mine.id}$`));
});

test('Join is grey until a code is typed, and creating a table is the secondary button', async ({ page }) => {
  const { me } = names();
  await devLogin(page.context().request, me);
  await page.goto('/profile');

  const join = page.getByRole('button', { name: 'Join', exact: true });
  await expect(join).toBeDisabled();
  await page.getByLabel('Join code').fill('abc123');
  await expect(join).toBeEnabled();

  await expect(join).toHaveClass(/primary/);
  await expect(page.getByRole('button', { name: 'Create a new table' })).toHaveClass(/secondary/);
  // Join is the only filled button on the page.
  await expect(page.locator('button.action-btn.primary')).toHaveCount(1);
});

test('the page never scrolls sideways, at the narrowest phone', async ({ page }) => {
  await page.setViewportSize({ width: 344, height: 800 });
  const { me, other } = names();
  await openProfile(page, me, other);
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  );
  expect(overflow).toBeLessThanOrEqual(0);
});
