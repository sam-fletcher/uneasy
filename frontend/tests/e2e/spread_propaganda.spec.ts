import {
	test, expect, request as pwRequest,
	type APIRequestContext, type BrowserContext, type Page,
} from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';

// Regression specs for what the 12-plan UX audit fixed in Spread Propaganda.
//
//   - pre-roll narration (Tier 3 #10): the plan parks with no roll until the
//     preparer states the message; the table reads it first, and the Buffet
//     ("What can happen?") is previewable to everyone before the dice
//   - the mar pick flow on the shared Stepper (Pass 4): every option is capped at
//     its live target count, so the picker can no longer commit a set of picks
//     nobody can satisfy (audit #3: the wedged row), and the exact total is enforced
//   - the make has no "Apply choices" gate (Tier 4): the roll applies it, the
//     preparer goes straight to the artifact + narration (Tier 3 #13)
//   - a mar committed with no picks hands the wait on (choice-committed marker)
//   - hidden information: a secret on an asset the preparer breaks never reaches
//     a bystander, through any response or the action log
//
// Setup (seed, resolve, force the dice) goes over the API; the subject is what
// each player *sees and can do* in the rendered panels. Players get separate
// browser contexts because contexts, not pages, are what keep cookies apart.
//
// Not pinned: who learns the broken asset's secrets when a Make Demands
// "perform steps" winner stands in for break-yourself — that waits on an owner
// question (docs/work/questions/spread-propaganda-demand-secrets.md).

interface PlanRow {
	id: number;
	plan_type: string;
	status: string;
	resolution_data?: unknown;
}
interface RollRow {
	id: number;
	result: number | null;
	outcome: 'make' | 'mar' | null;
	difficulty: number;
}
interface AssetRow {
	id: number;
	owner_id: number;
	name: string;
	asset_type: string;
	is_destroyed: boolean;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}

const track = cleanupGameAfterEach();

// Contexts are closed in an afterEach, so a failing assertion can't leak them.
const openContexts: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of openContexts.splice(0)) await c.close().catch(() => {});
});

interface Table {
	gameID: number;
	planID: number;
	alice: { ctx: BrowserContext; page: Page };
	bob: { ctx: BrowserContext; page: Page };
}

/** Seed a 3-player main event with alice's Spread Propaganda on the current row.
 *  By default alice is ranked last on the esteem track, so her difficulty is high
 *  enough to force a mar owing two picks; ranked first, a make is reachable (the
 *  actor rolls only two dice, so a make needs a difficulty of 2 or less). */
async function setUp(
	browser: import('@playwright/test').Browser,
	// Esteem order, best first, as player indexes. Alice (0) last by default.
	esteem = [1, 2, 0],
): Promise<Table> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: ['alice', 'bob', 'carol'],
			current_row: 5,
			// value[k] = player index at rank k+1.
			rankings: { esteem },
			plans: [{ preparer_idx: 0, plan_type: 'spread_propaganda', category: 'esteem', row: 5, row_order: 0 }],
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id } = await seedRes.json();
	track(game_id);
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 812 } });
	const bobCtx = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 812 } });
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');
	openContexts.push(aliceCtx, bobCtx);

	const plansRes = await aliceCtx.request.get(`/api/tables/${game_id}/plans`);
	const { plans } = (await plansRes.json()) as { plans: PlanRow[] };
	const planID = plans.find(p => p.plan_type === 'spread_propaganda')!.id;

	const resolveRes = await aliceCtx.request.post(`/api/plans/${planID}/resolve`);
	expect(resolveRes.ok(), `resolve failed: ${await resolveRes.text()}`).toBeTruthy();

	return {
		gameID: game_id,
		planID,
		alice: { ctx: aliceCtx, page: await aliceCtx.newPage() },
		bob: { ctx: bobCtx, page: await bobCtx.newPage() },
	};
}

async function openTables(t: Table) {
	await Promise.all([
		t.alice.page.goto(`/table/${t.gameID}`),
		t.bob.page.goto(`/table/${t.gameID}`),
	]);
}

async function activeRoll(api: APIRequestContext, gameID: number): Promise<RollRow> {
	const res = await api.get(`/api/tables/${gameID}/rolls/active`);
	expect(res.ok(), `active roll: ${await res.text()}`).toBeTruthy();
	return (await res.json()).roll as RollRow;
}

