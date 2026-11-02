import { test, expect, request as pwRequest, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';
import {
	cleanupDuelsAfterEach, seedDuel, addPeer, openDuel, openTable, duelWithDice, aliceCall, duelState,
	selectStakes, declare, respond, duelLog, planWithData, rowState, assetsOf, section, openTableWithFrames, planJSON,
	type Duel, type DiceDuel, type Who,
} from './helpers/propose_duel';

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


// ─────────────────────────────────────────────────────────────────────────────
// Audit regression specs. The helpers (helpers/propose_duel.ts) seed a five-
// player game with bob last on the esteem track (difficulty 1, at most two
// stakes) and let each duellist read their OWN dice, so a spec can choose the
// declaration that makes a given duellist win the bout — and with it the
// outcome: with one bout, the bout winner takes both dice, so alice winning is
// a make and bob winning is a mar. Everything the UI shows is read at 375px.
// ─────────────────────────────────────────────────────────────────────────────

const PHONE = { width: 375, height: 812 };
cleanupDuelsAfterEach();

function bodies(posts: string[], needle: string) {
	return posts.filter(p => p.includes(needle));
}

/** Waits for the log to carry a post containing `needle`, then returns it. */
async function logHas(g: Duel, who: Who, needle: string): Promise<string> {
	await expect.poll(async () => bodies(await duelLog(g, who), needle).length, {
		message: `log never said "${needle}"`,
	}).toBeGreaterThan(0);
	return bodies(await duelLog(g, who), needle)[0];
}

/** Plays bout 1 through the UI: alice calls, bob answers. */
async function playBoutInUI(alice: Page, bob: Page, call: 'high' | 'low', aliceStake: string, bobStake: string) {
	const aBout = section(alice, 'Bout');
	await aBout.locator('.stake-line.pick', { hasText: aliceStake }).click();
	await aBout.getByRole('button', { name: call === 'high' ? 'Higher' : 'Lower' }).click();
	await aBout.getByRole('button', { name: 'Declare' }).click();
	const bBout = section(bob, 'Bout');
	await expect(bBout.getByRole('button', { name: 'Respond' })).toBeVisible({ timeout: 10_000 });
	await bBout.locator('.stake-line.pick', { hasText: bobStake }).click();
	await bBout.getByRole('button', { name: 'Respond' }).click();
}

test('setup: who duels, blind stakes, what a bystander sees', async ({ browser }) => {
	const g = await seedDuel(browser, { viewport: PHONE });
	const a1 = await addPeer(g, 'alice', 'Alice Peer 1');
	const a2 = await addPeer(g, 'alice', 'Alice Peer 2');
	const b1 = await addPeer(g, 'bob', 'Bob Peer 1');
	const b2 = await addPeer(g, 'bob', 'Bob Peer 2');
	const b3 = await addPeer(g, 'bob', 'Bob Peer 3');
	await openDuel(g);
	// Every socket frame the opponent and the bystander receive, from the start:
	// the blind commit must not leak through the live path either.
	const frames: Record<Who, string[]> = { alice: [], bob: [], carol: [] };
	const [alice, bob, carol] = await Promise.all([
		openTableWithFrames(g, 'alice', frames.alice),
		openTableWithFrames(g, 'bob', frames.bob),
		openTableWithFrames(g, 'carol', frames.carol),
	]);

	// The arms/wits framing is the heading, and the main character is the default
	// duellist. The bystander sees the matchup but no controls.
	await expect(alice.getByText('Duel of arms — who fights for you?')).toBeVisible({ timeout: 10_000 });
	await expect(alice.getByText('fights for themselves')).toBeVisible();
	await expect(bob.getByText('Duel of arms — who fights for you?')).toBeVisible();
	await expect(carol.getByText('The duellists are choosing their stakes.')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Send someone else' })).toHaveCount(0);
	await expect(carol.getByRole('button', { name: /^Commit/ })).toHaveCount(0);

	// Sending a peer in your stead: the picker holds a local choice, the log gets
	// ONE line on Confirm, and the opponent's "Facing …" line follows live.
	await alice.getByRole('button', { name: 'Send someone else' }).click();
	await section(alice, 'who fights for you').locator('.card', { hasText: 'Alice Peer 1' }).getByRole('checkbox').click();
	await alice.getByRole('button', { name: 'Confirm' }).click();
	await expect(alice.getByText('Alice Peer 1 fights for you.')).toBeVisible();
	await logHas(g, 'bob', 'elects Alice Peer 1 as their champion');
	await expect(bob.getByText('Facing Alice Peer 1.')).toBeVisible();
	await expect(carol.getByText("Alice Peer 1 vs bob's main character.")).toBeVisible();
	expect(bodies(await duelLog(g, 'bob'), 'champion')).toHaveLength(1);

	// Stake limits, as exact statuses. Bob is last on the track: two stakes at most.
	expect((await selectStakes(g, 'bob', [b1, b2, b3])).status(), 'over the esteem cap').toBe(400);
	expect((await selectStakes(g, 'bob', [a1])).status(), "someone else's asset").toBe(403);
	expect((await selectStakes(g, 'carol', [a2])).status(), 'a bystander cannot stake').toBe(403);
	expect((await selectStakes(g, 'bob', [])).status(), 'at least one stake').toBe(400);
	await expect(section(bob, 'Choose assets to stake')).toContainText('pick 1–2');

	// Alice commits one stake through the UI. Her commit is private: bob's view of
	// the duel holds none of her stakes, and nothing on the log gives her count away.
	const aStake = section(alice, 'Choose assets to stake');
	await aStake.locator('.card', { hasText: 'Alice Peer 2' }).getByRole('checkbox').click();
	await aStake.getByRole('button', { name: 'Commit 1 stake' }).click();
	await expect(alice.getByText("You've staked 1 asset.")).toBeVisible();
	expect((await duelState(g, 'bob')).stakes, "bob must not see alice's stakes yet").toHaveLength(0);
	expect((await duelState(g, 'carol')).stakes, 'a bystander sees no stakes in setup').toHaveLength(0);
	expect(bodies(await duelLog(g, 'bob'), 'on the line'), 'the stake log waits for both').toHaveLength(0);
	// Nor does her stake COUNT leak through the plan: resolution_data goes to every
	// client, so the counts are only written once both have committed.
	for (const w of ['bob', 'carol'] as Who[]) {
		expect(await planJSON(g, w), `${w} must not read a stake count yet`).not.toMatch(/stake_count/);
		expect(frames[w].join('\n'), `${w}'s socket frames carry no stake count`).not.toMatch(/stake_count/);
	}
	// Bob is the only one still owing a commit, and the bar says so.
	expect(await rowState(g)).toMatchObject({ kind: 'await_duel_staking', acting_player_ids: [g.ids.bob] });
	expect((await selectStakes(g, 'alice', [a2])).status(), 'a second commit').toBe(409);

	// Bob commits; both lay their stakes down together and the bouts begin for all.
	const bStake = section(bob, 'Choose assets to stake');
	await bStake.locator('.card', { hasText: 'Bob Peer 2' }).getByRole('checkbox').click();
	await bStake.getByRole('button', { name: 'Commit 1 stake' }).click();
	for (const page of [alice, bob, carol]) {
		await expect(section(page, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
	}
	// The counts appear only now (so the absence above was not vacuous).
	await expect.poll(() => planJSON(g, 'carol')).toMatch(/preparer_stake_count/);
	expect(await logHas(g, 'carol', 'lays 1 stake on the line')).toContain('hidden die');
	expect(bodies(await duelLog(g, 'carol'), 'on the line')).toHaveLength(2);
	// Alice (top of the track) opens; the bystander gets no declare controls.
	await expect(section(alice, 'Bout').getByRole('button', { name: 'Declare' })).toBeVisible();
	await expect(section(carol, 'Bout').getByRole('button', { name: /Declare|Respond/ })).toHaveCount(0);
	// Real stake ids, so the refusals below come from the turn/seat checks and not
	// from an unknown or foreign stake.
	const aliceStakeID = (await duelState(g, 'alice')).stakes.find(x => x.player_id === g.ids.alice)!.id;
	const bobStakeID = (await duelState(g, 'bob')).stakes.find(x => x.player_id === g.ids.bob)!.id;
	expect((await declare(g, 'carol', aliceStakeID, 'high')).status(), 'a bystander cannot declare').toBe(403);
	expect((await declare(g, 'bob', bobStakeID, 'high')).status(), 'only the player with initiative declares').toBe(403);
	expect((await respond(g, 'alice', aliceStakeID)).status(), 'no bout is open to answer').toBe(409);
	expect((await respond(g, 'carol', bobStakeID)).status(), 'a bystander cannot answer').toBe(403);
});

test('bouts: each viewer sees only their own dice — over the API, the DOM and the socket', async ({ browser }) => {
	const g = await seedDuel(browser, { viewport: PHONE });
	const aIDs = [await addPeer(g, 'alice', 'Alice Peer 1'), await addPeer(g, 'alice', 'Alice Peer 2')];
	const bIDs = [await addPeer(g, 'bob', 'Bob Peer 1'), await addPeer(g, 'bob', 'Bob Peer 2')];
	await openDuel(g);
	expect((await selectStakes(g, 'alice', aIDs)).ok()).toBeTruthy();
	expect((await selectStakes(g, 'bob', bIDs)).ok()).toBeTruthy();

	// Record every socket frame each page receives BEFORE it loads the table, so
	// the live path (the bout_declared push) is covered, not just the fetch.
	const frames: Record<Who, string[]> = { alice: [], bob: [], carol: [] };
	const pages = {} as Record<Who, Page>;
	for (const w of ['alice', 'bob', 'carol'] as Who[]) pages[w] = await openTableWithFrames(g, w, frames[w]);
	const { alice, bob, carol } = pages;

	const aliceDice = (await duelState(g, 'alice')).stakes.filter(s => s.player_id === g.ids.alice);
	expect(aliceDice.map(s => s.hidden_die).every(d => d != null && d >= 1 && d <= 6)).toBe(true);
	// Nobody can read the other side's dice, at any phase of the bouts.
	const seenByBob = (await duelState(g, 'bob')).stakes.filter(s => s.player_id === g.ids.alice);
	const seenByCarol = (await duelState(g, 'carol')).stakes;
	expect(seenByBob).toHaveLength(2);
	expect(seenByBob.every(s => s.hidden_die === null)).toBe(true);
	expect(seenByCarol).toHaveLength(4);
	expect(seenByCarol.every(s => s.hidden_die === null), 'a bystander reads no die').toBe(true);

	// DOM: bob sees his own dice too (not only alice), and the duellist's own side shows real numbers, the other side and a
	// bystander's whole view show "?". (Positive check first: the tracker rendered.)
	const aBout = section(alice, 'Bout');
	await expect(aBout.locator('.duel-side.is-mine .stake-line')).toHaveCount(2, { timeout: 10_000 });
	const aliceShown = await aBout.locator('.duel-side.is-mine .die').allTextContents();
	expect(aliceShown.map(Number).sort()).toEqual(aliceDice.map(s => s.hidden_die as number).sort());
	await expect(aBout.locator('.duel-side:not(.is-mine) .die')).toHaveText(['?', '?']);
	const bobDice = (await duelState(g, 'bob')).stakes.filter(s => s.player_id === g.ids.bob);
	const bobShown = await section(bob, 'Bout').locator('.duel-side.is-mine .die').allTextContents();
	expect(bobShown.map(Number).sort()).toEqual(bobDice.map(s => s.hidden_die as number).sort());
	const cBout = section(carol, 'Bout');
	await expect(cBout.locator('.duel-side .stake-line')).toHaveCount(4, { timeout: 10_000 });
	const carolDice = await cBout.locator('.duel-side .stake-line .die').allTextContents();
	expect(carolDice.filter(d => d !== '?'), 'a bystander sees no die value').toEqual([]);

	// Alice opens through the UI with her first stake. Bob's page updates live.
	const bBout = section(bob, 'Bout');
	await aBout.locator('.stake-line.pick', { hasText: 'Alice Peer 1' }).click();
	await aBout.getByRole('button', { name: 'Lower' }).click();
	await aBout.getByRole('button', { name: 'Declare' }).click();
	await expect(bBout.locator('.open-call')).toContainText('Alice Peer 1', { timeout: 10_000 });
	await expect(bBout.getByText('calling lower')).toBeVisible();
	await expect(bBout.getByText('you can see your own die, not theirs', { exact: false })).toBeVisible();
	await expect(cBout.getByText('must answer', { exact: false })).toBeVisible();
	await expect(bBout.locator('.duel-side:not(.is-mine) .die')).toHaveText(['?', '?']);

	// The call is public — on the wire and on the log — but no die travels with it.
	// Every frame each page has received so far (duel.*, plan.*, roll.*, row state,
	// including alice's own) is searched for a die-bearing key.
	const DIE_KEYS = /"(declarer_die|responder_die|hidden_die)"/;
	for (const w of ['alice', 'bob', 'carol'] as Who[]) {
		expect(frames[w].some(f => f.includes('duel.bout_declared')), `${w} was pushed the declaration`).toBe(true);
		for (const f of frames[w]) expect(f, `${w}'s frames carry no die`).not.toMatch(DIE_KEYS);
	}
	const open = await logHas(g, 'bob', 'opens with Alice Peer 1, calling low');
	expect(open).toContain('bob must answer');
	expect(bodies(await duelLog(g, 'bob'), 'rolled'), 'no dice on the log while the bout is open').toHaveLength(0);
	const openBob = (await duelState(g, 'bob')).bouts;
	expect(openBob).toHaveLength(1);
	expect(openBob[0].declaration).toBe('low');
	expect(openBob[0].declarer_die, 'the responder cannot read the declarer die').toBeNull();
	expect((await duelState(g, 'carol')).bouts[0].declarer_die).toBeNull();
	expect((await duelState(g, 'alice')).bouts[0].declarer_die).toBe(aliceDice.find(s => s.id === openBob[0].declarer_stake_id)?.hidden_die);

	// Bob answers with his first stake; the bout resolves and its dice go public.
	await bBout.locator('.stake-line.pick', { hasText: 'Bob Peer 1' }).click();
	await bBout.getByRole('button', { name: 'Respond' }).click();
	for (const page of [alice, bob, carol]) {
		await expect(section(page, 'Bout 2 of 2')).toBeVisible({ timeout: 10_000 });
		const resolved = section(page, 'Bout').locator('.stake-line.resolved .die');
		await expect(resolved).toHaveCount(2);
		for (const d of await resolved.allTextContents()) expect(d).toMatch(/^[1-6]$/);
	}
	const round = await logHas(g, 'carol', 'Bout 1: alice rolled');
	expect(round).toMatch(/bob rolled \d/);
	// The second bout's dice stay sealed: bob (declarer now) still can't see
	// alice's remaining die, and the bystander still can't see either.
	const nowBob = (await duelState(g, 'bob')).stakes;
	expect(nowBob.filter(s => s.player_id === g.ids.alice && !s.is_resolved).map(s => s.hidden_die)).toEqual([null]);
	const nowCarol = (await duelState(g, 'carol')).stakes;
	expect(nowCarol.filter(s => !s.is_resolved).map(s => s.hidden_die)).toEqual([null, null]);
	await expect(section(bob, 'Bout').locator('.duel-side:not(.is-mine) .stake-line:not(.resolved) .die')).toHaveText(['?']);
	// Bout 2 is under way and the bystander is told who declares it. (Whether
	// each duellist's own page agrees is the 'both duellists agree' test below.)
	// The responder's own page always has the new initiative (their click refreshes
	// the plan): bob declares bout 2, since initiative passes to the other side.
	// The OTHER pages agreeing is the test further down.
	await expect(section(bob, 'Bout')).toContainText('bob to declare');
	await expect(section(bob, 'Bout').getByRole('button', { name: 'Declare' })).toBeVisible();
});

test('make: the winner takes the loser’s stake, everything staked is leveraged', async ({ browser }) => {
	const g: DiceDuel = await duelWithDice(browser, 'aliceWins', { alice: 1, bob: 1 }, { viewport: PHONE });
	const [alice, bob, carol] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob'), openTable(g, 'carol')]);
	await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
	// Initiative is only surfaced here, in the bouts: the setup never named it.
	await expect(section(carol, 'Bout').getByText('alice to declare')).toBeVisible();

	await playBoutInUI(alice, bob, aliceCall(g, true), 'Alice Peer 1', 'Bob Peer 1');

	await expect.poll(async () => (await planWithData(g)).plan.status, { timeout: 10_000 }).toBe('resolved');
	const done = await planWithData(g);
	expect(done.plan.result).toBe('make');
	// The claim is recorded as a committed choice even though nobody was asked.
	expect(done.resolution_data?.choice_submitted).toBe(true);
	expect(done.resolution_data?.duel?.phase).toBe('done');

	const settled = await logHas(g, 'carol', 'The bouts are settled');
	expect(settled).toContain('alice carries 2 dice');
	expect(settled).toContain('bob carries 0');
	expect(await logHas(g, 'carol', 'won the duel')).toMatch(/alice won the duel and took 1 staked asset from bob: .*Bob Peer 1.*all staked assets are leveraged/);
	const assets = await assetsOf(g);
	expect(assets.find(a => a.name === 'Bob Peer 1')).toMatchObject({ owner_id: g.ids.alice, is_leveraged: true });
	expect(assets.find(a => a.name === 'Alice Peer 1')).toMatchObject({ owner_id: g.ids.alice, is_leveraged: true });

	// Nobody has anything left to click — not the winner, not the loser, not a bystander.
	for (const page of [alice, bob, carol]) {
		await expect(section(page, 'Bout')).toHaveCount(0);
		await expect(page.getByRole('button', { name: /Complete plan|Apply result|^Take / })).toHaveCount(0);
	}
	// And the Waiting On bar no longer holds the duel's seats.
	expect((await rowState(g)).kind).not.toMatch(/await_duel/);
});

test('make with a real pick: the preparer chooses which of the target’s stakes to take', async ({ browser }) => {
	// Bob is fourth on the track: difficulty 2 and up to three stakes. Alice's
	// two winning dice make 2, against a pile of 3 — she picks two.
	const g = await duelWithDice(browser, 'aliceWins', { alice: 1, bob: 3 }, { viewport: PHONE, bobRank: 4 });
	const [alice, bob, carol] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob'), openTable(g, 'carol')]);
	await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
	await playBoutInUI(alice, bob, aliceCall(g, true), 'Alice Peer 1', 'Bob Peer 1');

	const claim = section(alice, 'The final roll');
	await expect(claim.getByText("Choose 2 of bob's staked assets to take:")).toBeVisible({ timeout: 10_000 });
	await expect(bob.getByText('Waiting for alice to claim stakes…')).toBeVisible();
	await expect(carol.getByText('Waiting for alice to claim stakes…')).toBeVisible();
	expect(await rowState(g)).toMatchObject({ kind: 'plan_resolving' });
	// The picker is capped at the budget: a third card cannot be ticked.
	await claim.locator('.card', { hasText: 'Bob Peer 1' }).getByRole('checkbox').click();
	await expect(claim.getByRole('button', { name: 'Take 1/2' }), 'one pick is not enough').toBeDisabled();
	await claim.locator('.card', { hasText: 'Bob Peer 3' }).getByRole('checkbox').click();
	await expect(claim.locator('.card', { hasText: 'Bob Peer 2' }).getByRole('checkbox'), 'a third pick is refused').toBeDisabled();
	await expect(claim.getByRole('button', { name: 'Take 2/2' })).toBeEnabled();
	await claim.getByRole('button', { name: 'Take 2/2' }).click();

	await expect.poll(async () => (await planWithData(g)).plan.status, { timeout: 10_000 }).toBe('resolved');
	expect((await planWithData(g)).plan.result).toBe('make');
	const assets = await assetsOf(g);
	for (const n of ['Bob Peer 1', 'Bob Peer 3']) expect(assets.find(a => a.name === n)).toMatchObject({ owner_id: g.ids.alice, is_leveraged: true });
	expect(assets.find(a => a.name === 'Bob Peer 2')).toMatchObject({ owner_id: g.ids.bob, is_leveraged: true });
	expect(await logHas(g, 'bob', 'won the duel')).toContain('took 2 staked assets from bob');
	for (const page of [alice, bob, carol]) await expect(section(page, 'The final roll')).toHaveCount(0);
});

// After a bout resolves the server hands initiative to the other side. The
// bout.resolved broadcast used to fire before that was saved, so the DECLARER's
// page (not the one who just clicked) sometimes refetched the old initiative and
// offered a Declare button the server refuses. It was intermittent (about one run
// in four), so this loops over a few games rather than trusting a single one.
test('bout 2: both duellists agree whose turn it is', async ({ browser }) => {
	for (let i = 0; i < 6; i++) {
		const g = await duelWithDice(browser, 'aliceWins', { alice: 2, bob: 2 }, { viewport: PHONE });
		const [alice, bob, carol] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob'), openTable(g, 'carol')]);
		await expect(section(alice, 'Bout 1 of 2')).toBeVisible({ timeout: 10_000 });
		await playBoutInUI(alice, bob, aliceCall(g, true), 'Alice Peer 1', 'Bob Peer 1');
		await expect(section(bob, 'Bout 2 of 2')).toBeVisible({ timeout: 10_000 });
		const initiative = (await planWithData(g)).resolution_data?.duel as { initiative_player_id?: number } | undefined;
		const next: Who = initiative?.initiative_player_id === g.ids.alice ? 'alice' : 'bob';
		const other: Who = next === 'alice' ? 'bob' : 'alice';
		const pages = { alice, bob };
		await expect(section(carol, 'Bout'), "the bystander's header names the real declarer").toContainText(`${next} to declare`);
		await expect(section(pages[next], 'Bout').getByRole('button', { name: 'Declare' })).toBeVisible();
		await expect(section(pages[other], 'Bout').getByRole('button', { name: 'Declare' }), `${other}'s page offers a turn it does not have`).toHaveCount(0);
	}
});

// ─────────────────────────────────────────────────────────────────────────────
// OWNER QUESTION (docs/work/questions/propose-duel-take-compulsory.md): is the
// take after a mar compulsory? The code says yes — the mar winner takes exactly
// the difficulty (capped by the pile), and a forced claim applies itself — though
// the book words it as "may take". Everything in this block follows the CURRENT
// implementation, and changes if the owner rules "may take": the forced mar's
// auto-claim, the tie's automatic mar claim, and the picker that offers only an
// exact take (Take 1/1, with nothing smaller allowed).
// ─────────────────────────────────────────────────────────────────────────────
test.describe('current implementation: the take after a mar is compulsory', () => {
	test('mar: the target takes the preparer’s stake and the preparer is not asked to close it', async ({ browser }) => {
		const g = await duelWithDice(browser, 'bobWins', { alice: 1, bob: 1 }, { viewport: PHONE });
		const [alice, bob, carol] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob'), openTable(g, 'carol')]);
		await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });

		await playBoutInUI(alice, bob, aliceCall(g, false), 'Alice Peer 1', 'Bob Peer 1');

		await expect.poll(async () => (await planWithData(g)).plan.status, { timeout: 10_000 }).toBe('resolved');
		const done = await planWithData(g);
		expect(done.plan.result).toBe('mar');
		expect(done.resolution_data?.choice_submitted).toBe(true);

		const settled = await logHas(g, 'alice', 'The bouts are settled');
		expect(settled).toContain('alice carries 0 dice');
		expect(await logHas(g, 'alice', 'won the duel')).toMatch(/bob won the duel and took 1 staked asset from alice: .*Alice Peer 1/);
		const assets = await assetsOf(g);
		expect(assets.find(a => a.name === 'Alice Peer 1')).toMatchObject({ owner_id: g.ids.bob, is_leveraged: true });
		expect(assets.find(a => a.name === 'Bob Peer 1')).toMatchObject({ owner_id: g.ids.bob, is_leveraged: true });

		// The preparer just lost: no "Complete plan" lands on her. Same for the rest.
		for (const page of [alice, bob, carol]) {
			await expect(section(page, 'Bout')).toHaveCount(0);
			await expect(page.getByRole('button', { name: /Complete plan|Apply result|^Take / })).toHaveCount(0);
		}
	});

	test('mar with a real pick: the target chooses, everyone else waits on them', async ({ browser }) => {
		// Alice stakes three, bob one: one bout, bob wins it, and the mar lets him take
		// 1 (the difficulty) of her 3 — a genuine choice, so the duel waits for it.
		const g = await duelWithDice(browser, 'bobWins', { alice: 3, bob: 1 }, { viewport: PHONE });
		const [alice, bob, carol] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob'), openTable(g, 'carol')]);
		await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
		await playBoutInUI(alice, bob, aliceCall(g, false), 'Alice Peer 1', 'Bob Peer 1');

		const claim = section(bob, 'The final roll');
		await expect(claim.getByText("Choose 1 of alice's staked asset to take:")).toBeVisible({ timeout: 10_000 });
		await expect(alice.getByText('Waiting for bob to claim stakes…')).toBeVisible();
		await expect(carol.getByText('Waiting for bob to claim stakes…')).toBeVisible();
		await expect(section(alice, 'The final roll').getByRole('button', { name: /^Take/ })).toHaveCount(0);
		expect((await planWithData(g)).plan.status).toBe('resolving');
		// The bar names the mar winner, not the preparer.
		expect(await rowState(g)).toMatchObject({ kind: 'plan_resolving', acting_player_ids: [g.ids.bob] });
		// Nothing leveraged and nothing moved until he chooses.
		expect((await assetsOf(g)).filter(a => a.name.startsWith('Alice Peer')).every(a => !a.is_leveraged)).toBe(true);

		// The claim is the mar winner's alone, and bounded by the budget.
		const claimRoute = (who: Who, ids: number[]) => g.ctx[who].request.post(`/api/plans/${g.planID}/make-choice`, {
			data: { result: 'mar', choices: ids.map(String) },
		});
		expect((await claimRoute('alice', [g.aliceAssets[1]])).status(), 'the loser cannot claim').toBe(403);
		expect((await claimRoute('bob', [g.aliceAssets[1], g.aliceAssets[2]])).status(), 'over budget').toBe(422);
		expect((await planWithData(g)).plan.status).toBe('resolving');

		await claim.locator('.card', { hasText: 'Alice Peer 2' }).getByRole('checkbox').click();
		await claim.getByRole('button', { name: 'Take 1/1' }).click();
		await expect.poll(async () => (await planWithData(g)).plan.status, { timeout: 10_000 }).toBe('resolved');
		expect((await planWithData(g)).plan.result).toBe('mar');
		expect(await logHas(g, 'carol', 'won the duel')).toMatch(/bob won the duel and took 1 staked asset from alice: .*Alice Peer 2/);
		const assets = await assetsOf(g);
		expect(assets.find(a => a.name === 'Alice Peer 2')).toMatchObject({ owner_id: g.ids.bob, is_leveraged: true });
		// The two she kept are still hers, and still leveraged.
		for (const n of ['Alice Peer 1', 'Alice Peer 3']) {
			expect(assets.find(a => a.name === n)).toMatchObject({ owner_id: g.ids.alice, is_leveraged: true });
		}
		for (const page of [alice, bob, carol]) await expect(section(page, 'The final roll')).toHaveCount(0);
	});

	test('a tie on the final bout costs the dice: no dice, a mar, and the log says why', async ({ browser }) => {
		const g = await duelWithDice(browser, 'tie', { alice: 1, bob: 1 }, { viewport: PHONE });
		const [alice, bob] = await Promise.all([openTable(g, 'alice'), openTable(g, 'bob')]);
		await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
		await playBoutInUI(alice, bob, 'high', 'Alice Peer 1', 'Bob Peer 1');

		await expect.poll(async () => (await planWithData(g)).plan.status, { timeout: 10_000 }).toBe('resolved');
		expect((await planWithData(g)).plan.result, 'a drawn 1v1 rolls zero dice').toBe('mar');
		expect(await logHas(g, 'alice', 'an even match')).toContain('nobody wins the dice');
		expect(await logHas(g, 'alice', 'The bouts are settled')).toContain('The 2 dice set aside by the final tie were won by nobody and drop out');
		expect((await assetsOf(g)).find(a => a.name === 'Alice Peer 1')).toMatchObject({ owner_id: g.ids.bob });
	});

});

// Known bug, not fixed here — docs/work/next/claim-non-staked-asset-returns-500.md.
// A claim naming an asset the loser never staked is a client error, but the
// server answers 500 (ApplyChoice returns a plain error that the route maps to
// an internal failure).
test.fixme('a claim naming a non-staked asset is refused with a 4xx, not a 500', async ({ browser }) => {
	const g = await duelWithDice(browser, 'bobWins', { alice: 3, bob: 1 }, { viewport: PHONE });
	const spare = await addPeer(g, 'alice', 'Alice Spare'); // owned, never staked
	const alice = await openTable(g, 'alice');
	const bob = await openTable(g, 'bob');
	await expect(section(alice, 'Bout 1 of 1')).toBeVisible({ timeout: 10_000 });
	await playBoutInUI(alice, bob, aliceCall(g, false), 'Alice Peer 1', 'Bob Peer 1');
	await expect(section(bob, 'The final roll')).toContainText("Choose 1 of alice's staked asset to take:", { timeout: 10_000 });
	const res = await g.ctx.bob.request.post(`/api/plans/${g.planID}/make-choice`, {
		data: { result: 'mar', choices: [String(spare)] },
	});
	expect(res.status()).toBe(422);
	expect((await planWithData(g)).plan.status).toBe('resolving');
});
