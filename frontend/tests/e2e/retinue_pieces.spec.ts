import { test, expect, request as pwRequest, type Page, type Locator } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';

// The retinue and the game pieces, in a real browser.
//
// The unit suite (src/lib/components/retinuePieces.test.ts) checks what the
// components emit and what their stylesheets say. This reads what the engine
// actually painted on a seeded table: the warm border and piece corner on a
// card, the bright gold selected state, the orange "Leveraged" word, the
// at-risk count in the retinue, and the 44px touch floor on its controls.

const track = cleanupGameAfterEach();

/** What `var(token)` resolves to, as the browser's own colour string. */
async function resolve(page: Page, token: string): Promise<string> {
	return page.evaluate((t) => {
		const probe = document.createElement('div');
		probe.style.color = `var(${t})`;
		document.body.appendChild(probe);
		const value = getComputedStyle(probe).color;
		probe.remove();
		return value;
	}, token);
}

const paint = (el: Locator) =>
	el.evaluate((node) => {
		const s = getComputedStyle(node);
		return {
			border: s.borderTopColor,
			borderWidth: s.borderTopWidth,
			radius: s.borderTopLeftRadius,
			background: s.backgroundColor,
			color: s.color,
			shadow: s.boxShadow,
		};
	});

async function seed(players: string[]) {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const res = await fixture.post('/api/dev/seed', { data: { phase: 'main_event', players, current_row: 3 } });
	expect(res.ok(), 'seed failed').toBeTruthy();
	const body = await res.json();
	await fixture.dispose();
	track(body.game_id);
	return body as { game_id: number; players: { id: number; display_name: string }[] };
}

async function playerPage(browser: import('@playwright/test').Browser, name: string, gameID: number) {
	const ctx = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 1400 } });
	await ctx.request.post(`/api/dev/login?username=${name}`);
	const page = await ctx.newPage();
	await page.goto(`/table/${gameID}`);
	return { ctx, page };
}

async function openRetinue(page: Page, name: string) {
	await page.getByRole('button', { name: new RegExp(`^View ${name}'s retinue`) }).click();
	const sheet = page.getByRole('dialog');
	await expect(sheet.locator('.sheet-title')).toBeVisible();
	return sheet;
}

test("your retinue: pieces, the at-risk count, 'Leveraged' in words, and 44px controls", async ({ browser }) => {
	const g = await seed(['ret_alice', 'ret_bob']);
	const { ctx, page } = await playerPage(browser, 'ret_alice', g.game_id);
	try {
		// Leverage alice's holding so one tile carries the status word.
		const assets = (await (await ctx.request.get(`/api/tables/${g.game_id}/assets`)).json()) as {
			assets?: { id: number; asset_type: string; owner_id: number }[];
		};
		const list = assets.assets ?? (assets as unknown as { id: number; asset_type: string; owner_id: number }[]);
		const mine = g.players.find((p) => p.display_name === 'ret_alice')!.id;
		const holding = list.find((a) => a.owner_id === mine && a.asset_type === 'holding')!;
		expect((await ctx.request.post(`/api/assets/${holding.id}/leverage`)).ok()).toBeTruthy();

		const sheet = await openRetinue(page, 'ret_alice');
		await expect(sheet.locator('.sheet-title')).toContainText('Your Retinue');

		// The owner's shape (seat 1: the diamond) sits beside the title, once.
		await expect(sheet.locator('.sheet-title svg[data-shape="diamond"]')).toBeVisible();
		await expect(sheet.locator('[data-shape]')).toHaveCount(1);

		// A seeded asset carries one note, so all four are one tear from
		// destruction with a slot free: the count is here, in words.
		await expect(sheet).toContainText('4 assets need marginalia');

		// An asset is a game piece: the warm border and the piece corner, on the
		// raised surface.
		const tile = sheet.locator('.asset-tile').first();
		const p = await paint(tile);
		expect(p.border).toBe(await resolve(page, '--color-border-piece'));
		expect(p.borderWidth).toBe('1px');
		expect(p.radius).toBe('4px');
		expect(p.background).toBe(await resolve(page, '--color-surface'));

		// So is a marginalia tile inside it.
		const m = await paint(sheet.locator('.m-tile.filled').first());
		expect(m.border).toBe(await resolve(page, '--color-border-piece'));
		expect(m.radius).toBe('4px');

		// "Leveraged" is a word, in orange, not a lone glyph.
		const word = sheet.locator('.asset-tile', { hasText: 'Leveraged' }).locator('.lev-word').first();
		await expect(word).toHaveText(/Leveraged/);
		expect((await paint(word)).color).toBe(await resolve(page, '--color-leveraged'));

		// The type is plain words, not a chip: no box of any kind.
		const type = tile.locator('.tile-status .status-word').first();
		const t = await paint(type);
		expect(t.borderWidth).toBe('0px');
		expect(t.background).toBe('rgba(0, 0, 0, 0)');

		// Every control on the tile head meets the touch floor.
		for (const eye of await sheet.locator('.secrets-btn').all()) {
			const box = (await eye.boundingBox())!;
			expect(box.width).toBeGreaterThanOrEqual(44);
			expect(box.height).toBeGreaterThanOrEqual(44);
		}
	} finally {
		await ctx.close();
	}
});

test("someone else's retinue says who can still shore the assets up", async ({ browser }) => {
	const g = await seed(['ret_cara', 'ret_dan']);
	const { ctx, page } = await playerPage(browser, 'ret_cara', g.game_id);
	try {
		const sheet = await openRetinue(page, 'ret_dan');
		// Dan is seat 2: the triangle, and he is named in the sentence.
		await expect(sheet.locator('.sheet-title svg[data-shape="triangle"]')).toBeVisible();
		await expect(sheet).toContainText('4 assets have too few marginalia. ret_dan can still shore them up.');
		// Nothing here is theirs to edit: no add slot, no rename, no controls.
		await expect(sheet.locator('.m-tile.add')).toHaveCount(0);
		await expect(sheet.locator('button.asset-name')).toHaveCount(0);
	} finally {
		await ctx.close();
	}
});