/** Settle the open roll through the real stage machine (POST /api/dev/force-roll). */
async function forceRoll(api: APIRequestContext, rollID: number, body: { outcome: 'make' | 'mar'; result?: number }) {
	const res = await api.post('/api/dev/force-roll', { data: { roll_id: rollID, ...body } });
	expect(res.ok(), `force-roll failed: ${await res.text()}`).toBeTruthy();
}

async function planStatus(api: APIRequestContext, planID: number): Promise<string> {
	const res = await api.get(`/api/plans/${planID}`);
	expect(res.ok(), `GET plan: ${await res.text()}`).toBeTruthy();
	return ((await res.json()) as { plan: PlanRow }).plan.status;
}

/** Alice casts from the pre-roll form in her browser; returns once the roll exists. */
async function castFromUI(t: Table, narration: string) {
	const form = t.alice.page.locator('.pre-roll');
	await expect(form).toBeVisible({ timeout: 10_000 });
	await form.locator('textarea').fill(narration);
	await form.getByRole('button', { name: 'Post & open the roll' }).click();
	await expect(form).toHaveCount(0);
	return activeRoll(t.alice.ctx.request, t.gameID);
}

// Every text a player could read about this game, as one blob, so a secret's
// text can be searched for across all of them.
async function everythingVisibleTo(api: APIRequestContext, t: Table, assetIDs: number[]): Promise<string> {
	const urls = [
		`/api/tables/${t.gameID}/assets`,
		`/api/tables/${t.gameID}/plans`,
		`/api/plans/${t.planID}`,
		`/api/tables/${t.gameID}/secrets/visible`,
		`/api/tables/${t.gameID}/posts`,
		...assetIDs.map(id => `/api/assets/${id}/secrets`),
	];
	const parts: string[] = [];
	for (const u of urls) {
		const res = await api.get(u);
		parts.push(`${u} ${res.status()} ${await res.text()}`);
	}
	return parts.join('\n');
}

