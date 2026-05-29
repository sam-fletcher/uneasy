import { test, expect, request as pwRequest } from '@playwright/test';

// First game-flow spec: lobby → prologue transition driven by the
// facilitator, observed live on the non-facilitator's page.
//
// Why this flow: it's the smallest end-to-end game action that exercises
// (a) facilitator-only gating on a real game mutation, (b) the
// `phase.changed` WebSocket broadcast, and (c) both clients swapping out
// their entire phase-view component (lobby → PrologueView). It does not
// require any prologue inputs (cards dealt, hearts committed, etc.) —
// those belong to later game-flow specs.

test('facilitator starts prologue; non-facilitator sees the phase change live', async ({ browser }) => {
  const reset = await pwRequest.newContext();
  await reset.post('http://localhost:8090/api/dev/reset');
  await reset.dispose();

  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  await aliceCtx.request.post('/api/dev/login?username=alice');
  await bobCtx.request.post('/api/dev/login?username=bob');

  // Alice creates and is therefore the facilitator. Bob joins by code.
  const { game } = await (await aliceCtx.request.post('/api/tables')).json();
  await bobCtx.request.post('/api/tables/join', {
    data: { join_code: game.join_code },
  });

  const alicePage = await aliceCtx.newPage();
  const bobPage = await bobCtx.newPage();
  await Promise.all([
    alicePage.goto(`/table/${game.id}`),
    bobPage.goto(`/table/${game.id}`),
  ]);

  // Sanity: both clients render the lobby. The phase badge sits in the
  // game-info header strip with class .phase-badge.
  await expect(alicePage.locator('.phase-badge')).toHaveText('Lobby');
  await expect(bobPage.locator('.phase-badge')).toHaveText('Lobby');

  // Only the facilitator may start the prologue. The button is rendered
  // exclusively in alice's tree.
  const startButton = alicePage.getByRole('button', { name: 'Start Prologue' });
  await expect(startButton).toBeVisible();
  await expect(bobPage.getByRole('button', { name: 'Start Prologue' })).toHaveCount(0);

  await startButton.click();

  // Phase transition: both pages flip to Prologue. Bob's transition is
  // driven purely by the `phase.changed` WebSocket event (no user input,
  // no navigation).
  await expect(alicePage.locator('.phase-badge')).toHaveText('Prologue');
  await expect(bobPage.locator('.phase-badge')).toHaveText('Prologue');

  // Lobby-only affordances should be gone: the join-code badge only
  // renders while phase === 'lobby'.
  await expect(alicePage.locator('.code-badge')).toHaveCount(0);
  await expect(bobPage.locator('.code-badge')).toHaveCount(0);

  await aliceCtx.close();
  await bobCtx.close();
});
