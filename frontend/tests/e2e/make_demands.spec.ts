import { test, expect, type Page } from '@playwright/test';
import * as H from './helpers/make_demands';

// Regression specs for what the 12-plan UX audit and the Make Demands audit
// fixed in Make Demands (an overlay plan attached to another plan):
//
//  - attaching: a demand picks a pending plan of someone else's, lands on its
//    row right before it, and needs a motivation;
//  - the demand's own roll: the actor can call or skip the difficulty vote,
//    everyone else just waits;
//  - the draft of the four options: alternating picks, higher power rank first;
//    options that would do nothing against the target are greyed with a reason
//    while a live one remains, and are never blocked once only duds are left,
//    so the forced last pick can always be taken;
//  - control_leverage: "leverage none — let the roll fail" is a real
//    submission, and it holds the target's roll open until it is made;
//    against plans with no roll of the preparer's own, the option is a dud;
//  - keep_or_change_target: the roll waits on the winner, who settles it either
//    way and may re-aim;
//  - perform_steps: the demander acts after the roll, in the preparer's place,
//    while the preparer and a bystander see who is acting.
//
// Not pinned, because the owner has not ruled (docs/work/questions/):
// whether the server should refuse an inert draft pick while a live one
// remains, whether Propose Duel and Clandestinely Liaise should wait on the
// retarget winner, and the severity of the draft-pick log lines.
//
// Setup (seeding, preparing, reaching the row, forcing dice, the draft) goes
// over the API; every assertion about what a player sees is made in a real page.

H.cleanupAfterEach();

const waiting = (page: Page) => H.waitingBar(page);
const card = (page: Page, label: string) => page.locator('.choice-card', { hasText: label });

test('attaching: bob picks alices pending plan, gives a motivation, and the demand lands right before it', async ({ browser }) => {
	const f = await H.seedMD();
	await H.runScene(f, 'bob');
	const { bob } = await H.openPages(browser, f);

	await bob.getByText('Make Demands', { exact: true }).click();
	const prepare = bob.getByRole('button', { name: 'Prepare Plan' });
	await expect(prepare).toBeDisabled();

	// Only plans that could be demanded against are offered: alice's, not bob's own.
	const select = bob.locator('select');
	const optionTexts = (await select.locator('option').allInnerTexts()).map(t => t.replace(/\s+/g, ' ').trim());
	expect(optionTexts).toEqual(['— pick a plan to demand against —', 'Seek Answers by alice (row 5)']);
	await select.selectOption({ label: 'Seek Answers by alice (row 5)' });
	await expect(prepare).toBeDisabled(); // a motivation is required too
	await bob.getByPlaceholder('Why are you getting involved?').fill('The ledgers are mine.');
	await expect(prepare).toBeEnabled();
	await prepare.click();

	// Outcome: a pending demand aimed at the target, on the target's row and
	// ahead of it in the row's order, so it resolves first.
	await expect.poll(async () => (await H.plansOf(f)).some(p => p.plan_type === 'make_demands')).toBe(true);
	const demand = (await H.plansOf(f)).find(p => p.plan_type === 'make_demands')!;
	const target = await H.planOf(f, f.targetID);
	expect(demand.preparer_id).toBe(f.ids.bob);
	expect(demand.targeted_plan_id).toBe(f.targetID);
	expect(demand.status).toBe('pending');
	expect(demand.row_number).toBe(target.row_number);
	expect(demand.row_order).toBeLessThan(target.row_order);
});

test('the demands roll: the actor may skip the vote, everyone else waits on them', async ({ browser }) => {
	const f = await H.seedMD();
	await H.prepareDemand(f);
	await H.reachRow(f);
	const pages = await H.openPages(browser, f);

	// bob is the actor on the demand's own roll and the table waits on him.
	await expect(pages.bob.getByRole('button', { name: 'Skip vote' })).toBeVisible();
	await expect(pages.bob.getByRole('button', { name: 'Call difficulty vote' })).toBeVisible();
	for (const n of ['alice', 'carol'] as const) {
		await expect(pages[n].getByText('Waiting for bob to decide about a difficulty vote…')).toBeVisible();
		await expect(pages[n].getByRole('button', { name: 'Skip vote' })).toHaveCount(0);
		await expect(waiting(pages[n])).toContainText('bob');
	}

	await pages.bob.getByRole('button', { name: 'Skip vote' }).click();
	for (const n of H.PLAYERS) {
		await expect(pages[n].getByRole('button', { name: 'Skip vote' })).toHaveCount(0);
		await expect(pages[n].getByText('Waiting for bob to decide about a difficulty vote…')).toHaveCount(0);
	}
	const { roll } = await H.call(f.api.alice, 'get', `/api/tables/${f.gameID}/rolls/active`);
	expect(roll.stage).not.toBe('decide_vote');
});