test('spread propaganda mar: pre-roll, Buffet, capped stepper picks, break-yourself, and the bystander sees no secret', async ({ browser }) => {
	const t = await setUp(browser);
	const { alice, bob } = t;

	const assetsRes = await alice.ctx.request.get(`/api/tables/${t.gameID}/assets`);
	const { assets } = (await assetsRes.json()) as { assets: AssetRow[] };
	const holding = assets.find(a => a.asset_type === 'holding' && a.name.startsWith('alice'))!;
	const SECRET = 'The vault holds forged deeds, not gold.';

	// Everything bob's open page receives over the WebSocket, so a secret leaked by a
	// broadcast (secret.created has leaked before) is caught as well as one in a response.
	const bobFrames: string[] = [];
	bob.page.on('websocket', ws => ws.on('framereceived', f => bobFrames.push(String(f.payload))));
	await openTables(t);

	// A secret on the asset alice will break, written once both tables are open so the
	// live event has a watcher. Existence is public, the text is not.
	const secretRes = await alice.ctx.request.post(`/api/assets/${holding.id}/secrets`, { data: { text: SECRET } });
	expect(secretRes.ok(), `write secret: ${await secretRes.text()}`).toBeTruthy();

	// ── Pre-roll: no roll exists; the preparer is asked, the bystander waits ──
	await expect(alice.page.getByText('Spread your message')).toBeVisible({ timeout: 10_000 });
	await expect(bob.page.getByText('alice is about to say how they\'re spreading their message')).toBeVisible();
	await expect(bob.page.locator('.pre-roll')).toHaveCount(0);

	// The server refuses a choice before the dice are cast.
	const early = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: ['lay_low'] },
	});
	expect(early.status(), 'a choice before the pre-roll is refused').toBe(409);

	// ── The pre-roll Buffet: previewable by the bystander, before any dice ──
	const buffetToggle = bob.page.getByRole('button', { name: 'What can happen?' });
	await expect(buffetToggle).toBeVisible();
	await buffetToggle.click();
	await expect(bob.page.getByText('Create an artifact representing the societal shift')).toBeVisible();
	await bob.page.getByRole('tab', { name: 'Mar' }).click();
	await expect(bob.page.getByText('Pick options equal to (difficulty − result), repeatable:')).toBeVisible();
	await expect(bob.page.getByText('The preparer keeps their head down').first()).toBeVisible();

	// ── Alice states the message; it reaches bob before/with the roll ──
	const MESSAGE = 'The crown hoards grain while the markets starve.';
	const roll = await castFromUI(t, MESSAGE);
	// A dummy can hold a rank on a 3-player track, so don't hard-code alice's difficulty:
	// force the result two below it and the mar owes exactly two picks.
	expect(roll.difficulty).toBeGreaterThanOrEqual(3);
	await expect(bob.page.getByText('The message, as stated:')).toBeVisible({ timeout: 10_000 });
	await expect(bob.page.locator('.plan-notes', { hasText: MESSAGE })).toBeVisible();

	// ── The dice land as a mar owing difficulty − result = 2 picks ──
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: roll.difficulty - 2 });
	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });
	// The bystander gets a wait line and no picker.
	await expect(bob.page.getByText('alice is resolving Spread Propaganda…')).toBeVisible();
	await expect(bob.page.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	await expect(bob.page.locator('.stepper-row')).toHaveCount(0);

	// ── The shared Stepper: capped per option, exact total, 44px targets ──
	const more = (name: string) => alice.page.getByRole('button', { name: `More ${name}` });
	const fewer = (name: string) => alice.page.getByRole('button', { name: `Fewer ${name}` });
	const LAY_LOW = '(b) Keep your head down';
	const BREAK_SELF = '(c) Word of your laughable ideas';
	const COUNTER = '(d) Top interferer';
	const apply = alice.page.getByRole('button', { name: 'Apply choices' });
	const total = alice.page.locator('.choices-note', { hasText: 'Total picks:' });

	await expect(apply).toBeDisabled();
	await expect(total).toContainText('0 / 2');
	// Nobody interfered with this roll, so counter-propaganda is greyed with the reason.
	await expect(more(COUNTER)).toBeDisabled();
	await expect(alice.page.getByText('nobody interfered with this roll')).toBeVisible();
	// Tap targets are the 44px the Stepper promises.
	const box = await more(BREAK_SELF).boundingBox();
	expect(box!.width).toBeGreaterThanOrEqual(44);
	expect(box!.height).toBeGreaterThanOrEqual(44);

	// "Keep your head down" is capped at one pick — a deliberate deviation from the
	// book's repeatable options (owner ruling 2026-11-03; the server enforces it too,
	// pinned in the handler integration tests). The picker shows why.
	await more(LAY_LOW).click();
	await expect(more(LAY_LOW)).toBeDisabled();
	await expect(alice.page.getByText('already chosen')).toBeVisible();
	await expect(total).toContainText('1 / 2');
	await expect(apply).toBeDisabled();

	// A second pick fills the budget; every + then shuts, whatever its own cap.
	await more(BREAK_SELF).click();
	await expect(total).toContainText('2 / 2');
	await expect(more(BREAK_SELF)).toBeDisabled();
	await expect(apply).toBeEnabled();
	// Stepping back down reopens it.
	await fewer(BREAK_SELF).click();
	await expect(total).toContainText('1 / 2');
	await expect(apply).toBeDisabled();
	await more(BREAK_SELF).click();

	await apply.click();

	// ── Committed: bob is told what alice faces; the panel is server-authoritative ──
	await expect(bob.page.getByText('Waiting for alice to face the fallout…')).toBeVisible({ timeout: 10_000 });
	await expect(alice.page.getByText('Break yourself')).toBeVisible();

	// ── Break yourself, from the picker: the holding that carries the secret ──
	const card = alice.page.locator('.peer-cards', { hasText: holding.name }).locator('..');
	await card.getByText(holding.name).first().click();
	// The row's checkbox is what takes the tap (the text sits under it).
	await alice.page.locator('li', { hasText: holding.marginalia[0].text }).getByRole('checkbox').click();
	await alice.page.getByRole('button', { name: 'Break asset' }).click();

	// Nothing else is owed: the plan resolves itself, with no "Complete plan" to click.
	await expect.poll(() => planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	await expect(alice.page.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);

	// ── Hidden information ──
	// The preparer holds her secret; the bystander has no route to its text.
	const aliceSees = await everythingVisibleTo(alice.ctx.request, t, [holding.id]);
	expect(aliceSees, 'the author still reads her own secret').toContain(SECRET);
	const bobSees = await everythingVisibleTo(bob.ctx.request, t, [holding.id]);
	expect(bobFrames.length, 'bob\'s socket was listening').toBeGreaterThan(0);
	expect(bobFrames.join('\n').includes(SECRET), 'secret text broadcast to a bystander').toBe(false);
	// (a boolean, so a failure doesn't dump every response into the report)
	expect(bobSees.includes(SECRET), 'secret text reached a bystander').toBe(false);
	// Its existence, by contrast, is public.
	expect(bobSees, 'the secret\'s existence is public').toContain('"secret_count":1');
	// The break itself is public: the table is told an asset was torn, never what
	// was written under it.
	expect(bobSees, 'the break is logged for the table').toContain('broke their own');
	await bob.page.reload();
	await bob.page.waitForLoadState('networkidle');
	expect((await bob.page.content()).includes(SECRET), 'secret text rendered in a bystander\'s page').toBe(false);

});

