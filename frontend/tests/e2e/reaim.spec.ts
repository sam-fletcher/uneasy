import { test, expect, request as pwRequest, type APIRequestContext, type Browser, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';
import * as MD from './helpers/make_demands';

// The Re-aim panel (ReAimPanel.svelte) stands in for a plan's own panel while the
// plan is paused because its prepare-time target is gone. Exchange Courtiers'
// branch (a player, then one of their peers) is covered in exchange_courtiers.spec.ts.
// This file covers the other two:
//
//   · Spread Rumors: the rumor's subject was destroyed, so the preparer names
//     another asset. A real pause, driven over the API.
//   · Make Demands: the demand's target plan is no longer pending. The server only
//     reaches that state through its database (no route can make it), so the page
//     is given the paused plan and the options by intercepting the requests that
//     carry them (and told there is no roll, as in a real pause); the rest is the
//     real app. What it checks is what the page DOES with
//     the options: a card that picks with a tick, a button that follows the pick, and
//     the option sent back verbatim.

const track = cleanupGameAfterEach();
MD.cleanupAfterEach();

const opened: { close(): Promise<void> }[] = [];
test.afterEach(async () => {
	for (const o of opened.splice(0)) await o.close().catch(() => {});
});

async function api(ctx: APIRequestContext, method: 'get' | 'post', path: string, data?: object) {
	const res = await ctx[method](path, data ? { data } : undefined);
	expect(res.ok(), `${method.toUpperCase()} ${path} failed (${res.status()}): ${await res.text()}`).toBeTruthy();
	return res.json();
}

async function pageAs(browser: Browser, gameID: number, name: string): Promise<Page> {
	const c = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 900 } });
	opened.push(c);
	await c.request.post(`/api/dev/login?username=${name}`);
	return c.newPage();
}

