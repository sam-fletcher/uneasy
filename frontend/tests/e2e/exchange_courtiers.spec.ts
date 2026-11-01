import { test, expect, type APIResponse, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedECGame, runScene, prepareEC, resolveEC, openAll, offerTrade, declineAndForce,
	ownerOf, planStatus, expectPlanLog, closeContextsAfterEach, PEER, type ECOptions,
} from './helpers/exchange_courtiers';

// Regression specs for what the 12-plan UX audit fixed in Exchange Courtiers:
//   · the plan names the peer it is angling for, and offers the rules reference
//     (the Buffet) at preparation (it is shown up to the roll as well);
//   · the fair-trade pre-roll (target names a peer, preparer accepts/declines),
//     with both legs of the swap in the action log *before* the resolution line;
//   · the mar branch is the target's to choose, and "A Fair Trade" still swaps;
//   · a make owes the target a messy break, and the plan then closes itself —
//     no "Complete plan" button;
//   · a peer destroyed between prepare and resolve pauses the plan for a re-aim
//     (audit Tier 1 #5) instead of transferring a corpse;
//   · who sees what: the target acts, the preparer waits, a bystander only
//     watches.
//
// Setup (seed, scene, prepare, fast-forward, forcing the dice) goes over the API
// — it isn't the subject. Dice are forced with /api/dev/force-roll. The tests
// that read the action log use a desktop viewport, where the chat column is
// always open.
//
// Seed ranks put bob at power rank 3, so the roll's difficulty is 3 (2 with
// `easyTarget`, bob last). Alice rolls two dice, so a make needs the easy target:
// result 2 vs 2 is a make with margin 0; against 3, result 2 is a mar by 1 and
// result 1 a mar by 2.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

/** A refusal, not a crash: a 4xx, so a 500 can't pass for "the server said no". */
function expectRefused(res: APIResponse) {
	expect(res.status()).toBeGreaterThanOrEqual(400);
	expect(res.status()).toBeLessThan(500);
}

/** A player chip in the target-player row (the table header has same-named buttons). */
function chip(page: Page, name: string) {
	return page.locator('.player-chip', { hasText: name });
}

/** Tick a card in a CardPicker (peers and marginalia are chosen by checkbox). */
async function pickCard(page: Page, name: string) {
	await page.locator('.card', { hasText: name }).getByRole('checkbox').click();
}

/** A start-of-resolution game: alice's plan is open and waiting on bob's offer. */
async function resolvingGame(browser: Parameters<typeof seedECGame>[0], opts?: ECOptions) {
	const g = await seedECGame(browser, opts);
	track(g.gameID);
	await runScene(g);
	const planID = await prepareEC(g);
	await resolveEC(g, planID);
	return { g, planID };
}

/** The resolving card's text, sans the surrounding table chrome. */
function card(page: Page) {
	return page.locator('.resolving-card, [class*="resolving"]').first();
}

test('preparation: the target peer is picked from the target player', async ({ browser }) => {
	const g = await seedECGame(browser, { viewport: { width: 375, height: 812 } });
	track(g.gameID);
	await runScene(g);
	const alice = await g.ctx.alice.newPage();
	await alice.goto(`/table/${g.gameID}`);

	await alice.getByText('Exchange Courtiers', { exact: true }).click();

	// Nothing can be submitted until a peer and notes are given.
	const prepare = alice.getByRole('button', { name: 'Prepare Plan' });
	await expect(prepare).toBeVisible();
	await expect(prepare).toBeDisabled();

	// The rules reference is available before committing.
	await alice.getByText('What can happen?').click();
	await expect(alice.getByText('the target may name one of the preparer')).toBeVisible();

	// Target player defaults to the first other player (bob); pick his courtier.
	await pickCard(alice, PEER.bob);
	await alice.getByPlaceholder('How are you planning to take them into your retinue?').fill('Sweet words and a better stipend.');
	await expect(prepare).toBeEnabled();

	// Switching to carol drops the peer pick: one target overall.
	await chip(alice, 'carol').click();
	await expect(prepare).toBeDisabled();
	await chip(alice, 'bob').click();
	await pickCard(alice, PEER.bob);
	await expect(prepare).toBeEnabled();
	await prepare.click();

	await expect.poll(async () => {
		const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
		const { plans } = (await res.json()) as { plans: { plan_type: string; target_player_id: number; target_asset_id: number }[] };
		const ec = plans.find(p => p.plan_type === 'exchange_courtiers');
		return ec ? [ec.target_player_id, ec.target_asset_id] : null;
	}).toEqual([g.ids.bob, g.peers.bob]);
});

