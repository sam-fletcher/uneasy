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
 * before its fetch has landed. The page then ignores it, and the late fetch
 * puts back the unresolved roll: "Dice roll in progress…" stays up until a
 * reload. A slow runner (GitHub's) hit this in two specs on the first nightly
 * run; delaying `GET /api/rolls/<id>` by 400 ms made 14 fail. Waiting for the
 * panel is the real signal that the page has the roll.
 *
 * (The page's handler is the cause; it is tracked in
 * docs/work/next/roll-fetch-overwrites-resolved.md. Specs should still wait,
 * as a player would.)
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
