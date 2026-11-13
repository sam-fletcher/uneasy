import { test, expect } from '@playwright/test';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { paint, playerContext, resolveToken, seedGame } from './helpers/endgame';

// The row 7 → 8 ending vote, read at phone width as the players see it.
//
// What this pins down is the redesign's ruling about a CHOICE: the two endings
// are chips, neither is coloured to mean anything, and the one you picked has
// the bright gold border and nothing else (no fill). The vote's behaviour (counts,
// changing your mind, the last vote closing it) is exercised through the real
// screen, and the vote is opened through the real gate: the focus player
// passing on row 7.

const track = cleanupGameAfterEach();

test('the ending vote is two chips, the pick is bright gold, and the last vote closes it', async ({ browser }) => {
  const seeded = await seedGame({ phase: 'main_event', players: ['alice', 'bob', 'carol'], current_row: 7 });
  track(seeded.game_id);
  const id = seeded.game_id;

  const [aliceCtx, bobCtx, carolCtx] = await Promise.all(
    ['alice', 'bob', 'carol'].map((n) => playerContext(browser, n)),
  );
  // Row 7 is complete once the focus player (alice) passes: that is what opens the vote.
  const pass = await aliceCtx.request.post(`/api/tables/${id}/pass-focus`);
  expect(pass.ok(), 'pass-focus should open the vote').toBeTruthy();

  const alice = await aliceCtx.newPage();
  const bob = await bobCtx.newPage();
  await Promise.all([alice.goto(`/table/${id}`), bob.goto(`/table/${id}`)]);

  const panel = (page: typeof alice) => page.locator('.endgame-vote');
  await expect(panel(alice)).toBeVisible();
  await expect(panel(bob)).toBeVisible();
  await expect(tableTitle(alice)).toContainText('Ending vote');

  // The step has no outline and no fill (it used to be a violet-framed panel).
  const frame = await paint(panel(alice));
  expect(frame.borderWidth, 'no border around the step').toBe('0px');
  expect(frame.background, 'no fill behind the step').toBe('rgba(0, 0, 0, 0)');

  const smooth = (page: typeof alice) => page.getByRole('button', { name: 'Smooth Landing' });
  const explosive = (page: typeof alice) => page.getByRole('button', { name: 'Explosive Finale' });
  const mode = (page: typeof alice, label: string) => page.locator('.mode', { hasText: label });

  // Nobody has voted: neither chip is picked, and each one is a 44px target.
  await expect(smooth(alice)).toHaveAttribute('aria-pressed', 'false');
  await expect(explosive(alice)).toHaveAttribute('aria-pressed', 'false');
  for (const chip of [smooth(alice), explosive(alice)]) {
    expect((await chip.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  }
  await expect(alice.locator('.endgame-vote .tally')).toContainText('0 of 3 votes in');

  // Alice picks Explosive Finale. The chip gets the bright gold border, in the
  // same raised fill as its neighbour: the pick is the border, never a fill.
  const you = await resolveToken(alice, '--color-you');
  await explosive(alice).click();
  await expect(explosive(alice)).toHaveAttribute('aria-pressed', 'true');
  const picked = await paint(explosive(alice));
  const other = await paint(smooth(alice));
  expect(picked.borderColor, 'the pick wears bright gold').toBe(you);
  expect(picked.color, 'its words wear it too').toBe(you);
  expect(other.borderColor, 'the other chip is not gold').not.toBe(you);
  expect(picked.background, 'the pick adds no fill').toBe(other.background);

  // The count and the words that say whose vote it is, beside the chip.
  await expect(mode(alice, 'Explosive Finale')).toContainText('1 vote');
  await expect(mode(alice, 'Explosive Finale')).toContainText('Your vote');
  await expect(mode(alice, 'Smooth Landing')).toContainText('0 votes');
  await expect(alice.locator('.endgame-vote .tally')).toContainText('1 of 3 votes in');

  // Bob hears it live: the count moves, and nothing is picked on his screen.
  await expect(mode(bob, 'Explosive Finale')).toContainText('1 vote');
  await expect(explosive(bob)).toHaveAttribute('aria-pressed', 'false');
  await expect(mode(bob, 'Explosive Finale')).not.toContainText('Your vote');
  await expect(bob.getByText('Yours is one of the ones missing')).toBeVisible();

  // Alice may change her mind: the pick moves, and so does the tally.
  await smooth(alice).click();
  await expect(smooth(alice)).toHaveAttribute('aria-pressed', 'true');
  await expect(explosive(alice)).toHaveAttribute('aria-pressed', 'false');
  await expect(mode(alice, 'Smooth Landing')).toContainText('1 vote');
  await expect(mode(alice, 'Explosive Finale')).toContainText('0 votes');

  // The last vote closes it, and the panel goes from both screens.
  await smooth(bob).click();
  const last = await carolCtx.request.post(`/api/tables/${id}/ending-vote`, { data: { mode: 'smooth_landing' } });
  expect(last.ok(), 'carol\'s vote').toBeTruthy();
  await expect(panel(alice)).toHaveCount(0);
  await expect(panel(bob)).toHaveCount(0);

  await Promise.all([aliceCtx.close(), bobCtx.close(), carolCtx.close()]);
});