test('preparation: the server refuses a peer that is not the target player\'s', async ({ browser }) => {
	const g = await seedECGame(browser);
	track(g.gameID);
	await runScene(g);
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: {
			plan_type: 'exchange_courtiers',
			target_player_id: g.ids.bob,
			target_asset_id: g.peers.carol,
			preparation_notes: 'Wrong courtier.',
		},
	});
	expectRefused(res);
});

test('fair trade accepted: both peers change hands, the log names both, then the resolution', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const pages = await openAll(g);
	const { alice, bob, carol } = pages;

	// Pre-roll, before any offer: the target is asked, the preparer waits, a
	// bystander sees only who is resolving (and never an action).
	await expect(bob.getByText('You may name one of their peers to receive in exchange')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Propose trade' })).toBeDisabled();
	await expect(alice.getByText(`Waiting for bob to propose a fair trade for their peer, ${PEER.bob}`)).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Propose trade' })).toHaveCount(0);

	// The preparation line names the exact peer being angled for.
	await expect(carol.getByRole('complementary', { name: 'Chat' }).locator('.feed')).toContainText(
		`alice prepared Exchange Courtiers, angling for the peer ${PEER.bob}`);

	// The target proposes through the UI.
	await pickCard(bob, PEER.alice);
	await bob.getByRole('button', { name: 'Propose trade' }).click();

	// The preparer is handed the decision live; the target waits on it.
	await expect(alice.getByText(`bob will hand over ${PEER.bob} if you give them ${PEER.alice}`)).toBeVisible();
	await expect(bob.getByText("Waiting for alice's decision…")).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Accept — exchange without rolling' })).toBeVisible();
	// Nothing has moved yet.
	expect(await ownerOf(g, g.peers.bob)).toBe(g.ids.bob);
	expect(await ownerOf(g, g.peers.alice)).toBe(g.ids.alice);

	await alice.getByRole('button', { name: 'Accept — exchange without rolling' }).click();

	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	// Each peer crossed over.
	expect(await ownerOf(g, g.peers.bob)).toBe(g.ids.alice);
	expect(await ownerOf(g, g.peers.alice)).toBe(g.ids.bob);

	// The log: the offer, then BOTH transfers, then the resolution line.
	await expectPlanLog(carol, planID, [
		`bob proposed a fair trade: alice's ${PEER.alice} in exchange.`,
		`alice took ${PEER.bob} from bob`,
		`bob took ${PEER.alice} from alice`,
		'Exchange Courtiers succeeded.',
	]);
	// A resolved plan offers nobody a "Complete" step.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByRole('button', { name: /Complete plan/ })).toHaveCount(0);
	}
});

test('mar: the target chooses; capped by the margin; "A Fair Trade" still swaps both peers', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const pages = await openAll(g);
	const { alice, bob, carol } = pages;
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 2); // difficulty 3, result 2: a mar by 1

	// The target — not the preparer — is handed the choice, with the margin.
	await expect(bob.getByText("alice's plan was marred by 1")).toBeVisible();
	await expect(bob.getByRole('button', { name: 'A Fair Trade' })).toBeVisible();
	await expect(bob.getByText('needs −2')).toBeVisible(); // Riposte is out of reach
	await expect(bob.getByText('needs −3')).toBeVisible(); // so is Forfeit
	await expect(alice.getByText('marred by 1')).toHaveCount(0);
	await expect(alice.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	await expect(carol.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	// (No assertion on carol's wait line here: when the roll is created and forced
	// within milliseconds her page can keep "Dice roll in progress…" until reload —
	// see the note.)

	await bob.getByRole('button', { name: 'A Fair Trade' }).click();
	await bob.getByRole('button', { name: 'Apply choices' }).click();

	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	expect(await ownerOf(g, g.peers.bob)).toBe(g.ids.alice);
	expect(await ownerOf(g, g.peers.alice)).toBe(g.ids.bob);

	await expectPlanLog(carol, planID, [
		`alice declined the fair trade`,
		`A fair trade: ${PEER.bob} passed to alice after all.`,
		`bob received ${PEER.alice} in exchange.`,
		'Exchange Courtiers marred.',
	]);
});