test('the draft: duds are greyed with a reason while a live option remains, and the forced last pick is open', async ({ browser }) => {
	// Seek Answers awards nothing and has no aim to redirect, so two of the four
	// options do nothing against it; control_leverage and perform_steps do.
	const f = await H.seedMD({ target: 'seek_answers' });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	const pages = await H.openPages(browser, f);
	const first = await H.firstPicker(f);
	const second = first === 'alice' ? 'bob' : 'alice';
	const [pick, other] = [pages[first], pages[second]];

	await expect(pick.getByText('Your turn to pick.')).toBeVisible();
	await expect(other.getByText(`Waiting for ${first} to pick…`)).toBeVisible();
	await expect(pages.carol.getByText(`Waiting for ${first} to pick…`)).toBeVisible();
	await expect(pages.carol.locator('.choice-card')).toHaveCount(0);
	await expect(other.locator('.choice-card')).toHaveCount(0);

	// Duds are shown, greyed, with the reason; the live ones are not.
	const dud = card(pick, 'Keep or change target');
	await expect(dud).toHaveAttribute('aria-disabled', 'true');
	await expect(dud).toContainText('Seek Answers isn’t aimed at anything a demand could redirect.');
	await expect(card(pick, 'Keep assets')).toHaveAttribute('aria-disabled', 'true');
	await expect(card(pick, 'Keep assets')).toContainText('gives its preparer no assets to keep.');
	await expect(card(pick, 'Control leverage')).toHaveAttribute('aria-disabled', 'false');
	await expect(card(pick, 'Perform steps')).toHaveAttribute('aria-disabled', 'false');
	await expect(pick.getByText("None of what's left does anything against this plan")).toHaveCount(0);

	// Tapping a dud selects nothing; tapping a live card does, and a later tap
	// on a dud doesn't clear that live pick.
	const submit = pick.getByRole('button', { name: 'Pick option' });
	// (aria-disabled, not disabled, so a tap still reaches the handler.)
	await dud.dispatchEvent('click');
	await expect(submit).toBeDisabled();
	await card(pick, 'Control leverage').click();
	await expect(submit).toBeEnabled();
	await dud.dispatchEvent('click');
	await expect(card(pick, 'Control leverage')).toHaveAttribute('aria-pressed', 'true');
	await submit.click();

	await expect.poll(async () => draftOf(f)).toEqual([[first, 'control_leverage']]);

	// The second picker takes the other live option; now only duds remain.
	await expect(other.getByText('Your turn to pick.')).toBeVisible();
	await card(other, 'Perform steps').click();
	await other.getByRole('button', { name: 'Pick option' }).click();
	await expect.poll(async () => (await draftOf(f)).length).toBe(2);

	// First picker again: only duds are left, so none is blocked and they're told so.
	await expect(pick.getByText("None of what's left does anything against this plan — pick whichever.")).toBeVisible();
	await expect(card(pick, 'Keep or change target')).toHaveAttribute('aria-disabled', 'false');
	await expect(card(pick, 'Keep assets')).toHaveAttribute('aria-disabled', 'false');
	await card(pick, 'Keep assets').click();
	await pick.getByRole('button', { name: 'Pick option' }).click();

	// The final pick is forced: the second picker takes the last card, dud or not.
	await expect(other.getByText("None of what's left does anything against this plan — pick whichever.")).toBeVisible();
	await expect(card(other, 'Keep or change target')).toHaveAttribute('aria-disabled', 'false');
	await card(other, 'Keep or change target').click();
	await other.getByRole('button', { name: 'Pick option' }).click();

	await expect.poll(async () => draftOf(f)).toEqual([
		[first, 'control_leverage'], [second, 'perform_steps'], [first, 'keep_assets'], [second, 'keep_or_change_target'],
	]);
});

