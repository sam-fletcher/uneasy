import { test, type APIRequestContext } from '@playwright/test';

/** Hard-deletes a game via the dev-only endpoint (handler/dev.go: DevDeleteGame). */
export async function deleteGame(request: APIRequestContext, gameId: number): Promise<void> {
  const res = await request.post('/api/dev/delete-game', { data: { game_id: gameId } });
  if (!res.ok()) {
    console.warn(`cleanup: delete-game ${gameId} failed: ${res.status()} ${await res.text()}`);
  }
}

/**
 * Registers an `afterEach` hook that deletes whichever game the test tracks,
 * so the shared e2e database doesn't accumulate games across runs. Call the
 * returned function with a game/table id right after creating or seeding it;
 * cleanup runs even if the test fails partway through.
 */
export function cleanupGameAfterEach(): (gameId: number) => void {
  let gameId: number | undefined;
  test.afterEach(async ({ request }) => {
    if (gameId === undefined) return;
    await deleteGame(request, gameId);
    gameId = undefined;
  });
  return (id: number) => {
    gameId = id;
  };
}