test('mar by 2: riposte lets the preparer break the requested peer before bob takes it', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const { alice, bob, carol } = await openAll(g);
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 1); // difficulty 3, result 1: a mar by 2

	await expect(bob.getByText("alice's plan was marred by 2")).toBeVisible();
	await expect(bob.getByText('needs −3')).toBeVisible();
	await bob.getByText('Riposte').click();
	await bob.getByRole('button', { name: 'Apply choices' }).click();

	// The preparer is now the one who acts; the target and bystander wait.
	await expect(alice.getByText(`Riposte: bob takes your ${PEER.alice}`)).toBeVisible();
	await expect(bob.getByText(`Riposte — waiting for alice to break or surrender ${PEER.alice}`)).toBeVisible();
	await expect(carol.getByText(`Riposte — waiting for alice to break or surrender ${PEER.alice}`)).toBeVisible();
	// Still alice's until she decides.
	expect(await ownerOf(g, g.peers.alice)).toBe(g.ids.alice);

	await alice.getByRole('button', { name: 'Surrender intact' }).click();

	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	expect(await ownerOf(g, g.peers.alice)).toBe(g.ids.bob);
	// A mar by riposte leaves the targeted peer where it was.
	expect(await ownerOf(g, g.peers.bob)).toBe(g.ids.bob);
});

test('make: the targeted peer transfers, the target owes a messy break, then it closes itself', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser, { easyTarget: true });
	const { alice, bob, carol } = await openAll(g);
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 2); // difficulty 2, result 2: a make with margin 0

	// Only Messy is open at margin 0; the others show what they would need.
	await expect(alice.getByText('You beat the difficulty by 0')).toBeVisible();
	await expect(alice.getByText('needs +1')).toBeVisible();
	await expect(alice.getByText('needs +2')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	await expect(carol.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);

	await alice.getByText('Messy').click();
	await alice.getByRole('button', { name: 'Apply choices' }).click();

	// The peer moved; the target now owes a break on one of alice's assets.
	await expect.poll(() => ownerOf(g, g.peers.bob)).toBe(g.ids.alice);
	await expect(bob.getByText("The exchange was messy. You may break one of alice's marginalia")).toBeVisible();
	await expect(alice.getByText('Waiting for bob to break a marginalia…')).toBeVisible();
	await expect(carol.getByText('Waiting for bob to break a marginalia…')).toBeVisible();
	expect(await planStatus(g, planID)).toBe('resolving');

	// bob breaks the sole marginalia on alice's courtier (destroying it).
	await expect(bob.getByRole('button', { name: 'Break marginalia' })).toBeDisabled();
	await bob.locator('.card', { hasText: PEER.alice }).getByRole('checkbox').click();
	await bob.getByRole('button', { name: 'Break marginalia' }).click();

	// The break was the last step: the plan resolves with no Complete click.
	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	for (const p of [alice, bob, carol]) {
		await expect(p.getByRole('button', { name: /Complete plan/ })).toHaveCount(0);
	}
	await expectPlanLog(carol, planID, [
		`alice took the peer ${PEER.bob} from bob.`,
		'It got messy — bob may break one of alice\'s assets.',
		`bob destroyed alice's ${PEER.alice} in the mess.`,
		'Exchange Courtiers succeeded.',
	]);
});

test('make: a messy break that only tears (does not destroy) still posts to the log', async ({ browser }) => {
	// alice's courtier has two marginalia, so tearing one leaves it standing.
	const { g, planID } = await resolvingGame(browser, {
		easyTarget: true,
		aliceMarginalia: ['quick with a knife', 'owes the steward money'],
	});
	const { alice, bob } = await openAll(g);
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 2);

	await alice.getByText('Messy').click();
	await alice.getByRole('button', { name: 'Apply choices' }).click();
	await expect(bob.getByText("The exchange was messy.")).toBeVisible();

	// Tear the first marginalia on alice's courtier.
	await bob.locator('.card', { hasText: PEER.alice }).getByRole('checkbox').first().click();
	await bob.getByRole('button', { name: 'Break marginalia' }).click();

	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	// The courtier survives, one marginalia down; the break still has its own line.
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`);
	const { assets } = (await res.json()) as {
		assets: { id: number; is_destroyed: boolean; marginalia: { is_torn: boolean }[] }[];
	};
	const courtier = assets.find(a => a.id === g.peers.alice)!;
	expect(courtier.is_destroyed).toBe(false);
	expect(courtier.marginalia.filter(m => m.is_torn)).toHaveLength(1);
	await expectPlanLog(bob, planID, [
		`bob broke alice's ${PEER.alice} in the mess.`,
		'Exchange Courtiers succeeded.',
	]);
});

