import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

// Regression cover for the tone-tile write path ($lib/toneWrites.ts).
//
// A tile used to repaint instantly on tap and then visibly *replay* the
// cycle, one step per round trip: writes were chained per topic, and the
// server broadcasts tone.updated to every client including the sender, so
// each in-flight PUT's echo repainted the tile with a status the player had
// already moved past. The unit tests pin the coalescer and the echo filter
// in isolation; only an end-to-end run covers the wiring between them —
// +page.svelte's context, ws-handlers' dispatch, and a live WebSocket.
//
// The assertion that matters is the *sequence* of statuses the tile takes
// on, not just where it lands: the old code reached the right final value
// too, several hundred milliseconds late and after walking backwards.

const track = cleanupGameAfterEach();

/** Records every data-status the first tone tile takes on, in order. */
async function watchFirstTile(page: Page): Promise<void> {
	await page.evaluate(() => {
		const tile = document.querySelector('.tone-tile');
		if (!tile) throw new Error('no tone tile rendered');
		const seen: string[] = [tile.getAttribute('data-status') ?? ''];
		(window as unknown as { __toneSeen: string[] }).__toneSeen = seen;
		new MutationObserver(() => {
			const now = tile.getAttribute('data-status') ?? '';
			if (now !== seen[seen.length - 1]) seen.push(now);
		}).observe(tile, { attributes: true, attributeFilter: ['data-status'] });
	});
}

function tileHistory(page: Page): Promise<string[]> {
	return page.evaluate(() => (window as unknown as { __toneSeen: string[] }).__toneSeen);
}

test('rapid tone taps coalesce and never replay the cycle', async ({ browser }) => {
	const ctx = await browser.newContext({ baseURL: 'http://localhost:8090' });
	await ctx.request.post('/api/dev/login?username=alice');

	// A real table, not a seeded board: tone topics are seeded at table
	// creation, and tones are only editable in lobby/prologue.
	const { game } = await (await ctx.request.post('/api/tables')).json();
	track(game.id);

	const page = await ctx.newPage();
	const puts: string[] = [];
	page.on('request', (req) => {
		if (req.method() === 'PUT' && /\/tone\/\d+$/.test(req.url())) {
			puts.push(JSON.parse(req.postData() ?? '{}').status);
		}
	});

	// Hold each tone PUT open for roughly a production round trip. Against a
	// local server the endpoint answers in a few milliseconds — faster than
	// the taps arrive — so nothing would ever be in flight to coalesce or to
	// echo late, and the test would pass without exercising either path. The
	// bug this spec covers only exists at real latency, so the spec supplies
	// it rather than depending on the machine it runs on.
	await page.route(/\/tone\/\d+$/, async (route) => {
		if (route.request().method() !== 'PUT') return route.fallback();
		await new Promise((r) => setTimeout(r, 400));
		await route.continue();
	});

	await page.goto(`/table/${game.id}`);
	await page.getByRole('button', { name: 'Open tones' }).click();

	const tile = page.locator('.tone-tile').first();
	await expect(tile).toHaveAttribute('data-status', 'default');
	await watchFirstTile(page);

	// Three taps as fast as the browser will dispatch them — the burst that
	// used to produce three chained PUTs and three backwards repaints.
	await tile.click();
	await tile.click();
	await tile.click();

	// The tile is on the third step immediately, before any response lands.
	await expect(tile).toHaveAttribute('data-status', 'never');

	// Let every delayed response and its echo arrive. This is the window in
	// which the old code replayed the cycle.
	await page.waitForTimeout(2000);
	await expect(tile).toHaveAttribute('data-status', 'never');

	// The tile only ever moved forwards through the cycle it was tapped
	// through — no step repeated, nothing walked back. Under the old chained
	// writes this read default, include, avoid_detail, never, include,
	// avoid_detail, never: the replay, spelled out.
	expect(await tileHistory(page)).toEqual(['default', 'include', 'avoid_detail', 'never']);

	// Coalescing: three taps while the wire was busy produce two writes —
	// the one already in flight, then the value the player stopped on.
	// 'avoid_detail' is passed through and never sent.
	expect(puts).toEqual(['include', 'never']);

	// The server agrees with the tile.
	const { topics } = await (await ctx.request.get(`/api/tables/${game.id}/tone`)).json();
	expect(topics[0].status).toBe('never');

	await ctx.close();
});

test('another player\'s tone edit still lands live', async ({ browser }) => {
	// The echo filter drops broadcasts for topics this client has in flight.
	// This is the other side of that: with nothing in flight, a peer's edit
	// must still repaint the tile through the WebSocket alone.
	const aliceCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
	const bobCtx = await browser.newContext({ baseURL: 'http://localhost:8090' });
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');

	const { game } = await (await aliceCtx.request.post('/api/tables')).json();
	track(game.id);
	await bobCtx.request.post('/api/tables/join', { data: { join_code: game.join_code } });

	const alicePage = await aliceCtx.newPage();
	await alicePage.goto(`/table/${game.id}`);
	await alicePage.getByRole('button', { name: 'Open tones' }).click();

	const tile = alicePage.locator('.tone-tile').first();
	await expect(tile).toHaveAttribute('data-status', 'default');

	const { topics } = await (await bobCtx.request.get(`/api/tables/${game.id}/tone`)).json();
	await bobCtx.request.put(`/api/tables/${game.id}/tone/${topics[0].id}`, {
		data: { status: 'avoid_detail' },
	});

	await expect(tile).toHaveAttribute('data-status', 'avoid_detail');

	await aliceCtx.close();
	await bobCtx.close();
});
