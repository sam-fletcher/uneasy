import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedWarGame, runScene, declareWar, joinWar, revealFaces, advanceTo, warAtCostRow, warState,
	assetNamed, frontPayer, openAll, waitingBar, expectLogged, postWith, closeContextsAfterEach,
	type WarGame, type Who,
} from './helpers/make_war';

// Regression specs for what the 12-plan UX audit fixed in Make War:
//   · declaring war: pick the enemies, give a casus belli; the war opens a
//     delay vote (average of the revealed faces, rounded up) that anyone not
//     yet in may join either side of, or sit out;
//   · the cost of battle is owed per opponent, in reverse power order, and is
//     shown in the PLAY AREA to everyone (audit #2: it used to hide behind a
//     32px header chip); the Waiting On bar names the one payer;
//   · peace (audit #8): the bar names the voters, a rejection is announced
//     and hands the gate back to the payer, unanimity ends the war;
//   · surrender is a form of paying the cost; when it empties a side the war
//     ends in the same call, the claims stay open, and the panel stays up for
//     the claimant (f6d17fd, fa13795) while bystanders are told whose choice
//     the table is waiting on.
//
// Setup goes over the API; the panels under test are driven through the UI.
// Dev seed ranks make carol the lowest power, then bob, then alice, so the
// payment order is carol, bob, alice (the specs read it from the war rather
// than assume it). Every seeded player owns "<name>'s holding", "...'s
// artifact" and so on, each with one marginalia.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const NOTES = 'The border has been insulted once too often.';

async function game(browser: Parameters<typeof seedWarGame>[0], viewport = { width: 375, height: 812 }) {
	const g = await seedWarGame(browser, viewport);
	track(g.gameID);
	return g;
}

/** alice (side 1) and carol (side 1) against bob; carol pays first at row 5. */
function threeWayWar(g: WarGame) {
	return warAtCostRow(g, { joins: [['carol', 1]], faces: { alice: 2, bob: 3, carol: 1 } });
}

/** Open a peace proposal from the payer's cost-of-battle form. */
async function proposePeace(page: Page, opponent: string, terms: string) {
	await opponentChip(page, opponent);
	await page.getByRole('button', { name: 'Propose peace' }).click();
	await page.getByPlaceholder('Describe the terms you propose…').fill(terms);
	await page.getByRole('button', { name: 'Propose peace' }).last().click();
}

/** Tick the (only) marginalia on `assetName`'s card in the "Your cost of battle" picker. */
async function tearMarginalia(page: Page, assetName: string) {
	const form = page.locator('.choices-section', { hasText: 'Your cost of battle' });
	await form.locator('.card', { hasText: assetName }).locator('.m-tap').click();
}

async function opponentChip(page: Page, name: string) {
	await page.locator('.choices-section', { hasText: 'Your cost of battle' })
		.locator('.player-chip', { hasText: name }).click();
}

async function planRow(g: WarGame): Promise<number | null> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as { plans: { id: number; row_number: number | null }[] };
	return plans.find(p => p.id === g.planID)!.row_number;
}

test('declaring war: enemies and a casus belli are both required, and the declaration is public', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	const alice = await g.ctx.alice.newPage();
	await alice.goto(`/table/${g.gameID}`);

	await alice.getByText('Make War', { exact: true }).click();
	const prepare = alice.getByRole('button', { name: 'Prepare Plan' });
	await expect(prepare).toBeDisabled();

	await alice.locator('.player-chip', { hasText: 'bob' }).click();
	await expect(prepare).toBeDisabled(); // enemies alone are not enough
	await alice.getByPlaceholder('Casus belli, opening move, rally cry, et cetera').fill(NOTES);
	await expect(prepare).toBeEnabled();
	await prepare.click();

	await expect.poll(async () => {
		const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
		const { plans } = (await res.json()) as { plans: { plan_type: string; preparer_id: number; resolution_data: string }[] };
		const war = plans.find(p => p.plan_type === 'make_war');
		return war ? [war.preparer_id, JSON.parse(war.resolution_data).make_war.enemy_player_ids] : null;
	}).toEqual([g.ids.alice, [g.ids.bob]]);

	await expectLogged(g, 'declaring war on');
});

