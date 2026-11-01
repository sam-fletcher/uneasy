import { test, expect, request as pwRequest, type APIRequestContext, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';

// Regression specs for what the 12-plan UX audit fixed in Spread Rumors:
//
//   - make flow: the pre-roll narration is a step of its own (the roll opens
//     only after it), the Stepper picker, and take-consent — the victim agrees
//     or refuses, a repeat request is refused, and a refusal locks the option
//     off for good.
//   - mar flow: the target-asset owner (not the preparer) is the actor. They
//     write the counter-rumor first, the Waiting On set names them, and the
//     consent screen the preparer sees names the preparer's own main character
//     (the rumor that spreads is about it), not the rumor's original target.
//   - a zero-pick commit closes the counter-rumor and hands the wait on.
//   - the shared Stepper: named buttons, quota cap, 44px targets at 375px.
//
// Setup (seed, prepare, advance, cast the roll, force the dice) goes over the
// API — it is not the subject. Everything a player does in the plan is driven
// through the rendered panel as that player, in a separate browser context per
// player (Playwright pages in one context share cookies).
//
// Seeded geometry: alice prepares, about "bob's holding"; bob owns the target
// asset, so on a mar bob is the actor. cara is a bystander. Every player holds a
// main character and one asset of each type, each with a marginalia.

const MOBILE = { width: 375, height: 812 };
const RUMOR = 'The Duchess waters her own wine with hemlock.';
const HOW = 'Whispered over every tavern table in the lower city.';
const COUNTER = 'Her accuser keeps a second set of books.';

interface Players {
	alice: number;
	bob: number;
	cara: number;
}

interface Table {
	gameID: number;
	planID: number;
	rollID: number;
	players: Players;
	alice: BrowserContext;
	bob: BrowserContext;
	cara: BrowserContext;
	/** Asset id by name, e.g. "bob's artifact". */
	asset: (name: string) => number;
}

const track = cleanupGameAfterEach();

// Every context a test opens, closed after it whether it passed or not.
const opened: BrowserContext[] = [];
test.afterEach(async () => {
	await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
});

async function ok(res: { ok(): boolean; text(): Promise<string> }, what: string) {
	expect(res.ok(), `${what} failed: ${await res.text()}`).toBeTruthy();
}

/**
 * A three-player main_event game with alice's Spread Rumors about bob's holding,
 * advanced to its row and resolving. With `castRoll` the preparer's pre-roll is
 * also posted over the API and the dice forced to `roll`; without it the plan
 * parks at the pre-roll step so the UI can be driven through it.
 *
 * A mar needs a difficulty above 1 and the dice give it a result of 1, so
 * `difficulty: 3` (alice last on the esteem track) buys a mar two picks.
 */
async function openTable(
	browser: Browser,
	opts: { roll?: 'make' | 'mar'; difficulty?: 2 | 3 } = {},
): Promise<Table> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: ['alice', 'bob', 'cara'],
			rankings: opts.difficulty === 3 ? { esteem: [1, 2, 0] } : undefined,
		},
	});
	await ok(seedRes, 'seed');
	const seeded = await seedRes.json();
	await fixture.dispose();
	const gameID: number = seeded.game_id;
	track(gameID);
	const players: Players = {
		alice: seeded.players[0].id,
		bob: seeded.players[1].id,
		cara: seeded.players[2].id,
	};

	const mk = async (name: string) => {
		const c = await browser.newContext({ baseURL: E2E, viewport: MOBILE });
		opened.push(c);
		await ok(await c.request.post(`/api/dev/login?username=${name}`), `login ${name}`);
		return c;
	};
	const [alice, bob, cara] = await Promise.all([mk('alice'), mk('bob'), mk('cara')]);

	const assetsRes = await alice.request.get(`/api/tables/${gameID}/assets`);
	await ok(assetsRes, 'GET assets');
	const byName = new Map<string, number>(
		((await assetsRes.json()).assets as { id: number; name: string }[]).map(a => [a.name, a.id]),
	);
	const asset = (name: string) => {
		const id = byName.get(name);
		expect(id, `seeded asset "${name}"`).toBeTruthy();
		return id!;
	};

	// Prepare at row 1 (the plan lands at row 5), jump there and resolve. Advancing
	// onto the row may already have kicked resolution off; either is fine.
	const prep = await alice.request.post(`/api/tables/${gameID}/prepare-plan`, {
		data: {
			plan_type: 'spread_rumors',
			target_asset_id: asset("bob's holding"),
			preparation_notes: 'A sketch: something about the Duchess and her wine.',
		},
	});
	await ok(prep, 'prepare-plan');
	const planID: number = (await prep.json()).plan.id;
	await ok(
		await alice.request.post('/api/dev/advance-row', { data: { game_id: gameID, advance_to: 5 } }),
		'advance-row',
	);
	const resolveRes = await alice.request.post(`/api/plans/${planID}/resolve`);
	if (!resolveRes.ok()) expect(await resolveRes.text()).toContain('not in pending status');

	let rollID = 0;
	if (opts.roll) {
		rollID = await castAndForce(alice.request, planID, opts.roll);
	}
	return { gameID, planID, rollID, players, alice, bob, cara, asset };
}