test('spread propaganda make: no Apply gate, the artifact + changes are one step, the others wait on it', async ({ browser }) => {
	const t = await setUp(browser, [0, 1, 2]);
	const { alice, bob } = t;
	await openTables(t);

	const roll = await castFromUI(t, 'Every sermon this month preaches the same line.');
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'make' });

	// The make's single option applies itself when the dice land, server-side...
	await expect.poll(async () => {
		const res = await alice.ctx.request.get(`/api/plans/${t.planID}`);
		const body = (await res.json()) as { resolution_data?: { choice_auto_applied?: boolean } };
		return body.resolution_data?.choice_auto_applied ?? false;
	}, { timeout: 10_000 }).toBe(true);
	// The save is announced, so the open panels move on without a reload.

	// Alice is handed the artifact step directly, with no "Apply choices" to click first.
	await expect(alice.page.getByText('Your message sets society ablaze')).toBeVisible({ timeout: 10_000 });
	await expect(alice.page.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
	await expect(bob.page.getByText('Waiting for alice to author the artifact…')).toBeVisible();
	await expect(bob.page.getByText('Your message sets society ablaze')).toHaveCount(0);

	// The narration of the changes, the name and the first note are all required.
	const create = alice.page.getByRole('button', { name: 'Create artifact' });
	await expect(create).toBeDisabled();
	const CHANGES = 'Guilds close their doors to the crown\'s tax collectors.';
	await alice.page.getByPlaceholder('How is the realm different now?').fill(CHANGES);
	await expect(create).toBeDisabled();
	await alice.page.getByPlaceholder('Name your new asset…').fill('The Closed Doors');
	await expect(create).toBeDisabled();
	await alice.page.getByPlaceholder('A description, a function, a fun fact', { exact: false }).fill('Chalked on every guildhall');
	await expect(create).toBeEnabled();
	await create.click();

	// One step: the plan resolves on the artifact, nobody is asked to "Complete".
	await expect.poll(() => planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	await expect(alice.page.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);

	// The artifact exists, owned by alice and visible to the bystander; the changes
	// are on the record in alice's words.
	const bobAssets = (await (await bob.ctx.request.get(`/api/tables/${t.gameID}/assets`)).json()).assets as AssetRow[];
	const artifact = bobAssets.find(a => a.name === 'The Closed Doors');
	expect(artifact, 'the authored artifact is on the board').toBeTruthy();
	expect(artifact!.asset_type).toBe('artifact');
	const posts = await (await bob.ctx.request.get(`/api/tables/${t.gameID}/posts`)).text();
	expect(posts).toContain(`The changes society undergoes: ${CHANGES}`);

});

test('spread propaganda mar: a zero-pick commit auto-resolves the plan, leaving nobody waiting', async ({ browser }) => {
	const t = await setUp(browser);
	const { alice, bob } = t;
	await openTables(t);

	const roll = await castFromUI(t, 'Whispers in every tavern.');
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: 1 });
	await expect(alice.page.getByText('choose exactly')).toBeVisible({ timeout: 10_000 });

	// A hand-built commit with nothing picked (the picker never offers one). Nothing is
	// owed, so the plan resolves itself and the picker never returns. The committed marker is
	// asserted below; the panel's reading of it is pinned by the render test, since
	// auto-resolve hides the difference here.
	const res = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: [] },
	});
	expect(res.ok(), `zero-pick commit: ${await res.text()}`).toBeTruthy();

	await expect.poll(() => planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	// The panel reads done-state from the server's marker, not from the picked options
	// (there are none), so the picker never comes back.
	const rd = await (await alice.ctx.request.get(`/api/plans/${t.planID}`)).json();
	expect(rd.resolution_data?.choice_submitted).toBe(true);
	expect(rd.resolution_data?.make_mar_choices ?? []).toHaveLength(0);
	for (const p of [alice.page, bob.page]) {
		await expect(p.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
		await expect(p.getByText('Dice roll in progress…')).toHaveCount(0);
		await expect(p.getByText('alice is resolving Spread Propaganda…')).toHaveCount(0);
		await expect(p.getByText('Waiting for alice', { exact: false })).toHaveCount(0);
	}

});

