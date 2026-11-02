import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedCHGame, runScene, prepareCH, resolveCH, castCH, forceCH, rolledCH, openAll, artifactOf,
	assetsOf, addMarginalia, planStatus, chronicleData, expectPlanLog, expectNoHorizontalScroll,
	closeContextsAfterEach, type CHGame, type Who,
} from './helpers/chronicle_histories';

// Regression specs for Chronicle Histories, the knowledge plan with a pre-roll
// scene and no asset reward:
//   · preparation takes only the historical problem (notes), nothing else;
//   · pre-roll: everyone sees the live difficulty meter (max of the preparer's
//     knowledge rank and the artifacts invoked), only the preparer picks, and
//     one submit invokes, posts the scene and casts the dice;
//   · the roll's outcome is applied the moment the dice land: a make owes the
//     preparer exactly `result` options, one at a time; a mar owes every player
//     one option;
//   · break_artifact is depletable (a torn marginalia stays torn, a last one
//     destroys the artifact) and the plan awards no asset of any kind;
//   · who sees what at 375px: the preparer acts, the others see the committed
//     steps, and a bystander never gets a live control.
//
// Setup goes over the API. Dice are forced with /api/dev/force-roll. Seed ranks
// put alice at knowledge rank 2, so the roll's difficulty is 2 unless three
// artifacts are invoked; alice rolls two dice, so a make is result 2 (two
// options) and a mar is result 1.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const WHO_ALL: Who[] = ['alice', 'bob', 'carol'];
const PHONE = { width: 375, height: 812 };

/** A 4xx, exactly: a 500 can't pass for "the server said no". */
async function expectStatus(res: { status(): number; text(): Promise<string> }, status: number) {
	expect(res.status(), await res.text()).toBe(status);
}

/** Tick an artifact card in the pre-roll / invoke pickers. */
async function pickArtifact(page: Page, name: string) {
	await page.locator('.card', { hasText: name }).getByRole('checkbox').first().click();
}

/** Tick one marginalia line of an artifact card in a break picker. */
async function pickMarginalia(page: Page, artifact: string, text: string) {
	await page.locator('.card', { hasText: artifact }).locator('li', { hasText: text }).getByRole('checkbox').click();
}

async function chooseOption(page: Page, label: string) {
	await page.getByRole('button', { name: label, exact: true }).click();
}

async function resolvingGame(browser: Parameters<typeof seedCHGame>[0], viewport?: { width: number; height: number }) {
	const g = await seedCHGame(browser, viewport);
	track(g.gameID);
	await runScene(g);
	const planID = await prepareCH(g);
	await resolveCH(g, planID);
	return { g, planID };
}

const OPT = {
	break: 'Break an invoked artifact',
	invoke: 'Invoke another artifact',
	echo: "Cut to the present to show history's impact",
	control: 'Take narrative control of a moment',
};

test('preparation: only the historical problem is asked, and it is required', async ({ browser }) => {
	const g = await seedCHGame(browser, PHONE);
	track(g.gameID);
	await runScene(g);
	const alice = await g.ctx.alice.newPage();
	await alice.goto(`/table/${g.gameID}`);

	await alice.getByText('Chronicle Histories', { exact: true }).click();
	const prepare = alice.getByRole('button', { name: 'Prepare Plan' });
	await expect(prepare).toBeVisible();
	await expect(prepare).toBeDisabled();
	await expectNoHorizontalScroll(alice);

	// The server refuses an empty problem with a 400, and a target nobody needs.
	const empty = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'chronicle_histories', preparation_notes: '' },
	});
	await expectStatus(empty, 400);

	await alice.getByPlaceholder('What problem are you solving?', { exact: false }).fill('Why was the old treaty broken?');
	await expect(prepare).toBeEnabled();
	await prepare.click();

	await expect.poll(async () => {
		const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
		const { plans } = (await res.json()) as { plans: { plan_type: string; preparation_notes: string; delay?: number }[] };
		return plans.find(p => p.plan_type === 'chronicle_histories')?.preparation_notes;
	}).toBe('Why was the old treaty broken?');
});

