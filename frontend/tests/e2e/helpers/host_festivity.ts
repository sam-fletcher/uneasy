import { test, expect, request as pwRequest, type APIResponse, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Host Festivity specs: a four-player main-event game where
// alice hosts and bob, carol and dave are guests.
//
// Alice is seeded at esteem rank 4 (a dummy holds rank 3), so the guests' roll
// difficulty is 6 − 4 = 2. A guest rolls two dice, so a make is reachable with
// /api/dev/force-roll (at difficulty 3 or more it would not be). Nobody needs
// a scene or a peer: Host Festivity has no target and no plan-level roll.

export const NAMES = ['alice', 'bob', 'carol', 'dave'] as const;
export type Name = (typeof NAMES)[number];

export interface HFGame {
	gameID: number;
	ids: Record<Name, number>;
	ctx: Record<Name, BrowserContext>;
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedHFGame opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

/** Assert an exact refusal status, so a 500 can't pass for "the server said no". */
export async function expectStatus(res: APIResponse, status: 400 | 403 | 409) {
	expect(res.status(), await res.text()).toBe(status);
}

/**
 * The one-open-roll lock specifically: a 409 whose message is the lock's. Other
 * 409s (already acted, nothing left to spend) would pass a bare status check
 * for the wrong reason.
 */
export async function expectLocked(res: APIResponse) {
	const text = await res.text();
	expect(res.status(), text).toBe(409);
	expect(text).toContain('a dice roll is in progress');
}

export async function seedHFGame(browser: Browser, viewport = { width: 375, height: 812 }): Promise<HFGame> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: [...NAMES],
			current_row: 1,
			// value[k] = player index; a dummy holds rank 3 in a four-player game, so this puts alice at rank 4.
			rankings: { esteem: [1, 2, 0, 3] },
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	await fixture.dispose();

	const ctx = {} as HFGame['ctx'];
	for (const n of NAMES) {
		ctx[n] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[n]);
		await ctx[n].request.post(`/api/dev/login?username=${n}`);
	}
	const ids = {} as HFGame['ids'];
	NAMES.forEach((n, i) => { ids[n] = seeded.players[i].id; });
	return { gameID: seeded.game_id, ids, ctx };
}

/** Alice prepares Host Festivity and the record jumps to its row; it opens as resolving. */
export async function startFestivity(g: HFGame): Promise<number> {
	const prep = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'host_festivity', preparation_notes: 'A masque at the Orangery.' },
	});
	expect(prep.ok(), `prepare failed: ${await prep.text()}`).toBeTruthy();
	const planID = (await prep.json()).plan.id as number;
	const adv = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: planID } });
	expect(adv.ok(), `advance-row failed: ${await adv.text()}`).toBeTruthy();
	await expect.poll(() => planStatus(g, planID)).toBe('resolving');
	return planID;
}