test('declaring war: the server refuses a war on nobody, on yourself, or on someone outside the game', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	const prep = (enemy_player_ids: number[]) => g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'make_war', enemy_player_ids, preparation_notes: NOTES },
	});
	expect((await prep([])).status()).toBe(400);
	expect((await prep([g.ids.bob, g.ids.bob])).status()).toBe(400);
	expect((await prep([g.ids.alice])).status()).toBe(400);
	expect((await prep([999_999])).status()).toBe(400);
});

test('delay vote: a bystander sees the sides and the join choices; the participants do not', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	await declareWar(g);
	const { alice, bob, carol } = await openAll(g);

	for (const p of [alice, bob, carol]) {
		await expect(p.getByText("Vote for the war's delay")).toBeVisible();
		await expect(p.getByText('Side 1 — Declarer')).toBeVisible();
		await expect(p.getByText('Side 2 — Enemies')).toBeVisible();
		await expect(waitingBar(p)).toContainText('Make War — delay reveal');
	}
	// The two in the war reveal a face and owe nothing else; carol only watches.
	await expect(alice.getByRole('button', { name: 'Submit' })).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Submit' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Submit' })).toHaveCount(0);
	await expect(carol.getByText('Waiting for participants to reveal their delay dice…')).toBeVisible();
	await expect(waitingBar(carol)).toContainText('alice');
	await expect(waitingBar(carol)).toContainText('bob');

	await expect(carol.getByRole('button', { name: 'Join Side 1' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Join Side 2' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Stay out of it' })).toBeVisible();
	for (const p of [alice, bob]) {
		await expect(p.getByRole('button', { name: 'Join Side 1' })).toHaveCount(0);
		await expect(p.getByRole('button', { name: 'Stay out of it' })).toHaveCount(0);
	}
});

