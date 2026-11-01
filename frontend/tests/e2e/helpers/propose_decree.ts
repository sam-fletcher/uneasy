import { test, expect, request as pwRequest, type APIResponse, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Propose Decree specs: a three-player main-event game where
// alice prepares the decree. Where she sits on the power track decides who is
// seated for free and who must leverage in:
//   · `powerOrder: 'alice-first'` (the seed default) — alice is rank 1, so she is
//     the council and the signatory, and bob and carol each owe a join/decline;
//   · `powerOrder: 'alice-last'` — bob rank 1, carol rank 2, alice rank 3: all
//     three are seated at once, bob signs, and a mar is amended carol-then-bob.

export type Who = 'alice' | 'bob' | 'carol';
const NAMES: Who[] = ['alice', 'bob', 'carol'];

export interface PDGame {
	gameID: number;
	ids: Record<Who, number>;
	ctx: Record<Who, BrowserContext>;
}

export interface PDOptions {
	viewport?: { width: number; height: number };
	powerOrder?: 'alice-first' | 'alice-last';
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedPD opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

export async function seedPD(browser: Browser, opts: PDOptions = {}): Promise<PDGame> {
	const viewport = opts.viewport ?? { width: 1280, height: 900 };
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: NAMES,
			current_row: 1,
			// value[k] = player index at rank k+1
			...(opts.powerOrder === 'alice-last' ? { rankings: { power: [1, 2, 0] } } : {}),
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	await fixture.dispose();

	const ctx = {} as PDGame['ctx'];
	for (const n of NAMES) {
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

/** Alice prepares Propose Decree (a scene must have run first). Returns the plan id. */
export async function prepareDecree(g: PDGame, notes: string): Promise<number> {
	const scene = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/scenes`, {
		data: { location_custom: 'The Council Chamber', time_elapsed: 'days', present_peer_ids: [] },
	});
	expect(scene.ok(), `scene failed: ${await scene.text()}`).toBeTruthy();
	const end = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/end-scene`);
	expect(end.ok(), `end-scene failed: ${await end.text()}`).toBeTruthy();
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'propose_decree', preparation_notes: notes },
	});
	expect(res.ok(), `prepare failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).plan.id as number;
}

/** Fast-forward to the plan's row; it opens as resolving with the council seated. */
export async function resolveDecree(g: PDGame, planID: number) {
	const res = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: planID } });
	expect(res.ok(), `advance-row failed: ${await res.text()}`).toBeTruthy();
}

/** A plan in the resolving state, council seated, debate not yet opened. */
export async function resolvingDecree(browser: Browser, opts: PDOptions & { notes?: string } = {}) {
	const g = await seedPD(browser, opts);
	const planID = await prepareDecree(g, opts.notes ?? 'No private armies within the capital.');
	await resolveDecree(g, planID);
	return { g, planID };
}

export async function openAll(g: PDGame): Promise<Record<Who, Page>> {
	const out = {} as Record<Who, Page>;
	for (const n of NAMES) {
		out[n] = await g.ctx[n].newPage();
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

/** POST one of the decree's routes as `who`. */
export function act(g: PDGame, who: Who, planID: number, route: string, data?: object): Promise<APIResponse> {
	return g.ctx[who].request.post(`/api/plans/${planID}/${route}`, data === undefined ? {} : { data });
}

/** Like `act`, but the call must succeed. */
export async function actOK(g: PDGame, who: Who, planID: number, route: string, data?: object) {
	const res = await act(g, who, planID, route, data);
	expect(res.status(), `${route} as ${who}: ${await res.text()}`).toBe(200);
	return res.json();
}

/** Force the open roll. A mar is forced by exact result (1), since "outcome: mar" asks for difficulty-1 dice alice may not have. */
export async function forceRoll(g: PDGame, rollID: number, outcome: 'make' | 'mar') {
	const data = outcome === 'make' ? { roll_id: rollID, outcome } : { roll_id: rollID, result: 1 };
	const res = await g.ctx.alice.request.post('/api/dev/force-roll', { data });
	expect(res.ok(), `force-roll failed: ${await res.text()}`).toBeTruthy();
}

interface PlanRow {
	id: number;
	status: string;
	resolution_data: unknown;
}

export async function getPlan(g: PDGame, planID: number): Promise<PlanRow> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as { plans: PlanRow[] };
	return plans.find(p => p.id === planID)!;
}

/** The decree's own slice of resolution_data (the API ships it as a JSON string or an object). */
export async function decreeState(g: PDGame, planID: number): Promise<{
	signatory_id: number;
	signatory_player_ids: number[];
	declined_player_ids?: number[];
	amendment_order?: number[];
	law_text?: string;
	outcome?: string;
}> {
	const raw = (await getPlan(g, planID)).resolution_data;
	const rd = typeof raw === 'string' ? JSON.parse(raw) : raw;
	return (rd as { propose_decree: never }).propose_decree;
}

export interface LawRow {
	id: number;
	text: string;
	addendum: string | null;
	signatory_id: number | null;
	origin_plan_id: number | null;
}

export async function laws(g: PDGame, who: Who = 'alice'): Promise<LawRow[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/laws`);
	expect(res.status()).toBe(200);
	return ((await res.json()) as { laws: LawRow[] }).laws;
}