/** Cast the pre-roll over the API and force the dice: a make of 2, or a mar of 1. */
async function castAndForce(ctx: APIRequestContext, planID: number, outcome: 'make' | 'mar'): Promise<number> {
	const cast = await ctx.post(`/api/plans/${planID}/sr-cast-roll`, { data: { rumor: RUMOR, how: HOW } });
	await ok(cast, 'sr-cast-roll');
	const rollID: number = (await cast.json()).roll.id;
	await forceRoll(ctx, rollID, outcome);
	return rollID;
}

async function forceRoll(ctx: APIRequestContext, rollID: number, outcome: 'make' | 'mar') {
	const res = await ctx.post('/api/dev/force-roll', {
		data: { roll_id: rollID, outcome, result: outcome === 'make' ? 2 : 1 },
	});
	await ok(res, 'force-roll');
}

async function view(ctx: BrowserContext, gameID: number): Promise<Page> {
	const page = await ctx.newPage();
	await page.goto(`/table/${gameID}`);
	return page;
}

async function getPlan(ctx: APIRequestContext, planID: number) {
	const res = await ctx.get(`/api/plans/${planID}`);
	await ok(res, 'GET plan');
	return (await res.json()) as {
		plan: { status: string };
		resolution_data?: { choice_submitted?: boolean; spread_rumors?: Record<string, unknown> };
	};
}

async function rowState(ctx: APIRequestContext, gameID: number): Promise<{ kind: string; acting_player_ids?: number[] }> {
	const res = await ctx.get(`/api/tables/${gameID}/state`);
	await ok(res, 'GET state');
	return (await res.json()).row_state;
}

async function actingIDs(ctx: APIRequestContext, gameID: number): Promise<number[]> {
	return (await rowState(ctx, gameID)).acting_player_ids ?? [];
}

interface AssetFull {
	id: number;
	owner_id: number;
	is_leveraged: boolean;
	secret_count: number;
	marginalia: { id: number; is_torn: boolean }[];
}

async function assetsByID(ctx: APIRequestContext, gameID: number): Promise<Map<number, AssetFull>> {
	const res = await ctx.get(`/api/tables/${gameID}/assets`);
	await ok(res, 'GET assets');
	return new Map(((await res.json()).assets as AssetFull[]).map(a => [a.id, a]));
}

async function sheetRumors(ctx: APIRequestContext, gameID: number) {
	const res = await ctx.get(`/api/tables/${gameID}/state`);
	await ok(res, 'GET state');
	return (await res.json()).rumors as { text: string; target_asset_id: number | null; source_player_id: number | null }[];
}

async function assetOwner(ctx: APIRequestContext, gameID: number, assetID: number): Promise<number> {
	const res = await ctx.get(`/api/tables/${gameID}/assets`);
	await ok(res, 'GET assets');
	const a = ((await res.json()).assets as { id: number; owner_id: number }[]).find(x => x.id === assetID);
	return a!.owner_id;
}

/** The "−/+" stepper for the option whose label starts with `option`. */
function more(page: Page, option: string) {
	return page.getByRole('button', { name: `More ${option}` });
}
function fewer(page: Page, option: string) {
	return page.getByRole('button', { name: `Fewer ${option}` });
}

const TAKE = 'Take an asset from';