test('delay vote: joining a side is free before the war begins and puts the joiner in the vote', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	await declareWar(g);
	const { alice, carol } = await openAll(g);

	// A side other than 1 or 2 is a bad request — sent by carol, who is not yet in
	// the war, so only the side itself can be what is wrong.
	const bad = await g.ctx.carol.request.post(`/api/plans/${g.planID}/join-war`, { data: { side: 3 } });
	expect(bad.status()).toBe(400);
	expect((await warState(g)).participants).toHaveLength(2);

	await carol.getByRole('button', { name: 'Join Side 2' }).click();

	await expect.poll(async () => {
		const s = await warState(g);
		return s.participants.map(p => [p.player_id, p.side, p.entry_payment_complete]);
	}).toEqual([[g.ids.alice, 1, true], [g.ids.bob, 2, true], [g.ids.carol, 2, true]]);

	// Carol now owes a face like the other two, and the others see her seated.
	await expect(carol.getByRole('button', { name: 'Submit' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Join Side 1' })).toHaveCount(0);
	await expect(alice.locator('.side-column', { hasText: 'Side 2 — Enemies' })).toContainText('carol');

	// Once in, a second join is refused rather than silently moving sides.
	const again = await g.ctx.carol.request.post(`/api/plans/${g.planID}/join-war`, { data: { side: 1 } });
	expect(again.status()).toBe(409);

});

// "Stay out of it" is a client-only note (warStayOuts), not a server state: the
// war itself is unchanged by it, so there is no server read to pin here.
test('delay vote: "Stay out of it" collapses the join choices into a note', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	await declareWar(g);
	const { carol } = await openAll(g);

	await carol.getByRole('button', { name: 'Stay out of it' }).click();
	await expect(carol.getByText("You're staying out of it.")).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Join Side 1' })).toHaveCount(0);
	await expect(carol.getByRole('button', { name: 'Join Side 2' })).toHaveCount(0);
});

test('delay vote: the delay is the average of the faces rounded up, and the vote closes on the last face', async ({ browser }) => {
	// 2 and 3 average 2.5 → 3 rows after the declaration (row 1).
	const g = await game(browser);
	await runScene(g);
	await declareWar(g);
	expect(await planRow(g)).toBeNull();
	await revealFaces(g, { alice: 2 });
	expect(await planRow(g), 'one face is not the whole vote').toBeNull();
	await revealFaces(g, { bob: 3 });
	expect(await planRow(g)).toBe(4);

	// Three faces, 2+3+1 = 6/3 = 2 exactly → row 3.
	const g2 = await game(browser);
	await runScene(g2);
	await declareWar(g2);
	await joinWar(g2, 'carol', 1);
	await revealFaces(g2, { alice: 2, bob: 3, carol: 1 });
	expect(await planRow(g2)).toBe(3);
});

test('cost of battle: owed per opponent, shown in the play area to everyone, and the bar names the one payer', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);

	// carol (side 1) owes bob; bob (alone on side 2) owes both alice and carol;
	// alice owes bob — one cost per opponent on the other side.
	const s = await warState(g);
	const owed = s.outstanding_costs.map(c => `${c.payer_id}>${c.opponent_id}`).sort();
	expect(owed).toEqual([
		`${g.ids.alice}>${g.ids.bob}`, `${g.ids.bob}>${g.ids.alice}`,
		`${g.ids.bob}>${g.ids.carol}`, `${g.ids.carol}>${g.ids.bob}`,
	].sort());
	const payer = await frontPayer(g);
	expect(payer).toBe('carol');

	const pages = await openAll(g);
	for (const who of ['alice', 'bob', 'carol'] as Who[]) {
		const p = pages[who];
		// The whole thing is in the play area, not behind the header chip.
		await expect(p.getByText('Cost of battle — row 5')).toBeVisible();
		await expect(waitingBar(p)).toContainText('Row advance blocked');
		await expect(waitingBar(p)).toContainText('cost of battle');
		await expect(p.getByText('Side 1 — Declarer')).toBeVisible();
	}
	// The payer is told it is their turn and given the form; the rest are told who.
	await expect(pages.carol.getByText('It is YOUR turn to pay cost.')).toBeVisible();
	await expect(pages.carol.getByText('Your cost of battle')).toBeVisible();
	await expect(waitingBar(pages.carol)).toContainText('You');
	for (const who of ['alice', 'bob'] as Who[]) {
		await expect(pages[who].getByText('Waiting on carol to pay cost of battle.')).toBeVisible();
		await expect(pages[who].getByText('Your cost of battle')).toHaveCount(0);
		await expect(waitingBar(pages[who])).toContainText('carol');
		await expect(waitingBar(pages[who])).not.toContainText('You');
	}
	// The unfamiliar bystander view too: the word "Join" is gone once the war is on
	// for people already in it.
	await expect(pages.alice.getByRole('button', { name: 'Join Side 1' })).toHaveCount(0);
});