/** The demand's draft picks as [pickerName, option], in order. */
async function draftOf(f: H.MDFixture): Promise<[string, string][]> {
	const { resolution_data } = await H.call(f.api.alice, 'get', `/api/plans/${f.demandID}`);
	const picks: { player_id: number; option: string }[] = resolution_data?.make_demands?.draft_choices ?? [];
	const names = Object.fromEntries(Object.entries(f.ids).map(([n, id]) => [id, n]));
	return picks.map(c => [names[c.player_id], c.option]);
}

/**
 * Bob wins control_leverage over alice's Seek Answers (alice keeps perform_steps and, as the
 * preparer, retarget, so only the leverage gate is in play; the two live
 * options are drafted before the duds), the demand is done and
 * alice has cast her roll. Returns the open roll's id, at the leverage stage.
 */
async function leverageHeld(f: H.MDFixture): Promise<number> {
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['control_leverage', 'keep_assets'], alice: ['perform_steps', 'keep_or_change_target'] });
	await H.completeDemand(f);
	await H.startTarget(f);
	const cast = await H.call(f.api.alice, 'post', `/api/plans/${f.targetID}/seek-cast-roll`, { narration: 'I read the ledgers.' });
	await H.call(f.api.alice, 'post', `/api/rolls/${cast.roll.id}/skip-vote`);
	return cast.roll.id;
}

async function rollOf(f: H.MDFixture) {
	return (await H.call(f.api.alice, 'get', `/api/tables/${f.gameID}/rolls/active`)).roll;
}

test('control leverage: "leverage none" is offered to the winner alone and is a real submission; the preparer cannot leverage for themself', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	const rollID = await leverageHeld(f);
	const pages = await H.openPages(browser, f);
	const { alice, bob, carol } = pages;

	// Only the winner gets the control; zero is a first-class choice, offered
	// even before anything is picked.
	const none = bob.getByRole('button', { name: 'Leverage none — let the roll fail' });
	await expect(none).toBeVisible();
	await expect(none).toBeEnabled();
	await expect(bob.getByText('Leveraging none is a real move: it can guarantee the roll fails.')).toBeVisible();
	for (const p of [alice, carol]) {
		await expect(p.getByText('bob controls leverage of alice\'s assets on this plan\'s roll.')).toBeVisible();
		await expect(p.getByRole('button', { name: 'Leverage none — let the roll fail' })).toHaveCount(0);
	}

	// The preparer is locked out of leveraging their own assets (the 403 is the
	// explicit "taken over" refusal, not a generic one).
	const assets = (await H.call(f.api.alice, 'get', `/api/tables/${f.gameID}/assets`)).assets as { id: number; owner_id: number }[];
	const mine = assets.find(a => a.owner_id === f.ids.alice)!;
	const own = await f.api.alice.post(`/api/rolls/${rollID}/leverage`, { data: { asset_id: mine.id } });
	expect(own.status()).toBe(403);
	expect(await own.text()).toContain('control_leverage winner has taken over');
	// Neither can anyone else make the decision for bob.
	const carolTries = await f.api.carol.post(`/api/plans/${f.targetID}/demand-leverage`, { data: { asset_ids: [] } });
	expect(carolTries.status()).toBe(403);

	await none.click();
	await expect(bob.getByText('Your leverage decision is in — the roll is no longer held for you.')).toBeVisible();
	for (const n of H.PLAYERS) await H.call(f.api[n], 'post', `/api/rolls/${rollID}/ready`, { is_ready: true });

	// The roll ran with none of alice's assets behind it.
	await expect.poll(async () => (await rollOf(f)).result, { timeout: 10_000 }).not.toBeNull();
	const after = (await H.call(f.api.alice, 'get', `/api/tables/${f.gameID}/assets`)).assets as { owner_id: number; is_leveraged: boolean }[];
	expect(after.filter(a => a.owner_id === f.ids.alice && a.is_leveraged)).toEqual([]);
	await expect.poll(() => H.postsText(f)).toContain('bob leveraged none of their assets');
});

