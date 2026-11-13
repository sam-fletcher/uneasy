import { test, expect } from '@playwright/test';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { paint, playerContext } from './helpers/endgame';

// The lobby, at phone width, for the host and a guest.
//
// player_join and start_prologue cover who is seated and who may start. This
// covers what the redesign changed on the screen: the verdict is a heading and
// words rather than a framed card, the Start Prologue button is the one main
// button, and the invite chair, the one object in the roster, is a button that
// copies the join code and goes away when the table is full.

const track = cleanupGameAfterEach();

test('the lobby: a verdict with no frame, and an invite chair that copies the code', async ({ browser }) => {
  const clipboard = { permissions: ['clipboard-read', 'clipboard-write'] as string[] };
  const aliceCtx = await playerContext(browser, 'alice', clipboard);
  const bobCtx = await playerContext(browser, 'bob');
  const { game } = await (await aliceCtx.request.post('/api/tables')).json();
  track(game.id);
  expect((await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } })).ok()).toBeTruthy();

  const alice = await aliceCtx.newPage();
  const bob = await bobCtx.newPage();
  await Promise.all([alice.goto(`/table/${game.id}`), bob.goto(`/table/${game.id}`)]);
  await expect(tableTitle(alice)).toHaveText('Lobby');

  // The host's step: a heading, the words, and the one main button. No frame.
  await expect(alice.getByRole('heading', { name: 'Is everyone here?' })).toBeVisible();
  const verdict = await paint(alice.locator('.verdict'));
  expect(verdict.borderWidth, 'the verdict is not a card').toBe('0px');
  expect(verdict.background).toBe('rgba(0, 0, 0, 0)');
  await expect(alice.locator('.lobby .action-btn.primary')).toHaveCount(1);
  await expect(alice.getByRole('button', { name: 'Start Prologue' })).toBeEnabled();

  // The guest's step is the same shape, with nothing to press.
  await expect(bob.getByRole('heading', { name: "You're seated. Nothing to do yet." })).toBeVisible();
  await expect(bob.locator('.lobby .action-btn.primary')).toHaveCount(0);

  // The invite chair: a control (dashed, the control corner, 44px), with the
  // code as plain text on it rather than a badge.
  const chair = alice.getByRole('button', { name: /^Invite a friend/ });
  await expect(chair).toBeVisible();
  await expect(chair.locator('.code-badge')).toHaveText(new RegExp(`^${game.join_code}`));
  const edge = await chair.evaluate((el) => {
    const s = getComputedStyle(el);
    return { style: s.borderTopStyle, radius: s.borderTopLeftRadius, height: el.getBoundingClientRect().height };
  });
  const startRadius = (await paint(alice.getByRole('button', { name: 'Start Prologue' }))).radius;
  expect(edge.style, 'dashed: not filled in yet').toBe('dashed');
  expect(edge.radius, 'the control corner, like any button').toBe(startRadius);
  expect(edge.height).toBeGreaterThanOrEqual(44);
  expect((await paint(chair.locator('.code-badge'))).background, 'the code has no badge fill').toBe('rgba(0, 0, 0, 0)');

  // Pressing it copies the invitation and says so, in words.
  await chair.click();
  await expect(chair).toContainText('Copied!');
  expect(await alice.evaluate(() => navigator.clipboard.readText())).toContain(game.join_code);

  // A full table has no empty chair: it goes from both screens as the fifth sits down.
  for (const name of ['carol', 'dave', 'erin']) {
    const ctx = await playerContext(browser, name);
    expect((await ctx.request.post('/api/tables/join', { data: { join_code: game.join_code } })).ok()).toBeTruthy();
    await ctx.close();
  }
  await expect(alice.getByText('5 seated · table full')).toBeVisible();
  await expect(alice.getByRole('button', { name: /^Invite a friend/ })).toHaveCount(0);
  await expect(bob.getByRole('button', { name: /^Invite a friend/ })).toHaveCount(0);

  await Promise.all([aliceCtx.close(), bobCtx.close()]);
});
