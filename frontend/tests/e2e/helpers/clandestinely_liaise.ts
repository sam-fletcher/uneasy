import { test, expect, request as pwRequest, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Clandestinely Liaise specs: a three-player main-event game
// where alice prepares a liaison with bob and carol is the bystander. The seeded
// retinues hold one main-character peer, a holding, an artifact and a resource
// each; the liaison needs a dedicated meeting peer per side, so each of alice and
// bob gets a "Confidant" (one marginalia, so breaking it destroys it).
//
// `drive` says how far the API takes the plan before the test starts:
//   · 'none'     — peers made, nothing prepared yet (for prepare-time validation);
//   · 'prepared' — prepared, delay reveal open, nobody has submitted;
//   · 'together' — resolving at Together at Last;
//   · 'keep'     — Secrets We Keep, nobody has submitted;
//   · 'share'    — Things We Share, both secrets kept, nobody has picked;
//   · 'redelay'  — When Will I See You Again?, picks revealed, no face in.

export type Who = 'alice' | 'bob' | 'carol';
const NAMES: Who[] = ['alice', 'bob', 'carol'];
export type Drive = 'none' | 'prepared' | 'together' | 'keep' | 'share' | 'redelay';

export const NOTES = 'A quiet word in the orangery.';
/** Every secret this plan writes starts with this; it must never reach a bystander. */
export const SECRET_PREFIX = 'Clandestine meeting with';

export interface AssetRow {
	id: number;
	owner_id: number;
	asset_type: string;
	name: string;
	is_destroyed: boolean;
	secret_count: number;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}

export interface PlanRow {
	id: number;
	plan_type: string;
	status: string;
	result: string | null;
	row_number: number | null;
	resolution_data: string | null;
}

export interface LGame {
	gameID: number;
	planID: number;
	ids: Record<Who, number>;
	ctx: Record<Who, BrowserContext>;
	/** Alice's and bob's dedicated meeting peers. */
	peers: { alice: number; bob: number };
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedLiaise opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

export async function seedLiaise(
	browser: Browser,
	drive: Drive,
	viewport = { width: 375, height: 812 },
	seedExtra: Record<string, unknown> = {},
): Promise<LGame> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: NAMES, ...seedExtra },
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	await fixture.dispose();

	const ctx = {} as LGame['ctx'];
	for (const n of NAMES) {
		ctx[n] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[n]);
		await ctx[n].request.post(`/api/dev/login?username=${n}`);
	}
	const gameID: number = seeded.game_id;
	const g: LGame = {
		gameID,
		planID: 0,
		ids: { alice: seeded.players[0].id, bob: seeded.players[1].id, carol: seeded.players[2].id },
		ctx,
		peers: { alice: 0, bob: 0 },
	};

	for (const who of ['alice', 'bob'] as const) {
		const res = await ctx[who].request.post(`/api/tables/${gameID}/assets`, {
			data: {
				asset_type: 'peer',
				name: `${cap(who)} Confidant`,
				is_main_character: false,
				marginalia: ['trusted go-between'],
			},
		});
		expect(res.ok(), `create peer failed: ${await res.text()}`).toBeTruthy();
		g.peers[who] = (await res.json()).asset.id as number;
	}

	if (drive === 'none') return g;

	g.planID = await prepareLiaison(g);
	if (drive === 'prepared') return g;

	await settleDelay(g, 1, 1);
	await resolveLiaison(g);
	if (drive === 'together') return g;

	await post(g, 'alice', `/api/plans/${g.planID}/advance-liaise`, {}, 200);
	if (drive === 'keep') return g;

	await post(g, 'alice', `/api/plans/${g.planID}/keep-secret`, { asset_id: g.peers.alice }, 200);
	await post(g, 'bob', `/api/plans/${g.planID}/keep-secret`, { asset_id: g.peers.bob }, 200);
	if (drive === 'share') return g;

	await post(g, 'alice', `/api/plans/${g.planID}/share-choice`, {
		choice: 'leverage_partner', target_asset_id: g.peers.bob,
	}, 200);
	await post(g, 'bob', `/api/plans/${g.planID}/share-choice`, {
		choice: 'leverage_partner', target_asset_id: g.peers.alice,
	}, 200);
	return g;
}

export const cap = (s: string) => s[0].toUpperCase() + s.slice(1);

/** POST as `who`, asserting the exact status; returns the parsed body (or {} if none). */
export async function post(g: LGame, who: Who, url: string, data: unknown, status: number) {
	const res = await g.ctx[who].request.post(url, { data });
	expect(res.status(), `${who} POST ${url} → ${await res.text()}`).toBe(status);
	try { return await res.json(); } catch { return {}; }
}