/** Tick an asset's checkbox on the take-assets picker. */
async function takeCard(page: Page, assetName: string) {
	const card = page.locator('.choices-section .card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
}

/** Pick one take_asset and one leverage on the stepper, then name `assetName` to take. */
async function requestTake(page: Page, assetName: string) {
	await more(page, TAKE).click();
	await more(page, 'Leverage').click();
	await page.getByRole('button', { name: 'Next: choose assets' }).click();
	await takeCard(page, assetName);
	await page.getByRole('button', { name: 'Request consent' }).click();
}

// ── Make ─────────────────────────────────────────────────────────────────────

test('make: pre-roll narration opens the roll; a take needs consent, a repeat request is refused, agreeing transfers', async ({ browser }) => {
	const t = await openTable(browser); // parks at the pre-roll
	const alice = await view(t.alice, t.gameID);
	const bob = await view(t.bob, t.gameID);
	const cara = await view(t.cara, t.gameID);

	// The preparer states the rumor and how it spreads; nobody else can, and no
	// roll exists until she does.
	await expect(alice.getByText('Spread the rumor', { exact: true })).toBeVisible({ timeout: 10_000 });
	await expect(bob.getByText('is about to state the rumor')).toBeVisible();
	const early = await t.alice.request.get(`/api/tables/${t.gameID}/rolls/active`);
	expect((await early.json()).roll ?? null).toBeNull();

	await alice.getByLabel('The rumor', { exact: true }).fill(RUMOR);
	await alice.getByLabel('How you are spreading it').fill(HOW);
	await alice.getByRole('button', { name: 'Post & open the roll' }).click();
	await expect.poll(async () => {
		const r = await t.alice.request.get(`/api/tables/${t.gameID}/rolls/active`);
		return (await r.json()).roll?.id ?? null;
	}).not.toBeNull();
	const rollRes = await t.alice.request.get(`/api/tables/${t.gameID}/rolls/active`);
	await forceRoll(t.alice.request, (await rollRes.json()).roll.id, 'make');

	// The stated rumor is public to the whole table once the roll is settled.
	await expect(bob.locator('.rumor-quote', { hasText: RUMOR })).toBeVisible({ timeout: 10_000 });
	await expect(cara.locator('.rumor-target', { hasText: HOW })).toBeVisible();

	// Alice drives the picker; bob and cara see it inert and are told who is choosing.
	await expect(more(alice, TAKE)).toBeEnabled({ timeout: 10_000 });
	await expect(bob.getByText('alice is choosing options…')).toBeVisible();
	await expect(more(bob, TAKE)).toHaveCount(1);
	await expect(more(bob, TAKE)).toBeDisabled();

	await requestTake(alice, "bob's artifact");

	// The victim is asked, by name, for exactly that asset; the others just wait.
	await expect(bob.getByText('Consent needed')).toBeVisible({ timeout: 10_000 });
	await expect(bob.getByText('alice wants to take')).toBeVisible();
	await expect(cara.getByText('Waiting for bob to agree to give up')).toBeVisible();
	await expect(alice.getByText('Waiting for bob to agree to give up')).toBeVisible();
	expect(await actingIDs(t.alice.request, t.gameID)).toEqual([t.players.bob]);

	// A second request while one is open is refused, not silently re-opened.
	const again = await t.alice.request.post(`/api/plans/${t.planID}/request-take-consent`, {
		data: { result: 'make', choices: ['take_asset', 'leverage_target'], take_asset_ids: [t.asset("bob's resource")] },
	});
	expect(again.status()).toBe(409);
	// Only the victim may answer.
	const wrong = await t.alice.request.post(`/api/plans/${t.planID}/respond-take-consent`, { data: { agree: true } });
	expect(wrong.status()).toBe(403);
	// Nothing moved yet.
	expect(await assetOwner(t.alice.request, t.gameID, t.asset("bob's artifact"))).toBe(t.players.bob);

	await bob.getByRole('button', { name: 'Agree', exact: true }).click();

	await expect.poll(async () => assetOwner(t.alice.request, t.gameID, t.asset("bob's artifact"))).toBe(t.players.alice);
	await expect.poll(async () => (await getPlan(t.alice.request, t.planID)).plan.status).toBe('resolved');

});

test('make: a refused take locks the option off and the rest of the quota goes elsewhere', async ({ browser }) => {
	const t = await openTable(browser, { roll: 'make' });
	const alice = await view(t.alice, t.gameID);
	const bob = await view(t.bob, t.gameID);

	await expect(more(alice, TAKE)).toBeEnabled({ timeout: 10_000 });
	await requestTake(alice, "bob's artifact");
	await expect(bob.getByText('Consent needed')).toBeVisible({ timeout: 10_000 });
	await bob.getByRole('button', { name: 'Disagree' }).click();

	// Back at the picker with the take greyed out and marked declined; the two
	// picks the request carried are not committed.
	await expect(alice.getByText('The owner declined the asset transfer')).toBeVisible({ timeout: 10_000 });
	await expect(alice.getByText('(declined)')).toBeVisible();
	await expect(more(alice, TAKE)).toBeDisabled();
	expect(await assetOwner(t.alice.request, t.gameID, t.asset("bob's artifact"))).toBe(t.players.bob);
	expect((await getPlan(t.alice.request, t.planID)).plan.status).toBe('resolving');

	// A fresh request for the declined option is refused at the server too.
	const retry = await t.alice.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'make', choices: ['take_asset', 'leverage_target'] },
	});
	expect(retry.status()).toBe(400);

	// Alice fills the quota with something else and the plan resolves.
	// (The request's leverage pick is still counted; the take's is dropped.)
	await expect(alice.getByText('Total picks: 1 / 2')).toBeVisible();
	await more(alice, 'Leverage').click();
	await alice.getByRole('button', { name: 'Apply choices' }).click();
	await expect.poll(async () => (await getPlan(t.alice.request, t.planID)).plan.status).toBe('resolved');

});

