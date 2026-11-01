import { test, expect, request as pwRequest, type APIRequestContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';

// Regression specs for what the 12-plan UX audit fixed in Make Introductions:
//
//  - before the dice, every player sees the difficulty meter (2 + peers) and the
//    "What can happen?" buffet;
//  - the preparer — and only the preparer — names the peers before the roll,
//    even when a Make Demands perform_steps winner exists (the win is post-roll);
//  - the make's arrival form: name pre-filled, a marginalia required, the last
//    arrival ends the plan;
//  - the mar fates: each peer gets a *different* outcome, used chips are
//    greyed with a reason, and the people who owe marginalia are asked for it;
//  - a perform_steps winner drives the post-roll steps while the preparer sees
//    a wait line.
//
// Setup (seeding, preparing, reaching the row, forcing dice) goes over the
// API; every assertion about what a player sees is made in a real page.

const PLAYERS = ['alice', 'bob', 'carol'] as const;
type Name = (typeof PLAYERS)[number];

interface Fixture {
	gameID: number;
	ctx: Record<Name, APIRequestContext>;
	ids: Record<Name, number>;
	planID: number;
}

const track = cleanupGameAfterEach();

async function ok(res: { ok(): boolean; text(): Promise<string>; status(): number }, what: string) {
	expect(res.ok(), `${what} failed (${res.status()}): ${await res.text()}`).toBeTruthy();
}

async function api(ctx: APIRequestContext, method: 'get' | 'post', path: string, data?: object) {
	const res = await ctx[method](path, data ? { data } : undefined);
	await ok(res, `${method.toUpperCase()} ${path}`);
	return res.json();
}

/**
 * Seeds a main-event game and has alice prepare Make Introductions. With
 * `performSteps`, bob also prepares a Make Demands against it and wins the
 * perform_steps option; either way the record is advanced to the plan's row so
 * the plan is resolving and alice is at the pre-roll naming step.
 */
async function setup(peerCount: number, performSteps = false): Promise<Fixture> {
	const seeder = await pwRequest.newContext({ baseURL: E2E });
	const seed = await api(seeder, 'post', '/api/dev/seed', { phase: 'main_event', players: [...PLAYERS] });
	await seeder.dispose();
	const gameID: number = seed.game_id;
	track(gameID);

	const ctx = {} as Record<Name, APIRequestContext>;
	const ids = {} as Record<Name, number>;
	for (const [i, n] of PLAYERS.entries()) {
		ctx[n] = await pwRequest.newContext({ baseURL: E2E });
		await ctx[n].post(`/api/dev/login?username=${n}`);
		ids[n] = seed.players[i].id;
	}

	const mi = await api(ctx.alice, 'post', `/api/tables/${gameID}/prepare-plan`, {
		plan_type: 'make_introductions', peer_count: peerCount, preparation_notes: 'Two useful people.',
	});
	const planID: number = mi.plan.id;
	const row: number = mi.plan.row_number;

	let demandID = 0;
	if (performSteps) {
		const md = await api(ctx.bob, 'post', `/api/tables/${gameID}/prepare-plan`, {
			plan_type: 'make_demands', target_plan_id: planID, preparation_notes: 'A demand.',
		});
		demandID = md.plan.id;
	}

	await api(ctx.alice, 'post', '/api/dev/advance-row', { game_id: gameID, advance_to: row });
	if (performSteps) await winPerformSteps(ctx, gameID, demandID);

	// Advancing onto the row kicks the plan off itself; tolerate the explicit
	// resolve having nothing left to do.
	const res = await ctx.alice.post(`/api/plans/${planID}/resolve`);
	if (!res.ok()) expect(await res.text()).toContain('not in pending status');
	return { gameID, ctx, ids, planID };
}

/** Resolve bob's demand as a make and draft so bob holds perform_steps. */
async function winPerformSteps(ctx: Record<Name, APIRequestContext>, gameID: number, demandID: number) {
	const { roll } = await api(ctx.alice, 'get', `/api/tables/${gameID}/rolls/active`);
	await api(ctx.bob, 'post', `/api/rolls/${roll.id}/skip-vote`);
	// The demand's difficulty outruns the two base dice; leverage enough assets.
	const assets = await api(ctx.bob, 'get', `/api/tables/${gameID}/assets`);
	const mine = (assets.assets as { id: number; owner_id: number }[]).filter(a => a.owner_id === roll.actor_id);
	for (const a of mine.slice(0, 3)) {
		await api(ctx.bob, 'post', `/api/rolls/${roll.id}/leverage`, { asset_id: a.id });
	}
	await api(ctx.alice, 'post', '/api/dev/force-roll', { roll_id: roll.id, outcome: 'make' });

	// The draft alternates by power rank; whoever's turn it is picks. Bob wants
	// perform_steps (and keep_assets); alice takes whatever is left.
	const bobWants = ['perform_steps', 'keep_assets'];
	const aliceWants = ['control_leverage', 'keep_or_change_target'];
	for (let picks = 0; picks < 4; picks++) {
		let done = false;
		for (const [who, wants] of [['bob', bobWants], ['alice', aliceWants]] as const) {
			const opt = wants[0];
			if (!opt) continue;
			const res = await ctx[who].post(`/api/plans/${demandID}/draft-choice`, { data: { option: opt } });
			if (res.ok()) { wants.shift(); done = true; break; }
		}
		expect(done, 'no player could take the next draft pick').toBeTruthy();
	}
	// The last pick may complete the demand itself.
	const done = await ctx.bob.post(`/api/plans/${demandID}/complete`);
	if (!done.ok()) expect(await done.text()).toContain('not in resolving status');
}

async function openPages(browser: import('@playwright/test').Browser, f: Fixture) {
	const pages = {} as Record<Name, Page>;
	for (const n of PLAYERS) {
		const c = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 812 } });
		await c.request.post(`/api/dev/login?username=${n}`);
		pages[n] = await c.newPage();
		await pages[n].goto(`/table/${f.gameID}`);
	}
	return pages;
}

