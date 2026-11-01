import { test, expect, request as pwRequest, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { E2E } from '../baseUrl';

// Setup shared by the Exchange Courtiers specs: a three-player main-event game
// where alice prepares, bob is the target and carol is a bystander. Everyone
// gets one named non-main-character peer — the dev seed's retinue holds only the
// main character, and the plan needs a peer that is neither the table's last
// nor a main character (taking an MC trips the replacement gate).

export interface ECGame {
	gameID: number;
	ids: { alice: number; bob: number; carol: number };
	peers: { alice: number; bob: number; carol: number };
	ctx: { alice: BrowserContext; bob: BrowserContext; carol: BrowserContext };
}

export const PEER = { alice: 'Alice Courtier', bob: 'Bob Courtier', carol: 'Carol Courtier' } as const;

export interface ECOptions {
	viewport?: { width: number; height: number };
	/** Seed bob last on the power track, so the roll's difficulty is 2 (default 3). */
	easyTarget?: boolean;
	/** Marginalia on alice's courtier (default one). Two lets a break tear without destroying. */
	aliceMarginalia?: string[];
}

const opened: BrowserContext[] = [];

/** Registers an afterEach that closes every context seedECGame opened. */
export function closeContextsAfterEach() {
	test.afterEach(async () => {
		await Promise.all(opened.splice(0).map(c => c.close().catch(() => {})));
	});
}

export async function seedECGame(browser: Browser, opts: ECOptions = {}): Promise<ECGame> {
	const viewport = opts.viewport ?? { width: 1280, height: 900 };
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: ['alice', 'bob', 'carol'],
			current_row: 1,
			...(opts.easyTarget ? { rankings: { power: [0, 2, 1] } } : {}),
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	await fixture.dispose();

	const names = ['alice', 'bob', 'carol'] as const;
	const ctx = {} as ECGame['ctx'];
	for (const n of names) {
		ctx[n] = await browser.newContext({ baseURL: E2E, viewport });
		opened.push(ctx[n]);
		await ctx[n].request.post(`/api/dev/login?username=${n}`);
	}
	const peers = {} as ECGame['peers'];
	for (const n of names) {
		const res = await ctx[n].request.post(`/api/tables/${seeded.game_id}/assets`, {
			data: { asset_type: 'peer', name: PEER[n], is_main_character: false, marginalia: [n === 'alice' ? (opts.aliceMarginalia ?? ['sharp-tongued'])[0] : 'sharp-tongued'] },
		});
		expect(res.ok(), `create peer failed: ${await res.text()}`).toBeTruthy();
		peers[n] = (await res.json()).asset.id;
	}
	// A new asset takes exactly one marginalia; any more are added afterwards.
	for (const text of (opts.aliceMarginalia ?? []).slice(1)) {
		const res = await ctx.alice.request.post(`/api/assets/${peers.alice}/marginalia`, { data: { text } });
		expect(res.ok(), `add marginalia failed: ${await res.text()}`).toBeTruthy();
	}
	return {
		gameID: seeded.game_id,
		ids: { alice: seeded.players[0].id, bob: seeded.players[1].id, carol: seeded.players[2].id },
		peers,
		ctx,
	};
}

/** Alice opens and ends a scene — the table won't offer a plan until one has run. */
export async function runScene(g: ECGame) {
	const sceneRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/scenes`, {
		data: { location_custom: 'The Orangery', time_elapsed: 'days', present_peer_ids: [] },
	});
	expect(sceneRes.ok(), `scene failed: ${await sceneRes.text()}`).toBeTruthy();
	const endRes = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/end-scene`);
	expect(endRes.ok(), `end-scene failed: ${await endRes.text()}`).toBeTruthy();
}

/** Alice prepares Exchange Courtiers aimed at bob's peer. Returns the plan id. */
export async function prepareEC(g: ECGame): Promise<number> {
	const res = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: {
			plan_type: 'exchange_courtiers',
			target_player_id: g.ids.bob,
			target_asset_id: g.peers.bob,
			preparation_notes: 'I want your man in my household.',
		},
	});
	expect(res.ok(), `prepare failed: ${await res.text()}`).toBeTruthy();
	return (await res.json()).plan.id as number;
}