test('cost of battle: paying by breaking tears the marginalia, hands the turn on, and refuses the wrong payer', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);
	const { alice, bob, carol } = await openAll(g);

	// Not alice's turn: refused with a conflict, not a 500, and nothing is spent.
	const aliceHolding = await assetNamed(g, "alice's holding");
	const early = await g.ctx.alice.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: { opponent_id: g.ids.bob, choice: 'break_asset', marginalia_id: aliceHolding.marginalia[0].id },
	});
	expect(early.status()).toBe(409);
	expect((await assetNamed(g, "alice's holding")).marginalia[0].is_torn).toBe(false);

	// Carol pays through the form: opponent, break an asset, tap the marginalia.
	await opponentChip(carol, 'bob');
	await carol.getByRole('button', { name: 'Break an asset' }).click();
	const pay = carol.getByRole('button', { name: 'Pay cost' });
	await expect(pay).toBeDisabled(); // nothing chosen to tear yet
	await tearMarginalia(carol, "carol's holding");
	await expect(pay).toBeEnabled();
	await pay.click();

	await expect.poll(async () => (await assetNamed(g, "carol's holding")).marginalia[0].is_torn).toBe(true);
	// She is now clear, so the next payer is bob, and everyone is told.
	await expect.poll(() => frontPayer(g)).toBe('bob');
	await expect(bob.getByText('It is YOUR turn to pay cost.')).toBeVisible();
	await expect(alice.getByText('Waiting on bob to pay cost of battle.')).toBeVisible();
	await expect(carol.getByText('Waiting on bob to pay cost of battle.')).toBeVisible();
	await expect(waitingBar(alice)).toContainText('bob');
	await expect(carol.getByText('Your cost of battle')).toHaveCount(0);

	// The post sits on the row being paid (5), not the war's own row (3).
	expect((await postWith(g, 'broke')).row_number).toBe(5);
	// The same payer cannot pay the same opponent twice.
	const twice = await g.ctx.carol.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: { opponent_id: g.ids.bob, choice: 'break_asset', marginalia_id: (await assetNamed(g, "carol's artifact")).marginalia[0].id },
	});
	expect(twice.status()).toBe(409);
	expect((await assetNamed(g, "carol's artifact")).marginalia[0].is_torn).toBe(false);
});

test('cost of battle: leverage two pays it, and once every cost is paid the row is unblocked for all', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);
	const { alice, bob, carol } = await openAll(g);

	const pay = async (who: Who, body: Record<string, unknown>) => {
		const res = await g.ctx[who].request.post(`/api/plans/${g.planID}/pay-battle-cost`, { data: body });
		expect(res.status(), `${who} pay failed: ${await res.text()}`).toBe(200);
	};
	const leverage = async (name: string) => (await assetNamed(g, name)).id;

	// carol → bob by leverage: both named assets end up leveraged.
	await pay('carol', { opponent_id: g.ids.bob, choice: 'leverage_two', asset_id_1: await leverage("carol's holding"), asset_id_2: await leverage("carol's resource") });
	expect((await assetNamed(g, "carol's holding")).is_leveraged).toBe(true);
	expect((await assetNamed(g, "carol's resource")).is_leveraged).toBe(true);
	expect((await assetNamed(g, "carol's artifact")).is_leveraged).toBe(false);

	// bob owes two costs, one per opponent; paying one leaves him next in line.
	await pay('bob', { opponent_id: g.ids.alice, choice: 'leverage_two', asset_id_1: await leverage("bob's holding"), asset_id_2: await leverage("bob's resource") });
	expect(await frontPayer(g)).toBe('bob');
	await expect(bob.getByText('It is YOUR turn to pay cost.')).toBeVisible();
	await pay('bob', { opponent_id: g.ids.carol, choice: 'break_asset', marginalia_id: (await assetNamed(g, "bob's artifact")).marginalia[0].id });
	expect(await frontPayer(g)).toBe('alice');
	await pay('alice', { opponent_id: g.ids.bob, choice: 'leverage_two', asset_id_1: await leverage("alice's holding"), asset_id_2: await leverage("alice's resource") });

	expect((await warState(g)).outstanding_costs).toEqual([]);
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Cost of battle — row 5')).toHaveCount(0);
		await expect(waitingBar(p)).not.toContainText('cost of battle');
		await expect(waitingBar(p)).not.toContainText('Row advance blocked');
	}
	// An overpayment has nothing left to pay against.
	const extra = await g.ctx.alice.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: { opponent_id: g.ids.bob, choice: 'break_asset', marginalia_id: (await assetNamed(g, "alice's artifact")).marginalia[0].id },
	});
	expect(extra.status()).toBe(409);
});

