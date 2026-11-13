import { test, type APIRequestContext, type Page } from '@playwright/test';
import { THREE_COLUMN_PX, TWO_COLUMN_PX } from '../../src/lib/breakpoints';

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

/** The table page's three panes, by the names on their tabs. */
export type Pane = 'Public Record' | 'Play' | 'Chat';

/**
 * Brings a pane onto the screen by tapping its tab.
 *
 * Below the two-column breakpoint the table shows one pane at a time and the
 * tab bar at the foot chooses it; from there Chat is docked, so it has no tab
 * (the left column's two tabs choose between Public Record and Play); from the
 * three-column breakpoint every pane is a column and there are no tabs at all.
 * The widths come from the app's own `lib/breakpoints.ts`, so this can't drift
 * from the layout. So this taps the tab where the viewport
 * has one, and does nothing where it has not, because then the pane is already
 * on screen. That lets one spec read the same at a phone's width and a
 * desktop's. It decides from the viewport, not from what is on the page yet,
 * so it can't mistake a table that is still loading for one with no tabs.
 */
export async function openPane(page: Page, pane: Pane): Promise<void> {
  const width = page.viewportSize()?.width ?? 1280;
  const hasTab = width < TWO_COLUMN_PX || (width < THREE_COLUMN_PX && pane !== 'Chat');
  if (!hasTab) return;
  await page
    .getByRole('navigation', { name: 'Panes' })
    .getByRole('button', { name: pane })
    .click();
}

/** The table's title in the title bar: "Row 6 · Seek Answers", "Lobby", and so on. */
export function tableTitle(page: Page) {
  return page.getByRole('heading', { level: 1 });
}

/** What the Public Record pane shows: the chips at its foot. */
export type RecordTab = 'Events' | 'Laws' | 'Rumors';

/**
 * Opens one of the Public Record's lists: brings the pane on screen if it is a
 * tab, then taps the chip. Laws and Rumors used to be header buttons that opened
 * sheets; they are the record's own lists now.
 */
export async function openRecordTab(page: Page, tab: RecordTab): Promise<void> {
  await openPane(page, 'Public Record');
  await page
    .getByRole('navigation', { name: 'Public Record' })
    .getByRole('button', { name: tab })
    .click();
}

/** Opens the Tones sheet from the chat header's "Tones · N set" chip. */
export async function openTones(page: Page): Promise<void> {
  await openPane(page, 'Chat');
  await page.getByRole('button', { name: /^Tones/ }).click();
}
