import { test, expect, request as pwRequest, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Chronicle Histories specs: a three-player main-event game
// where alice prepares and bob and carol are the other chroniclers' audience.
// The dev seed gives every player one artifact ("<player>'s artifact") with one
// marginalia; bob's gets a second so a break can tear without destroying.
// Setup goes over the API; the specs drive the panels they pin.

export type Who = 'alice' | 'bob' | 'carol';
export const WHO: Who[] = ['alice', 'bob', 'carol'];

export interface CHGame {
	gameID: number;
	ids: Record<Who, number>;
	ctx: Record<Who, BrowserContext>;
}

export interface ApiAsset {
	id: number;
	owner_id: number;
	name: string;
	asset_type: string;
	is_destroyed: boolean;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedCHGame opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

export async function seedCHGame(
	browser: Browser,
	viewport: { width: number; height: number } = { width: 1280, height: 900 },
): Promise<CHGame> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: WHO, current_row: 1 },
	});
	expect(seedRes.status(), `seed failed: ${await seedRes.text()}`).toBe(200);
	const seeded = await seedRes.json();
	await fixture.dispose();

	const ctx = {} as CHGame['ctx'];
	for (const n of WHO) {
		ctx[n] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[n]);
		await ctx[n].request.post(`/api/dev/login?username=${n}`);
	}
	return {
		gameID: seeded.game_id,
		ids: { alice: seeded.players[0].id, bob: seeded.players[1].id, carol: seeded.players[2].id },
		ctx,
	};
}

/** Alice opens and ends a scene — the table won't offer a plan until one has run. */
export async function runScene(g: CHGame) {
	const sceneRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/scenes`, {
		data: { location_custom: 'The Orangery', time_elapsed: 'days', present_peer_ids: [] },
	});
	expect(sceneRes.status(), `scene failed: ${await sceneRes.text()}`).toBe(201);
	const endRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/end-scene`);
	expect(endRes.status(), `end-scene failed: ${await endRes.text()}`).toBe(200);
}

export async function assetsOf(g: CHGame): Promise<ApiAsset[]> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`);
	expect(res.status()).toBe(200);
	return ((await res.json()) as { assets: ApiAsset[] }).assets;
}

/** The seeded artifact of `who`. */
export async function artifactOf(g: CHGame, who: Who): Promise<ApiAsset> {
	const a = (await assetsOf(g)).find(x => x.asset_type === 'artifact' && x.owner_id === g.ids[who]);
	expect(a, `${who} has no artifact`).toBeTruthy();
	return a!;
}

/** Give `who`'s artifact a second marginalia, so one tear leaves it standing. */
export async function addMarginalia(g: CHGame, who: Who, text: string) {
	const a = await artifactOf(g, who);
	const res = await g.ctx[who].request.post(`/api/assets/${a.id}/marginalia`, { data: { text } });
	expect(res.status(), `add marginalia failed: ${await res.text()}`).toBeLessThan(300);
}

/** Alice prepares Chronicle Histories over the API. Returns the plan id. */
export async function prepareCH(g: CHGame): Promise<number> {
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'chronicle_histories', preparation_notes: 'Why was the old treaty broken?' },
	});
	expect(res.status(), `prepare failed: ${await res.text()}`).toBe(201);
	return (await res.json()).plan.id as number;
}

/** Fast-forward the record to the plan's row; the plan opens as resolving, pre-roll. */
export async function resolveCH(g: CHGame, planID: number) {
	const res = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: planID } });
	expect(res.status(), `advance-row failed: ${await res.text()}`).toBe(200);
}

/** Alice casts the roll over the API. Returns the roll id. */
export async function castCH(g: CHGame, planID: number, artifactIDs: number[], scene = 'A cold archive, long ago.') {
	const res = await g.ctx.alice.request.post(`/api/plans/${planID}/cast-roll`, {
		data: { artifact_ids: artifactIDs, scene },
	});
	expect(res.status(), `cast-roll failed: ${await res.text()}`).toBe(200);
	return (await res.json()).roll.id as number;
}

/** Force the roll to a make (result = difficulty) or mar (difficulty − 1). */
export async function forceCH(g: CHGame, rollID: number, outcome: 'make' | 'mar') {
	const res = await g.ctx.alice.request.post('/api/dev/force-roll', { data: { roll_id: rollID, outcome } });
	expect(res.status(), `force-roll failed: ${await res.text()}`).toBe(200);
	return ((await res.json()) as { roll: { result: number; difficulty: number } }).roll;
}

/**
 * Alice's plan is resolving with the dice cast over `invoke` (the owners of the
 * artifacts invoked) and forced. Returns the plan, roll and invoked ids.
 */
export async function rolledCH(g: CHGame, invoke: Who[], outcome: 'make' | 'mar') {
	await runScene(g);
	const planID = await prepareCH(g);
	await resolveCH(g, planID);
	const artifacts = await Promise.all(invoke.map(w => artifactOf(g, w)));
	const rollID = await castCH(g, planID, artifacts.map(a => a.id));
	const roll = await forceCH(g, rollID, outcome);
	return { planID, rollID, roll, artifacts };
}

export async function openAll(g: CHGame): Promise<Record<Who, Page>> {
	const out = {} as Record<Who, Page>;
	for (const n of WHO) {
		out[n] = await g.ctx[n].newPage();
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

export async function planStatus(g: CHGame, planID: number): Promise<string> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as { plans: { id: number; status: string }[] };
	return plans.find(p => p.id === planID)!.status;
}

/** The chronicle slice of the plan's resolution_data. */
export async function chronicleData(g: CHGame, planID: number) {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as { plans: { id: number; resolution_data: unknown }[] };
	const raw = plans.find(p => p.id === planID)!.resolution_data;
	const rd = typeof raw === 'string' ? JSON.parse(raw) : raw;
	return (rd?.chronicle_histories ?? {}) as {
		invoked_artifact_ids?: number[];
		invoke_phase_closed?: boolean;
		make_budget?: number;
		make_choices_done?: number;
		mar_active?: boolean;
		mar_required_choices?: number;
		steps?: { option: string; asset_id?: number; player_id?: number; narration?: string }[];
	};
}

/** The page has no sideways scroll: nothing is wider than the 375px viewport. */
export async function expectNoHorizontalScroll(page: Page) {
	const over = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
	expect(over, 'page scrolls sideways').toBeLessThanOrEqual(0);
}

/**
 * Everything the desktop chat column shows. While the plan's scene is open its
 * entries sit loose in the feed; once the plan resolves they fold into
 * collapsed containers, which are expanded here so the entries are readable.
 */
async function readPlanLog(page: Page, _planID: number): Promise<string> {
	const chat = page.getByRole('complementary', { name: 'Chat' });
	const closed = chat.locator('button.scene-header[aria-expanded="false"]:not([disabled])');
	for (let i = 0; i < 6 && (await closed.count()) > 0; i++) await closed.first().click();
	return chat.innerText();
}

/** Waits until the desktop chat column shows every snippet, in order. */
export async function expectPlanLog(page: Page, planID: number, snippets: string[]) {
	let last = '';
	await expect
		.poll(
			async () => {
				last = await readPlanLog(page, planID);
				let at = -1;
				for (const sn of snippets) {
					const i = last.indexOf(sn, at + 1);
					if (i <= at) return sn;
					at = i;
				}
				return null;
			},
			{ timeout: 10_000, message: 'plan log should show the entries in order' },
		)
		.toBeNull()
		.catch(err => {
			throw new Error(`${err.message}\nlog was:\n${last}`);
		});
}