test('pre-roll: everyone sees the live meter, only the preparer picks, one submit casts the dice', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser, PHONE);
	const { alice, bob, carol } = await openAll(g);

	// Everyone sees the same starting difficulty, floored by alice's knowledge rank.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('knowledge rank 2 · 0 artifacts invoked → max = 2')).toBeVisible();
	}
	// The others wait; they get neither the submit button nor an enabled scene field.
	await expect(bob.getByText('alice is setting the scene…')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Set the scene & gather your dice' })).toHaveCount(0);
	await expect(carol.getByPlaceholder('Describe the moment from the past')).toBeDisabled();

	// Invoking three artifacts raises the meter past the rank floor, live, for alice.
	await pickArtifact(alice, "alice's artifact");
	await pickArtifact(alice, "bob's artifact");
	await pickArtifact(alice, "carol's artifact");
	await expect(alice.getByText('knowledge rank 2 · 3 artifacts invoked → max = 3')).toBeVisible();
	// Two invocations sit under the floor: the difficulty stays at the rank.
	await pickArtifact(alice, "carol's artifact");
	await expect(alice.getByText('knowledge rank 2 · 2 artifacts invoked → max = 2')).toBeVisible();

	// The submit needs a scene.
	const cast = alice.getByRole('button', { name: 'Set the scene & gather your dice' });
	await expect(cast).toBeDisabled();
	await alice.getByPlaceholder('Describe the moment from the past').fill('A cold archive, long ago.');
	await expectNoHorizontalScroll(alice);
	await cast.click();

	// The roll exists with difficulty 2, and the invocations are recorded.
	await expect.poll(async () => (await chronicleData(g, planID)).invoke_phase_closed).toBe(true);
	const art = await Promise.all((['alice', 'bob'] as Who[]).map(w => artifactOf(g, w)));
	expect((await chronicleData(g, planID)).invoked_artifact_ids?.sort()).toEqual(art.map(a => a.id).sort());
	const active = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/rolls/active`)).json();
	expect(active.roll.difficulty).toBe(2);

	// The invoke phase is closed for good: the meter and picker are gone for all,
	// and the invoked list is shown instead.
	await expect(bob.getByText('Invoked artifacts (2)')).toBeVisible();
	await expect(bob.getByText('Starting difficulty')).toHaveCount(0);
	await expectNoHorizontalScroll(bob);
});

test('pre-roll: only the preparer may cast, and only real artifacts of this game', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const bobArtifact = await artifactOf(g, 'bob');
	const notArtifact = (await assetsOf(g)).find(a => a.asset_type !== 'artifact')!;

	await expectStatus(
		await g.ctx.bob.request.post(`/api/plans/${planID}/cast-roll`, { data: { artifact_ids: [bobArtifact.id], scene: 'x' } }),
		403);
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/cast-roll`, { data: { artifact_ids: [notArtifact.id], scene: 'x' } }),
		400);
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/cast-roll`, { data: { artifact_ids: [999999], scene: 'x' } }),
		400);
	// None of the refusals closed the phase.
	expect((await chronicleData(g, planID)).invoke_phase_closed ?? false).toBe(false);

	await castCH(g, planID, [bobArtifact.id]);
	// …and the dice can only be cast once.
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/cast-roll`, { data: { artifact_ids: [], scene: 'again' } }),
		409);
});