// The audit's headline Spread Propaganda bug (finding 3): "a peer leaves your retinue"
// could be picked more times than the preparer had peers, after which /complete refused
// forever and nobody could finish the row. Two layers now stop it: the picker caps each
// option at its live target count, and sp-forfeit-step discharges a pick that has
// lost its last target. Alice has exactly one peer (her main character).
async function castOverAPI(t: Table, narration: string): Promise<RollRow> {
	const res = await t.alice.ctx.request.post(`/api/plans/${t.planID}/sp-cast-roll`, { data: { narration } });
	expect(res.ok(), `sp-cast-roll failed: ${await res.text()}`).toBeTruthy();
	return activeRoll(t.alice.ctx.request, t.gameID);
}

test('spread propaganda mar: "a peer leaves" cannot be picked past the peers there are', async ({ browser }) => {
	const t = await setUp(browser);
	const { alice } = t;
	const roll = await castOverAPI(t, 'Seditious verses, set to a tavern tune.');
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: roll.difficulty - 2 });
	await openTables(t);
	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });

	const GIVE_PEER = '(a) A peer leaves your retinue';
	const more = alice.page.getByRole('button', { name: `More ${GIVE_PEER}` });
	const total = alice.page.locator('.choices-note', { hasText: 'Total picks:' });
	await more.click();
	await expect(total).toContainText('1 / 2');
	// One peer, one pick: the option is shut with the reason, though the budget has room.
	await expect(more).toBeDisabled();
	await expect(alice.page.getByText('all targets picked')).toBeVisible();
	await expect(alice.page.getByRole('button', { name: 'Apply choices' })).toBeDisabled();
});

test('spread propaganda mar: a pick that lost its last peer is forfeited, and the row finishes', async ({ browser }) => {
	const t = await setUp(browser);
	const { alice, bob } = t;
	const roll = await castOverAPI(t, 'A ballad that names no names, but everyone knows.');
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: roll.difficulty - 2 });

	// A stale or hand-built client over-picks: two hand-overs against one peer. (The
	// picker above no longer offers this; the server still accepts it, which is what
	// the forfeit hatch exists for.)
	const commit = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: ['give_peer', 'give_peer'] },
	});
	expect(commit.ok(), `over-pick commit: ${await commit.text()}`).toBeTruthy();
	await openTables(t);

	// Hand over the one peer.
	await expect(alice.page.getByText('A peer leaves your retinue — hand it to another player'))
		.toBeVisible({ timeout: 10_000 });
	await expect(bob.page.getByText('Waiting for alice to face the fallout…')).toBeVisible();
	const step = alice.page.locator('.plan-form', { hasText: 'A peer leaves your retinue' });
	await step.getByRole('checkbox').first().click();
	await step.getByRole('button', { name: 'bob' }).click();
	await step.getByRole('button', { name: 'Give up peer' }).click();

	// One pick is still owed and there is nothing left to give. The plan used to wedge
	// here; now the step says so and offers the way out, and the plan won't resolve
	// until it is taken.
	await expect(alice.page.getByText('Nothing left to give — this pick has no valid target.'))
		.toBeVisible({ timeout: 10_000 });
	expect(await planStatus(alice.ctx.request, t.planID)).toBe('resolving');
	await alice.page.getByRole('button', { name: 'Skip — no valid targets' }).click();

	await expect.poll(() => planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	await expect(alice.page.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);
});

// When the dice land and a ForcedChoicer plan applies its make by itself, the
// save is announced, so open panels move on with no reload.
test('spread propaganda make: the artifact step appears live, with no reload', async ({ browser }) => {
	const t = await setUp(browser, [0, 1, 2]);
	const { alice, bob } = t;
	await openTables(t);
	const roll = await castFromUI(t, 'Pamphlets, pamphlets everywhere.');
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'make' });

	await expect(alice.page.getByText('Your message sets society ablaze')).toBeVisible({ timeout: 10_000 });
	await expect(bob.page.getByText('Waiting for alice to author the artifact…')).toBeVisible({ timeout: 10_000 });

});

// ── Counter-propaganda (mar option d): the interferer states a message or declines ──

/** Drive a marred plan to alice's pick, with bob as the one interferer. Returns the
 *  counter-propaganda plan's id once alice's picks are committed. */
