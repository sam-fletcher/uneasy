import { test, expect, request as pwRequest, type APIResponse, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Propose Duel specs: a five-player main-event game where
// alice challenges bob. The esteem track is pinned so bob is last (rank 5): the
// duel's difficulty is then 1 and bob may stake at most two assets, which makes
// every outcome small enough to reason about. alice (rank 1) holds initiative.
// carol (rank 2) is the bystander; dave and erin only fill the track.
//
// The bout dice are server-rolled and the final roll is created already
// resolved, so a spec cannot force the outcome. It does the next best thing:
// each duellist can read their OWN stake dice from duel-state, so the helpers
// below read both and pick the declaration that produces the outcome wanted —
// or report that these dice cannot (a tie), so the caller can seed another game.

export type Who = 'alice' | 'bob' | 'carol';
const NAMES = ['alice', 'bob', 'carol', 'dave', 'erin'];
const WHO: Who[] = ['alice', 'bob', 'carol'];

export interface Duel {
	gameID: number;
	planID: number;
	ids: Record<Who, number>;
	ctx: Record<Who, BrowserContext>;
}

export interface DuelOptions {
	viewport?: { width: number; height: number };
	/** bob's esteem rank: 5 (default) → difficulty 1, at most 2 stakes; 4 → difficulty 2, at most 3. */
	bobRank?: 4 | 5;
}

const opened: BrowserContext[] = [];
const games: number[] = [];

/** Registers an afterEach that closes every context seedDuel opened and deletes its games. */
export function cleanupDuelsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
		const ids = games.splice(0);
		if (ids.length === 0) return;
		const api = await pwRequest.newContext({ baseURL: E2E });
		for (const id of ids) await api.post('/api/dev/delete-game', { data: { game_id: id } });
		await api.dispose();
	});
}

export async function deleteDuel(g: Duel) {
	await Promise.all(WHO.map(w => g.ctx[w].close().catch(() => {})));
	for (const w of WHO) opened.splice(opened.indexOf(g.ctx[w]), 1);
	games.splice(games.indexOf(g.gameID), 1);
	const api = await pwRequest.newContext({ baseURL: E2E });
	await api.post('/api/dev/delete-game', { data: { game_id: g.gameID } });
	await api.dispose();
}

/**
 * Seeds the game, logs the three seats in and has alice prepare the duel; the
 * plan is left prepared, one row before it opens. The caller gives the duellists
 * whatever assets they want to stake, then calls openDuel().
 */
export async function seedDuel(browser: Browser, opts: DuelOptions = {}): Promise<Duel> {
	const viewport = opts.viewport ?? { width: 1280, height: 900 };
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: NAMES,
			// value[k] = player index at rank k+1: bob (1) fifth, or fourth.
			rankings: { esteem: opts.bobRank === 4 ? [0, 2, 3, 1, 4] : [0, 2, 3, 4, 1] },
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	await fixture.dispose();
	games.push(seeded.game_id);

	const ctx = {} as Duel['ctx'];
	for (const w of WHO) {
		ctx[w] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[w]);
		await ctx[w].request.post(`/api/dev/login?username=${w}`);
	}
	const ids = { alice: seeded.players[0].id, bob: seeded.players[1].id, carol: seeded.players[2].id };

	const prep = await ctx.alice.request.post(`/api/tables/${seeded.game_id}/prepare-plan`, {
		data: {
			plan_type: 'propose_duel',
			target_player_id: ids.bob,
			duel_type: 'arms',
			preparation_notes: 'On the dueling green at dawn.',
		},
	});
	expect(prep.ok(), `prepare failed: ${await prep.text()}`).toBeTruthy();
	return { gameID: seeded.game_id, planID: (await prep.json()).plan.id, ids, ctx };
}

/** Creates an unleveraged peer for `who` (a stake the duel can take). */
export async function addPeer(g: Duel, who: Who, name: string): Promise<number> {
	const res = await g.ctx[who].request.post(`/api/tables/${g.gameID}/assets`, {
		data: { asset_type: 'peer', name, is_main_character: false, marginalia: ['unleveraged'] },
	});
	expect(res.ok(), `peer ${name}: ${await res.text()}`).toBeTruthy();
	return assetID(g, name);
}

