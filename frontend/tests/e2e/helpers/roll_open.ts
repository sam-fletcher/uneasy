import { expect, type Page } from '@playwright/test';

/**
 * Waits until every page has the open roll on screen (the "Dice Roll" panel).
 *
 * Call it between a roll opening and a `/api/dev/force-roll` on it, with every
 * page that is open on the table.
 *
 * Why: when a roll opens, each page hears `roll.created` and then fetches the
 * roll's details; only when that fetch lands does the page know the roll exists.
 * A person can't settle a roll they can't see, but a spec can: force-roll fires
 * a few milliseconds after the roll opens, so `roll.resolved` can reach a page
 * before its fetch has landed. The page used to ignore it and let the late
 * fetch put back the unresolved roll ("Dice roll in progress…" until a reload);
 * that is fixed now (a roll fetch applies only if nothing newer has happened,
 * see `fetchRoll` in the table page's `ws-handlers.ts`). A slow runner
 * (GitHub's) hit it in two specs on the first nightly run; delaying
 * `GET /api/rolls/<id>` by 400 ms made 14 fail. Specs should still wait for the
 * panel, as a player would: it is the signal that the page has the roll.
 */
export async function expectRollOpen(...pages: Page[]): Promise<void> {
	await Promise.all(
		pages.map(page =>
			expect(page.locator('.roll-title', { hasText: 'Dice Roll' }).first(), 'the open roll is on screen').toBeVisible({
				timeout: 10_000,
			}),
		),
	);
}