/** Fast-forward the record to the plan's row; the plan opens as resolving. */
export async function resolveEC(g: ECGame, planID: number) {
	const res = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: planID } });
	expect(res.ok(), `advance-row failed: ${await res.text()}`).toBeTruthy();
}

export async function openAll(g: ECGame): Promise<Record<'alice' | 'bob' | 'carol', Page>> {
	const out = {} as Record<'alice' | 'bob' | 'carol', Page>;
	for (const n of ['alice', 'bob', 'carol'] as const) {
		out[n] = await g.ctx[n].newPage();
		await out[n].goto(`/table/${g.gameID}`);
	}
	return out;
}

export async function offerTrade(g: ECGame, planID: number) {
	const res = await g.ctx.bob.request.post(`/api/plans/${planID}/fair-trade`, {
		data: { action: 'offer', offered_asset_id: g.peers.alice },
	});
	expect(res.ok(), `offer failed: ${await res.text()}`).toBeTruthy();
}

/** Alice declines the trade; the roll is forced to `result` distinct faces. */
export async function declineAndForce(g: ECGame, planID: number, result: number) {
	const dec = await g.ctx.alice.request.post(`/api/plans/${planID}/fair-trade`, { data: { action: 'decline' } });
	expect(dec.ok(), `decline failed: ${await dec.text()}`).toBeTruthy();
	const { roll } = await dec.json();
	const force = await g.ctx.alice.request.post('/api/dev/force-roll', { data: { roll_id: roll.id, result } });
	expect(force.ok(), `force-roll failed: ${await force.text()}`).toBeTruthy();
	return roll.id as number;
}

export async function ownerOf(g: ECGame, assetID: number): Promise<number> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/assets`);
	expect(res.ok()).toBeTruthy();
	const { assets } = (await res.json()) as { assets: { id: number; owner_id: number }[] };
	const a = assets.find(x => x.id === assetID);
	expect(a, `asset ${assetID} not found`).toBeTruthy();
	return a!.owner_id;
}

export async function planStatus(g: ECGame, planID: number): Promise<string> {
	const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
	const { plans } = (await res.json()) as { plans: { id: number; status: string }[] };
	return plans.find(p => p.id === planID)!.status;
}

/**
 * Text of the plan's action-log group on the always-open desktop chat column,
 * or "" if the group isn't there yet. The group collapses once the plan resolves
 * and its entries arrive over the WebSocket, so this expands it and reads what is
 * there now, without asserting. Use expectPlanLog to wait for the entries.
 */
async function readPlanLog(page: Page, planID: number): Promise<string> {
	const group = page
		.getByRole('complementary', { name: 'Chat' })
		.locator('.plan-group')
		.filter({ has: page.locator(`.plan-header[data-plan-id="${planID}"]`) });
	const header = group.locator('.plan-header');
	if ((await header.count()) === 0) return '';
	if ((await header.getAttribute('aria-expanded')) !== 'true') await header.click();
	return group.innerText();
}

/** Index of the first snippet missing from `log`, in order, or null if all appear in order. */
function firstOutOfOrder(log: string, snippets: string[]): string | null {
	let at = -1;
	for (const sn of snippets) {
		const i = log.indexOf(sn, at + 1);
		if (i <= at) return sn;
		at = i;
	}
	return null;
}

/**
 * Waits until the plan's log group shows every snippet, in the given order.
 * Retries, since entries can trail the plan's `resolved` status.
 */
export async function expectPlanLog(page: Page, planID: number, snippets: string[]) {
	let last = '';
	await expect
		.poll(
			async () => {
				last = await readPlanLog(page, planID);
				return firstOutOfOrder(last, snippets);
			},
			{ timeout: 10_000, message: 'plan log should show the entries in order' },
		)
		.toBeNull()
		.catch(err => {
			throw new Error(`${err.message}\nlog was:\n${last}`);
		});
}
