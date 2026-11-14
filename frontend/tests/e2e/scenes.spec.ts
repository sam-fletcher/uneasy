import { test, expect, request as pwRequest, type Browser, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach, stepHeading } from './helpers';
import * as sa from './helpers/seek_answers';
import { E2E } from './baseUrl';

// The two scene screens: the form the focus player fills in to set a scene
// (and the read-only mirror of it everyone else watches), and the panel that
// sits on the Play pane while the scene is on. Neither has a box or a wash of
// its own now; the cards and buttons are the shared pieces.
//
// The state (the seed, a peer each, starting and ending a scene) goes over the
// API; what the three players see is read from the page at 375px, a phone, as
// each of them would. Scene BEHAVIOUR (who may claim a peer, the draft reaching
// a watcher, the submit gate) is unchanged by the redesign and was already
// covered by the Go suites; this pins that the screens still do it.

const track = cleanupGameAfterEach();
const opened: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of opened.splice(0)) await c.close().catch(() => {});
});

interface Seat { ctx: BrowserContext; page: Page }
interface Table {
	gameID: number;
	ids: Record<string, number>;
	alice: Seat;
	bob: Seat;
	carol: Seat;
}

/** A player's own browser context (cookies are per context), at phone width. */
async function seat(browser: Browser, name: string): Promise<Seat> {
	const ctx = await browser.newContext({ baseURL: E2E, viewport: { width: 375, height: 812 } });
	opened.push(ctx);
	const login = await ctx.request.post(`/api/dev/login?username=${name}`);
	expect(login.ok(), `login ${name}: ${await login.text()}`).toBeTruthy();
	return { ctx, page: await ctx.newPage() };
}

/**
 * A three-player main event at row 3 (so it is not the opening scene), alice
 * holding the focus marker, with a named peer each for alice and bob so there is
 * someone to claim and someone locked to their owner.
 */
async function setUp(browser: Browser): Promise<Table> {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const res = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob', 'carol'], current_row: 3 },
	});
	expect(res.ok(), `seed failed: ${await res.text()}`).toBeTruthy();
	const seeded = await res.json();
	const gameID = seeded.game_id as number;
	track(gameID);
	await fixture.dispose();
	const ids = Object.fromEntries(
		(seeded.players as { id: number; display_name: string }[]).map(p => [p.display_name, p.id]),
	);

	const [alice, bob, carol] = [await seat(browser, 'alice'), await seat(browser, 'bob'), await seat(browser, 'carol')];
	const peer = (who: Seat, name: string, note: string) =>
		who.ctx.request.post(`/api/tables/${gameID}/assets`, {
			data: { asset_type: 'peer', name, marginalia: [note] },
		});
	expect((await peer(alice, 'Captain Vell', 'Owes the crown a ship.')).ok()).toBeTruthy();
	expect((await peer(bob, 'Brother Ansel', 'Speaks for the chapter house.')).ok()).toBeTruthy();
	return { gameID, ids, alice, bob, carol };
}

const form = (page: Page) => page.locator('.scene-setup');
const details = (page: Page) => page.locator('.scene-details');

/** A box's look as the browser computes it: no fill, and no border on any side. */
async function boxless(page: Page, selector: string) {
	return page.locator(selector).first().evaluate((el) => {
		const cs = getComputedStyle(el);
		return {
			background: cs.backgroundColor,
			borders: [cs.borderTopWidth, cs.borderRightWidth, cs.borderBottomWidth, cs.borderLeftWidth],
			shadow: cs.boxShadow,
		};
	});
}
const NO_BOX = { background: 'rgba(0, 0, 0, 0)', borders: ['0px', '0px', '0px', '0px'], shadow: 'none' };

/** Every control in `locator` is at least a finger tall. */
async function allAtLeast44(controls: ReturnType<Page['locator']>) {
	const heights = await controls.evaluateAll(els => els.map(e => Math.round(e.getBoundingClientRect().height)));
	expect(heights.length).toBeGreaterThan(0);
	for (const h of heights) expect(h).toBeGreaterThanOrEqual(44);
}