test('spread rumors: a rumor whose subject was destroyed pauses for the preparer to name a new one', async ({ browser }) => {
	const seeder = await pwRequest.newContext({ baseURL: E2E });
	opened.push({ close: () => seeder.dispose() });
	const seed = await api(seeder, 'post', '/api/dev/seed', {
		phase: 'main_event', players: ['alice', 'bob', 'carol'], current_row: 1,
	});
	const gameID: number = seed.game_id;
	track(gameID);
	const ids = { alice: seed.players[0].id, bob: seed.players[1].id, carol: seed.players[2].id };
	const alice = await pwRequest.newContext({ baseURL: E2E });
	opened.push({ close: () => alice.dispose() });
	await alice.post('/api/dev/login?username=alice');

	// Alice spreads a rumor about bob's artifact (one marginalia, so one tear destroys it).
	const { assets } = await api(alice, 'get', `/api/tables/${gameID}/assets`);
	const artifact = (assets as { id: number; owner_id: number; asset_type: string; name: string }[])
		.find(a => a.owner_id === ids.bob && a.asset_type === 'artifact')!;
	const { plan } = await api(alice, 'post', `/api/tables/${gameID}/prepare-plan`, {
		plan_type: 'spread_rumors', target_asset_id: artifact.id, preparation_notes: 'A debt was never paid.',
	});
	await api(seeder, 'post', '/api/dev/tear-marginalia', {
		asset_id: artifact.id, position: 1, player_id: ids.bob,
	});
	await api(alice, 'post', '/api/dev/advance-row', { game_id: gameID, advance_to: plan.row_number });
	const kick = await alice.post(`/api/plans/${plan.id}/resolve`);
	// Advancing onto the row already kicked the plan off and paused it, so this either
	// finds it not pending, or is refused because the re-aim is owed: the state we want.
	if (!kick.ok()) expect(await kick.text()).toMatch(/not in pending status|target is gone/);

	const pa = await pageAs(browser, gameID, 'alice');
	const pb = await pageAs(browser, gameID, 'bob');
	const pc = await pageAs(browser, gameID, 'carol');
	for (const p of [pa, pb, pc]) await p.goto(`/table/${gameID}`);

	// The preparer is told what happened and asked for a new subject; nothing else is offered.
	const panel = pa.locator('.reaim');
	await expect(panel.getByText('has lost its mark')).toBeVisible({ timeout: 10_000 });
	await expect(panel.getByText('Choose a new target. The plan carries on from the top once you do.')).toBeVisible();
	const reaim = pa.getByRole('button', { name: 'Re-aim this plan' });
	await expect(reaim).toBeDisabled();

	// Bob (whose artifact it was) and carol only wait, and get no controls.
	for (const p of [pb, pc]) {
		await expect(p.getByText('Waiting for alice to choose a new target')).toBeVisible({ timeout: 10_000 });
		await expect(p.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
	}

	// Picking a card enables the button; picking another moves the pick.
	const cards = panel.locator('.card');
	await cards.filter({ hasText: "carol's artifact" }).getByRole('checkbox').click();
	await expect(reaim).toBeEnabled();
	await cards.filter({ hasText: "carol's holding" }).getByRole('checkbox').click();
	await expect(cards.filter({ hasText: "carol's artifact" }).getByRole('checkbox')).not.toBeChecked();
	await cards.filter({ hasText: "carol's artifact" }).getByRole('checkbox').click();

	await reaim.click();
	// The plan carries on from the top: the pause is gone for everyone.
	for (const p of [pa, pb, pc]) await expect(p.locator('.reaim')).toHaveCount(0, { timeout: 10_000 });
});

test('make demands: the demand offers the plans it can still be aimed at, picked with a tick and sent back verbatim', async ({ browser }) => {
	const f = await MD.seedMD({ target: 'seek_answers' });
	await MD.prepareDemand(f);
	await MD.reachRow(f);

	const reason = "alice's Seek Answers is no longer waiting to resolve.";
	let sent: unknown = null;

	const open = async (name: 'bob' | 'carol') => {
		const p = await pageAs(browser, f.gameID, name);
		// The plan list, with the demand marked as paused for a re-aim.
		await p.route(new RegExp(`/api/tables/${f.gameID}/plans(\\?.*)?$`), async (route) => {
			const res = await route.fetch();
			const body = await res.json();
			for (const plan of body.plans as { id: number; resolution_data: string | null }[]) {
				if (plan.id !== f.demandID) continue;
				const data = plan.resolution_data ? JSON.parse(plan.resolution_data) : {};
				plan.resolution_data = JSON.stringify({ ...data, reaim: { owed: true, reason } });
			}
			await route.fulfill({ response: res, json: body });
		});
		// A real pause comes before the demand's roll is made, so there is no roll: without
		// this the page would show a live vote step above the panel, a state that cannot occur.
		await p.route(new RegExp(`/api/tables/${f.gameID}/rolls/active$`), (route) =>
			route.fulfill({ json: { roll: null } }));
		// What the server would offer, and what the page sends back.
		await p.route(new RegExp(`/api/plans/${f.demandID}/reaim$`), async (route) => {
			if (route.request().method() === 'POST') {
				sent = route.request().postDataJSON();
				await route.fulfill({ json: {} });
			} else {
				await route.fulfill({
					json: { options: [{ target_player_id: null, target_asset_id: null, target_plan_id: f.targetID }] },
				});
			}
		});
		await p.goto(`/table/${f.gameID}`);
		return p;
	};

	const bob = await open('bob');
	const panel = bob.locator('.reaim');
	await expect(panel.getByText(reason)).toBeVisible({ timeout: 10_000 });
	await expect(panel.getByText('Demand against:')).toBeVisible();

	// One option card: the plan and what its preparer wrote. Nothing is picked yet.
	const card = panel.locator('.choice-card');
	await expect(card).toHaveCount(1);
	await expect(card).toContainText("alice's Seek Answers");
	await expect(card).toContainText('Old ledgers and older grudges.');
	await expect(card).toHaveAttribute('aria-pressed', 'false');
	await expect(card.getByRole('img', { name: 'Picked' })).toHaveCount(0);
	const reaim = bob.getByRole('button', { name: 'Re-aim this plan' });
	await expect(reaim).toBeDisabled();

	// Picking marks it with the tick; picking it again lets go.
	await card.click();
	await expect(card).toHaveAttribute('aria-pressed', 'true');
	await expect(card.getByRole('img', { name: 'Picked' })).toBeVisible();
	await expect(reaim).toBeEnabled();
	await card.click();
	await expect(card.getByRole('img', { name: 'Picked' })).toHaveCount(0);
	await expect(reaim).toBeDisabled();

	// Pick it and re-aim: the option goes back exactly as the server listed it.
	await card.click();
	await reaim.click();
	await expect.poll(() => sent).toEqual({ target_player_id: null, target_asset_id: null, target_plan_id: f.targetID });

	// A bystander gets the wait line and no controls.
	const carol = await open('carol');
	await expect(carol.getByText('Waiting for bob to choose a new target')).toBeVisible({ timeout: 10_000 });
	await expect(carol.getByRole('button', { name: 'Re-aim this plan' })).toHaveCount(0);
	await expect(carol.locator('.choice-card')).toHaveCount(0);
});
