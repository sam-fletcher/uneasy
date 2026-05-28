import { test, expect, request as pwRequest } from '@playwright/test';

// First main_event spec, exercising the new /api/dev/seed fixture path.
//
// This spec deliberately does *not* drive the prologue or any main-event
// inputs — its job is to prove that:
//
//   (a) the dev seed endpoint produces a game the real client can load
//       and render, and
//   (b) future main_event specs have a one-line setup they can copy
//       instead of re-driving the entire game from lobby every time.
//
// Mechanics-level main_event specs (dice rolls, plan voting, scene
// resolution) will be built on top of this same fixture.

test('seeded main_event game renders the main-event view for both players', async ({ browser }) => {
  // Reset uneasy_test, then seed a fresh game in main_event with alice
  // and bob as players. The seed endpoint creates accounts on demand,
  // so we don't need a prior dev-login on either side just to seed.
  const fixture = await pwRequest.newContext({ baseURL: 'http://localhost:8090' });
  await fixture.post('/api/dev/reset');
  const seedRes = await fixture.post('/api/dev/seed', {
    data: { phase: 'main_event', players: ['alice', 'bob'] },
  });
  expect(seedRes.ok(), 'seed call failed').toBeTruthy();
  const { game_id, phase, players } = await seedRes.json();
  expect(phase).toBe('main_event');
  expect(players).toHaveLength(2);
  expect(players[0].is_facilitator).toBe(true);
  await fixture.dispose();

  // Open one browser context per player so each carries its own session.
  // dev-login finds the account the seed already created and just opens
  // a session for it.
  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await aliceCtx.request.post('/api/dev/login?username=alice');
  await bobCtx.request.post('/api/dev/login?username=bob');

  const alicePage = await aliceCtx.newPage();
  const bobPage = await bobCtx.newPage();
  await Promise.all([
    alicePage.goto(`/table/${game_id}`),
    bobPage.goto(`/table/${game_id}`),
  ]);

  // Both clients land directly in main_event — no lobby pass-through.
  await expect(alicePage.locator('.phase-badge')).toHaveText('Main Event');
  await expect(bobPage.locator('.phase-badge')).toHaveText('Main Event');

  // MainEventView mounts on both sides.
  await expect(alicePage.locator('.main-event-view')).toBeVisible();
  await expect(bobPage.locator('.main-event-view')).toBeVisible();

  // Lobby-only affordances are absent — guards against a regression
  // where leftover lobby UI bleeds into seeded games.
  await expect(alicePage.locator('.code-badge')).toHaveCount(0);
  await expect(bobPage.getByRole('button', { name: 'Start Prologue' })).toHaveCount(0);

  // Sanity that the game is functional, not just statically rendered:
  // chat still works in main_event, with bob picking up alice's message
  // live via WebSocket — same path tested in chat.spec.ts, but proving
  // the seed produced a connectable hub.
  const message = `main-event-${Date.now()}`;
  const aliceChat = alicePage.getByRole('complementary', { name: 'Chat' });
  await aliceChat.getByPlaceholder('Write a message…').fill(message);
  await aliceChat.getByRole('button', { name: 'Send' }).click();
  await expect(bobPage.getByRole('complementary', { name: 'Chat' }).getByText(message))
    .toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});
