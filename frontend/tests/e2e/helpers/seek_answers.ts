import {
	expect, request as pwRequest,
	type APIRequestContext, type Browser, type BrowserContext, type Page,
} from '@playwright/test';
import { E2E } from '../baseUrl';

// Shared setup for the Seek Answers regression specs: a three-player main event
// with alice's Seek Answers on the current row, one browser context per player
// (cookies are per context, not per page), and API shortcuts for the parts that
// are not the subject (forcing the dice, reading plan state).

export interface RollRow {
	id: number;
	result: number | null;
	outcome: 'make' | 'mar' | null;
	difficulty: number;
}
export interface AssetRow {
	id: number;
	owner_id: number;
	name: string;
	asset_type: string;
	is_destroyed: boolean;
	secret_count: number;
	marginalia: { id: number; text: string; is_torn: boolean }[];
}
export interface Player { ctx: BrowserContext; page: Page }
export interface Table {
	gameID: number;
	planID: number;
	alice: Player;
	bob: Player;
	carol: Player;
	/** Player ids by display name, from the seed response. */
	ids: Record<string, number>;
}

/** Seed a 3-player main event with alice's Seek Answers on the current row and
 *  open it for resolution. Knowledge order (best first, player indexes) defaults
 *  to alice last, so her difficulty is high enough to force a mar owing two picks;
 *  ranked first, a make is reachable. */
export async function setUp(
	browser: Browser,
	track: (gameID: number) => void,
	opened: BrowserContext[],
	knowledge = [1, 2, 0],
): Promise<Table> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: ['alice', 'bob', 'carol'],
			current_row: 5,
			rankings: { knowledge },
			plans: [{ preparer_idx: 0, plan_type: 'seek_answers', category: 'knowledge', row: 5, row_order: 0 }],
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const seeded = await seedRes.json();
	const game_id = seeded.game_id as number;
	const ids = Object.fromEntries(
		(seeded.players as { id: number; display_name: string }[]).map(p => [p.display_name, p.id]),
	);
	track(game_id);
	await fixture.dispose();

	const mk = async (name: string): Promise<Player> => {
		const ctx = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 812 } });
		opened.push(ctx);
		const login = await ctx.request.post(`/api/dev/login?username=${name}`);
		expect(login.ok(), `login ${name}: ${await login.text()}`).toBeTruthy();
		return { ctx, page: await ctx.newPage() };
	};
	const [alice, bob, carol] = [await mk('alice'), await mk('bob'), await mk('carol')];

	const plansRes = await alice.ctx.request.get(`/api/tables/${game_id}/plans`);
	const { plans } = (await plansRes.json()) as { plans: { id: number; plan_type: string }[] };
	const planID = plans.find(p => p.plan_type === 'seek_answers')!.id;
	const resolveRes = await alice.ctx.request.post(`/api/plans/${planID}/resolve`);
	expect(resolveRes.ok(), `resolve failed: ${await resolveRes.text()}`).toBeTruthy();

	return { gameID: game_id, planID, alice, bob, carol, ids };
}

export async function openTables(t: Table) {
	await Promise.all([t.alice, t.bob, t.carol].map(p => p.page.goto(`/table/${t.gameID}`)));
}

export async function activeRoll(api: APIRequestContext, gameID: number): Promise<RollRow> {
	const res = await api.get(`/api/tables/${gameID}/rolls/active`);
	expect(res.ok(), `active roll: ${await res.text()}`).toBeTruthy();
	return (await res.json()).roll as RollRow;
}

/** Settle the open roll through the real stage machine (POST /api/dev/force-roll). */
export async function forceRoll(api: APIRequestContext, rollID: number, body: { outcome: 'make' | 'mar'; result?: number }) {
	const res = await api.post('/api/dev/force-roll', { data: { roll_id: rollID, ...body } });
	expect(res.ok(), `force-roll failed: ${await res.text()}`).toBeTruthy();
}

export interface PlanBody {
	plan: { status: string; preparer_id: number };
	resolution_data?: {
		choice_submitted?: boolean;
		make_mar_choices?: { option: string }[];
		seek_answers?: {
			mar_self_flaws_required?: number;
			mar_self_flaws_applied?: number;
			flawed_resource_ids?: number[];
			revealed_asset_ids?: number[];
		};
	};
}
export async function getPlan(api: APIRequestContext, planID: number): Promise<PlanBody> {
	const res = await api.get(`/api/plans/${planID}`);
	expect(res.ok(), `GET plan: ${await res.text()}`).toBeTruthy();
	return (await res.json()) as PlanBody;
}
export async function planStatus(api: APIRequestContext, planID: number): Promise<string> {
	return (await getPlan(api, planID)).plan.status;
}

export async function listAssets(api: APIRequestContext, gameID: number): Promise<AssetRow[]> {
	const res = await api.get(`/api/tables/${gameID}/assets`);
	expect(res.ok(), `GET assets: ${await res.text()}`).toBeTruthy();
	return ((await res.json()) as { assets: AssetRow[] }).assets;
}

export async function actingIDs(api: APIRequestContext, gameID: number): Promise<{ kind: string; ids: number[] }> {
	const res = await api.get(`/api/tables/${gameID}/state`);
	expect(res.ok(), `GET state: ${await res.text()}`).toBeTruthy();
	const rs = (await res.json()).row_state as { kind: string; acting_player_ids?: number[] };
	return { kind: rs.kind, ids: rs.acting_player_ids ?? [] };
}

/** Alice types her pre-roll narration into the panel and casts; returns the roll. */
export async function castFromUI(t: Table, narration: string): Promise<RollRow> {
	const { page } = t.alice;
	await expect(page.getByText('What have you learned?')).toBeVisible({ timeout: 10_000 });
	await page.getByPlaceholder('Restate your research methods').fill(narration);
	await page.getByRole('button', { name: 'Submit', exact: true }).click();
	await expect(page.getByText('What have you learned?')).toHaveCount(0);
	return activeRoll(t.alice.ctx.request, t.gameID);
}

export const more = (page: Page, option: string) => page.getByRole('button', { name: `More ${option}` });
export const fewer = (page: Page, option: string) => page.getByRole('button', { name: `Fewer ${option}` });

export const OPT = {
	breakResource: 'Break a resource (describe a flaw)',
	declareTruth: 'Declare something true about the world',
	askQuestion: 'Ask a player a question (they must be truthful)',
	revealSecret: 'Learn the secrets of an asset',
} as const;

/** Every text a player could read about this game, as one blob, so a secret's or an
 *  answer's text can be searched for across all of them. */
export async function everythingVisibleTo(api: APIRequestContext, t: Table, assetIDs: number[]): Promise<string> {
	const urls = [
		`/api/tables/${t.gameID}/assets`,
		`/api/tables/${t.gameID}/plans`,
		`/api/plans/${t.planID}`,
		`/api/tables/${t.gameID}/secrets/visible`,
		`/api/tables/${t.gameID}/posts`,
		`/api/tables/${t.gameID}/state`,
		...assetIDs.map(id => `/api/assets/${id}/secrets`),
	];
	const parts: string[] = [];
	for (const u of urls) {
		const res = await api.get(u);
		parts.push(`${u} ${res.status()} ${await res.text()}`);
	}
	return parts.join('\n');
}