test('control leverage against a plan with no roll of the preparer\'s own is a dud: greyed in the draft, named a dud afterwards, refused, and it holds nothing up', async ({ browser }) => {
	// Propose Duel has no preparer roll (its final roll is built from the bouts),
	// so leverage control has nothing to attach to. This power order puts alice
	// first in the draft, which leaves bob the forced last pick.
	const f = await H.seedMD({ target: 'propose_duel', power: [0, 2, 1] });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	expect(await H.firstPicker(f)).toBe('alice');
	const pages = await H.openPages(browser, f);
	const { alice, bob, carol } = pages;

	// A live option remains, so the dud is greyed with its reason and the rest are open.
	const dud = card(alice, 'Control leverage');
	await expect(dud).toHaveAttribute('aria-disabled', 'true');
	await expect(dud).toContainText('Propose Duel has no dice roll of its preparer’s own to leverage into.');
	for (const live of ['Keep or change target', 'Keep assets', 'Perform steps']) {
		await expect(card(alice, live)).toHaveAttribute('aria-disabled', 'false');
	}

	// Alice, bob, alice: three live picks. What is left is the dud, so bob's
	// forced last pick is open to him — not blocked — and he is told why.
	await H.draftWants(f, { alice: ['keep_or_change_target', 'keep_assets'], bob: ['perform_steps', 'control_leverage'] }, 3);
	await expect(bob.getByText("None of what's left does anything against this plan — pick whichever.")).toBeVisible();
	await expect(card(bob, 'Control leverage')).toHaveAttribute('aria-disabled', 'false');
	await card(bob, 'Control leverage').click();
	await bob.getByRole('button', { name: 'Pick option' }).click();
	await expect.poll(async () => (await draftOf(f)).map(c => c[1])).toEqual(
		['keep_or_change_target', 'perform_steps', 'keep_assets', 'control_leverage'],
	);

	await H.completeDemand(f);
	await H.startTarget(f);

	// Everyone is told, in the banner, that the option came to nothing here.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('bob drew leverage control, but Propose Duel has no roll of its own — that option came to nothing here.')).toBeVisible();
	}
	// The winner gets an explanation instead of a picker (and no "leverage none" button).
	await expect(bob.getByText('This option drew a dud.')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Leverage none — let the roll fail' })).toHaveCount(0);
	// The server agrees, with a specific refusal rather than a failure.
	const res = await f.api.bob.post(`/api/plans/${f.targetID}/demand-leverage`, { data: { asset_ids: [] } });
	expect(res.status()).toBe(409);
	expect(await res.text()).toContain('leverage control is inert');
	// And nobody is kept waiting on bob for it: the bar names alice, the duelist.
	await expect(waiting(alice)).toContainText('You');
	await expect(waiting(carol)).toContainText('alice');
	for (const p of [alice, carol]) await expect(waiting(p)).not.toContainText('control leverage');
	await expect(waiting(alice)).not.toContainText('bob');
});

/**
 * Bob wins keep_or_change_target over alice's Spread Rumors (aimed at bob's
 * holding), alice keeps the rest, and alice has stated the rumor and skipped
 * the vote, so the roll is open. Returns the roll id.
 */
async function retargetHeld(f: H.MDFixture): Promise<number> {
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['keep_or_change_target', 'keep_assets'], alice: ['control_leverage', 'perform_steps'] });
	await H.completeDemand(f);
	await H.startTarget(f);
	const cast = await H.call(f.api.alice, 'post', `/api/plans/${f.targetID}/sr-cast-roll`, { rumor: 'A rumor.', how: 'Over the wine.' });
	await H.call(f.api.alice, 'post', `/api/rolls/${cast.roll.id}/skip-vote`);
	return cast.roll.id;
}

async function targetOf(f: H.MDFixture) {
	const p = await H.planOf(f, f.targetID);
	const rd = JSON.parse(p.resolution_data ?? '{}');
	return { asset: p.target_asset_id as number | null, player: p.target_player_id as number | null, settled: rd.demand_retarget_finalized === true };
}