test('peace: the bar names the voters, a rejection is announced and returns the gate to the payer', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);
	const { alice, bob, carol } = await openAll(g);

	// Carol is next to pay, and proposes peace in place of paying (UI).
	await proposePeace(carol, 'bob', 'Both hosts withdraw to the old border.');

	await expect.poll(async () => (await warState(g)).open_proposal?.proposer_id).toBe(g.ids.carol);

	// The vote is the wait: alice and bob are named, carol (the proposer, who
	// has already accepted) is not, and nobody is told to pay meanwhile.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Peace proposal from carol')).toBeVisible();
		await expect(p.getByText('Peace terms are on the table — the cost of battle waits on the vote.')).toBeVisible();
	}
	// Each voter sees themselves as "You" and the other by name.
	await expect(waitingBar(alice)).toContainText('You');
	await expect(waitingBar(alice)).toContainText('bob');
	await expect(waitingBar(bob)).toContainText('You');
	await expect(waitingBar(bob)).toContainText('alice');
	await expect(waitingBar(carol)).toContainText('alice');
	await expect(waitingBar(carol)).toContainText('bob');
	await expect(waitingBar(carol)).not.toContainText('You');
	await expect(carol.getByText('Your cost of battle')).toHaveCount(0);
	await expect(alice.getByRole('button', { name: 'Accept' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Accept' })).toHaveCount(0);

	// The proposer has no Accept/Reject to press on their own terms.
	await expect(carol.getByRole('button', { name: 'Reject' })).toHaveCount(0);

	// Bob accepts, alice rejects: the rejection is announced, naming both.
	await bob.getByRole('button', { name: 'Accept' }).click();
	await expect(waitingBar(alice)).not.toContainText('bob');
	await expect(waitingBar(alice)).toContainText('You');
	await alice.getByRole('button', { name: 'Reject' }).click();

	await expect.poll(async () => (await warState(g)).open_proposal).toBeNull();
	expect((await warState(g)).status).toBe('active');
	await expectLogged(g, 'rejected');
	// The gate is back with the payer.
	await expect(carol.getByText('It is YOUR turn to pay cost.')).toBeVisible();
	await expect(waitingBar(alice)).toContainText('carol');
});

test('peace: unanimous acceptance ends the war and clears the table', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);
	const { alice, bob, carol } = await openAll(g);

	await proposePeace(carol, 'bob', 'A white peace.');
	await expect(bob.getByText('Peace proposal from carol')).toBeVisible();
	const proposalID = (await warState(g)).open_proposal!.id;

	await bob.getByRole('button', { name: 'Accept' }).click();
	await expect(bob.getByText('You voted: accept')).toBeVisible();
	expect((await warState(g)).status, 'one vote of two is not enough').toBe('active');
	await alice.getByRole('button', { name: 'Accept' }).click();

	await expect.poll(async () => (await warState(g)).status).toBe('ended');
	const s = await warState(g);
	expect(s.end_reason).toBe('peace');
	expect(s.open_claims).toEqual([]);
	await expectLogged(g, 'agreed to peace terms');

	// Nothing was spent and nothing is owed; the war panel is gone for everyone.
	expect((await assetNamed(g, "carol's holding")).marginalia[0].is_torn).toBe(false);
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Cost of battle — row 5')).toHaveCount(0);
		await expect(p.getByText('Peace proposal from carol')).toHaveCount(0);
		await expect(waitingBar(p)).not.toContainText('Row advance blocked');
	}
	// Voting after the war is over is refused.
	const late = await g.ctx.alice.request.post(`/api/plans/${g.planID}/vote-peace`, { data: { proposal_id: proposalID, accepted: true } });
	expect(late.status()).toBe(409);
});