test('re-aim: a peer destroyed before resolution pauses the plan for the preparer to re-aim', async ({ browser }) => {
	const g = await seedECGame(browser);
	track(g.gameID);
	await runScene(g);
	const planID = await prepareEC(g);

	// Five rows pass; meanwhile bob's courtier loses its only marginalia.
	const tear = await g.ctx.bob.request.delete(`/api/assets/${g.peers.bob}/marginalia/1`);
	expect(tear.ok(), `tear failed: ${await tear.text()}`).toBeTruthy();
	await resolveEC(g, planID);
	const { alice, bob, carol } = await openAll(g);

	// The plan did not transfer the corpse or open a trade over it.
	const panel = alice.locator('.reaim');
	await expect(panel.getByText('has lost its mark')).toBeVisible();
	await expect(panel.getByText(`The peer ${PEER.bob} has been destroyed.`)).toBeVisible();
	expect(await ownerOf(g, g.peers.bob)).toBe(g.ids.bob);
	await expect(alice.getByRole('button', { name: 'Re-aim this plan' })).toBeDisabled();
	// Target and bystander just wait for the preparer — and get no controls.
	for (const p of [bob, carol]) {
		await expect(p.getByText('Waiting for alice to choose a new target')).toBeVisible();
		await expect(p.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
		await expect(p.getByRole('button', { name: 'Propose trade' })).toHaveCount(0);
	}
	// The server refuses every other route while the re-aim is owed.
	const offer = await g.ctx.bob.request.post(`/api/plans/${planID}/fair-trade`, {
		data: { action: 'offer', offered_asset_id: g.peers.alice },
	});
	expectRefused(offer);

	// Only the preparer may re-aim.
	const bobTry = await g.ctx.bob.request.post(`/api/plans/${planID}/reaim`, {
		data: { target_player_id: g.ids.bob, target_asset_id: g.peers.bob },
	});
	expectRefused(bobTry);

	// alice re-aims at carol's courtier through the panel.
	await chip(alice, 'carol').click();
	await pickCard(alice, PEER.carol);
	await alice.getByRole('button', { name: 'Re-aim this plan' }).click();

	// The plan carries on from the top, now about carol — who is the one asked
	// to name a trade; bob (no longer the target) only watches.
	await expect(carol.getByText('You may name one of their peers to receive in exchange')).toBeVisible();
	await expect(alice.getByText(`Waiting for carol to propose a fair trade for their peer, ${PEER.carol}`)).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Propose trade' })).toHaveCount(0);

	await expectPlanLog(carol, planID, [
		`has lost its mark: The peer ${PEER.bob} has been destroyed.`,
		`alice re-aimed their Exchange Courtiers at carol's peer ${PEER.carol}.`,
	]);
});

// Audit Tier 6: "second-person wording shown to non-actors" — still open for the
// post-offer wait line, which tells a bystander "You asked for …". See
// docs/work/next/exchange-courtiers-bystander-second-person.md.
test.fixme('after the offer, a bystander is not addressed as the one who asked', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const { alice, bob, carol } = await openAll(g);
	await offerTrade(g, planID);
	await expect(bob.getByText("You asked for")).toBeVisible();
	await expect(alice.getByText('will hand over')).toBeVisible();
	await expect(carol.getByText('You asked for')).toHaveCount(0);
});

// Audit Tier 6 still open: the non-acting seat is left with no line saying who
// the plan is waiting on. Pinned by shape (a wait line naming the actor), not
// wording. See docs/work/next/exchange-courtiers-blank-waiting-panels.md.
test.fixme('every seat that is not acting is told who the plan is waiting on', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser, { easyTarget: true });
	const { alice, bob, carol } = await openAll(g);
	const waitLine = (page: Page, who: string) => page.locator('.ft-prompt').filter({ hasText: who });

	// A bystander before the offer: waiting on bob.
	await expect(waitLine(carol, 'bob')).toBeVisible();

	// The preparer while the target chooses the mar response: waiting on bob.
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 1); // difficulty 2, result 1: a mar by 1
	await expect(bob.getByRole('button', { name: 'Apply choices' })).toBeVisible();
	await expect(waitLine(alice, 'bob')).toBeVisible();
});

test.fixme('the target is told the preparer is choosing, during a make', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser, { easyTarget: true });
	const { alice, bob } = await openAll(g);
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 2); // difficulty 2, result 2: a make
	await expect(alice.getByRole('button', { name: 'Apply choices' })).toBeVisible();
	await expect(bob.locator('.ft-prompt').filter({ hasText: 'alice' })).toBeVisible();
});

// Audit Tier 6 still open: the Riposte option is described to its chooser, the
// target, in the preparer's voice ("you may break it first" is the preparer's
// move). See docs/work/next/exchange-courtiers-riposte-wording.md.
test.fixme('riposte is not described to the target as something "you" may break', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const { bob } = await openAll(g);
	await offerTrade(g, planID);
	await declineAndForce(g, planID, 1); // difficulty 3, result 1: a mar by 2
	const riposte = bob.getByRole('button', { name: 'Riposte' });
	await expect(riposte).toBeVisible();
	await expect(riposte).not.toContainText('you may break');
});