test('pre-roll: the roll\'s difficulty is the higher of the rank and the artifacts invoked', async ({ browser }) => {
	const { g, planID } = await resolvingGame(browser);
	const ids = await Promise.all(WHO_ALL.map(async w => (await artifactOf(g, w)).id));
	// Three artifacts beat alice's rank of 2; a repeated id counts once.
	await castCH(g, planID, [...ids, ids[0]]);
	const active = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/rolls/active`)).json();
	expect(active.roll.difficulty).toBe(3);
	expect((await chronicleData(g, planID)).invoked_artifact_ids).toEqual(ids);
});

// Owner ruling (2026-11-01): make options are repeatable. The best result is 6,
// so the picker must not retire an option once it has been used.
test('make: the same option can be chosen again, through the UI', async ({ browser }) => {
	const g = await seedCHGame(browser);
	track(g.gameID);
	const { planID, roll } = await rolledCH(g, ['bob', 'carol'], 'make');
	expect(roll.result).toBe(2);
	const { alice } = await openAll(g);

	for (const n of [1, 2]) {
		await chooseOption(alice, OPT.echo);
		await alice.getByPlaceholder('Describe this beat of the scene').fill(`Another turn of the page, ${n}.`);
		await alice.getByRole('button', { name: 'Submit choice' }).click();
		await expect.poll(async () => (await chronicleData(g, planID)).make_choices_done ?? 0).toBe(n);
	}
	expect((await chronicleData(g, planID)).steps?.map(s => s.option)).toEqual(['echo_present', 'echo_present']);
	await expect(alice.getByRole('button', { name: 'Wrap the scene' })).toBeVisible();
});

test('make: the outcome is applied on the roll; two options, one at a time, and no asset is gained', async ({ browser }) => {
	const g = await seedCHGame(browser);
	track(g.gameID);
	await addMarginalia(g, 'bob', 'a second, kept note');
	const before = await assetsOf(g);
	const { planID, roll, artifacts } = await rolledCH(g, ['bob', 'carol'], 'make');
	expect(roll.result).toBe(2);
	const [bobArt, carolArt] = artifacts;
	const pages = await openAll(g);
	const { alice, bob, carol } = pages;

	// The budget is on record from the instant the dice landed — before any step.
	await expect.poll(async () => (await chronicleData(g, planID)).make_budget).toBe(2);
	expect((await chronicleData(g, planID)).make_choices_done ?? 0).toBe(0);
	await expect(alice.getByText('2 of 2 choices remaining')).toBeVisible();
	await expect(bob.getByText('alice is choosing options…')).toBeVisible();
	await expectPlanLog(alice, ['Chronicle Histories succeeded']);

	// The plan cannot be wrapped with options still owed.
	await expectStatus(await g.ctx.alice.request.post(`/api/plans/${planID}/complete`), 409);
	// Nor can anyone but the preparer choose.
	await expectStatus(
		await g.ctx.bob.request.post(`/api/plans/${planID}/make-step`, { data: { option: 'echo_present' } }), 403);

	// Step 1 through the UI: tear bob's second note. His artifact survives the tear.
	await chooseOption(alice, OPT.break);
	await expect(alice.getByRole('button', { name: 'Submit choice' })).toBeDisabled();
	await pickMarginalia(alice, "bob's artifact", 'a second, kept note');
	await alice.getByPlaceholder('Describe this beat of the scene').fill('The ledger is ripped.');
	await alice.getByRole('button', { name: 'Submit choice' }).click();

	await expect(alice.getByText('1 of 2 choices remaining')).toBeVisible();
	// The committed step is on every screen, at full strength, with its words.
	// (The others read it on their next load; live push is the fixme below.)
	await Promise.all([bob.reload(), carol.reload()]);
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Resolved so far:')).toBeVisible();
		await expect(p.locator('.resolved-note', { hasText: 'The ledger is ripped.' })).toBeVisible();
	}
	const torn = (await assetsOf(g)).find(a => a.id === bobArt.id)!;
	expect(torn.is_destroyed).toBe(false);
	expect(torn.marginalia.filter(m => m.is_torn).map(m => m.text)).toEqual(['a second, kept note']);

	// Step 2: a narrative option, sent with a stale asset_id as an out-of-date
	// client might. The last step makes the wrap-up button appear.
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/make-step`, {
			data: { option: 'echo_present', asset_id: bobArt.id, narration: 'The treaty reads differently now.' } }),
		200);
	await Promise.all([alice.reload(), bob.reload()]);
	await expect(alice.getByRole('button', { name: 'Wrap the scene' })).toBeVisible();
	await expect(bob.getByText(/All choices are made\. The scene stays open until\s+alice wraps it\./)).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Wrap the scene' })).toHaveCount(0);

	// A third option is refused, exactly.
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/make-step`, { data: { option: 'total_control' } }), 409);
	const done = await chronicleData(g, planID);
	expect(done.make_choices_done).toBe(2);
	expect(done.steps?.map(s => s.option)).toEqual(['break_artifact', 'echo_present']);
	expect(done.steps?.[0].asset_id).toBe(bobArt.id);
	// The narrative option records no target even if a stale client sent one.
	expect(done.steps?.[1].asset_id ?? null).toBeNull();

	await alice.getByRole('button', { name: 'Wrap the scene' }).click();
	await expect.poll(() => planStatus(g, planID)).toBe('resolved');

	// No asset was gained, lost or moved: the same assets, the same owners, only
	// bob's one tear differs. Carol's invoked artifact is untouched.
	const after = await assetsOf(g);
	expect(after.map(a => [a.id, a.owner_id, a.is_destroyed])).toEqual(before.map(a => [a.id, a.owner_id, a.is_destroyed]));
	expect(after.find(a => a.id === carolArt.id)!.marginalia.every(m => !m.is_torn)).toBe(true);

	await expectPlanLog(alice, [
		'Chronicle Histories succeeded',
		"The ledger is ripped.",
		"cut to the present to show history's impact",
	]);
});

test('make: breaking the last marginalia destroys the artifact, and a torn line cannot be torn again', async ({ browser }) => {
	const g = await seedCHGame(browser);
	track(g.gameID);
	const { planID, artifacts } = await rolledCH(g, ['carol', 'bob'], 'make');
	const [carolArt] = artifacts;
	const { alice, bob } = await openAll(g);

	const marg = carolArt.marginalia.find(m => !m.is_torn)!;
	await expect(alice.getByText('2 of 2 choices remaining')).toBeVisible();
	await chooseOption(alice, OPT.break);
	await pickMarginalia(alice, "carol's artifact", marg.text);
	// A one-line artifact torn means destroyed — the picker warns before the pick commits.
	await expect(alice.locator('.res-warning')).toBeVisible();
	await alice.getByPlaceholder('Describe this beat of the scene').fill('Her relic crumbles.');
	await alice.getByRole('button', { name: 'Submit choice' }).click();
	await expect(alice.getByText('1 of 2 choices remaining')).toBeVisible();

	expect((await assetsOf(g)).find(a => a.id === carolArt.id)!.is_destroyed).toBe(true);
	// A destroyed artifact is not offered again for a break.
	await chooseOption(alice, OPT.break);
	await expect(alice.locator('.card', { hasText: "bob's artifact" })).toBeVisible();
	await expect(alice.locator('.card', { hasText: "carol's artifact" })).toHaveCount(0);

	// The server holds the line on a stale pick: that marginalia is already torn.
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/make-step`, {
			data: { option: 'break_artifact', asset_id: carolArt.id, marginalia_id: marg.id } }),
		409);
	// …and a break needs an artifact that was invoked in this plan.
	const unInvoked = await artifactOf(g, 'alice');
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/make-step`, {
			data: { option: 'break_artifact', asset_id: unInvoked.id, marginalia_id: unInvoked.marginalia[0].id } }),
		400);
	expect((await chronicleData(g, planID)).make_choices_done).toBe(1);

	// Invoke another: alice's own artifact joins the scene (invoked, not gained).
	await chooseOption(alice, OPT.invoke);
	await pickArtifact(alice, "alice's artifact");
	await alice.getByPlaceholder('Describe this beat of the scene').fill('Her own house appears in the record.');
	await alice.getByRole('button', { name: 'Submit choice' }).click();
	await expect(alice.getByRole('button', { name: 'Wrap the scene' })).toBeVisible();
	expect((await chronicleData(g, planID)).invoked_artifact_ids).toContain(unInvoked.id);
	// Everyone reads the scene's list; a destroyed relic stays on it as history.
	await bob.reload();
	await expect(bob.getByText('Invoked artifacts (3)')).toBeVisible();
	await expect(bob.getByText("carol's artifact (carol)")).toBeVisible();
});

test('mar: every player, the preparer included, chooses exactly one; the plan waits for all', async ({ browser }) => {
	const g = await seedCHGame(browser);
	track(g.gameID);
	await addMarginalia(g, 'bob', 'a second, kept note');
	const before = await assetsOf(g);
	const { planID, roll, artifacts } = await rolledCH(g, ['bob', 'carol'], 'mar');
	expect(roll.result).toBe(1);
	const [bobArt, carolArt] = artifacts;
	const { alice, bob, carol } = await openAll(g);

	// The gate is on record from the dice: three choosers, none yet.
	await expect.poll(async () => (await chronicleData(g, planID)).mar_required_choices).toBe(3);
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('Submitted so far (0):')).toBeVisible();
		await expect(p.getByText('Your choice')).toBeVisible();
	}
	// The wrap button is the preparer's, and shut until everyone has chosen.
	await expect(alice.getByRole('button', { name: 'Wrap the scene' })).toBeDisabled();
	await expect(alice.getByText('Waiting for all players to choose (0/3)…')).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Wrap the scene' })).toHaveCount(0);
	await expectStatus(await g.ctx.alice.request.post(`/api/plans/${planID}/complete`), 409);

	// Bob tears a line on his own artifact through the UI.
	await chooseOption(bob, OPT.break);
	await pickMarginalia(bob, "bob's artifact", 'a second, kept note');
	await bob.getByRole('button', { name: 'Submit choice' }).click();
	await expect(bob.getByText('Your choice has been submitted.')).toBeVisible();
	// The others read it on their next load (live push: see the fixme below).
	await Promise.all([alice.reload(), carol.reload()]);
	await expect(alice.getByText('bob: broke')).toBeVisible();
	await expect(alice.getByText('Waiting for all players to choose (1/3)…')).toBeVisible();
	await expect(carol.getByText('bob: broke')).toBeVisible();
	// One choice per player, exactly.
	await expectStatus(
		await g.ctx.bob.request.post(`/api/plans/${planID}/mar-choice`, { data: { choice: 'echo_present' } }), 409);

	// Carol picks a narrative option; alice (the preparer) chooses too.
	await chooseOption(carol, OPT.echo);
	await carol.getByRole('button', { name: 'Submit choice' }).click();
	await expect(carol.getByText('Your choice has been submitted.')).toBeVisible();
	await alice.reload();
	await expect(alice.getByText('Waiting for all players to choose (2/3)…')).toBeVisible();
	await expectStatus(await g.ctx.alice.request.post(`/api/plans/${planID}/complete`), 409);
	await chooseOption(alice, OPT.control);
	await alice.getByRole('button', { name: 'Submit choice' }).click();

	await expect(alice.getByText('Every player has chosen.')).toBeVisible();
	await alice.getByRole('button', { name: 'Wrap the scene' }).click();
	await expect.poll(() => planStatus(g, planID)).toBe('resolved');

	const d = await chronicleData(g, planID);
	expect(d.steps?.map(s => [s.player_id, s.option])).toEqual([
		[expect.any(Number), 'break_artifact'],
		[expect.any(Number), 'echo_present'],
		[expect.any(Number), 'total_control'],
	]);
	expect(new Set(d.steps?.map(s => s.player_id)).size).toBe(3);

	// A mar awards nothing either: same assets, same owners, one tear on bob's.
	const after = await assetsOf(g);
	expect(after.map(a => [a.id, a.owner_id, a.is_destroyed])).toEqual(before.map(a => [a.id, a.owner_id, a.is_destroyed]));
	expect(after.find(a => a.id === bobArt.id)!.marginalia.filter(m => m.is_torn)).toHaveLength(1);
	expect(after.find(a => a.id === carolArt.id)!.marginalia.every(m => !m.is_torn)).toBe(true);
	await expectPlanLog(alice, ['broke', "cut to the present to show history's impact", 'took narrative control of a moment']);
});

test('mar: a depleted artifact is gone from the next chooser\'s picker, and make-step is refused', async ({ browser }) => {
	const g = await seedCHGame(browser);
	track(g.gameID);
	const { planID, artifacts } = await rolledCH(g, ['carol', 'bob'], 'mar');
	const [carolArt] = artifacts;
	const { bob, carol } = await openAll(g);

	// Make-path routes don't apply to a mar.
	await expectStatus(
		await g.ctx.alice.request.post(`/api/plans/${planID}/make-step`, { data: { option: 'echo_present' } }), 409);

	// Bob destroys carol's one-line artifact.
	const marg = carolArt.marginalia.find(m => !m.is_torn)!;
	await chooseOption(bob, OPT.break);
	await pickMarginalia(bob, "carol's artifact", marg.text);
	await bob.getByRole('button', { name: 'Submit choice' }).click();
	await expect(bob.getByText('Your choice has been submitted.')).toBeVisible();

	// Carol's own picker no longer offers it.
	await carol.reload();
	await expect(carol.getByText('bob: broke')).toBeVisible();
	await chooseOption(carol, OPT.break);
	await expect(carol.locator('.card', { hasText: "bob's artifact" })).toBeVisible();
	await expect(carol.locator('.card', { hasText: "carol's artifact" })).toHaveCount(0);
	// A stale client is held to the same rule.
	await expectStatus(
		await g.ctx.carol.request.post(`/api/plans/${planID}/mar-choice`, {
			data: { choice: 'break_artifact', asset_id: carolArt.id, marginalia_id: marg.id } }),
		409);
	// A break needs both ids and a real choice name.
	await expectStatus(
		await g.ctx.carol.request.post(`/api/plans/${planID}/mar-choice`, { data: { choice: 'break_artifact' } }), 400);
	await expectStatus(
		await g.ctx.carol.request.post(`/api/plans/${planID}/mar-choice`, { data: { choice: 'sack_the_archive' } }), 400);

	// Carol invokes alice's artifact instead, through the picker: the scene grows
	// by one, and the step is recorded against carol with that artifact.
	const aliceArt = await artifactOf(g, 'alice');
	await chooseOption(carol, OPT.invoke);
	await pickArtifact(carol, "alice's artifact");
	await carol.getByRole('button', { name: 'Submit choice' }).click();
	await expect(carol.getByText('Your choice has been submitted.')).toBeVisible();
	const d = await chronicleData(g, planID);
	expect(d.invoked_artifact_ids).toContain(aliceArt.id);
	expect(d.steps?.find(x => x.option === 'invoke_another')?.asset_id).toBe(aliceArt.id);
});

for (const outcome of ['make', 'mar'] as const) {
	test(`375px, ${outcome}: preparer, other player and bystander each see what is theirs`, async ({ browser }) => {
		const g = await seedCHGame(browser, PHONE);
		track(g.gameID);
		// Only bob's artifact is invoked, so carol is genuinely uninvolved: no
		// artifact of hers is in the scene, and she is neither preparer nor invoked.
		await rolledCH(g, ['bob'], outcome);
		const { alice, bob, carol } = await openAll(g);

		for (const p of [alice, bob, carol]) {
			await expect(p.getByText(outcome === 'make' ? '✓ Make' : '✗ Mar')).toBeVisible();
			await expect(p.getByText('Invoked artifacts (1)')).toBeVisible();
			await expectNoHorizontalScroll(p);
		}
		// The waiting bar names the people who owe something, and not the others.
		if (outcome === 'make') {
			await expect(alice.getByRole('button', { name: 'Submit choice' })).toBeVisible();
			// Non-preparers see the controls greyed out, never live.
			for (const p of [bob, carol]) {
				await expect(p.getByText('alice is choosing options…')).toBeVisible();
				await expect(p.getByRole('button', { name: OPT.echo, exact: true })).toBeDisabled();
				await expect(p.getByRole('button', { name: 'Submit choice' })).toHaveCount(0);
			}
		} else {
			// Everyone gets their own live picker.
			for (const p of [alice, bob, carol]) {
				await expect(p.getByRole('button', { name: OPT.echo, exact: true })).toBeEnabled();
			}
			await expect(alice.getByRole('button', { name: 'Wrap the scene' })).toBeDisabled();
		}
		// Tap targets: the option chips are at least 44px tall.
		const h = await alice.getByRole('button', { name: OPT.echo, exact: true }).boundingBox();
		expect(h!.height).toBeGreaterThanOrEqual(44);
	});
}

test('the auto-applied outcome reaches open panels live, without a reload', async ({ browser }) => {
	const g = await seedCHGame(browser, PHONE);
	track(g.gameID);
	await runScene(g);
	const planID = await prepareCH(g);
	await resolveCH(g, planID);
	const { alice, bob } = await openAll(g);
	const bobArt = await artifactOf(g, 'bob');
	const rollID = await castCH(g, planID, [bobArt.id]);
	await expect(bob.getByText('Invoked artifacts (1)')).toBeVisible();
	await forceCH(g, rollID, 'make');
	// No reload: the preparer's choices and the others' waiting line arrive live.
	await expect(alice.getByText('2 of 2 choices remaining')).toBeVisible();
	await expect(bob.getByText('alice is choosing options…')).toBeVisible();
});

test.fixme('another player\'s mar choice reaches open panels and the Waiting On bar without a reload', async ({ browser }) => {
	// Pinned by docs/work/next/chronicle-choices-not-pushed-to-open-panels.md:
	// make-step and mar-choice save resolution_data and post to the log but never
	// broadcast plan.choice_applied, so everyone else's panel and bar stay stale.
	const g = await seedCHGame(browser);
	track(g.gameID);
	const { planID } = await rolledCH(g, ['bob', 'carol'], 'mar');
	const { alice, carol } = await openAll(g);
	await expect(alice.getByText('Submitted so far (0):')).toBeVisible();
	await expectStatus(
		await g.ctx.bob.request.post(`/api/plans/${planID}/mar-choice`, { data: { choice: 'echo_present' } }), 200);
	for (const p of [alice, carol]) {
		await expect(p.getByText('Submitted so far (1):')).toBeVisible();
	}
	// bob has chosen, so the bar must stop naming him and still name carol.
	const bar = alice.getByText('Waiting On:').locator('..');
	await expect(bar).toContainText('carol');
	await expect(bar).not.toContainText('bob');
});

test.fixme('the Buffet\'s Mar tab says every player chooses, the preparer included', async ({ browser }) => {
	// Pinned by docs/work/next/chronicle-buffet-mar-says-other-players.md: the rules
	// have all players choose on a mar, and the panel's own note says "including
	// the preparer", but the reference tab says "Every OTHER player".
	const { g } = await resolvingGame(browser, PHONE);
	const alice = await g.ctx.alice.newPage();
	await alice.goto(`/table/${g.gameID}`);
	await alice.getByText('What can happen?').click();
	await alice.getByRole('tab', { name: 'Mar', exact: true }).click();
	await expect(alice.getByText('Every player chooses one of these instead', { exact: false })).toBeVisible();
	await expect(alice.getByText('Every OTHER player')).toHaveCount(0);
});