export async function openPages(g: HFGame, who: readonly Name[] = NAMES): Promise<Record<Name, Page>> {
	const out = {} as Record<Name, Page>;
	for (const n of who) {
		out[n] = await g.ctx[n].newPage();
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

/** POST one of the plan's extra routes as `who`. */
export function act(g: HFGame, who: Name, planID: number, route: string, data: object = {}) {
	return g.ctx[who].request.post(`/api/plans/${planID}/${route}`, { data });
}

/** `who` starts a roll. Returns the roll id (the roll is open, in leverage). */
export async function startRoll(g: HFGame, who: Name, planID: number): Promise<number> {
	const res = await act(g, who, planID, 'guest-roll', { action: 'roll' });
	expect(res.status(), `guest-roll: ${await res.text()}`).toBe(201);
	return (await res.json()).roll.id as number;
}

export async function forceRoll(g: HFGame, rollID: number, outcome: 'make' | 'mar') {
	const res = await g.ctx.alice.request.post('/api/dev/force-roll', { data: { roll_id: rollID, outcome } });
	expect(res.ok(), `force-roll failed: ${await res.text()}`).toBeTruthy();
}

export async function choose(g: HFGame, who: Name, planID: number, body: object) {
	const res = await act(g, who, planID, 'guest-choice', body);
	expect(res.ok(), `guest-choice failed: ${await res.text()}`).toBeTruthy();
}

/** `who` rolls, the dice are forced, and the option is chosen — a whole roll-and-choice. */
export async function rollAndChoose(
	g: HFGame, who: Name, planID: number, outcome: 'make' | 'mar', body: object,
) {
	await forceRoll(g, await startRoll(g, who, planID), outcome);
	await choose(g, who, planID, body);
}

export async function optOut(g: HFGame, who: Name, planID: number) {
	const res = await act(g, who, planID, 'guest-roll', { action: 'opt_out' });
	expect(res.ok(), `opt-out failed: ${await res.text()}`).toBeTruthy();
}

export interface Festivity {
	outcomes?: Record<string, string>;
	guest_roll_ids?: Record<string, number>;
	guest_ious?: number[];
	host_makes_taken?: string[];
	host_mar_insists?: string[];
	centered_asset_ids?: number[];
	disagreement_asset_ids?: number[];
	pending_challenge?: { challenger_id: number; target_id: number } | null;
	unchosen_host_mars?: number[];
}

async function planRow(g: HFGame, planID: number) {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as {
		plans: { id: number; status: string; result: string | null; resolution_data: string | null }[];
	};
	return plans.find(p => p.id === planID)!;
}

export async function planStatus(g: HFGame, planID: number): Promise<string> {
	return (await planRow(g, planID)).status;
}

export async function planResult(g: HFGame, planID: number): Promise<string | null> {
	return (await planRow(g, planID)).result;
}

/** The festivity's server-side state, as the plans API serves it. */
export async function festivity(g: HFGame, planID: number): Promise<Festivity> {
	const raw = (await planRow(g, planID)).resolution_data;
	return raw ? (JSON.parse(raw).festivity ?? {}) : {};
}

/**
 * Every Host Festivity line in the plan's action log, oldest first, with the
 * `@@id|name@@` player markers flattened to the name. Read over the API rather
 * than the chat column so a line at any severity is visible (the default chat
 * filter hides the quiet ones). Poll it — entries can trail the state change.
 */
export async function festivityLog(g: HFGame, planID: number, onlyFestivity = true): Promise<string[]> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/posts`);
	const { posts } = (await res.json()) as {
		posts: { plan_id: number | null; system_code: string | null; body: string }[];
	};
	return posts
		.filter(p => p.plan_id === planID && (!onlyFestivity || p.system_code === 'plan.host_festivity'))
		.map(p => p.body.replace(/@@\d+\|([^@]*)@@/g, '$1'));
}

export async function expectLogLine(g: HFGame, planID: number, line: string) {
	await expect
		.poll(async () => (await festivityLog(g, planID)).includes(line), {
			message: `action log should contain: ${line}`,
		})
		.toBe(true);
}

/** Waits for an action-log line containing `part` (for lines carrying asset marks or quoted text). */
export async function expectLogContaining(g: HFGame, planID: number, part: string) {
	await expect
		.poll(async () => (await festivityLog(g, planID)).some(l => l.includes(part)), {
			message: `action log should contain: ${part}`,
		})
		.toBe(true);
}

export interface TestAsset {
	id: number;
	owner_id: number;
	name: string;
	is_main_character: boolean;
	is_destroyed: boolean;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}

export async function assetsOf(g: HFGame): Promise<TestAsset[]> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`);
	return ((await res.json()) as { assets: TestAsset[] }).assets;
}

export async function mainCharacter(g: HFGame, who: Name): Promise<TestAsset> {
	const mc = (await assetsOf(g)).find(a => a.owner_id === g.ids[who] && a.is_main_character);
	expect(mc, `${who} has no main character`).toBeTruthy();
	return mc!;
}

/** A named peer for `who`, with the given marginalia (a new asset takes exactly one; the rest are added). */
export async function addPeer(g: HFGame, who: Name, name: string, marginalia: string[]): Promise<number> {
	const res = await g.ctx[who].request.post(`/api/tables/${g.gameID}/assets`, {
		data: { asset_type: 'peer', name, is_main_character: false, marginalia: [marginalia[0]] },
	});
	expect(res.ok(), `create peer failed: ${await res.text()}`).toBeTruthy();
	const id = (await res.json()).asset.id as number;
	for (const text of marginalia.slice(1)) await addMarginalia(g, who, id, text);
	return id;
}

export async function addMarginalia(g: HFGame, who: Name, assetID: number, text: string) {
	const res = await g.ctx[who].request.post(`/api/assets/${assetID}/marginalia`, { data: { text } });
	expect(res.ok(), `add marginalia failed: ${await res.text()}`).toBeTruthy();
}

/** The scorecard row ("The Talk of the Event") for one player. */
export function scoreRow(page: Page, name: string) {
	return page.locator('.fest-scorecard li').filter({ has: page.locator('.who', { hasText: name }) });
}