/** A one-on-one war at the cost row where bob pays first and surrenders on paying. */
async function surrenderedWar(browser: Parameters<typeof seedWarGame>[0]) {
	const g = await game(browser);
	await warAtCostRow(g);
	expect(await frontPayer(g)).toBe('bob');
	const pages = await openAll(g);
	await opponentChip(pages.bob, 'alice');
	await pages.bob.getByRole('button', { name: 'Break an asset' }).click();
	await tearMarginalia(pages.bob, "bob's holding");
	await pages.bob.getByText('Surrender after this payment').click();
	await pages.bob.getByRole('button', { name: 'Pay cost' }).click();
	await expect.poll(async () => (await warState(g)).status).toBe('ended');
	return { g, pages };
}

test('surrender: emptying a side ends the war while the claim stays open for the claimant', async ({ browser }) => {
	const { g, pages } = await surrenderedWar(browser);
	const { alice, bob, carol } = pages;

	const s = await warState(g);
	expect(s.end_reason).toBe('surrender');
	expect(s.participants.find(p => p.player_id === g.ids.bob)!.surrendered_at_row).not.toBeNull();
	expect(s.open_claims.map(c => [c.claimant_id, c.surrendered_id])).toEqual([[g.ids.alice, g.ids.bob]]);
	// The surrender was paid for: bob's marginalia was torn first.
	expect((await assetNamed(g, "bob's holding")).marginalia[0].is_torn).toBe(true);
	await expectLogged(g, 'surrendered unconditionally');

	// The war is over but the table is still held on the claim, on every screen.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('War (ended)')).toBeVisible();
		await expect(p.getByText('The war is over.')).toBeVisible();
		await expect(p.getByText('(a side surrendered)')).toBeVisible();
		await expect(waitingBar(p)).toContainText('Row advance blocked');
		await expect(waitingBar(p)).toContainText('surrender-asset claims');
	}
	// The claimant is "You"; the surrendered player and the bystander see alice named.
	await expect(waitingBar(alice)).toContainText('You');
	for (const p of [bob, carol]) await expect(waitingBar(p)).toContainText('alice');
	// Claimant gets the form; the surrendered player and a bystander are told why
	// the panel is up, in words, and given no form.
	await expect(alice.getByText('Surrender claims')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Claim' })).toBeVisible();
	for (const p of [bob, carol]) {
		await expect(p.getByText("alice is choosing one of bob's assets to take.")).toBeVisible();
		await expect(p.getByRole('button', { name: 'Claim' })).toHaveCount(0);
	}
	await expect(waitingBar(alice)).toContainText('You');
	await expect(waitingBar(carol)).not.toContainText('You');
});

test('surrender: the claimant takes an asset of their choice; then the panel and the block lift', async ({ browser }) => {
	const { g, pages } = await surrenderedWar(browser);
	const { alice, bob, carol } = pages;

	const claim = alice.getByRole('button', { name: 'Claim' });
	await expect(claim).toBeDisabled(); // nothing picked
	// The destroyed holding is not on offer; the artifact is.
	const picker = alice.locator('.choices-section', { hasText: 'Surrender claims' });
	await expect(picker.locator('.card', { hasText: "bob's artifact" })).toBeVisible();
	await expect(picker.locator('.card', { hasText: "bob's holding" })).toHaveCount(0);
	await alice.locator('.card', { hasText: "bob's artifact" }).getByRole('checkbox').click();
	await expect(claim).toBeEnabled();
	await claim.click();

	await expect.poll(async () => (await assetNamed(g, "bob's artifact")).owner_id).toBe(g.ids.alice);
	// Only the chosen asset moved.
	expect((await assetNamed(g, "bob's resource")).owner_id).toBe(g.ids.bob);
	expect((await warState(g)).open_claims).toEqual([]);
	await expectLogged(g, 'seized');

	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Surrender claims')).toHaveCount(0);
		await expect(p.getByText('War (ended)')).toHaveCount(0);
		await expect(waitingBar(p)).not.toContainText('Row advance blocked');
	}
	// Taking twice is refused: the claim is spent.
	const again = await g.ctx.alice.request.post(`/api/plans/${g.planID}/take-surrender-asset`, {
		data: { surrendered_id: g.ids.bob, asset_id: (await assetNamed(g, "bob's resource")).id },
	});
	expect(again.status()).toBe(409);
	expect((await assetNamed(g, "bob's resource")).owner_id).toBe(g.ids.bob);
});