/** The preparer names the peers through the UI; returns after the roll opens. */
async function nameAndRoll(alice: Page, names: string[]) {
	const inputs = alice.getByPlaceholder('Name, title, role…');
	await expect(inputs).toHaveCount(names.length, { timeout: 10_000 });
	for (const [i, n] of names.entries()) await inputs.nth(i).fill(n);
	await alice.getByRole('button', { name: 'Name peers & roll' }).click();
	await expect(alice.getByText('Dice roll in progress…')).toBeVisible({ timeout: 10_000 });
}

async function forceRoll(f: Fixture, outcome: 'make' | 'mar') {
	const { roll } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/rolls/active`);
	// A one-die-per-peer difficulty still needs dice: skip the vote and leverage
	// nothing; force-roll writes every die's face itself.
	await f.ctx.alice.post(`/api/rolls/${roll.id}/skip-vote`);
	const assets = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/assets`);
	const mine = (assets.assets as { id: number; owner_id: number }[]).filter(a => a.owner_id === f.ids.alice);
	const need = Math.max(0, roll.difficulty - 2);
	for (const a of mine.slice(0, need)) {
		await api(f.ctx.alice, 'post', `/api/rolls/${roll.id}/leverage`, { asset_id: a.id });
	}
	await api(f.ctx.alice, 'post', '/api/dev/force-roll', { roll_id: roll.id, outcome });
}

/**
 * A make/mar that the dice apply on their own reaches the table only on a
 * reload — see the fixme at the bottom of this file — so reload to see it.
 */
async function reloadAll(pages: Record<Name, Page>) {
	await Promise.all(PLAYERS.map(n => pages[n].reload()));
}

async function disposeAll(f: Fixture, pages?: Record<Name, Page>) {
	for (const n of PLAYERS) {
		await f.ctx[n].dispose();
		await pages?.[n].context().close();
	}
}

test('pre-roll: everyone sees the difficulty meter and the buffet; only the preparer names the peers', async ({ browser }) => {
	const f = await setup(2);
	const pages = await openPages(browser, f);
	const { alice, bob, carol } = pages;

	// The meter (2 + 2 peers = 4) and its reason are public.
	for (const p of [alice, bob, carol]) {
		await expect(p.getByText('2, plus 1 for each of the 2 peers introduced.')).toBeVisible({ timeout: 10_000 });
		await expect(p.getByRole('img', { name: 'Difficulty 4 of 6' })).toBeVisible();
	}

	// The buffet is collapsed until tapped, then previews the make and the mar.
	await bob.getByRole('button', { name: 'What can happen?' }).click();
	await expect(bob.getByText('Every named peer arrives, and the newcomer is described with a marginalia.')).toBeVisible();
	await bob.getByRole('tab', { name: 'Mar' }).click();
	await expect(bob.getByText('Each peer gets a different one of these:')).toBeVisible();
	await expect(bob.getByText('Broken journey', { exact: false }).first()).toBeVisible();

	// Only alice gets the naming form; the others are told who is naming.
	await expect(alice.getByRole('button', { name: 'Name peers & roll' })).toBeVisible();
	for (const p of [bob, carol]) {
		await expect(p.getByText('alice is naming the new peers…')).toBeVisible();
		await expect(p.getByRole('button', { name: 'Name peers & roll' })).toHaveCount(0);
	}
	// ...and the server agrees: a non-preparer cannot name a peer.
	const res = await f.ctx.bob.post(`/api/plans/${f.planID}/create-peer`, { data: { name: 'Interloper' } });
	expect(res.status()).toBe(403);

	await nameAndRoll(alice, ['Lady Ashby', 'Sir Oswin']);

	await disposeAll(f, pages);
});