export async function assetID(g: Duel, name: string): Promise<number> {
	const { assets } = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`)).json();
	const a = assets.find((x: { name: string }) => x.name === name);
	expect(a, `asset ${name} missing`).toBeTruthy();
	return a.id;
}

/** Fast-forwards to the duel's row, which opens the plan as resolving (setup phase). */
export async function openDuel(g: Duel) {
	const adv = await g.ctx.alice.request.post('/api/dev/advance-row', {
		data: { game_id: g.gameID, advance_to: 6 },
	});
	expect(adv.ok(), `advance-row failed: ${await adv.text()}`).toBeTruthy();
	// Advancing onto the plan's row auto-kicks-off resolution; an explicit resolve
	// races with that and may 409 once the plan is already resolving.
	const res = await g.ctx.alice.request.post(`/api/plans/${g.planID}/resolve`);
	if (!res.ok()) expect(await res.text(), 'unexpected resolve failure').toContain('not in pending status');
}

export interface DuelStateStake { id: number; player_id: number; asset_id: number; is_resolved: boolean; hidden_die: number | null }
export interface DuelStateBout {
	id: number; declarer_id: number; responder_id: number; declarer_stake_id: number;
	declaration: string | null; declarer_die: number | null; responder_die: number | null;
	winner_id: number | null; is_match: boolean; resolved_at: string | null;
}

export async function duelState(g: Duel, who: Who): Promise<{ stakes: DuelStateStake[]; bouts: DuelStateBout[] }> {
	const res = await g.ctx[who].request.get(`/api/plans/${g.planID}/duel-state`);
	expect(res.ok(), `duel-state as ${who}: ${await res.text()}`).toBeTruthy();
	return res.json();
}

/** Commit stakes over the API: ok() is the response so a spec can assert a refusal's status. */
export function selectStakes(g: Duel, who: Who, assetIDs: number[]): Promise<APIResponse> {
	return g.ctx[who].request.post(`/api/plans/${g.planID}/select-stakes`, { data: { asset_ids: assetIDs } });
}

/** The caller's own unresolved stake dice, in stake order. Only their own are readable. */
export async function myDice(g: Duel, who: Who): Promise<{ stakeID: number; die: number }[]> {
	const { stakes } = await duelState(g, who);
	return stakes
		.filter(s => s.player_id === g.ids[who] && !s.is_resolved && s.hidden_die != null)
		.map(s => ({ stakeID: s.id, die: s.hidden_die as number }));
}

export async function declare(g: Duel, who: Who, stakeID: number, declaration: 'high' | 'low'): Promise<APIResponse> {
	return g.ctx[who].request.post(`/api/plans/${g.planID}/bout-declare`, {
		data: { stake_id: stakeID, declaration },
	});
}

export async function respond(g: Duel, who: Who, stakeID: number): Promise<APIResponse> {
	return g.ctx[who].request.post(`/api/plans/${g.planID}/bout-respond`, { data: { stake_id: stakeID } });
}

/**
 * The declaration that makes the declarer win the bout (`win`) or lose it, given
 * both dice; null on a tie, which sets the dice aside and decides nothing.
 */
export function callFor(declarerDie: number, responderDie: number, win: boolean): 'high' | 'low' | null {
	if (declarerDie === responderDie) return null;
	const highWins = declarerDie > responderDie;
	return highWins === win ? 'high' : 'low';
}

export async function planOf(g: Duel): Promise<{ status: string; result: string | null; resolution_data: unknown }> {
	const { plans } = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`)).json();
	const p = plans.find((x: { id: number }) => x.id === g.planID);
	expect(p, 'duel plan missing').toBeTruthy();
	return p;
}

/** A log body without its markup: player mentions (@@id|name@@) and **bold** asset marks. */
export function plain(body: string): string {
	return body.replace(/@@\d+\|([^@]*)@@/g, '$1').replace(/\*\*/g, '');
}

