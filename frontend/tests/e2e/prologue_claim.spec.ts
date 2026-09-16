import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// A prologue claim reaches both players' screens from the socket payload
// and the POST reply alone — no refetch of sheets, cards or assets.
//
// Why this is asserted on network traffic and not just on the DOM: the
// production database is a ~25ms round trip away and every HTTP hop has a
// ~400ms floor, so a claim that *looks* right but silently triggers three
// reloads is the exact regression this spec exists to catch. The claim
// itself is made over the API rather than through the modal: the subject
// is what the clients do with the outcome, not the form.

const track = cleanupGameAfterEach();

/** Count GETs under the table's API prefix from now on. */
function countTableGets(page: Page, gameID: number): () => string[] {
  const seen: string[] = [];
  page.on('request', (req) => {
    if (req.method() === 'GET' && req.url().includes(`/api/tables/${gameID}/`)) {
      seen.push(new URL(req.url()).pathname);
    }
  });
  return () => seen;
}

test('a claim lands live on both screens with zero follow-up fetches', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await aliceCtx.request.post('/api/dev/login?username=alice');
  await bobCtx.request.post('/api/dev/login?username=bob');

  const { game } = await (await aliceCtx.request.post('/api/tables')).json();
  track(game.id);
  await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });
  await aliceCtx.request.post(`/api/tables/${game.id}/start-prologue`);

  const alicePage = await aliceCtx.newPage();
  const bobPage = await bobCtx.newPage();
  await Promise.all([alicePage.goto(`/table/${game.id}`), bobPage.goto(`/table/${game.id}`)]);
  await expect(alicePage.locator('.phase-badge')).toHaveText('Prologue');
  await expect(bobPage.locator('.phase-badge')).toHaveText('Prologue');

  // Pick the first tile of the first non-titles sheet (no main-character
  // dependency) and author both of its cards as makes.
  const sheets = await (await aliceCtx.request.get(`/api/tables/${game.id}/prologue/sheets`)).json();
  const sheet = sheets.sheets.find((s: { type: string }) => s.type !== 'titles');
  const choice = sheet.choices[0];
  const tileKey = `${sheet.type}::${choice.name}`;

  // Sheets are collapsed accordions; open the one we'll claim from on both
  // screens so the tile is rendered, and let the initial loads settle.
  const sheetHeader = (p: Page) => p.getByRole('button', { name: new RegExp(`^${sheet.display_name}`) });
  await sheetHeader(alicePage).click();
  await sheetHeader(bobPage).click();
  await expect(alicePage.locator(`[data-tile-key="${tileKey}"]`)).toBeVisible();
  await expect(bobPage.locator(`[data-tile-key="${tileKey}"]`)).toBeVisible();
  await expect(alicePage.getByText('0 of 6 chosen')).toBeVisible();
  const assetsBefore = (await (await aliceCtx.request.get(`/api/tables/${game.id}/assets`)).json()).assets.length;

  const aliceGets = countTableGets(alicePage, game.id);
  const bobGets = countTableGets(bobPage, game.id);

  const res = await aliceCtx.request.post(`/api/tables/${game.id}/prologue/choose`, {
    data: {
      sheet_type: sheet.type,
      choice_name: choice.name,
      asset_text: 'The Long Road',
      asset_marginalia: ['Dust on every boot'],
      card_assets: choice.cards.map((c: { suit: string; value: string }, i: number) => ({
        suit: c.suit, value: c.value, text: `Card asset ${i + 1}`,
      })),
    },
  });
  expect(res.ok()).toBeTruthy();
  const outcome = await res.json();
  expect(outcome.cards).toHaveLength(choice.cards.length);

  // Both screens mark the tile claimed and advance the progress line from
  // the socket payload alone.
  const claimedTile = (p: Page) => p.locator(`[data-tile-key="${tileKey}"].claimed`);
  await expect(claimedTile(alicePage)).toBeVisible();
  await expect(claimedTile(bobPage)).toBeVisible();
  await expect(alicePage.getByText('1 of 6 chosen')).toBeVisible();
  await expect(bobPage.getByText('1 of 6 chosen')).toBeVisible();

  // And both retinues hold the new assets — the sheet asset plus one per
  // card — which arrived as asset.created events, not a refetch. The header
  // exposes no count, so read the assets each page holds through the API
  // it would have used; the *absence* of that call below is the assertion.
  const assetsAfter = (await (await bobCtx.request.get(`/api/tables/${game.id}/assets`)).json()).assets.length;
  expect(assetsAfter).toBe(assetsBefore + 1 + choice.cards.length);

  // Give any stray reload a moment to fire, then assert none did.
  await alicePage.waitForTimeout(500);
  expect(aliceGets(), 'alice refetched after her own claim').toEqual([]);
  expect(bobGets(), 'bob refetched after alice\'s claim').toEqual([]);

  await aliceCtx.close();
  await bobCtx.close();
});