test('scene setup: the focus player fills it in; a watcher sees the same form, mirrored, with nothing of theirs to press', async ({ browser }) => {
	const t = await setUp(browser);
	await Promise.all([t.alice, t.bob].map(s => s.page.goto(`/table/${t.gameID}`)));
	const { alice, bob } = t;

	// ── The step ──────────────────────────────────────────────────────────────
	await expect(stepHeading(alice.page)).toContainText('Your move', { timeout: 10_000 });
	await expect(stepHeading(alice.page)).toContainText('set the scene');
	await expect(stepHeading(bob.page)).toContainText('Waiting on alice');
	await expect(stepHeading(bob.page)).toContainText('set the scene');

	// ── Prompt and groups: small headings and plain text, no box ─────────────
	for (const p of [alice.page, bob.page]) {
		await expect(form(p).getByRole('heading', { level: 3 })).toHaveText(['Prompt', 'Where', 'When', 'Who else is here']);
		await expect(form(p).getByText('No plan prompt — set any scene.')).toBeVisible();
		expect(await boxless(p, '.scene-setup .prompt')).toEqual(NO_BOX);
		expect(await boxless(p, '.scene-setup .group')).toEqual(NO_BOX);
	}

	// ── The focus player: one main button, grey until a place and a time ─────
	const begin = alice.page.getByRole('button', { name: 'Begin Scene' });
	await expect(begin).toBeDisabled();
	await expect(bob.page.getByRole('button', { name: 'Begin Scene' })).toHaveCount(0);

	const daysAlice = alice.page.getByRole('button', { name: 'Days later' });
	await daysAlice.click();
	await expect(daysAlice).toHaveAttribute('aria-pressed', 'true');
	await expect(begin).toBeDisabled(); // a time alone is not a scene

	// Chips and fields are a finger tall, the form fits the phone, and the six
	// chips sit three to a line (the squeeze the old container query was for).
	await allAtLeast44(form(alice.page).locator('.chips .chip'));
	await allAtLeast44(form(alice.page).locator('input[type="text"]'));
	await allAtLeast44(begin);
	const tops = await form(alice.page).locator('.chips .chip').evaluateAll(els => els.map(e => Math.round(e.getBoundingClientRect().top)));
	expect(new Set(tops).size, 'six chips wrap to two lines').toBe(2);
	expect(await form(alice.page).evaluate(el => el.scrollWidth <= el.clientWidth)).toBe(true);

	const place = alice.page.getByLabel('Another location');
	await place.fill('The Orangery');
	await expect(begin).toBeEnabled();

	// ── The watcher sees it live, every control in its own disabled look ─────
	const daysBob = bob.page.getByRole('button', { name: 'Days later' });
	await expect(daysBob).toHaveAttribute('aria-pressed', 'true', { timeout: 10_000 });
	await expect(bob.page.getByLabel('Another location')).toHaveValue('The Orangery');
	await expect(bob.page.getByLabel('Another location')).toBeDisabled();
	await expect(bob.page.getByLabel('Another time')).toBeDisabled();
	for (const chip of await form(bob.page).locator('.chips .chip').all()) await expect(chip).toBeDisabled();
	for (const box of await form(bob.page).getByRole('checkbox').all()) await expect(box).toHaveAttribute('aria-disabled', 'true');
	// The form is not faded: a fade would take its faint words under the contrast floor.
	expect(await form(bob.page).evaluate(el => getComputedStyle(el).opacity)).toBe('1');

	// ── Picking a place and typing one are alternatives ──────────────────────
	// Alice's own holding is the first card under Where. Picking it clears the
	// typed place; the watcher's mirror follows.
	const aliceHolding = form(alice.page).getByRole('checkbox').first();
	await aliceHolding.click();
	await expect(aliceHolding).toHaveAttribute('aria-checked', 'true');
	await expect(place).toHaveValue('');
	await expect(bob.page.getByLabel('Another location')).toHaveValue('', { timeout: 10_000 });
	await expect(form(bob.page).getByRole('checkbox').first()).toHaveAttribute('aria-checked', 'true');

	// A time chip and a typed time are alternatives too.
	await alice.page.getByLabel('Another time').fill('Past midnight');
	await expect(daysAlice).toHaveAttribute('aria-pressed', 'false');
	await expect(bob.page.getByLabel('Another time')).toHaveValue('Past midnight', { timeout: 10_000 });
	await expect(daysBob).toHaveAttribute('aria-pressed', 'false');

	// ── The help row is the one thing a watcher can still press ──────────────
	const help = bob.page.getByRole('button', { name: 'How to set a good scene' });
	await expect(help).toBeEnabled();
	await help.click();
	await expect(help).toHaveAttribute('aria-expanded', 'true');
	await expect(bob.page.getByText('Need ideas?')).toBeVisible();

	// ── Begin: both players land on the scene ────────────────────────────────
	await begin.click();
	await expect(details(alice.page)).toBeVisible({ timeout: 10_000 });
	await expect(details(bob.page)).toBeVisible({ timeout: 10_000 });
	await expect(form(alice.page)).toHaveCount(0);
});