test('keep or change target: the winner is asked either way, the roll waits on them, and re-aiming moves the plan for everyone', async ({ browser }) => {
	const f = await H.seedMD({ target: 'spread_rumors' });
	await retargetHeld(f);
	const pages = await H.openPages(browser, f);
	const { alice, bob, carol } = pages;
	const before = await targetOf(f);
	expect(before.settled).toBe(false);
	const carolsAsset = await assetNamed(f, "carol's holding");
	expect(before.asset).not.toBe(carolsAsset);

	// Wait state: the table is told the roll waits on bob; only bob is asked.
	for (const p of [alice, carol]) {
		await expect(p.getByText('bob decides whether this plan keeps or changes its target, and the roll waits until they say.')).toBeVisible();
		await expect(p.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
		await expect(p.getByRole('button', { name: 'Keep the current target' })).toHaveCount(0);
	}
	await expect(bob.getByText('Keeping the target is as much your call as re-aiming it — but the table needs to hear which, so the roll waits on you.')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Keep the current target' })).toBeEnabled();

	// Bob re-aims through the form: a different player, then one of their assets.
	await bob.locator('.demand-form', { hasText: "Keep or change this plan's target" }).getByRole('button', { name: 'carol' }).click();
	await bob.locator('.card', { hasText: "carol's holding" }).getByRole('checkbox').click();
	await bob.getByRole('button', { name: 'Re-aim this plan' }).click();

	await expect.poll(() => targetOf(f)).toEqual({ asset: carolsAsset, player: f.ids.carol, settled: true });
	await expect(bob.getByText("You've settled where this plan is aimed — the roll is no longer held for you.")).toBeVisible();
	// ...and the new aim reaches the others without a reload.
	for (const p of [alice, carol]) await expect(p.getByText("Targeting: carol's holding")).toBeVisible();
	await expect.poll(() => H.postsText(f)).toContain("bob re-aimed alice's Spread Rumors under their demand.");
});

test('keep or change target: "keep the current target" settles it without moving the plan', async ({ browser }) => {
	const f = await H.seedMD({ target: 'spread_rumors' });
	await retargetHeld(f);
	const { bob } = await H.openPages(browser, f);
	const before = await targetOf(f);

	await bob.getByRole('button', { name: 'Keep the current target' }).click();
	await expect.poll(() => targetOf(f)).toEqual({ ...before, settled: true });
	await expect(bob.getByText("You've settled where this plan is aimed — the roll is no longer held for you.")).toBeVisible();
	await expect.poll(() => H.postsText(f)).toContain("bob let alice's Spread Rumors stand exactly as aimed.");

	// Only bob may settle it.
	const carolTries = await f.api.carol.post(`/api/plans/${f.targetID}/demand-retarget`, { data: { keep: true } });
	expect(carolTries.status()).toBe(403);
});

async function assetNamed(f: H.MDFixture, name: string): Promise<number> {
	const { assets } = await H.call(f.api.alice, 'get', `/api/tables/${f.gameID}/assets`);
	const a = (assets as { id: number; name: string }[]).find(x => x.name === name);
	expect(a, name).toBeTruthy();
	return a!.id;
}

/**
 * Bob wins perform_steps (and keep_assets) over alice's Seek Answers; alice
 * keeps leverage and retarget, so neither pre-roll gate applies. The roll is
 * forced to a make with two distinct faces, so two picks are owed. Returns the
 * plan with the choice step open.
 */
async function performStepsOpen(f: H.MDFixture) {
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['perform_steps', 'keep_assets'], alice: ['control_leverage', 'keep_or_change_target'] });
	await H.completeDemand(f);
	await H.startTarget(f);
	// The pre-roll is still the preparer's: perform_steps starts after the dice.
	const cast = await H.call(f.api.alice, 'post', `/api/plans/${f.targetID}/seek-cast-roll`, { narration: 'I read the ledgers.' });
	await H.call(f.api.alice, 'post', `/api/rolls/${cast.roll.id}/skip-vote`);
	await H.call(f.api.alice, 'post', '/api/dev/force-roll', { roll_id: cast.roll.id, result: 2 });
}