/** Alice prepares the liaison with bob, naming both Confidants. */
export async function prepareLiaison(g: LGame, overrides: Record<string, unknown> = {}): Promise<number> {
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: {
			plan_type: 'clandestinely_liaise',
			target_player_id: g.ids.bob,
			preparer_peer_id: g.peers.alice,
			partner_peer_id: g.peers.bob,
			preparation_notes: NOTES,
			...overrides,
		},
	});
	expect(res.ok(), `prepare failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).plan.id as number;
}

export async function fetchPlans(g: LGame, who: Who = 'alice'): Promise<PlanRow[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/plans`);
	expect(res.ok(), `GET plans failed: ${await res.text()}`).toBeTruthy();
	return ((await res.json()) as { plans: PlanRow[] }).plans;
}

export async function fetchPlan(g: LGame, who: Who = 'alice', id = g.planID): Promise<PlanRow> {
	const p = (await fetchPlans(g, who)).find(x => x.id === id);
	expect(p, `plan ${id} not found`).toBeTruthy();
	return p!;
}

/** The liaise block of the plan's resolution_data, as `who` sees it. */
export async function liaiseData(g: LGame, who: Who = 'alice', id = g.planID): Promise<any> {
	const plan = await fetchPlan(g, who, id);
	return JSON.parse(plan.resolution_data ?? '{}').liaise ?? {};
}

export async function fetchAssets(g: LGame, who: Who = 'alice'): Promise<AssetRow[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/assets`);
	expect(res.ok(), `GET assets failed: ${await res.text()}`).toBeTruthy();
	return ((await res.json()) as { assets: AssetRow[] }).assets;
}

export async function asset(g: LGame, id: number, who: Who = 'alice'): Promise<AssetRow> {
	const a = (await fetchAssets(g, who)).find(x => x.id === id);
	expect(a, `asset ${id} not found`).toBeTruthy();
	return a!;
}

/** The secret texts `who` may read on an asset. */
export async function readableSecrets(g: LGame, who: Who, assetID: number): Promise<string[]> {
	const res = await g.ctx[who].request.get(`/api/assets/${assetID}/secrets`);
	expect(res.status(), `${who} GET secrets of ${assetID}`).toBe(200);
	return ((await res.json()).secrets as { text: string }[]).map(s => s.text);
}

/** Both players submit a delay face (alice first), setting the plan's row. */
export async function settleDelay(g: LGame, aliceFace: number, bobFace: number) {
	const liaise = await liaiseData(g);
	const revealID: number = liaise.delay_reveal_id;
	expect(revealID, 'delay_reveal_id missing').toBeTruthy();
	await post(g, 'alice', `/api/reveals/${revealID}/submit`, { face: aliceFace }, 200);
	await post(g, 'bob', `/api/reveals/${revealID}/submit`, { face: bobFace }, 200);
}

/** Fast-forward to the plan's row; the liaison opens as resolving at Together at Last. */
export async function resolveLiaison(g: LGame) {
	const plan = await fetchPlan(g);
	expect(plan.row_number, 'row_number not set after the delay reveal').toBeTruthy();
	const adv = await g.ctx.alice.request.post('/api/dev/advance-row', {
		data: { game_id: g.gameID, advance_to: plan.row_number },
	});
	expect(adv.ok(), `advance-row failed: ${await adv.text()}`).toBeTruthy();
	// Advancing onto the row may already have kicked resolution off (409 then).
	const res = await g.ctx.alice.request.post(`/api/plans/${g.planID}/resolve`);
	if (!res.ok()) expect(await res.text()).toContain('not in pending status');
	await expect
		.poll(async () => (await fetchPlan(g)).status, { message: 'plan should be resolving' })
		.toBe('resolving');
}

export async function openAll(
	g: LGame,
	beforeGoto?: (who: Who, page: Page) => void,
): Promise<Record<Who, Page>> {
	const out = {} as Record<Who, Page>;
	for (const n of NAMES) {
		out[n] = await g.ctx[n].newPage();
		beforeGoto?.(n, out[n]);
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

/** The Things We Share / Secrets We Keep etc. panel section with this header. */
export function section(page: Page, header: string) {
	return page.locator('.choices-section', { hasText: header });
}

/** Alice or bob's liaison panel: pick a card by asset name and tick it. */
export async function pickCard(sec: ReturnType<typeof section>, assetName: string) {
	const card = sec.locator('.card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
}

/** Posts in the action log as `who` reads it. */
export async function logBodies(g: LGame, who: Who = 'alice'): Promise<string[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/posts`);
	if (res.status() !== 200) return [];
	return ((await res.json()) as { posts: { body: string }[] }).posts.map(p => p.body);
}

/** Waits until an action-log post contains `snippet`; returns that post's body. */
export async function expectLogged(g: LGame, snippet: string, who: Who = 'alice'): Promise<string> {
	let found = '';
	await expect
		.poll(
			async () => {
				found = (await logBodies(g, who)).find(b => b.includes(snippet)) ?? '';
				return found !== '';
			},
			{ timeout: 10_000, message: `action log should contain "${snippet}"` },
		)
		.toBe(true);
	return found;
}

export async function currentRow(g: LGame): Promise<number> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}`);
	expect(res.ok(), `GET table failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).game.current_row as number;
}

