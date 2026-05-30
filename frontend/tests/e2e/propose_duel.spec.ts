import { test, expect, request as pwRequest, type BrowserContext, type Page } from '@playwright/test';

// End-to-end for Propose Duel — the gnarliest interactive plan flow, and
// the first plan spec to drive a full resolution through the UI.
//
// The duel's *setup* (assets, prep, fast-forwarding the public record to
// the plan's row, and the resolve trigger) is done over the API: it isn't
// what we're testing and driving it through the UI would dwarf the actual
// subject. The interactive, bug-prone part — stake-count reveal, stake
// selection, the bout declare/respond loop, and the accumulated-dice
// hand-off — is driven entirely through the rendered panels for both
// players, ending when the duel reaches its final dice roll.
//
// Dice are server-rolled (handler/rolls_dice.go) so bout outcomes are
// random; assertions check flow progression, never specific dice values.

const E2E = 'http://localhost:8090';

interface GameRow {
	current_row: number;
	focus_player_id: number | null;
}

async function fetchGame(ctx: BrowserContext, gameID: number): Promise<GameRow> {
	const res = await ctx.request.get(`/api/tables/${gameID}/state`);
	expect(res.ok(), `GET state failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).game as GameRow;
}

// Advance the public record to `targetRow`, always issuing advance-row from
// whichever player currently holds focus (prep/advance auto-passes the
// focus marker server-side, so it alternates in a 2-player game).
async function advanceToRow(
	gameID: number,
	targetRow: number,
	ctxByPlayer: Record<number, BrowserContext>,
	readCtx: BrowserContext,
) {
	for (let guard = 0; guard < 20; guard++) {
		const game = await fetchGame(readCtx, gameID);
		if (game.current_row >= targetRow) return;
		const focusCtx = game.focus_player_id != null ? ctxByPlayer[game.focus_player_id] : null;
		expect(focusCtx, `no context for focus player ${game.focus_player_id}`).toBeTruthy();
		const res = await focusCtx!.request.post(`/api/tables/${gameID}/advance-row`);
		expect(res.ok(), `advance-row failed: ${await res.text()}`).toBeTruthy();
	}
	throw new Error(`could not reach row ${targetRow} within guard limit`);
}

// The resolving duel panel auto-renders inside <PlanPanel> for every player
// once the plan flips to 'resolving' (PlanPanel.svelte:100). Scope locators
// to the choices section we expect so chip/card lookups stay unambiguous.
function section(page: Page, header: string) {
	return page.locator('.choices-section', { hasText: header });
}

async function submitStakeCount(page: Page, count: string) {
	const sec = section(page, 'Stake count');
	await expect(sec.getByRole('button', { name: 'Submit stake count' })).toBeVisible();
	await sec.getByRole('button', { name: count, exact: true }).click();
	await sec.getByRole('button', { name: 'Submit stake count' }).click();
}

async function stakeAsset(page: Page, assetName: string) {
	const sec = section(page, 'Selecting stakes');
	const card = sec.locator('.card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
	await sec.getByRole('button', { name: /^Stake 1\/1$/ }).click();
}

test('propose duel: setup → staking → bouts → final roll', async ({ browser }) => {
	// ── Seed a main_event game with alice (focus) + bob ──────────────────────
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	await fixture.post('/api/dev/reset');
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob'] },
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id, players } = await seedRes.json();
	const aliceID: number = players[0].id;
	const bobID: number = players[1].id;
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E });
	const bobCtx = await browser.newContext({ baseURL: E2E });
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');
	const ctxByPlayer = { [aliceID]: aliceCtx, [bobID]: bobCtx };

	// ── Each duelist needs an unleveraged peer to stake ──────────────────────
	const mkPeer = (ctx: BrowserContext, name: string) =>
		ctx.request.post(`/api/tables/${game_id}/assets`, {
			data: { asset_type: 'peer', name, is_main_character: false, marginalia: [] },
		});
	expect((await mkPeer(aliceCtx, 'Alice Peer')).ok()).toBeTruthy();
	expect((await mkPeer(bobCtx, 'Bob Peer')).ok()).toBeTruthy();

	// ── Alice (focus) prepares the duel vs bob. delay 5 → lands on row 6 ─────
	const prep = await aliceCtx.request.post(`/api/tables/${game_id}/prepare-plan`, {
		data: {
			plan_type: 'propose_duel',
			target_player_id: bobID,
			duel_type: 'arms',
			preparation_notes: 'On the dueling green at dawn.',
		},
	});
	expect(prep.ok(), `prepare failed: ${await prep.text()}`).toBeTruthy();

	// ── Fast-forward the record to the duel's row, then resolve it ───────────
	await advanceToRow(game_id, 6, ctxByPlayer, aliceCtx);

	const plansRes = await aliceCtx.request.get(`/api/tables/${game_id}/plans`);
	const { plans } = await plansRes.json();
	const duel = plans.find((p: { plan_type: string }) => p.plan_type === 'propose_duel');
	expect(duel, 'duel plan not found').toBeTruthy();

	// The preparer (alice) resolves their own plan. Advancing onto the plan's
	// row auto-kicks-off resolution (plans.go), so the explicit resolve here
	// races with that and may 409 once the plan is already 'resolving'. Either
	// outcome means the duel is now resolving.
	const resolveRes = await aliceCtx.request.post(`/api/plans/${duel.id}/resolve`);
	if (!resolveRes.ok()) {
		expect(await resolveRes.text(), 'unexpected resolve failure').toContain('not in pending status');
	}

	// ── Open both players' tables; the resolving duel panel auto-renders ─────
	const alice = await aliceCtx.newPage();
	const bob = await bobCtx.newPage();
	await Promise.all([
		alice.goto(`/table/${game_id}`),
		bob.goto(`/table/${game_id}`),
	]);

	// Setup phase: each secretly commits to staking 1 asset.
	await submitStakeCount(alice, '1');
	await submitStakeCount(bob, '1');

	// Staking phase: each picks their single peer.
	await stakeAsset(alice, 'Alice Peer');
	await stakeAsset(bob, 'Bob Peer');

	// Bouts phase: alice has initiative (higher esteem status) → she declares.
	const aliceBout = section(alice, 'Bout');
	await expect(aliceBout).toBeVisible({ timeout: 10_000 });
	await aliceBout.locator('.card', { hasText: 'Alice Peer' }).getByRole('checkbox').click();
	await aliceBout.getByRole('button', { name: 'High' }).click();
	await aliceBout.getByRole('button', { name: 'Declare' }).click();

	// Bob responds with his stake; the single bout resolves both stakes.
	const bobBout = section(bob, 'Bout');
	await expect(bobBout.getByRole('button', { name: 'Respond' })).toBeVisible({ timeout: 10_000 });
	await bobBout.locator('.card', { hasText: 'Bob Peer' }).getByRole('checkbox').click();
	await bobBout.getByRole('button', { name: 'Respond' }).click();

	// With one stake each, both run out → duel hands off to the final roll.
	await expect(section(alice, 'The final roll')).toBeVisible({ timeout: 10_000 });
	await expect(section(bob, 'The final roll')).toBeVisible({ timeout: 10_000 });

	await aliceCtx.close();
	await bobCtx.close();
});
