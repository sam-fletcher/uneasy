import { test, expect, request as pwRequest, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// End-to-end for Propose Duel — the gnarliest interactive plan flow, and
// the first plan spec to drive a full resolution through the UI.
//
// The duel's *setup* (assets, prep, fast-forwarding the public record to
// the plan's row, and the resolve trigger) is done over the API: it isn't
// what we're testing and driving it through the UI would dwarf the actual
// subject. The interactive, bug-prone part — stake selection, the bout
// declare/respond loop, and the hand-off to the final roll — is driven
// entirely through the rendered panels for both players.
//
// The duel's information model is asserted here too, because it is invisible
// in the UI and so cannot be caught by looking: while a bout is open, the
// responder must be able to read the declarer's CALL (stake + high/low) but
// never the declarer's DIE. See handler/plan_propose_duel_integration_test.go
// for the same property pinned over HTTP with fixed dice.
//
// Dice are server-rolled (handler/rolls_dice.go) so bout outcomes are
// random; assertions check flow progression, never specific dice values. With
// one stake a side the winner's claim is forced (it covers the whole pile), so
// the duel settles itself the instant the final roll lands — no take step and
// no "Complete plan" click for either player.

const E2E = process.env.E2E_BASE_URL ?? 'http://localhost:8090';

// Advance the public record to `targetRow` via the dev fast-forward, which runs
// the real advance (engrailed ranking updates, row.advanced posts) rather than
// writing current_row directly. There is no production row-advance route — see
// adr/FACILITATOR_POWERS_AUDIT.md. Focus is left wherever the advance leaves it.
async function advanceToRow(gameID: number, targetRow: number, ctx: BrowserContext) {
	const res = await ctx.request.post('/api/dev/advance-row', {
		data: { game_id: gameID, advance_to: targetRow },
	});
	expect(res.ok(), `advance-row failed: ${await res.text()}`).toBeTruthy();
}

// The resolving duel panel auto-renders inside <PlanPanel> for every player
// once the plan flips to 'resolving' (PlanPanel.svelte:100). Scope locators
// to the choices section we expect so chip/card lookups stay unambiguous.
function section(page: Page, header: string) {
	return page.locator('.choices-section', { hasText: header });
}

// Stakes are committed in a single step now: check the asset(s), then Commit.
// The count is derived from the selection, and stays hidden from the opponent
// until both sides commit.
async function stakeAsset(page: Page, assetName: string) {
	const sec = section(page, 'Choose assets to stake');
	const card = sec.locator('.card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
	await sec.getByRole('button', { name: /^Commit 1 stake$/ }).click();
}

const track = cleanupGameAfterEach();

test('propose duel: setup → staking → bouts → final roll → settles itself', async ({ browser }) => {
	// ── Seed a main_event game with alice (focus) + bob ──────────────────────
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob'] },
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id, players } = await seedRes.json();
	track(game_id);
	const aliceID: number = players[0].id;
	const bobID: number = players[1].id;
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E });
	const bobCtx = await browser.newContext({ baseURL: E2E });
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');

	// ── Each duelist needs an unleveraged peer to stake ──────────────────────
	const mkPeer = (ctx: BrowserContext, name: string) =>
		ctx.request.post(`/api/tables/${game_id}/assets`, {
			data: { asset_type: 'peer', name, is_main_character: false, marginalia: ['unleveraged'] },
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
	await advanceToRow(game_id, 6, aliceCtx);

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

	// Setup phase: each duellist commits their single staked peer. Alice's
	// stake stays hidden from bob until he also commits, then → bouts.
	await stakeAsset(alice, 'Alice Peer');
	await stakeAsset(bob, 'Bob Peer');

	// Bouts phase: alice has initiative (higher esteem status) → she declares.
	const aliceBout = section(alice, 'Bout');
	await expect(aliceBout).toBeVisible({ timeout: 10_000 });
	await aliceBout.locator('.stake-line.pick', { hasText: 'Alice Peer' }).click();
	await aliceBout.getByRole('button', { name: 'High' }).click();
	await aliceBout.getByRole('button', { name: 'Declare' }).click();

	// Bob responds with his stake; the single bout resolves both stakes.
	const bobBout = section(bob, 'Bout');
	await expect(bobBout.getByRole('button', { name: 'Respond' })).toBeVisible({ timeout: 10_000 });

	// ── The open bout's information model ────────────────────────────────────
	// The call is public: bob can SEE which stake alice opened with and that she
	// called higher, before he chooses his answer…
	// (String form on purpose: it normalises whitespace, whereas a regex matches the
	// raw DOM text, which keeps the template's line breaks.)
	await expect(bobBout.getByText('calling higher')).toBeVisible();
	await expect(bobBout.locator('.open-call')).toContainText('Alice Peer');
	// …and the call is on the table's record too.
	const feed = await bobCtx.request.get(`/api/tables/${game_id}/posts`);
	expect(feed.ok(), `posts: ${await feed.text()}`).toBeTruthy();
	expect(JSON.stringify(await feed.json())).toMatch(/opens with .*Alice Peer.*calling high/);

	// The die is not. The responder must answer blind — reading it from the API
	// is invisible in the UI, which is exactly how it went unnoticed.
	const bobView = await (await bobCtx.request.get(`/api/plans/${duel.id}/duel-state`)).json();
	expect(bobView.bouts, 'one bout declared').toHaveLength(1);
	expect(bobView.bouts[0].declaration).toBe('high');
	expect(
		bobView.bouts[0].declarer_die,
		"the responder must not be able to read the declarer's die",
	).toBeNull();
	// The declarer still sees her own.
	const aliceView = await (await aliceCtx.request.get(`/api/plans/${duel.id}/duel-state`)).json();
	expect(aliceView.bouts[0].declarer_die).toBeGreaterThanOrEqual(1);

	await bobBout.locator('.stake-line.pick', { hasText: 'Bob Peer' }).click();
	await bobBout.getByRole('button', { name: 'Respond' }).click();

	// With one stake each, both run out → the duel hands off to its final roll,
	// which is created already-resolved (finalizeRoll) and whose outcome is
	// applied on the spot. The winner's claim covers the loser's lone stake, so
	// there is nothing to choose: the plan resolves itself — and the preparer
	// (who, on a mar, has just LOST) is never handed a "Complete plan" button.
	await expect.poll(async () => {
		const { plans } = await (await aliceCtx.request.get(`/api/tables/${game_id}/plans`)).json();
		return plans.find((p: { id: number }) => p.id === duel.id)?.status;
	}, { timeout: 10_000, message: 'duel should resolve itself' }).toBe('resolved');

	const { plans: after } = await (await aliceCtx.request.get(`/api/tables/${game_id}/plans`)).json();
	const settled = after.find((p: { id: number }) => p.id === duel.id);
	expect(['make', 'mar']).toContain(settled.result);

	// The duel panel is gone for both players — nothing is left to click.
	await expect(section(alice, 'Bout')).toHaveCount(0);
	await expect(section(bob, 'Bout')).toHaveCount(0);
	await expect(alice.getByRole('button', { name: /Complete plan|Apply result|^Take / })).toHaveCount(0);
	await expect(bob.getByRole('button', { name: /Complete plan|Apply result|^Take / })).toHaveCount(0);

	// The claim really moved the asset: a make is the preparer's win, a mar the
	// target's, and the loser's lone staked peer changes hands.
	const winnerID = settled.result === 'make' ? aliceID : bobID;
	const loserPeerName = settled.result === 'make' ? 'Bob Peer' : 'Alice Peer';
	const assetsRes = await aliceCtx.request.get(`/api/tables/${game_id}/assets`);
	const { assets } = await assetsRes.json();
	const loserPeer = assets.find((a: { name: string }) => a.name === loserPeerName);
	expect(loserPeer, `loser peer ${loserPeerName} missing`).toBeTruthy();
	expect(loserPeer.owner_id).toBe(winnerID);
	// Every staked asset ends leveraged, taken or not.
	for (const name of ['Alice Peer', 'Bob Peer']) {
		const a = assets.find((x: { name: string }) => x.name === name);
		expect(a.is_leveraged, `${name} should end leveraged`).toBe(true);
	}

	await aliceCtx.close();
	await bobCtx.close();
});