test('a selected card is a bright gold border and a tick, with no fill, and does not move', async ({ browser }) => {
	const g = await seed(['ret_eve', 'ret_fay']);
	const eve = g.players.find((p) => p.display_name === 'ret_eve')!.id;
	const { ctx, page } = await playerPage(browser, 'ret_eve', g.game_id);
	try {
		// Eve gets two spare peers and loses her main character, which opens the
		// replacement choice: a picker of selectable cards.
		const api = ctx.request;
		for (const name of ['Captain Vell', 'Lady Merrow']) {
			const res = await api.post(`/api/tables/${g.game_id}/assets`, {
				data: { name, asset_type: 'peer', marginalia: ['Keeps the harbour ledgers.'] },
			});
			expect(res.ok(), `create ${name}`).toBeTruthy();
		}
		const all = (await (await api.get(`/api/tables/${g.game_id}/assets`)).json()) as
			| { id: number; owner_id: number; is_main_character: boolean }[]
			| { assets: { id: number; owner_id: number; is_main_character: boolean }[] };
		const list = Array.isArray(all) ? all : all.assets;
		const mc = list.find((a) => a.owner_id === eve && a.is_main_character)!;
		const fixture = await pwRequest.newContext({ baseURL: E2E });
		const fay = g.players.find((p) => p.display_name === 'ret_fay')!.id;
		expect((await fixture.post('/api/dev/take-asset', { data: { asset_id: mc.id, player_id: fay } })).ok()).toBeTruthy();
		await fixture.dispose();
		await page.reload();

		const panel = page.locator('.mc-choice');
		const card = panel.locator('.card', { hasText: 'Captain Vell' });
		await expect(card).toBeVisible();

		const before = await paint(card);
		const sizeBefore = (await card.boundingBox())!;
		expect(before.border).toBe(await resolve(page, '--color-border-piece'));
		expect(before.radius).toBe('4px');
		await expect(card.locator('.tone-mark')).toHaveCount(0);

		await card.getByRole('checkbox').click();
		await expect(card.getByRole('checkbox')).toHaveAttribute('aria-checked', 'true');

		const you = await resolve(page, '--color-you');
		const after = await paint(card);
		expect(after.border, 'selected is the bright gold border').toBe(you);
		expect(after.shadow, 'its second pixel').toContain(you);
		expect(after.background, 'selected has no fill').toBe(before.background);
		await expect(card.locator('.tone-mark')).toBeVisible();

		// The second pixel is an inset shadow, so nothing moves.
		const sizeAfter = (await card.boundingBox())!;
		expect(sizeAfter.width).toBeCloseTo(sizeBefore.width, 1);
		expect(sizeAfter.height).toBeCloseTo(sizeBefore.height, 1);

		// The card for another seat's peer wears that seat's shape, and no card
		// wears a coloured left edge: the border is the same on all four sides.
		await expect(card.locator('[data-shape="diamond"]')).toBeVisible();
		const sides = await card.evaluate((el) => {
			const s = getComputedStyle(el);
			return [s.borderTopColor, s.borderRightColor, s.borderBottomColor, s.borderLeftColor];
		});
		expect(new Set(sides).size).toBe(1);
	} finally {
		await ctx.close();
	}
});

test('conscripting a character: the preview is a piece, the name is plain text, the inputs are 16px', async ({ browser }) => {
	const g = await seed(['ret_gus', 'ret_hal']);
	const gus = g.players.find((p) => p.display_name === 'ret_gus')!.id;
	const { ctx, page } = await playerPage(browser, 'ret_gus', g.game_id);
	try {
		const all = (await (await ctx.request.get(`/api/tables/${g.game_id}/assets`)).json()) as
			| { id: number; owner_id: number; is_main_character: boolean }[]
			| { assets: { id: number; owner_id: number; is_main_character: boolean }[] };
		const list = Array.isArray(all) ? all : all.assets;
		const mc = list.find((a) => a.owner_id === gus && a.is_main_character)!;
		const hal = g.players.find((p) => p.display_name === 'ret_hal')!.id;
		const fixture = await pwRequest.newContext({ baseURL: E2E });
		expect((await fixture.post('/api/dev/take-asset', { data: { asset_id: mc.id, player_id: hal } })).ok()).toBeTruthy();
		await fixture.dispose();
		await page.reload();

		const panel = page.locator('.mc-choice');
		const preview = panel.locator('.acf-preview');
		await expect(preview).toBeVisible();

		// The preview stands for a card, so it is drawn as one.
		const p = await paint(preview);
		expect(p.border).toBe(await resolve(page, '--color-border-piece'));
		expect(p.radius).toBe('4px');

		// The name on it is just text: a line you can't press gets no box.
		const name = await paint(preview.locator('.acf-name'));
		expect(name.borderWidth).toBe('0px');

		// Inputs are 16px or iPhone Safari zooms the page when one is focused.
		for (const input of await panel.locator('.sp-input').all()) {
			expect(await input.evaluate((el) => getComputedStyle(el).fontSize)).toBe('16px');
		}

		// The reroll button is a 44px square.
		const reroll = (await panel.locator('.sp-reroll').first().boundingBox())!;
		expect(reroll.width).toBeGreaterThanOrEqual(44);
		expect(reroll.height).toBeGreaterThanOrEqual(44);
	} finally {
		await ctx.close();
	}
});
