import { test, expect, request as pwRequest, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Make War specs: a three-player main-event game. alice
// declares war on bob; carol is the third seat, who can join either side, stay
// out, or just watch. The dev seed gives every player one asset of each type,
// each with one marginalia, named "<player>'s holding", "<player>'s artifact",
// and so on. Setup goes over the API; the specs drive the panels they pin.

export type Who = 'alice' | 'bob' | 'carol';
export const WHO: Who[] = ['alice', 'bob', 'carol'];

export interface WarGame {
	gameID: number;
	ids: Record<Who, number>;
	ctx: Record<Who, BrowserContext>;
	/** Set by declareWar. */
	planID: number;
	revealID: number;
}

export interface WarParticipant {
	player_id: number;
	side: 1 | 2;
	surrendered_at_row: number | null;
	entry_payment_complete: boolean;
}
export interface WarState {
	status: 'active' | 'ended';
	end_reason: string | null;
	ended_at_row: number | null;
	participants: WarParticipant[];
	outstanding_costs: { payer_id: number; opponent_id: number }[];
	open_proposal: { id: number; proposer_id: number; terms: string } | null;
	open_claims: { id: number; claimant_id: number; surrendered_id: number }[];
}
export interface ApiAsset {
	id: number;
	owner_id: number;
	name: string;
	asset_type: string;
	is_leveraged: boolean;
	is_destroyed: boolean;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedWarGame opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

export async function seedWarGame(
	browser: Browser,
	viewport: { width: number; height: number } = { width: 1280, height: 900 },
): Promise<WarGame> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: WHO, current_row: 1 },
	});
	expect(seedRes.status(), `seed failed: ${await seedRes.text()}`).toBe(200);
	const seeded = await seedRes.json();
	await fixture.dispose();

	const ctx = {} as WarGame['ctx'];
	for (const n of WHO) {
		ctx[n] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[n]);
		await ctx[n].request.post(`/api/dev/login?username=${n}`);
	}
	return {
		gameID: seeded.game_id,
		ids: { alice: seeded.players[0].id, bob: seeded.players[1].id, carol: seeded.players[2].id },
		ctx,
		planID: 0,
		revealID: 0,
	};
}

/** Alice opens and ends a scene — the table won't offer a plan until one has run. */
export async function runScene(g: WarGame) {
	const sceneRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/scenes`, {
		data: { location_custom: 'The Orangery', time_elapsed: 'days', present_peer_ids: [] },
	});
	expect(sceneRes.status(), `scene failed: ${await sceneRes.text()}`).toBe(201);
	const endRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/end-scene`);
	expect(endRes.status(), `end-scene failed: ${await endRes.text()}`).toBe(200);
}

/** Alice declares war on `enemies` over the API. Fills planID and revealID. */
export async function declareWar(g: WarGame, enemies: Who[] = ['bob']) {
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: {
			plan_type: 'make_war',
			enemy_player_ids: enemies.map(e => g.ids[e]),
			preparation_notes: 'The border has been insulted once too often.',
		},
	});
	expect(res.status(), `declare failed: ${await res.text()}`).toBe(201);
	const { plan } = await res.json();
	const rd = typeof plan.resolution_data === 'string' ? JSON.parse(plan.resolution_data) : plan.resolution_data;
	g.planID = plan.id;
	g.revealID = rd.make_war.delay_reveal_id;
}

export async function joinWar(g: WarGame, who: Who, side: 1 | 2) {
	const res = await g.ctx[who].request.post(`/api/plans/${g.planID}/join-war`, { data: { side } });
	expect(res.status(), `join-war failed: ${await res.text()}`).toBe(200);
}

/** Every listed player reveals a delay face; the last one closes the vote. */
export async function revealFaces(g: WarGame, faces: Partial<Record<Who, number>>) {
	for (const [who, face] of Object.entries(faces) as [Who, number][]) {
		const res = await g.ctx[who].request.post(`/api/reveals/${g.revealID}/submit`, { data: { face } });
		expect(res.status(), `reveal ${who} failed: ${await res.text()}`).toBe(200);
	}
}

/** Jump the record to `row` (a row past the war's own, so a cost of battle is owed). */
export async function advanceTo(g: WarGame, row: number) {
	const res = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { game_id: g.gameID, row } });
	expect(res.status(), `advance-row failed: ${await res.text()}`).toBe(200);
}

/** Declare, join, reveal and fast-forward until the cost of battle is owed. */
export async function warAtCostRow(
	g: WarGame,
	opts: { enemies?: Who[]; joins?: [Who, 1 | 2][]; faces?: Partial<Record<Who, number>>; row?: number } = {},
) {
	await runScene(g);
	await declareWar(g, opts.enemies ?? ['bob']);
	for (const [who, side] of opts.joins ?? []) await joinWar(g, who, side);
	await revealFaces(g, opts.faces ?? { alice: 2, bob: 2 });
	// Land on the war's own row first, so the plan resolves (the panel's hide rule
	// keys off a resolved plan), then on a later row where the cost falls due.
	const own = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: g.planID } });
	expect(own.status(), `advance to war row failed: ${await own.text()}`).toBe(200);
	await advanceTo(g, opts.row ?? 5);
}

export async function warState(g: WarGame): Promise<WarState> {
	const res = await g.ctx.alice.request.get(`/api/plans/${g.planID}/war-state`);
	expect(res.status()).toBe(200);
	return res.json();
}

export async function assetsOf(g: WarGame): Promise<ApiAsset[]> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`);
	expect(res.status()).toBe(200);
	return (await res.json()).assets;
}

export async function assetNamed(g: WarGame, name: string): Promise<ApiAsset> {
	const a = (await assetsOf(g)).find(x => x.name === name);
	expect(a, `asset "${name}" not found`).toBeTruthy();
	return a!;
}

/** The player who pays first: the head of the outstanding list. */
export async function frontPayer(g: WarGame): Promise<Who> {
	const s = await warState(g);
	const id = s.outstanding_costs[0]?.payer_id;
	const who = WHO.find(w => g.ids[w] === id);
	expect(who, `no front payer in ${JSON.stringify(s.outstanding_costs)}`).toBeTruthy();
	return who!;
}

export async function openAll(g: WarGame): Promise<Record<Who, Page>> {
	const out = {} as Record<Who, Page>;
	for (const n of WHO) {
		out[n] = await g.ctx[n].newPage();
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

/** The Waiting On strip's full text. */
export function waitingBar(page: Page) {
	return page.locator('.waiting-on-bar');
}

/**
 * Waits until some post in the game's action log (read as `page`'s player)
 * contains `snippet`. Polls the API, not the DOM, so it is layout-independent.
 */
export async function expectLogged(g: WarGame, snippet: string) {
	await expect
		.poll(
			async () => {
				const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/posts`);
				if (res.status() !== 200) return `status ${res.status()}`;
				return JSON.stringify(await res.json()).includes(snippet);
			},
			{ timeout: 10_000, message: `action log should contain "${snippet}"` },
		)
		.toBe(true);
}

/** The first action-log post whose body contains `snippet`, polled until it exists. */
export async function postWith(g: WarGame, snippet: string): Promise<{ body: string; row_number: number | null }> {
	let found: { body: string; row_number: number | null } | undefined;
	await expect
		.poll(
			async () => {
				const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/posts`);
				if (res.status() !== 200) return false;
				const { posts } = (await res.json()) as { posts: { body: string; row_number: number | null }[] };
				found = posts.find(p => p.body.includes(snippet));
				return found !== undefined;
			},
			{ timeout: 10_000, message: `a post containing "${snippet}"` },
		)
		.toBe(true);
	return found!;
}