/** Every plan.propose_duel post body on the log, as seen by `who`. */
export async function duelLog(g: Duel, who: Who): Promise<string[]> {
	const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/posts`);
	expect(res.ok(), `posts: ${await res.text()}`).toBeTruthy();
	const body = await res.json();
	const posts: { body: string; system_code?: string }[] = body.posts ?? body;
	return posts.filter(p => p.system_code === 'plan.propose_duel').map(p => plain(p.body));
}

export async function openTable(g: Duel, who: Who): Promise<Page> {
	const page = await g.ctx[who].newPage();
	await page.goto(`/table/${g.gameID}`);
	return page;
}

/** The duel panel's `.choices-section` whose header contains `header`. */
export function section(page: Page, header: string) {
	return page.locator('.choices-section', { hasText: header });
}

export interface DiceDuel extends Duel {
	aliceAssets: number[];
	bobAssets: number[];
	/** Dice under the first stake each side will play in bout 1 (alice declares it). */
	aliceDie: number;
	bobDie: number;
	aliceStake: number;
	bobStake: number;
}

/**
 * Seeds a duel, stakes `aliceStakes` / `bobStakes` peers (named "Alice Peer 1…",
 * "Bob Peer 1…") over the API and reads both sides' first dice. Seeds again until
 * those dice give the wanted bout 1: alice wins it (a make if it is the only
 * bout), bob wins it (a mar), or they tie. The duel is left in its bouts phase
 * with nothing declared, so the caller can play the bout through the UI.
 */
export async function duelWithDice(
	browser: Browser,
	want: 'aliceWins' | 'bobWins' | 'tie',
	stakes: { alice: number; bob: number },
	opts: DuelOptions = {},
): Promise<DiceDuel> {
	for (let attempt = 0; attempt < 40; attempt++) {
		const g = await seedDuel(browser, opts);
		const aliceAssets: number[] = [];
		const bobAssets: number[] = [];
		for (let i = 1; i <= stakes.alice; i++) aliceAssets.push(await addPeer(g, 'alice', `Alice Peer ${i}`));
		for (let i = 1; i <= stakes.bob; i++) bobAssets.push(await addPeer(g, 'bob', `Bob Peer ${i}`));
		await openDuel(g);
		expect((await selectStakes(g, 'alice', aliceAssets)).ok()).toBeTruthy();
		expect((await selectStakes(g, 'bob', bobAssets)).ok()).toBeTruthy();
		const [a] = await stakeByAsset(g, 'alice', aliceAssets[0]);
		const [b] = await stakeByAsset(g, 'bob', bobAssets[0]);
		const tied = a.die === b.die;
		const call = callFor(a.die, b.die, want === 'aliceWins');
		if (want === 'tie' ? tied : !tied && call != null) {
			return { ...g, aliceAssets, bobAssets, aliceDie: a.die, bobDie: b.die, aliceStake: a.stakeID, bobStake: b.stakeID };
		}
		await deleteDuel(g);
	}
	throw new Error(`no seed gave dice for ${want} in 40 tries`);
}

async function stakeByAsset(g: Duel, who: Who, assetID: number) {
	const { stakes } = await duelState(g, who);
	return stakes
		.filter(s => s.player_id === g.ids[who] && s.asset_id === assetID && s.hidden_die != null)
		.map(s => ({ stakeID: s.id, die: s.hidden_die as number }));
}

/** The declaration alice should call in bout 1 so that she wins it (or loses it). */
export function aliceCall(d: DiceDuel, aliceWins: boolean): 'high' | 'low' {
	const c = callFor(d.aliceDie, d.bobDie, aliceWins);
	if (!c) throw new Error('tied dice have no winning call');
	return c;
}

export async function planWithData(g: Duel): Promise<{ plan: { status: string; result: string | null }; resolution_data?: { choice_submitted?: boolean; duel?: { phase?: string } } }> {
	const res = await g.ctx.alice.request.get(`/api/plans/${g.planID}`);
	expect(res.ok(), `GET plan: ${await res.text()}`).toBeTruthy();
	return res.json();
}

export async function rowState(g: Duel): Promise<{ kind: string; acting_player_ids?: number[] }> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/state`);
	expect(res.ok(), `state: ${await res.text()}`).toBeTruthy();
	return (await res.json()).row_state;
}

export async function assetsOf(g: Duel): Promise<{ name: string; owner_id: number; is_leveraged: boolean }[]> {
	const { assets } = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`)).json();
	return assets;
}