test('surrender: a lone side surrendering to two opponents opens two claims, one asset each', async ({ browser }) => {
	const g = await game(browser);
	// alice alone on side 1; bob and carol on side 2.
	await warAtCostRow(g, { joins: [['carol', 2]], faces: { alice: 2, bob: 3, carol: 1 } });

	// Everyone ahead of alice pays by breaking, in order, until it is her turn.
	const spare = ['holding', 'resource', 'artifact'];
	for (let guard = 0; guard < 6 && (await frontPayer(g)) !== 'alice'; guard++) {
		const s = await warState(g);
		const cost = s.outstanding_costs[0];
		const who = (['bob', 'carol'] as Who[]).find(w => g.ids[w] === cost.payer_id)!;
		const a = await assetNamed(g, `${who}'s ${spare.shift()}`);
		const res = await g.ctx[who].request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
			data: { opponent_id: cost.opponent_id, choice: 'break_asset', marginalia_id: a.marginalia[0].id },
		});
		expect(res.status(), await res.text()).toBe(200);
	}
	expect(await frontPayer(g)).toBe('alice');
	const sur = await g.ctx.alice.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: {
			opponent_id: g.ids.bob, choice: 'break_asset', surrender: true,
			marginalia_id: (await assetNamed(g, "alice's holding")).marginalia[0].id,
		},
	});
	expect(sur.status(), await sur.text()).toBe(200);

	const s = await warState(g);
	expect(s.status).toBe('ended');
	expect(s.open_claims.map(c => c.claimant_id).sort()).toEqual([g.ids.bob, g.ids.carol].sort());
	expect(s.open_claims.every(c => c.surrendered_id === g.ids.alice)).toBe(true);

	const { alice, bob, carol } = await openAll(g);
	for (const p of [bob, carol]) await expect(waitingBar(p)).toContainText('You');
	await expect(waitingBar(alice)).toContainText('bob');
	await expect(waitingBar(alice)).toContainText('carol');

	// Each takes a different asset of alice's; the block holds until both have.
	const take = (who: Who, name: string) => assetNamed(g, name).then(a =>
		g.ctx[who].request.post(`/api/plans/${g.planID}/take-surrender-asset`, {
			data: { surrendered_id: g.ids.alice, asset_id: a.id },
		}));
	expect((await take('bob', "alice's artifact")).status()).toBe(200);
	expect((await warState(g)).open_claims.map(c => c.claimant_id)).toEqual([g.ids.carol]);
	await expect(waitingBar(alice)).not.toContainText('bob');
	await expect(waitingBar(alice)).toContainText('carol');
	expect((await take('carol', "alice's resource")).status()).toBe(200);

	expect((await assetNamed(g, "alice's artifact")).owner_id).toBe(g.ids.bob);
	expect((await assetNamed(g, "alice's resource")).owner_id).toBe(g.ids.carol);
	expect((await warState(g)).open_claims).toEqual([]);
	for (const p of [alice, bob, carol]) await expect(waitingBar(p)).not.toContainText('Row advance blocked');
});

test('surrender: only the claimant may take, and only from the surrendered player', async ({ browser }) => {
	const { g } = await surrenderedWar(browser);
	const take = (who: Who, surrendered: Who, assetName: string) => assetNamed(g, assetName).then(a =>
		g.ctx[who].request.post(`/api/plans/${g.planID}/take-surrender-asset`, {
			data: { surrendered_id: g.ids[surrendered], asset_id: a.id },
		}));

	// The surrendered player and a bystander have no claim to answer.
	expect((await take('bob', 'bob', "bob's artifact")).status()).toBe(404);
	expect((await take('carol', 'bob', "bob's artifact")).status()).toBe(404);
	// The claimant cannot reach past the surrendered player to someone else's asset.
	expect((await take('alice', 'bob', "carol's artifact")).status()).toBe(403);
	expect((await assetNamed(g, "carol's artifact")).owner_id).toBe(g.ids.carol);
	expect((await assetNamed(g, "bob's artifact")).owner_id).toBe(g.ids.bob);
	expect((await warState(g)).open_claims).toHaveLength(1);
});

