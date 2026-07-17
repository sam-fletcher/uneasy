import { test, expect } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Second multi-context spec: alice is already on the table page when bob
// joins. Alice's roster should pick up bob's name live via the
// `player.joined` WebSocket event, with no reload.
//
// This is intentionally orthogonal to chat.spec.ts — same two-context
// pattern, but exercises a different event type (EventPlayerJoined,
// handled in routes/table/[id]/+page.svelte) and a different reactive
// surface (the members list, not the chat feed).

const track = cleanupGameAfterEach();

test('bob joining is reflected on alice\'s open table page', async ({ browser }) => {
  const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
  const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });

  await aliceCtx.request.post('/api/dev/login?username=alice');
  await bobCtx.request.post('/api/dev/login?username=bob');

  const createRes = await aliceCtx.request.post('/api/tables');
  const { game } = await createRes.json();
  const tableId: number = game.id;
  track(tableId);
  const joinCode: string = game.join_code;

  // Alice loads the table first and waits until her roster is rendered
  // (proxy for "WebSocket subscription is live"). Before bob joins her
  // own name is the only member shown.
  const alicePage = await aliceCtx.newPage();
  await alicePage.goto(`/table/${tableId}`);
  // The member chip is a button with aria-label "View {name}'s retinue".
  // Scoping to that role avoids matching the join-code badge or other
  // places where the username might appear.
  await expect(alicePage.getByRole('button', { name: /View alice's retinue/ })).toBeVisible();
  await expect(alicePage.getByRole('button', { name: /View bob's retinue/ })).toHaveCount(0);

  // Bob joins via API — no page navigation needed on his side. The point
  // here is that alice's already-open page picks up the event.
  const joinRes = await bobCtx.request.post('/api/tables/join', {
    data: { join_code: joinCode },
  });
  expect(joinRes.ok(), `bob could not join ${joinCode}`).toBeTruthy();

  // Live update: bob's member chip appears on alice's page without reload.
  await expect(alicePage.getByRole('button', { name: /View bob's retinue/ })).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});