test('make: the preparer describes each arrival; the last arrival ends the plan', async ({ browser }) => {
	const f = await setup(2);
	const pages = await openPages(browser, f);
	const { alice, bob } = pages;

	await nameAndRoll(alice, ['Lady Ashby', 'Sir Oswin']);
	await forceRoll(f, 'make');
	await reloadAll(pages);

	// Name pre-filled from the pre-roll; the button stays off until a marginalia is written.
	await expect(alice.getByText('arrives at court. Describe them', { exact: false })).toBeVisible({ timeout: 10_000 });
	await expect(alice.locator('.acf input').first()).toHaveValue('Lady Ashby');
	const arrive = alice.getByRole('button', { name: 'They arrive', exact: true });
	await expect(arrive).toBeDisabled();
	await expect(bob.getByText('alice is welcoming the new peers…')).toBeVisible();

	await alice.locator('.acf textarea').first().fill('Holds the northern road.');
	await expect(arrive).toBeEnabled();
	await arrive.click();

	// Second (last) peer: the button says the plan resolves.
	await expect(alice.locator('.acf input').first()).toHaveValue('Sir Oswin', { timeout: 10_000 });
	await alice.locator('.acf textarea').first().fill('Owes everyone money.');
	await alice.getByRole('button', { name: 'They arrive — the plan resolves' }).click();

	await expect.poll(async () => {
		const { plans } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/plans`);
		return plans.find((p: { id: number }) => p.id === f.planID).status;
	}, { timeout: 10_000 }).toBe('resolved');
	const { assets } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/assets`);
	const names = assets.filter((a: { owner_id: number }) => a.owner_id === f.ids.alice).map((a: { name: string }) => a.name);
	expect(names).toEqual(expect.arrayContaining(['Lady Ashby', 'Sir Oswin']));

	await disposeAll(f, pages);
});

test('mar: each peer gets a different fate, and the people who owe marginalia are asked', async ({ browser }) => {
	const f = await setup(2);
	const pages = await openPages(browser, f);
	const { alice, bob, carol } = pages;

	await nameAndRoll(alice, ['Lady Ashby', 'Sir Oswin']);
	await forceRoll(f, 'mar');
	await reloadAll(pages);

	await expect(alice.getByText('(2 remaining):')).toBeVisible({ timeout: 10_000 });
	for (const p of [bob, carol]) {
		await expect(p.getByText('alice is resolving the marred introductions…')).toBeVisible();
	}

	// First peer: another retinue — bob's. Needs a target before it can go.
	await alice.getByRole('button', { name: 'Another retinue', exact: true }).click();
	const resolve = alice.getByRole('button', { name: 'Resolve peer' });
	await expect(resolve).toBeDisabled();
	await alice.locator('.player-chip', { hasText: 'bob' }).click();
	await resolve.click();

	// Second peer: the outcome already handed out is greyed, with a reason on tap.
	await expect(alice.getByText('(1 remaining):')).toBeVisible({ timeout: 10_000 });
	const taken = alice.getByRole('button', { name: 'Another retinue', exact: true });
	await expect(taken).toHaveAttribute('aria-disabled', 'true');
	await taken.click({ force: true }); // aria-disabled reads as not actionable to Playwright
	await expect(alice.getByText('Another retinue is already taken by another peer — each new peer gets a different outcome.')).toBeVisible();
	// The server refuses a repeat, too.
	const { plans } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/plans`);
	const rd = JSON.parse(plans.find((p: { id: number }) => p.id === f.planID).resolution_data);
	const second = rd.make_introductions.drafts.find(
		(d: { id: string }) => !rd.make_introductions.mar_outcomes.some((o: { draft_id: string }) => o.draft_id === d.id));
	const dup = await f.ctx.alice.post(`/api/plans/${f.planID}/introductions-mar`, {
		data: { draft_id: second.id, outcome: 'other_retinue', target_player_id: f.ids.bob },
	});
	expect(dup.ok()).toBeFalsy();

	// Carol writes the second peer's real identity.
	await alice.getByRole('button', { name: 'Not who you thought', exact: true }).click();
	await alice.locator('.player-chip', { hasText: 'carol' }).click();
	await alice.getByRole('button', { name: 'Resolve peer' }).click();

	// Bob and carol are each asked for the marginalia they owe; alice is not.
	// (They need a reload to see it — second fixme below.)
	await Promise.all([bob.reload(), carol.reload()]);
	await expect(bob.locator('.ft-prompt', { hasText: 'is joining your retinue' })).toBeVisible({ timeout: 10_000 });
	await expect(carol.locator('.ft-prompt', { hasText: "isn't who alice thought they were" })).toBeVisible({ timeout: 10_000 });
	await expect(alice.locator('.ft-prompt', { hasText: 'Write the marginalia' })).toHaveCount(0);

	await bob.getByPlaceholder('Who has arrived at court?').fill('A cousin with debts.');
	await bob.getByRole('button', { name: 'Write marginalia' }).click();
	await carol.getByPlaceholder('Who has arrived at court?').fill('Really a tax assessor.');
	await carol.getByRole('button', { name: 'Write marginalia' }).click();

	await expect.poll(async () => {
		const { plans: ps } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/plans`);
		return ps.find((p: { id: number }) => p.id === f.planID).status;
	}, { timeout: 10_000 }).toBe('resolved');

	await disposeAll(f, pages);
});