test('surrender: a side with an ally left keeps the war going and the surrendered player stops paying', async ({ browser }) => {
	const g = await game(browser);
	await threeWayWar(g);
	expect(await frontPayer(g)).toBe('carol');
	const pay = await g.ctx.carol.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: {
			opponent_id: g.ids.bob, choice: 'break_asset', surrender: true,
			marginalia_id: (await assetNamed(g, "carol's holding")).marginalia[0].id,
		},
	});
	expect(pay.status(), await pay.text()).toBe(200);

	const s = await warState(g);
	expect(s.status, 'alice is still on the declarers\' side').toBe('active');
	expect(s.participants.find(p => p.player_id === g.ids.carol)!.surrendered_at_row).not.toBeNull();
	expect(s.open_claims.map(c => [c.claimant_id, c.surrendered_id])).toEqual([[g.ids.bob, g.ids.carol]]);
	expect(s.outstanding_costs.some(c => c.payer_id === g.ids.carol || c.opponent_id === g.ids.carol),
		'a surrendered player neither pays nor is owed').toBe(false);

	const { alice, bob, carol } = await openAll(g);
	await expect(bob.getByText('Surrender claims')).toBeVisible();
	await expect(alice.getByText("bob is choosing one of carol's assets to take.")).toHaveCount(0); // war still active: no ended-war note
	await expect(carol.getByText('(surrendered, row 5)')).toBeVisible();
	await expect(waitingBar(alice)).toContainText('bob');
});

test('a bystander who never joined sees the whole war but is never asked for anything', async ({ browser }) => {
	const g = await game(browser);
	await warAtCostRow(g); // alice v bob, carol outside
	const { carol } = await openAll(g);

	await expect(carol.getByText('Cost of battle — row 5')).toBeVisible();
	await expect(carol.getByText('Waiting on bob to pay cost of battle.')).toBeVisible();
	await expect(carol.getByText('Your cost of battle')).toHaveCount(0);
	await expect(waitingBar(carol)).toContainText('bob');
	await expect(waitingBar(carol)).not.toContainText('You');
	await expect(carol.locator('.choices-section', { hasText: 'Cost of battle — row 5' })).toContainText(
		'Outstanding (payer → opponent), in reverse-power order: bob → alice; alice → bob');

	// Not being in the war, carol cannot pay, vote or claim.
	const pay = await g.ctx.carol.request.post(`/api/plans/${g.planID}/pay-battle-cost`, {
		data: { opponent_id: g.ids.bob, choice: 'break_asset', marginalia_id: (await assetNamed(g, "carol's holding")).marginalia[0].id },
	});
	expect(pay.status()).toBe(409);
	expect((await assetNamed(g, "carol's holding")).marginalia[0].is_torn).toBe(false);
});

// A join during the delay vote adds the joiner to the vote (see the test above),
// but the other players' Waiting On bar still names only the original two until
// they reload. Tracked in docs/work/next/make-war-join-not-pushed-to-waiting-bar.md.
test.fixme('delay vote: a join reaches the other players\' Waiting On bar without a reload', async ({ browser }) => {
	const g = await game(browser);
	await runScene(g);
	await declareWar(g);
	const { alice, bob } = await openAll(g);
	await expect(waitingBar(alice)).toContainText('bob');

	await joinWar(g, 'carol', 2);

	await expect(waitingBar(alice)).toContainText('carol');
	await expect(waitingBar(bob)).toContainText('carol');
});