// ── Stepper (375px) ──────────────────────────────────────────────────────────

test('stepper at 375px: named buttons, 44px targets, capped at the quota, no sideways scroll', async ({ browser }) => {
	const t = await openTable(browser, { roll: 'make' });
	const alice = await view(t.alice, t.gameID);
	expect(alice.viewportSize()).toEqual(MOBILE);

	await expect(more(alice, TAKE)).toBeEnabled({ timeout: 10_000 });

	// Every option is its own labelled pair, so a screen reader hears which
	// option a "+" belongs to rather than five identical buttons.
	const rows = alice.locator('.stepper-row');
	await expect(rows).toHaveCount(5);
	for (const option of [
		'Break the target asset',
		'Leverage the target asset',
		TAKE,
		'Hide yourself as the source',
		'Reveal yourself as the source',
	]) {
		await expect(more(alice, option)).toHaveCount(1);
		await expect(fewer(alice, option)).toHaveCount(1);
	}

	// Tap targets: all ten buttons are at least 44px each way.
	const buttons = rows.locator('button');
	await expect(buttons).toHaveCount(10);
	for (let i = 0; i < 10; i++) {
		const box = await buttons.nth(i).boundingBox();
		expect(box, `stepper button ${i}`).not.toBeNull();
		expect(box!.width, `stepper button ${i} width`).toBeGreaterThanOrEqual(44);
		expect(box!.height, `stepper button ${i} height`).toBeGreaterThanOrEqual(44);
	}
	// No row pokes out of the viewport and the page does not scroll sideways.
	for (let i = 0; i < 5; i++) {
		const box = await rows.nth(i).boundingBox();
		expect(box!.x).toBeGreaterThanOrEqual(0);
		expect(box!.x + box!.width).toBeLessThanOrEqual(MOBILE.width);
	}
	const overflow = await alice.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
	expect(overflow).toBeLessThanOrEqual(0);

	// The count steps, and the quota (the dice result, 2) caps every "+".
	await expect(fewer(alice, 'Leverage')).toBeDisabled();
	await more(alice, 'Leverage').click();
	await more(alice, 'Reveal yourself').click();
	await expect(alice.getByText('Total picks: 2 / 2')).toBeVisible();
	for (const option of ['Break the target asset', 'Leverage', TAKE, 'Hide yourself', 'Reveal yourself']) {
		await expect(more(alice, option)).toBeDisabled();
	}
	await fewer(alice, 'Reveal yourself').click();
	await expect(alice.getByText('Total picks: 1 / 2')).toBeVisible();
	await expect(more(alice, 'Reveal yourself')).toBeEnabled();

});

// ── Mar ──────────────────────────────────────────────────────────────────────