test('perform_steps winner drives the post-roll steps; the preparer still names and waits', async ({ browser }) => {
	const f = await setup(1, true);
	const pages = await openPages(browser, f);
	const { alice, bob } = pages;

	// Pre-roll the winner is just a bystander: alice names, bob waits.
	await expect(bob.getByText('alice is naming the new peers…')).toBeVisible({ timeout: 10_000 });
	await expect(bob.getByRole('button', { name: 'Name peers & roll' })).toHaveCount(0);
	const res = await f.ctx.bob.post(`/api/plans/${f.planID}/create-peer`, { data: { name: 'Demander pick' } });
	expect(res.status()).toBe(403);

	await nameAndRoll(alice, ['Lady Ashby']);
	await forceRoll(f, 'make');
	await reloadAll(pages);

	// Post-roll the winner gets the arrival form and the preparer sees the wait.
	await expect(bob.getByText('arrives at court. Describe them', { exact: false })).toBeVisible({ timeout: 10_000 });
	await expect(alice.getByText('bob is welcoming the new peers…')).toBeVisible();
	await expect(alice.getByText('arrives at court', { exact: false })).toHaveCount(0);

	await bob.locator('.acf textarea').first().fill('Bob vouches for them.');
	await bob.getByRole('button', { name: 'They arrive — the plan resolves' }).click();
	await expect.poll(async () => {
		const { plans } = await api(f.ctx.alice, 'get', `/api/tables/${f.gameID}/plans`);
		return plans.find((p: { id: number }) => p.id === f.planID).status;
	}, { timeout: 10_000 }).toBe('resolved');

	await disposeAll(f, pages);
});

// FINDING: the dice apply a make/mar server-side (applyAutoChoiceOnRoll) and
// save resolution_data, but nothing tells the clients — unlike MakeChoice, the
// auto-apply path never broadcasts plan.choice_applied. The preparer's panel
// stays on "is welcoming the new peers…" until a reload. See
// docs/work/next/make-introductions-roll-not-live.md.
test.fixme('make: the arrival form appears live when the dice land, without a reload', async ({ browser }) => {
	const f = await setup(1);
	const pages = await openPages(browser, f);
	await nameAndRoll(pages.alice, ['Lady Ashby']);
	await forceRoll(f, 'make');
	await expect(pages.alice.getByText('arrives at court. Describe them', { exact: false }))
		.toBeVisible({ timeout: 10_000 });
	await disposeAll(f, pages);
});

// The same gap on the mar side: introductions-mar saves the fate and moves the
// Waiting On bar to the people who owe marginalia, but sends them no plan
// event, so their form is missing until they reload.
test.fixme('mar: a named marginalia author sees their form live, without a reload', async ({ browser }) => {
	const f = await setup(1);
	const pages = await openPages(browser, f);
	await nameAndRoll(pages.alice, ['Lady Ashby']);
	await forceRoll(f, 'mar');
	await reloadAll(pages);
	await pages.alice.getByRole('button', { name: 'Not who you thought', exact: true }).click();
	await pages.alice.locator('.player-chip', { hasText: 'bob' }).click();
	await pages.alice.getByRole('button', { name: 'Resolve peer' }).click();
	await expect(pages.bob.getByPlaceholder('Who has arrived at court?')).toBeVisible({ timeout: 10_000 });
	await disposeAll(f, pages);
});