test('perform steps: the roll stays the preparer\'s, then the winner takes over the choices and everyone is told who is acting', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await performStepsOpen(f);
	const { alice, bob, carol } = await H.openPages(browser, f);

	// Everyone sees the demand in effect.
	for (const p of [alice, bob, carol]) await expect(p.getByText('Demand in effect from bob')).toBeVisible();
	// The winner is told they act in the preparer's place; the others, who.
	await expect(bob.getByText("You will submit this plan's make/mar choices in alice's place.")).toBeVisible();
	for (const p of [alice, carol]) {
		await expect(p.getByText("bob will submit this plan's make/mar choices in alice's place.")).toBeVisible();
		await expect(waiting(p)).toContainText('bob');
	}
	await expect(waiting(bob)).toContainText('You');
	await expect(carol.getByText('bob is choosing make/mar options…')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);

	// Only bob can act: alice, though it is her plan, is refused.
	const refused = await f.api.alice.post(`/api/plans/${f.targetID}/make-choice`, { data: { result: 'make', choices: ['declare_truth', 'declare_truth'] } });
	expect(refused.status()).toBe(403);

	// Bob picks through the page: two declared truths, then writes them.
	const more = bob.getByRole('button', { name: 'More Declare something true about the world' });
	await more.click();
	await more.click();
	await expect(bob.getByText('Total picks: 2 / 2')).toBeVisible();
	await bob.getByRole('button', { name: 'Apply choices' }).click();
	await expect.poll(async () => (await H.planOf(f, f.targetID)).status).toBe('resolving');
	for (const text of ['The seal was forged.', 'The duke never left.']) {
		await H.call(f.api.bob, 'post', `/api/plans/${f.targetID}/declare-truth`, { text });
	}

	// Outcome: the plan is done, and the truths were declared in bob's name.
	await expect.poll(async () => (await H.planOf(f, f.targetID)).status, { timeout: 10_000 }).toBe('resolved');
	await expect.poll(() => H.postsText(f)).toContain('The seal was forged.');
	await expect.poll(() => H.postsText(f)).toContain('The duke never left.');
});

// The preparer is shown the same picker as the winner and told nothing is
// stopping them, although the server refuses their submission.
// See docs/work/next/perform-steps-panels-gate-on-preparer.md (six panels share this).
test.fixme('perform steps: the preparer sees a wait line, not a picker she cannot submit', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await performStepsOpen(f);
	const { alice } = await H.openPages(browser, f);
	await expect(alice.getByText('bob is choosing make/mar options…')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
});

test('a marred demand: the target preparer places the counter-demand, the demander waits, and placing it ends the plan', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'mar');
	const { alice, bob, carol } = await H.openPages(browser, f);

	// Only alice — whose plan was demanded against — is asked.
	await expect(alice.getByText('The demand was marred. You may place a free counter-demand')).toBeVisible();
	for (const p of [bob, carol]) {
		await expect(p.getByText('Waiting for alice to place a counter-demand…')).toBeVisible();
		await expect(p.getByRole('button', { name: 'Place counter-demand' })).toHaveCount(0);
	}
	await expect(bob.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);

	// bob's only unresolved plan is the demand itself, which can't be countered, so she defers.
	const place = alice.getByRole('button', { name: 'Place counter-demand' });
	await expect(place).toBeDisabled();
	await alice.locator('.choice-card', { hasText: 'Defer' }).click();
	await place.click();

	// Placing it is the last step: the demand completes itself, no click to agree.
	await expect.poll(async () => (await H.planOf(f, f.demandID)).status, { timeout: 10_000 }).toBe('resolved');
	await expect(bob.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);
	// A marred demand wins nobody any of the four options.
	expect((await H.planOf(f, f.demandID)).result).toBe('mar');
});

// The banner promises keep_assets spoils the plan can never produce; the draft
// card for the same option says Seek Answers gives its preparer no assets.
// See docs/work/next/demand-overlay-forms-shown-to-preparer-and-after-roll.md.
test.fixme('perform steps: the banner does not promise spoils from a plan that awards none', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await performStepsOpen(f);
	const { alice, bob, carol } = await H.openPages(browser, f);
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Demand in effect from bob')).toBeVisible();
		await expect(p.getByText('Any assets this plan would have given alice go to bob instead.')).toHaveCount(0);
	}
});