export interface AssetRow {
	id: number;
	name: string;
	owner_id: number;
	asset_type: string;
	is_destroyed: boolean;
	is_leveraged: boolean;
}

export async function assets(g: PDGame, who: Who = 'alice'): Promise<AssetRow[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/assets`);
	expect(res.status()).toBe(200);
	return ((await res.json()) as { assets: AssetRow[] }).assets;
}

/** An un-leveraged peer that `who` can offer to join the council. */
export async function spareAsset(g: PDGame, who: Who): Promise<AssetRow> {
	const mine = (await assets(g, who)).filter(a => a.owner_id === g.ids[who] && !a.is_leveraged && !a.is_destroyed);
	expect(mine.length, `${who} has no spare asset`).toBeGreaterThan(0);
	return mine.find(a => a.asset_type === 'peer') ?? mine[0];
}

/** Crown `who` through the dev shortcut, which does what the prologue's title claim does. */
export async function crown(g: PDGame, who: Who) {
	const res = await g.ctx[who].request.post('/api/dev/crown', { data: { game_id: g.gameID, player_id: g.ids[who] } });
	expect(res.status(), `crown failed: ${await res.text()}`).toBe(200);
}

/**
 * Propose Decree stages a scene, so its action-log entries sit in the chat's
 * scene group (not a `.plan-group`, which the other plan helpers read), and the
 * group collapses once the plan resolves. Expand whatever is collapsed and read
 * the feed on the always-open desktop chat column.
 */
async function readFeed(page: Page): Promise<string> {
	const feed = page.getByRole('complementary', { name: 'Chat' }).locator('.feed');
	const collapsed = feed.locator('.scene-header[aria-expanded="false"]');
	for (let i = await collapsed.count(); i > 0; i--) await collapsed.first().click();
	return feed.innerText();
}

/** Waits until the chat shows every snippet, in the given order (entries can trail the plan's status). */
export async function expectLog(page: Page, snippets: string[]) {
	let last = '';
	await expect
		.poll(
			async () => {
				last = await readFeed(page);
				let at = -1;
				for (const sn of snippets) {
					const i = last.indexOf(sn, at + 1);
					if (i <= at) return sn;
					at = i;
				}
				return null;
			},
			{ timeout: 10_000, message: 'chat should show the log entries in order' },
		)
		.toBeNull()
		.catch(err => {
			throw new Error(`${err.message}\nfeed was:\n${last}`);
		});
}

/** Reload every page: the roll's auto-applied outcome isn't pushed to open panels (docs/work/next/auto-applied-choice-not-broadcast.md). */
export async function reloadAll(pages: Page[]) {
	await Promise.all(pages.map(p => p.reload()));
}

/**
 * Force the roll, then wait until the server has recorded the outcome in the
 * decree (it applies it just after the dice land). A participant row can still
 * be appearing when the dev route marks everyone ready, which leaves the roll
 * open in the leverage stage; forcing again is harmless (a resolved roll is a
 * 409), so keep forcing until the outcome is in.
 */
export async function forceAndSettle(g: PDGame, planID: number, rollID: number, outcome: 'make' | 'mar') {
	await forceRoll(g, rollID, outcome);
	await expect
		.poll(
			async () => {
				const state = (await decreeState(g, planID)).outcome ?? '';
				if (state === '') {
					const data = outcome === 'make' ? { roll_id: rollID, outcome } : { roll_id: rollID, result: 1 };
					await g.ctx.alice.request.post('/api/dev/force-roll', { data });
				}
				return state;
			},
			{ timeout: 15_000 },
		)
		.toBe(outcome);
}

/** The calling player's unspent banked dice (a joiner holds one 'decree' die until the roll settles). */
export async function bankedDice(g: PDGame, who: Who): Promise<{ id: number; source: string }[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/banked-dice`);
	expect(res.status()).toBe(200);
	return ((await res.json()) as { dice: { id: number; source: string }[] }).dice;
}