async function marWithBobInterfering(t: Table, picks: string[]): Promise<number> {
	const { alice, bob } = t;
	const roll = await castFromUI(t, 'Grain is hoarded while the markets starve.');
	// The actor rolls two dice, so a result of 1 is always reachable and leaves the
	// mar a budget of difficulty − 1 (a dummy can hold a rank, so don't hard-code it).
	expect(roll.difficulty).toBeGreaterThanOrEqual(picks.length + 1);

	const post = async (who: { ctx: BrowserContext }, path: string, data?: object) => {
		const res = await who.ctx.request.post(path, data ? { data } : undefined);
		expect(res.ok(), `${path}: ${await res.text()}`).toBeTruthy();
	};
	await post(alice, `/api/rolls/${roll.id}/skip-vote`);
	await post(bob, `/api/rolls/${roll.id}/intent`, { intent: 'interfere' });
	const { assets } = (await (await bob.ctx.request.get(`/api/tables/${t.gameID}/assets`)).json()) as { assets: AssetRow[] };
	const bobHolding = assets.find(a => a.asset_type === 'holding' && a.name.startsWith('bob'))!;
	await post(bob, `/api/rolls/${roll.id}/leverage`, { asset_id: bobHolding.id });
	await forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: 1 });
	if (picks.includes('give_peer')) {
		await post(alice, `/api/tables/${t.gameID}/assets`, {
			asset_type: 'peer', name: 'A peer to give', is_main_character: false, marginalia: ['loyal'],
		});
	}
	await post(alice, `/api/plans/${t.planID}/make-choice`, { result: 'mar', choices: picks });
	const { plans } = (await (await alice.ctx.request.get(`/api/tables/${t.gameID}/plans`)).json()) as { plans: PlanRow[] };
	return plans.filter(p => p.plan_type === 'spread_propaganda').map(p => p.id).find(id => id !== t.planID)!;
}

test('counter-propaganda: the interferer, with the parent still owing a step, can state a message the table reads first', async ({ browser }) => {
	const t = await setUp(browser);
	await openTables(t);
	const childID = await marWithBobInterfering(t, ['give_peer', 'counter_prop']);
	const { alice, bob } = t;

	// The parent still owes alice's give_peer, so it stays open — and bob, who owes the
	// counter step, is shown ITS form, while alice keeps the parent's panel.
	await expect(bob.page.getByText('Answer with propaganda of your own')).toBeVisible({ timeout: 10_000 });
	await expect(bob.page.getByRole('button', { name: 'Decline: spread nothing' })).toBeVisible();
	await expect(alice.page.getByText('Answer with propaganda of your own')).toHaveCount(0);
	await expect(alice.page.getByText('hand it to another player')).toBeVisible();

	// No roll for the child until bob speaks; once he does, the message is on the record.
	const MESSAGE = 'The crown starves its own people.';
	const form = bob.page.locator('.pre-roll');
	await form.locator('textarea').fill(MESSAGE);
	await form.getByRole('button', { name: 'Post & open the roll' }).click();
	await expect(form).toHaveCount(0);
	await expect.poll(async () => {
		const res = await alice.ctx.request.get(`/api/tables/${t.gameID}/posts`);
		return JSON.stringify(await res.json()).includes(MESSAGE);
	}).toBe(true);
	const roll = await activeRoll(alice.ctx.request, t.gameID);
	expect(roll, 'the child roll opens only after the message').toBeTruthy();
	expect(await planStatus(alice.ctx.request, childID)).toBe('resolving');
});

test('counter-propaganda: the interferer may decline, and it is logged', async ({ browser }) => {
	const t = await setUp(browser);
	await openTables(t);
	const childID = await marWithBobInterfering(t, ['counter_prop']);
	const { alice, bob } = t;

	await expect(bob.page.getByText('Answer with propaganda of your own')).toBeVisible({ timeout: 10_000 });
	await expect(alice.page.getByText('bob is about to say how they\'re spreading their message')).toBeVisible();
	await bob.page.getByRole('button', { name: 'Decline: spread nothing' }).click();

	await expect.poll(() => planStatus(alice.ctx.request, childID), { timeout: 10_000 }).toBe('resolved');
	const posts = JSON.stringify(await (await alice.ctx.request.get(`/api/tables/${t.gameID}/posts`)).json());
	expect(posts).toContain('declined to spread propaganda of their own');
	await expect(bob.page.getByText('Answer with propaganda of your own')).toHaveCount(0);
});