test('the opening scene asks for a time in words, not chips', async ({ browser }) => {
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const res = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob'], current_row: 1 },
	});
	expect(res.ok(), `seed failed: ${await res.text()}`).toBeTruthy();
	const gameID = (await res.json()).game_id as number;
	track(gameID);
	await fixture.dispose();

	const alice = await seat(browser, 'alice');
	await alice.page.goto(`/table/${gameID}`);
	await expect(form(alice.page)).toBeVisible({ timeout: 10_000 });
	await expect(form(alice.page).locator('.chips .chip')).toHaveCount(0);
	const when = alice.page.getByLabel('When the scene opens');
	await expect(when).toHaveAttribute('placeholder', 'At night? At the coronation?');

	// The note is then required, since a place alone is not enough.
	const begin = alice.page.getByRole('button', { name: 'Begin Scene' });
	await alice.page.getByLabel('Another location').fill('The throne room');
	await expect(begin).toBeDisabled();
	await when.fill('At the coronation');
	await expect(begin).toBeEnabled();
});

test('scene details: the focus player ends it; another player takes over a peer nobody holds', async ({ browser }) => {
	const t = await setUp(browser);
	const { alice, bob, carol } = t;

	// The scene: a place nobody owns, a time, alice's Captain Vell (unclaimed)
	// and bob's Brother Ansel (locked to bob, as every non-focus player's peer is).
	const assets = await sa.listAssets(alice.ctx.request, t.gameID);
	const vell = assets.find(a => a.name === 'Captain Vell')!;
	const ansel = assets.find(a => a.name === 'Brother Ansel')!;
	const started = await alice.ctx.request.post(`/api/tables/${t.gameID}/scenes`, {
		data: { location_custom: 'The Orangery', time_elapsed: 'days', present_peer_ids: [vell.id, ansel.id] },
	});
	expect(started.status(), `start scene: ${await started.text()}`).toBe(201);
	await Promise.all([alice, bob, carol].map(s => s.page.goto(`/table/${t.gameID}`)));

	// ── The scene, as alice (focus) ──────────────────────────────────────────
	await expect(stepHeading(alice.page)).toContainText('Your move', { timeout: 10_000 });
	await expect(stepHeading(alice.page)).toContainText('the scene');
	const aliceScene = details(alice.page);
	await expect(aliceScene).toBeVisible();
	await expect(aliceScene.locator('.place')).toContainText('The Orangery');
	await expect(aliceScene.locator('.time')).toHaveText('Days later');
	await expect(aliceScene.locator('.focus-line')).toContainText('alice');
	// The place wears the house mark and the focus player a shape, not an emoji.
	await expect(aliceScene.locator('.place-mark svg')).toHaveCount(1);
	await expect(aliceScene.locator('.focus-line svg')).toHaveCount(1);
	await expect(alice.page.locator('body')).not.toContainText('📍');
	// No box of its own: the only boxes are the cards and the button.
	expect(await boxless(alice.page, '.scene-details')).toEqual(NO_BOX);
	expect(await boxless(alice.page, '.scene-details .scene-header')).toEqual(NO_BOX);
	// The scene's own labels, and Captain Vell is open to be played (alice cannot take over her own).
	await expect(aliceScene.getByRole('heading', { level: 3 })).toHaveText(['Present']);
	await expect(aliceScene.getByText('Unclaimed — open to be played')).toBeVisible();
	await expect(aliceScene.getByRole('button', { name: 'Take over' })).toHaveCount(0);
	// One main button, and it ends the scene.
	const end = aliceScene.getByRole('button', { name: 'End Scene' });
	await expect(end).toBeVisible();
	await allAtLeast44(end);
	await expect(aliceScene.locator('.action-btn.primary')).toHaveCount(1);

	// ── As bob (a player in the scene) and carol (a bystander): the same scene ─
	for (const s of [bob, carol]) {
		await expect(stepHeading(s.page)).toContainText('Waiting on alice');
		await expect(details(s.page).locator('.place')).toContainText('The Orangery');
		await expect(details(s.page).getByRole('button', { name: 'End Scene' })).toHaveCount(0);
		await expect(details(s.page).locator('.action-btn.primary')).toHaveCount(0);
		// "Take over" is offered for the one peer nobody holds, and only that one.
		const take = details(s.page).getByRole('button', { name: 'Take over' });
		await expect(take).toHaveCount(1);
		await allAtLeast44(take);
	}
	await expect(details(bob.page).getByText('Played by you')).toBeVisible(); // his own Brother Ansel

	// ── Bob takes Captain Vell over ──────────────────────────────────────────
	await details(bob.page).getByRole('button', { name: 'Take over' }).click();
	await expect(details(bob.page).getByRole('button', { name: 'Take over' })).toHaveCount(0, { timeout: 10_000 });
	await expect(details(bob.page).getByText('Played by you')).toHaveCount(2);
	// Everyone sees who plays him, and carol no longer has anything to take.
	await expect(details(carol.page).getByText('Played by bob')).toHaveCount(2, { timeout: 10_000 });
	await expect(details(carol.page).getByRole('button', { name: 'Take over' })).toHaveCount(0);
	await expect(details(alice.page).getByText('Played by bob')).toHaveCount(2);

	// ── Alice ends the scene: the panel goes for everyone ────────────────────
	await end.click();
	for (const s of [alice, bob, carol]) await expect(details(s.page)).toHaveCount(0, { timeout: 10_000 });
});

