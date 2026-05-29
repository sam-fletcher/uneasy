import { test, expect, request as pwRequest, type BrowserContext, type Page } from '@playwright/test';

// End-to-end for Clandestinely Liaise — specifically the "Secrets We Keep"
// hand-off, which had the same soft-lock bug class as Propose Duel: a
// two-sided submission step where one player's action wasn't pushed to the
// other, leaving the waiting party stuck on a stale panel.
//
// As with the duel spec, the *setup* (assets, the prep-time delay reveal,
// fast-forwarding the record to the plan's row, resolving, and advancing to
// the secrets phase) is driven over the API — it isn't the subject. The
// bug-prone part is the keep-secret loop: the preparer submits first, the
// partner submits second, and the preparer's "Advance" affordance must
// appear live, with no manual reload. Pre-fix it never did.

const E2E = 'http://localhost:8090';

interface GameRow {
	current_row: number;
	focus_player_id: number | null;
}

interface PlanRow {
	id: number;
	plan_type: string;
	row_number: number | null;
	resolution_data: string | null;
}

async function fetchGame(ctx: BrowserContext, gameID: number): Promise<GameRow> {
	const res = await ctx.request.get(`/api/tables/${gameID}/state`);
	expect(res.ok(), `GET state failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).game as GameRow;
}

async function fetchLiaisePlan(ctx: BrowserContext, gameID: number): Promise<PlanRow> {
	const res = await ctx.request.get(`/api/tables/${gameID}/plans`);
	expect(res.ok(), `GET plans failed: ${await res.text()}`).toBeTruthy();
	const { plans } = (await res.json()) as { plans: PlanRow[] };
	const cl = plans.find(p => p.plan_type === 'clandestinely_liaise');
	expect(cl, 'liaise plan not found').toBeTruthy();
	return cl!;
}

// Advance the public record to `targetRow`, issuing advance-row from whichever
// player currently holds focus (focus auto-passes server-side in a 2-player
// game). Mirrors the helper in propose_duel.spec.ts.
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

function section(page: Page, header: string) {
	return page.locator('.choices-section', { hasText: header });
}

async function keepSecret(page: Page, assetName: string) {
	const sec = section(page, 'Secrets we keep');
	const card = sec.locator('.card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
	await sec.getByRole('button', { name: 'Keep this secret' }).click();
}

test('clandestinely liaise: secrets-we-keep hand-off reaches the preparer live', async ({ browser }) => {
	// ── Seed a main_event game with alice (preparer) + bob (partner) ─────────
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

	// ── Each player needs an un-leveraged asset to bear the secret ───────────
	const mkPeer = (ctx: BrowserContext, name: string) =>
		ctx.request.post(`/api/tables/${game_id}/assets`, {
			data: { asset_type: 'peer', name, is_main_character: false, marginalia: [] },
		});
	expect((await mkPeer(aliceCtx, 'Alice Confidant')).ok()).toBeTruthy();
	expect((await mkPeer(bobCtx, 'Bob Confidant')).ok()).toBeTruthy();

	// ── Alice prepares the liaison with bob; OnPrepare opens a delay reveal ──
	const prep = await aliceCtx.request.post(`/api/tables/${game_id}/prepare-plan`, {
		data: {
			plan_type: 'clandestinely_liaise',
			target_player_id: bobID,
			preparation_notes: 'A quiet word in the orangery.',
		},
	});
	expect(prep.ok(), `prepare failed: ${await prep.text()}`).toBeTruthy();

	// ── Both submit the delay reveal (face 1 → delay 1) so the plan lands ────
	const planBeforeReveal = await fetchLiaisePlan(aliceCtx, game_id);
	const liaise = JSON.parse(planBeforeReveal.resolution_data ?? '{}').liaise ?? {};
	const revealID: number = liaise.delay_reveal_id;
	expect(revealID, 'delay_reveal_id missing from resolution_data').toBeTruthy();
	for (const ctx of [aliceCtx, bobCtx]) {
		const res = await ctx.request.post(`/api/reveals/${revealID}/submit`, { data: { face: 1 } });
		expect(res.ok(), `reveal submit failed: ${await res.text()}`).toBeTruthy();
	}

	// ── Fast-forward to the plan's row, then resolve it (focus player) ───────
	const planAfterReveal = await fetchLiaisePlan(aliceCtx, game_id);
	expect(planAfterReveal.row_number, 'row_number not set after reveal').toBeTruthy();
	await advanceToRow(game_id, planAfterReveal.row_number!, ctxByPlayer, aliceCtx);

	let focusGame = await fetchGame(aliceCtx, game_id);
	const resolveCtx = ctxByPlayer[focusGame.focus_player_id!];
	const resolveRes = await resolveCtx.request.post(`/api/plans/${planAfterReveal.id}/resolve`);
	// Advancing onto the plan's row auto-kicks-off resolution (plans.go:918), so
	// the explicit resolve here races with that and may 409 once the plan is
	// already 'resolving'. Either outcome means the liaison is now resolving.
	if (!resolveRes.ok()) {
		expect(await resolveRes.text(), 'unexpected resolve failure').toContain('not in pending status');
	}

	// ── Advance together_at_last → secrets_we_keep (focus player) ────────────
	focusGame = await fetchGame(aliceCtx, game_id);
	const advanceCtx = ctxByPlayer[focusGame.focus_player_id!];
	const advanceRes = await advanceCtx.request.post(`/api/plans/${planAfterReveal.id}/advance-liaise`);
	expect(advanceRes.ok(), `advance-liaise failed: ${await advanceRes.text()}`).toBeTruthy();

	// ── Open both players' tables; the resolving liaise panel auto-renders ───
	const alice = await aliceCtx.newPage();
	const bob = await bobCtx.newPage();
	await Promise.all([
		alice.goto(`/table/${game_id}`),
		bob.goto(`/table/${game_id}`),
	]);

	// Both land in the Secrets We Keep phase.
	await expect(section(alice, 'Secrets we keep')).toBeVisible({ timeout: 10_000 });
	await expect(section(bob, 'Secrets we keep')).toBeVisible({ timeout: 10_000 });

	// Preparer (alice) submits FIRST — this is the soft-lock ordering. Her own
	// refetch leaves her at 1/2 submitted, waiting on bob.
	await keepSecret(alice, 'Alice Confidant');
	await expect(section(alice, 'Secrets we keep').getByText(/Waiting for/)).toBeVisible();

	// Partner (bob) submits SECOND. Pre-fix this emitted no event, so alice's
	// panel never refetched and her advance button never appeared. With the
	// fix, the broadcast drives alice's plan refetch and the button shows live.
	await keepSecret(bob, 'Bob Confidant');
	await expect(
		section(alice, 'Secrets we keep').getByRole('button', { name: 'Advance to Things We Share' }),
	).toBeVisible({ timeout: 10_000 });

	await aliceCtx.close();
	await bobCtx.close();
});
