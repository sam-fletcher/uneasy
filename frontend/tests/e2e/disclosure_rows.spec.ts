import { test, expect } from './fixtures';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { E2E } from './baseUrl';

// The disclosure standard in a real browser: what a render can't show is a press.
// Pressing an open step shuts it (its up chevron means "tap to close"), a closed
// one opens, and the Prologue's standing card and category row open and shut with
// their chevron and aria wiring. The unit tests pin the markup and the stylesheet
// (shared/disclosure.test.ts); this pins the behaviour.

const track = cleanupGameAfterEach();

test('the Prologue: a claim step, a category row and the standing card open and shut', async ({ browser }) => {
  const ctx = await browser.newContext({ baseURL: E2E });
  await ctx.request.post('/api/dev/login?username=alice');
  const { game } = await (await ctx.request.post('/api/tables')).json();
  track(game.id);
  const bobCtx = await browser.newContext({ baseURL: E2E });
  await bobCtx.request.post('/api/dev/login?username=bob');
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });
  await ctx.request.post(`/api/tables/${game.id}/start-prologue`);

  const page = await ctx.newPage();
  await page.goto(`/table/${game.id}`);
  await expect(tableTitle(page)).toHaveText(/^Prologue/);

  // The standing card: its head is the control; the board is its body.
  const standing = page.locator('.standing-head');
  await expect(standing).toHaveAttribute('aria-expanded', 'false');
  await expect(page.locator('#standing-board')).toHaveCount(0);
  await standing.click();
  await expect(standing).toHaveAttribute('aria-expanded', 'true');
  await expect(standing).toHaveAttribute('aria-controls', 'standing-board');
  await expect(page.locator('#standing-board')).toBeVisible();
  await expect(standing.locator('.row-mark.caret.open')).toHaveCount(1);
  await standing.click();
  await expect(page.locator('#standing-board')).toHaveCount(0);

  // A category row: the line moves under its tiles when open.
  const sheets = await (await ctx.request.get(`/api/tables/${game.id}/prologue/sheets`)).json();
  const sheet = sheets.sheets.find((s: { type: string }) => s.type !== 'titles');
  const header = page.getByRole('button', { name: new RegExp(`^${sheet.display_name}`) });
  const panel = page.locator('.sheet-panel', { has: header });
  await expect(panel).not.toHaveClass(/has-body/);
  await header.click();
  await expect(panel).toHaveClass(/has-body/);
  await expect(header.locator('.row-mark.caret.open')).toHaveCount(1);
  // The head's own line is transparent while the body carries it.
  expect(await header.evaluate((el) => getComputedStyle(el).borderBottomColor)).toBe('rgba(0, 0, 0, 0)');
  expect(await panel.locator('.sheet-body').evaluate((el) => getComputedStyle(el).borderBottomWidth)).toBe('1px');

  // A claim's steps: pressing the open step shuts it, pressing it again opens it.
  const choice = sheet.choices[0];
  await page.locator(`[data-tile-key="${sheet.type}::${choice.name}"]`).click();
  await page.getByRole('button', { name: 'Claim this tile' }).click();
  const step = page.locator('.step-header').first();
  await expect(step).toHaveAttribute('aria-expanded', 'true');
  const bodyID = await step.getAttribute('aria-controls');
  expect(bodyID).toBeTruthy();
  await expect(page.locator(`#${bodyID}`)).toBeVisible();
  await step.click();
  await expect(step).toHaveAttribute('aria-expanded', 'false');
  // aria-controls only names an element that exists.
  await expect(step).not.toHaveAttribute('aria-controls', /.+/);
  await expect(step.locator('.row-mark.caret.open')).toHaveCount(0);
  await step.click();
  await expect(step).toHaveAttribute('aria-expanded', 'true');

  await ctx.close();
  await bobCtx.close();
});