test('mar: the target-asset owner is the actor, writes the counter-rumor, and the consent screen names the preparer\'s main character', async ({ browser }) => {
	const t = await openTable(browser, { roll: 'mar', difficulty: 3 });
	const alice = await view(t.alice, t.gameID);
	const bob = await view(t.bob, t.gameID);
	const cara = await view(t.cara, t.gameID);

	// The Waiting On set names bob — the target asset's owner — and not alice,
	// the preparer, who used to be named for the whole mar branch.
	await expect(bob.getByText('The rumor backfires')).toBeVisible({ timeout: 10_000 });
	expect(await actingIDs(t.alice.request, t.gameID)).toEqual([t.players.bob]);
	await expect(alice.getByText('bob is writing the rumor that spreads instead…')).toBeVisible();
	await expect(cara.getByText('bob is writing the rumor that spreads instead…')).toBeVisible();
	// The counter-rumor is about alice's main character, and says so.
	await expect(bob.getByText("alice's main character", { exact: true }).first()).toBeVisible();

	// The preparer can't write her own counter-rumor, nor pick the options.
	const forged = await t.alice.request.post(`/api/plans/${t.planID}/sr-counter-rumor`, { data: { text: 'Nothing happened.' } });
	expect(forged.status()).toBe(403);

	// Options are gated behind the counter-rumor.
	const early = await t.bob.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: ['leverage_target', 'reveal_source'] },
	});
	expect(early.ok()).toBeFalsy();

	await bob.getByLabel('The rumor that spreads instead').fill(COUNTER);
	await bob.getByRole('button', { name: 'Spread it' }).click();

	// The table reads the counter-rumor; the failed rumor is not what spreads.
	await expect(alice.locator('.rumor-quote', { hasText: COUNTER })).toBeVisible({ timeout: 10_000 });
	await expect(alice.getByText('The rumor that backfired')).toBeVisible();
	await expect(cara.getByText('bob is choosing options…')).toBeVisible();

	// Bob picks a take and a leverage. The take is from the PREPARER's assets.
	await more(bob, 'Take an asset from alice').click();
	await more(bob, 'Leverage alice').click();
	await bob.getByRole('button', { name: 'Next: choose assets' }).click();
	await takeCard(bob, "alice's artifact");
	await bob.getByRole('button', { name: 'Request consent' }).click();

	// Alice is the victim. The consent screen's "also hits" line names her main
	// character — the counter-rumor's subject — never bob's own target asset.
	await expect(alice.getByText('Consent needed')).toBeVisible({ timeout: 10_000 });
	await expect(alice.getByText('bob wants to take')).toBeVisible();
	const alsoHits = alice.locator('.choices-note', { hasText: 'also hits' });
	await expect(alsoHits).toContainText('their counter-rumor');
	await expect(alsoHits.locator('em')).toHaveText("alice's main character");
	await expect(alice.locator('.effect-fyi')).toContainText('Leverages it');
	await expect(alice.locator('.effect-fyi')).not.toContainText("bob's holding");
	await expect(alice.locator('.consent-assets')).toContainText("alice's artifact");
	expect(await actingIDs(t.alice.request, t.gameID)).toEqual([t.players.alice]);

	// Bystanders only see who is being waited on.
	await expect(cara.getByText('Consent needed')).toHaveCount(0);
	await expect(cara.getByText('Waiting for alice to agree to give up')).toBeVisible();

	await alice.getByRole('button', { name: 'Agree', exact: true }).click();
	await expect.poll(async () => (await getPlan(t.alice.request, t.planID)).plan.status).toBe('resolved');
	expect(await assetOwner(t.alice.request, t.gameID, t.asset("alice's artifact"))).toBe(t.players.bob);

	// The Rumors sheet carries the counter-rumor — about alice's main character,
	// sourced to its author bob — and the preparer's failed rumor never reaches it.
	const rumors = await sheetRumors(t.alice.request, t.gameID);
	expect(rumors.map(r => r.text)).toContain(COUNTER);
	expect(rumors.map(r => r.text)).not.toContain(RUMOR);
	expect(rumors.every(r => r.text !== '(no rumor text)')).toBeTruthy();
	const row = rumors.find(r => r.text === COUNTER)!;
	expect(row.target_asset_id).toBe(t.asset("alice's main character"));
	expect(row.source_player_id).toBe(t.players.bob);

	// The leverage landed on the preparer's main character, not on bob's target.
	const after = await assetsByID(t.alice.request, t.gameID);
	expect(after.get(t.asset("alice's main character"))!.is_leveraged).toBe(true);
	expect(after.get(t.asset("bob's holding"))!.is_leveraged).toBe(false);

});