test('the follow-up scene after a plan carries the plan\'s prompt, on the form and on the scene', async ({ browser }) => {
	// Alice's Seek Answers, played out over the API: ranked first on knowledge, so
	// the difficulty is low and a make is reachable; the forced roll makes it.
	const opened2: BrowserContext[] = [];
	const t = await sa.setUp(browser, track, opened2, [0, 1, 2]);
	try {
		const api = t.alice.ctx.request;
		const cast = await api.post(`/api/plans/${t.planID}/seek-cast-roll`, {
			data: { narration: 'I read the ledgers by candlelight.' },
		});
		expect(cast.ok(), `cast: ${await cast.text()}`).toBeTruthy();
		const roll = (await cast.json()).roll as sa.RollRow;
		await sa.forceRoll(api, roll.id, { outcome: 'make' });
		const resolved = await sa.activeRoll(api, t.gameID).catch(() => null);
		const picks = resolved?.result ?? roll.difficulty;
		const choice = await api.post(`/api/plans/${t.planID}/make-choice`, {
			data: { result: 'make', choices: Array(picks).fill('declare_truth') },
		});
		expect(choice.ok(), `make-choice: ${await choice.text()}`).toBeTruthy();
		for (let i = 0; i < picks; i++) {
			const truth = await api.post(`/api/plans/${t.planID}/declare-truth`, { data: { text: `A truth, ${i + 1}.` } });
			expect(truth.ok(), `declare-truth: ${await truth.text()}`).toBeTruthy();
		}
		expect(await sa.planStatus(api, t.planID)).toBe('resolved');

		await sa.openTables(t);
		const prompt = 'Set a scene of your character learning something new.';

		// The form, as alice and as a watcher: the plan's prompt, as plain text.
		for (const s of [t.alice, t.bob]) {
			await expect(form(s.page)).toBeVisible({ timeout: 10_000 });
			await expect(form(s.page).locator('.prompt')).toHaveText(prompt);
			expect(await boxless(s.page, '.scene-setup .prompt')).toEqual(NO_BOX);
		}

		// The scene, once started: the same prompt, under the same small heading.
		const started = await api.post(`/api/tables/${t.gameID}/scenes`, {
			data: { location_custom: 'The harbour office at dusk', time_elapsed: 'hours', present_peer_ids: [] },
		});
		expect(started.status(), `start scene: ${await started.text()}`).toBe(201);
		for (const s of [t.alice, t.bob]) {
			await expect(details(s.page)).toBeVisible({ timeout: 10_000 });
			await expect(details(s.page).getByRole('heading', { level: 3 }).first()).toHaveText('Prompt');
			await expect(details(s.page).locator('.prompt')).toHaveText(prompt);
			expect(await boxless(s.page, '.scene-details .prompt')).toEqual(NO_BOX);
		}
	} finally {
		for (const c of opened2) await c.close().catch(() => {});
	}
});