// ── Overlay forms that cannot do anything ─────────────────────────────────────
// See docs/work/next/demand-overlay-forms-shown-to-preparer-and-after-roll.md.

test.fixme('overlay: a preparer who won keep-or-change-target herself is not asked to settle it', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await leverageHeld(f); // alice, the preparer, holds keep_or_change_target
	const { alice } = await H.openPages(browser, f);
	await expect(alice.getByRole('button', { name: 'Keep the current target' })).toHaveCount(0);
	await expect(alice.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
});

test.fixme('overlay: once the dice have landed, the leverage form is gone', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	const rollID = await leverageHeld(f);
	const { bob } = await H.openPages(browser, f);
	await bob.getByRole('button', { name: 'Leverage none — let the roll fail' }).click();
	for (const n of H.PLAYERS) await H.call(f.api[n], 'post', `/api/rolls/${rollID}/ready`, { is_ready: true });
	await expect.poll(async () => (await rollOf(f)).result).not.toBeNull();
	await expect(bob.getByRole('button', { name: 'Leverage none — let the roll fail' })).toHaveCount(0);
});

test.fixme('overlay: once the dice have landed, the retarget form is gone — the server no longer takes it', async ({ browser }) => {
	const f = await H.seedMD({ target: 'spread_rumors' });
	const rollID = await retargetHeld(f);
	const { bob } = await H.openPages(browser, f);
	await bob.getByRole('button', { name: 'Keep the current target' }).click();
	for (const n of H.PLAYERS) await H.call(f.api[n], 'post', `/api/rolls/${rollID}/ready`, { is_ready: true });
	await expect.poll(async () => (await rollOf(f)).result).not.toBeNull();
	const late = await f.api.bob.post(`/api/plans/${f.targetID}/demand-retarget`, { data: { keep: true } });
	expect(late.status()).toBe(409);
	await expect(bob.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
	await expect(bob.getByText('You may still adjust it until the plan resolves.')).toHaveCount(0);
});

// ── Server rules ──────────────────────────────────────────────────────────────

test('one demand per target: a second demand against the same plan is refused', async () => {
	// carol outranks bob in power, so the shield doesn't turn her away first.
	const f = await H.seedMD({ target: 'seek_answers', power: [2, 1, 0] });
	await H.prepareDemand(f);
	const res = await f.api.carol.post(`/api/tables/${f.gameID}/prepare-plan`, {
		data: { plan_type: 'make_demands', target_plan_id: f.targetID, preparation_notes: 'Me too.' },
	});
	expect(res.status()).toBe(400);
	expect(await res.text()).toContain('another demand already targets that plan');
	expect((await H.plansOf(f)).filter(p => p.plan_type === 'make_demands')).toHaveLength(1);
});

test('a counter-demand may only be aimed at the demander\'s plans', async () => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'mar');
	// alice's only other plan is her own, which isn't bob's to be countered.
	const res = await f.api.alice.post(`/api/plans/${f.demandID}/counter-demand`, { data: { target_plan_id: f.targetID } });
	expect(res.status()).toBe(400);
	expect(await res.text()).toContain('a counter-demand may only target a plan prepared by bob');
	// Nothing was recorded, so she can still defer.
	await H.call(f.api.alice, 'post', `/api/plans/${f.demandID}/counter-demand`, { target_plan_id: null });
});

test('retarget before the roll exists: the winner can settle it in the window between draft and cast', async ({ browser }) => {
	const f = await H.seedMD({ target: 'spread_rumors' });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['keep_or_change_target', 'keep_assets'], alice: ['control_leverage', 'perform_steps'] });
	await H.completeDemand(f);
	await H.startTarget(f); // alice has not yet stated the rumor: there is no roll
	const carolsHolding = await assetNamed(f, "carol's holding");
	const res = await f.api.bob.post(`/api/plans/${f.targetID}/demand-retarget`, { data: { target_player_id: f.ids.carol, target_asset_id: carolsHolding } });
	expect(res.status(), await res.text()).toBe(200);
	expect(await targetOf(f)).toEqual({ asset: carolsHolding, player: f.ids.carol, settled: true });
	const { alice } = await H.openPages(browser, f);
	await expect(alice.getByText("Targeting: carol's holding")).toBeVisible();
});