test('mar: a zero-pick commit closes the counter-rumor and hands the wait on', async ({ browser }) => {
	const t = await openTable(browser, { roll: 'mar', difficulty: 3 });
	const alice = await view(t.alice, t.gameID);

	await ok(
		await t.bob.request.post(`/api/plans/${t.planID}/sr-counter-rumor`, { data: { text: COUNTER } }),
		'counter-rumor',
	);
	// Still bob's: the options are owed.
	await expect.poll(() => actingIDs(t.alice.request, t.gameID)).toEqual([t.players.bob]);

	// The server takes a commit of fewer picks than the quota (it only caps the
	// top), and bob commits none. Nothing is then owed.
	await ok(
		await t.bob.request.post(`/api/plans/${t.planID}/make-choice`, { data: { result: 'mar', choices: [] } }),
		'zero-pick make-choice',
	);
	const after = await getPlan(t.alice.request, t.planID);
	expect(after.resolution_data?.choice_submitted).toBe(true);
	expect(after.plan.status).toBe('resolved');
	// The mar's wait on bob is gone and the table is on to the follow scene. (Bob
	// still appears in the acting set, but only as the row's focus player.)
	expect((await rowState(t.alice.request, t.gameID)).kind).toBe('scene_setting');
	// A "closed" counter-rumor stays closed.
	const late = await t.bob.request.post(`/api/plans/${t.planID}/sr-counter-rumor`, { data: { text: 'Too late.' } });
	expect(late.ok()).toBeFalsy();

	// The panel no longer asks anyone for options.
	await expect(alice.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	await expect(alice.getByText('bob is choosing options…')).toHaveCount(0);

});

test('mar: break and hide land on the preparer\'s main character and the actor\'s asset, and the rumor is sourced to no one', async ({ browser }) => {
	const t = await openTable(browser, { roll: 'mar', difficulty: 3 });
	const bob = await view(t.bob, t.gameID);
	const alice = await view(t.alice, t.gameID);
	const mc = t.asset("alice's main character");

	await ok(
		await t.bob.request.post(`/api/plans/${t.planID}/sr-counter-rumor`, { data: { text: COUNTER } }),
		'counter-rumor',
	);
	await more(bob, 'Break alice').click();
	await more(bob, 'Hide yourself').click();
	await bob.getByRole('button', { name: 'Apply choices' }).click();

	// Break: the actor picks WHICH marginalia, on the preparer's main character —
	// the only card offered.
	const breakForm = bob.locator('.plan-form', { hasText: "Break alice's main character" });
	await expect(breakForm).toBeVisible({ timeout: 10_000 });
	await expect(breakForm.locator('.card')).toHaveCount(1);
	await expect(alice.getByText("Break alice's main character (1 remaining)")).toBeVisible();
	await breakForm.locator('.card', { hasText: "alice's main character" }).getByRole('checkbox').click();
	await breakForm.getByRole('button', { name: 'Tear marginalia' }).click();

	// Hide: the secret goes on one of the ACTOR's own assets.
	const hideForm = bob.locator('.plan-form', { hasText: 'Hide source' });
	await expect(hideForm).toBeVisible({ timeout: 10_000 });
	await hideForm.locator('.card', { hasText: "bob's resource" }).getByRole('checkbox').click();
	await hideForm.getByRole('button', { name: 'Hide source' }).click();

	await expect.poll(async () => (await getPlan(t.alice.request, t.planID)).plan.status).toBe('resolved');
	const assets = await assetsByID(t.alice.request, t.gameID);
	expect(assets.get(mc)!.marginalia.some(m => m.is_torn), 'a marginalia of alice\'s main character is torn').toBe(true);
	expect(assets.get(t.asset("bob's holding"))!.marginalia.every(m => !m.is_torn)).toBe(true);
	expect(assets.get(t.asset("bob's resource"))!.secret_count).toBe(1);
	expect(assets.get(t.asset("alice's artifact"))!.secret_count).toBe(0);

	// The sheet's row is about alice's main character, and its source is hidden.
	const row = (await sheetRumors(t.alice.request, t.gameID)).find(r => r.text === COUNTER)!;
	expect(row.target_asset_id).toBe(mc);
	expect(row.source_player_id).toBeNull();
});
