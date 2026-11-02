import { test, expect, request as pwRequest, type APIRequestContext, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Make Demands specs. Three players: alice prepares the
// TARGET plan, bob is the demander, carol a bystander. Everything up to the
// moment the spec cares about goes over the API; every assertion about what a
// player sees is made in a real page.

export const PLAYERS = ['alice', 'bob', 'carol'] as const;
export type Name = (typeof PLAYERS)[number];
export type DemandOption = 'control_leverage' | 'keep_or_change_target' | 'keep_assets' | 'perform_steps';
export type TargetKind = 'seek_answers' | 'spread_rumors' | 'propose_duel';

export interface MDFixture {
	gameID: number;
	api: Record<Name, APIRequestContext>;
	ids: Record<Name, number>;
	/** The target plan (alice's). */
	targetID: number;
	/** The demand (bob's); 0 until prepareDemand runs. */
	demandID: number;
	row: number;
}

export interface Opts {
	target?: TargetKind;
	/** Power track, player index at rank k+1 (default alice first). */
	power?: number[];
}

const disposables: { dispose(): Promise<void> }[] = [];
const gameIDs: number[] = [];

/** Registers the afterEach that closes every context and deletes every game this file's helpers opened. */
export function cleanupAfterEach() {
	test.afterEach(async () => {
		for (const d of disposables.splice(0)) await d.dispose().catch(() => {});
		const ids = gameIDs.splice(0);
		if (ids.length === 0) return;
		const c = await pwRequest.newContext({ baseURL: E2E });
		for (const id of ids) await c.post('/api/dev/delete-game', { data: { game_id: id } }).catch(() => {});
		await c.dispose();
	});
}

export async function ok(res: { ok(): boolean; text(): Promise<string>; status(): number }, what: string) {
	expect(res.ok(), `${what} failed (${res.status()}): ${await res.text()}`).toBeTruthy();
}

export async function call(ctx: APIRequestContext, method: 'get' | 'post', path: string, data?: object) {
	const res = await ctx[method](path, data ? { data } : undefined);
	await ok(res, `${method.toUpperCase()} ${path}`);
	return res.json();
}

/**
 * Seeds a main-event game with a pending target plan of alice's, (the record is at row 1). The demand is NOT prepared yet — see prepareDemand.
 */
export async function seedMD(opts: Opts = {}): Promise<MDFixture> {
	const target = opts.target ?? 'seek_answers';
	const seeder = await pwRequest.newContext({ baseURL: E2E });
	disposables.push(seeder);
	const seed = await call(seeder, 'post', '/api/dev/seed', {
		phase: 'main_event',
		players: [...PLAYERS],
		current_row: 1,
		rankings: { power: opts.power ?? [0, 1, 2], knowledge: [0, 1, 2] },
	});
	const gameID: number = seed.game_id;
	gameIDs.push(gameID);

	const api = {} as Record<Name, APIRequestContext>;
	const ids = {} as Record<Name, number>;
	for (const [i, n] of PLAYERS.entries()) {
		api[n] = await pwRequest.newContext({ baseURL: E2E });
		disposables.push(api[n]);
		await api[n].post(`/api/dev/login?username=${n}`);
		ids[n] = seed.players[i].id;
	}

	// Alice is the focus player and prepares the target; focus then passes to bob,
	// who may prepare the demand.
	const body = target === 'seek_answers'
		? { plan_type: 'seek_answers', preparation_notes: 'Old ledgers and older grudges.' }
		: target === 'spread_rumors'
			? { plan_type: 'spread_rumors', target_asset_id: await assetOf(api.alice, gameID, ids.bob), preparation_notes: 'Something about the Duchess and her wine.' }
			: { plan_type: 'propose_duel', target_player_id: ids.carol, duel_type: 'arms', preparation_notes: 'On the green at dawn.' };
	const targetID: number = (await call(api.alice, 'post', `/api/tables/${gameID}/prepare-plan`, body)).plan.id;
	const { plans } = await call(api.alice, 'get', `/api/tables/${gameID}/plans`);
	const row: number = plans.find((p: { id: number }) => p.id === targetID).row_number;
	return { gameID, api, ids, targetID, demandID: 0, row };
}

/** The id of some intact asset owned by `ownerID`. */
export async function assetOf(ctx: APIRequestContext, gameID: number, ownerID: number): Promise<number> {
	const { assets } = await call(ctx, 'get', `/api/tables/${gameID}/assets`);
	const a = (assets as { id: number; owner_id: number; is_destroyed: boolean }[]).find(x => x.owner_id === ownerID && !x.is_destroyed);
	expect(a, 'owner has an asset').toBeTruthy();
	return a!.id;
}

/** Bob prepares a Make Demands against the target plan. */
export async function prepareDemand(f: MDFixture): Promise<number> {
	const md = await call(f.api.bob, 'post', `/api/tables/${f.gameID}/prepare-plan`, {
		plan_type: 'make_demands', target_plan_id: f.targetID, preparation_notes: 'Hand it over, or else.',
	});
	f.demandID = md.plan.id;
	return f.demandID;
}

/** Jump the record to the demand's row; the demand opens first and its roll is bob's. */
export async function reachRow(f: MDFixture) {
	await call(f.api.alice, 'post', '/api/dev/advance-row', { game_id: f.gameID, advance_to: f.row });
	// The advance kicks the first plan off itself; tolerate it having nothing to do.
	const res = await f.api.bob.post(`/api/plans/${f.demandID}/resolve`);
	if (!res.ok()) expect(await res.text()).toContain('not in pending status');
}

/** Skip the difficulty vote, leverage bob's assets, and force the demand's roll. */
export async function rollDemand(f: MDFixture, outcome: 'make' | 'mar') {
	const { roll } = await call(f.api.alice, 'get', `/api/tables/${f.gameID}/rolls/active`);
	await call(f.api.bob, 'post', `/api/rolls/${roll.id}/skip-vote`);
	const { assets } = await call(f.api.bob, 'get', `/api/tables/${f.gameID}/assets`);
	const mine = (assets as { id: number; owner_id: number }[]).filter(a => a.owner_id === f.ids.bob);
	for (const a of mine.slice(0, 3)) await call(f.api.bob, 'post', `/api/rolls/${roll.id}/leverage`, { asset_id: a.id });
	await call(f.api.alice, 'post', '/api/dev/force-roll', { roll_id: roll.id, outcome });
}

/** Who drafts first: the higher-ranked of the demander and the target's preparer. */
export async function firstPicker(f: MDFixture): Promise<'alice' | 'bob'> {
	const { rankings } = await call(f.api.alice, 'get', `/api/tables/${f.gameID}/rankings`);
	const rank = (id: number) => (rankings as { category: string; player_id: number; rank: number }[])
		.find(r => r.category === 'power' && r.player_id === id)!.rank;
	return rank(f.ids.bob) < rank(f.ids.alice) ? 'bob' : 'alice';
}

/**
 * Take the draft by API. The picks alternate from the higher-ranked player, so
 * each player's list is consumed in turn: `wants.bob` is what bob takes on his
 * turns, `wants.alice` what alice does. Stops after `count` picks (default all 4).
 */
export async function draftWants(f: MDFixture, wants: { alice: DemandOption[]; bob: DemandOption[] }, count = 4) {
	const lists = { alice: [...wants.alice], bob: [...wants.bob] };
	let who = await firstPicker(f);
	for (let i = 0; i < count; i++) {
		const option = lists[who].shift();
		expect(option, `${who} has a pick planned`).toBeTruthy();
		await call(f.api[who], 'post', `/api/plans/${f.demandID}/draft-choice`, { option });
		who = who === 'alice' ? 'bob' : 'alice';
	}
}

/** Bob completes the made demand, so the target plan can resolve. Tolerates the draft having done it already. */
export async function completeDemand(f: MDFixture) {
	const res = await f.api.bob.post(`/api/plans/${f.demandID}/complete`);
	if (!res.ok()) expect(await res.text()).toContain('not in resolving status');
}

export async function plansOf(f: MDFixture) {
	const { plans } = await call(f.api.alice, 'get', `/api/tables/${f.gameID}/plans`);
	return plans as Record<string, any>[];
}

export async function planOf(f: MDFixture, id: number) {
	const p = (await plansOf(f)).find(x => x.id === id);
	expect(p, `plan ${id}`).toBeTruthy();
	return p!;
}

export async function openPages(browser: Browser, f: MDFixture, viewport = { width: 375, height: 812 }): Promise<Record<Name, Page>> {
	const pages = {} as Record<Name, Page>;
	for (const n of PLAYERS) {
		const c: BrowserContext = await browser.newContext({ baseURL: E2E, viewport });
		disposables.push({ dispose: () => c.close() });
		await c.request.post(`/api/dev/login?username=${n}`);
		pages[n] = await c.newPage();
		await pages[n].goto(`/table/${f.gameID}`);
	}
	return pages;
}

export const waitingBar = (page: Page) => page.locator('p', { hasText: 'Waiting On:' }).first();

/** The focus player opens and ends a scene — the table won't offer a plan until one has run. */
export async function runScene(f: MDFixture, who: Name) {
	await call(f.api[who], 'post', `/api/tables/${f.gameID}/scenes`, {
		location_custom: 'The Orangery', time_elapsed: 'days', present_peer_ids: [],
	});
	await call(f.api[who], 'post', `/api/tables/${f.gameID}/end-scene`);
}

/**
 * Once the demand is done a focus turn still sits between it and its target (the
 * general row rules put one between any two plans), so the third player runs a
 * scene and the target is then resolved by its preparer.
 */
export async function startTarget(f: MDFixture) {
	await runScene(f, 'carol');
	const res = await f.api.alice.post(`/api/plans/${f.targetID}/resolve`);
	if (!res.ok()) expect(await res.text()).toContain('not in pending status');
}

/** Every post on the table as plain lines, player mentions resolved to names, for polling. */
export async function postsText(f: MDFixture): Promise<string> {
	const res = await f.api.carol.get(`/api/tables/${f.gameID}/posts`);
	await ok(res, 'GET posts');
	const { posts } = (await res.json()) as { posts: { body: string }[] };
	return posts.map(p => p.body.replace(/@@\d+\|([^@]+)@@/g, '$1')).join('\n');
}
