import { test, expect } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Second multi-context spec: alice is already on the table page when bob
// joins. Alice's roster should pick up bob's name live via the
// `player.joined` WebSocket event, with no reload.
//
// This is intentionally orthogonal to chat.spec.ts — same two-context
// pattern, but exercises a different event type (EventPlayerJoined,
// handled in routes/table/[id]/ws-handlers.ts) and different reactive
// surfaces (the header member chips and the lobby roster, not the chat
// feed). Both surfaces are asserted because they read different state:
// the chips come from `members`, the lobby roster from `players`, and the
// handler has to update both. The lobby's "Start Prologue" button is
// the third derived surface — it's gated on `players.length >= 2`, so
// it doubles as the check that the count, not just the render, updated.
//
// The roster is shared/TableRoster.svelte since
// adr/LOBBY_AND_CHECKLIST_PLAN.md S2 (it was a bare `.player-list` of
// strings). Two consequences the assertions below encode: the current
// player's own row reads "You", never their name — their name lives in the
// row's aria-label — and the Start button now always renders for the
// facilitator, disabled with a reason until the table has two players.

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
  // Scoping to that role avoids matching the join code or other places
  // where the username might appear.
  await expect(alicePage.getByRole('button', { name: /View alice's retinue/ })).toBeVisible();
  await expect(alicePage.getByRole('button', { name: /View bob's retinue/ })).toHaveCount(0);

  // The lobby's own roster, under "The Table". Alice's seat reads "You" —
  // deliberately, so a player can find themselves in the list — with the
  // facilitator tag beside it. She can't start yet: one player short of the
  // 2-player minimum, so the button is present and disabled rather than
  // missing (a hidden control teaches nothing).
  const lobbyRoster = alicePage.locator('.table-roster');
  await expect(lobbyRoster).toContainText('You');
  await expect(lobbyRoster).toContainText('facilitator');
  await expect(lobbyRoster).not.toContainText('bob');
  const startButton = alicePage.getByRole('button', { name: 'Start Prologue' });
  await expect(startButton).toBeDisabled();
  await expect(alicePage.getByText('One more player and you can begin.')).toBeVisible();

  // Bob joins via API — no page navigation needed on his side. The point
  // here is that alice's already-open page picks up the event.
  const joinRes = await bobCtx.request.post('/api/tables/join', {
    data: { join_code: joinCode },
  });
  expect(joinRes.ok(), `bob could not join ${joinCode}`).toBeTruthy();

  // Live update, all three surfaces, no reload: the member chip, the lobby
  // roster, and the start button the roster count gates.
  await expect(alicePage.getByRole('button', { name: /View bob's retinue/ })).toBeVisible();
  await expect(lobbyRoster).toContainText('bob');
  await expect(startButton).toBeEnabled();

  // The "While you wait" tones row is the app's first `action='navigate'`
  // ChecklistRow (adr/LOBBY_AND_CHECKLIST_PLAN.md D1: an arrow opens a
  // panel elsewhere). Its onSelect is the one wiring the SSR unit tests in
  // shared/ChecklistRow.test.ts can't reach, so it is asserted here.
  await alicePage.getByRole('button', { name: /Adjust the tone of the game/ }).click();
  await expect(alicePage.getByRole('heading', { name: 'Tones' })).toBeVisible();

  await aliceCtx.close();
  await bobCtx.close();
});
