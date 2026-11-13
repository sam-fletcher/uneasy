import { test, expect, type BrowserContext, type Locator } from '@playwright/test';
import { cleanupGameAfterEach, stepHeading, expectWaitingOn } from './helpers';
import * as sa from './helpers/seek_answers';

// The Play pane reads as a plan's steps, in order: finished steps are one line
// each, the roll is one of the steps, and the open step's heading ("Your move —
// …" / "Waiting on otto — …") is the one place the game says what it asks of
// whom. This replaced the Waiting On bar and the roll panel that sat below the
// plan.
//
// Seek Answers is the subject only because it has a pre-roll step, a roll and
// a step after the roll, so every kind of step appears. The setup (seeding,
// casting, committing dice) goes over the API; the order and the words are read
// from the page, as each of the three players would.

const track = cleanupGameAfterEach();
const opened: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of opened.splice(0)) await c.close().catch(() => {});
});

/** `a` is drawn above `b` on the page. The whole point of the step order. */
async function above(a: Locator, b: Locator) {
	const [ba, bb] = [await a.boundingBox(), await b.boundingBox()];
	expect(ba, 'the upper element is on screen').not.toBeNull();
	expect(bb, 'the lower element is on screen').not.toBeNull();
	expect(ba!.y, 'upper element sits above the lower one').toBeLessThan(bb!.y);
}

test('the play area reads as the plan\'s steps, in order, under one heading', async ({ browser }) => {
	// Alice ranked first on knowledge keeps the difficulty low enough for a make to be reachable.
	const t = await sa.setUp(browser, track, opened, [0, 1, 2]);
	await sa.openTables(t);
	const { alice, bob, carol } = t;

	// ── Pre-roll: a finished line for the preparing, then the open step ───────
	// The preparer is asked; the others are told whose move it is.
	await expect(stepHeading(alice.page)).toContainText('Your move', { timeout: 10_000 });
	await expect(stepHeading(bob.page)).toContainText('Waiting on alice');
	await expect(alice.page.getByText('alice prepared Seek Answers')).toBeVisible();
	await above(alice.page.getByText('alice prepared Seek Answers'), stepHeading(alice.page));
	// The heading is the ONLY place the ask is stated in words: there is no bar.
	await expect(alice.page.getByText('Waiting On:')).toHaveCount(0);
	await expect(alice.page.locator('.waiting-on-bar')).toHaveCount(0);

	// ── The roll opens: it is a step, between the finished line and the plan's body
	const roll = await sa.castFromUI(t, 'I have learned the holdfast sits on an old smugglers road.');
	const region = (p: typeof alice.page) => p.getByRole('region', { name: 'Dice roll' });
	await expect(region(alice.page)).toBeVisible();
	await above(alice.page.getByText('alice prepared Seek Answers'), region(alice.page));
	// The plan's own body ("What can happen?") comes AFTER the roll, not before it.
	await above(region(alice.page), alice.page.getByRole('button', { name: 'What can happen?' }));
	// The roll's heading is the heading: exactly one on screen.
	await expect(alice.page.locator('.step-heading')).toHaveCount(1);
	await expect(stepHeading(alice.page)).toContainText('call a difficulty vote?');
	await expect(stepHeading(bob.page)).toContainText('Waiting on alice');
	// No emoji die: the house mark draws it.
	await expect(alice.page.locator('body')).not.toContainText('🎲');

	// ── Aid and interfere: the dice split into "For" and "Against" ────────────
	await alice.page.getByRole('button', { name: 'Skip vote' }).click();
	await expect(stepHeading(bob.page)).toContainText('aid or interfere', { timeout: 10_000 });
	await expect(region(bob.page).getByText('Against', { exact: true })).toHaveCount(0); // none yet

	const assets = await sa.listAssets(bob.ctx.request, t.gameID);
	const bobsHolding = assets.find(a => a.owner_id === t.ids.bob && a.asset_type === 'holding')!;
	const carolsHolding = assets.find(a => a.owner_id === t.ids.carol && a.asset_type === 'holding')!;
	const post = (who: typeof alice, path: string, data: object) =>
		who.ctx.request.post(`/api/rolls/${roll.id}/${path}`, { data });
	expect((await post(bob, 'intent', { intent: 'interfere' })).ok()).toBeTruthy();
	expect((await post(bob, 'leverage', { asset_id: bobsHolding.id })).ok()).toBeTruthy();
	expect((await post(carol, 'intent', { intent: 'aid' })).ok()).toBeTruthy();
	expect((await post(carol, 'leverage', { asset_id: carolsHolding.id })).ok()).toBeTruthy();

	for (const p of [alice.page, bob.page, carol.page]) {
		await expect(region(p).getByText('For', { exact: true })).toBeVisible();
		await expect(region(p).getByText('Against', { exact: true })).toBeVisible();
		// Three dice on the "For" side (alice's two and carol's), one against: each die is an image
		// named for its owner, and the edge is the owner's colour (read from the style, not guessed).
		await expect(region(p).getByRole('img', { name: /interfering/ })).toHaveCount(1);
	}
	// The interfering die's edge is bob's colour, and carol's aiding die wears hers.
	const edge = (p: typeof alice.page, who: RegExp) =>
		region(p).getByRole('img', { name: who }).first().evaluate(el => getComputedStyle(el).borderTopColor);
	const [bobEdge, carolEdge] = [
		await edge(alice.page, /bob's die, interfering/),
		await edge(alice.page, /carol's die/),
	];
	expect(bobEdge, 'two players, two colours').not.toBe(carolEdge);

	// ── The roll resolves: it collapses to one line ABOVE the step that acts on it
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'make' });
	for (const p of [alice.page, bob.page, carol.page]) {
		await expect(region(p)).toHaveCount(0);
		await expect(p.getByText(/^Rolled/)).toBeVisible({ timeout: 10_000 });
	}
	const line = alice.page.getByRole('button', { name: /Rolled/ });
	await above(line, stepHeading(alice.page));
	await expect(stepHeading(alice.page)).toContainText('Your move');
	await expect(stepHeading(bob.page)).toContainText('Waiting on alice');

	// The line opens in place to show the dice that were rolled, split as before.
	await expect(line).toHaveAttribute('aria-expanded', 'false');
	await line.click();
	await expect(line).toHaveAttribute('aria-expanded', 'true');
	await expect(alice.page.getByText('Against', { exact: true })).toBeVisible();

	// And the clocks say the same as the heading: it is alice the game waits on.
	await expectWaitingOn(alice.page, ['alice'], 'alice');
	await expectWaitingOn(bob.page, ['alice'], 'bob');
});

// The roll is drawn inside the plan's card, but a plan's panel is a lazy chunk. If the
// roll is already open when a page loads (a reload mid-roll, a player arriving late),
// the page has the roll before it has the card, draws it itself, and moves it into the
// card when the card arrives. That move must not lose what the player has picked, and
// the screen must not go without its heading while it waits: the page draws the heading
// by default, and the roll (then the card) claims it.
test('a roll open before its plan panel loads keeps the draft when it moves into the card', async ({ browser }) => {
	const t = await sa.setUp(browser, track, opened, [0, 1, 2]);
	const { alice, bob } = t;

	// The roll is open and past the vote, over the API, before bob's page loads.
	const cast = await alice.ctx.request.post(`/api/plans/${t.planID}/seek-cast-roll`, {
		data: { narration: 'I have learned the holdfast sits on an old smugglers road.' },
	});
	expect(cast.ok(), await cast.text()).toBeTruthy();
	const { roll } = await cast.json();
	expect((await alice.ctx.request.post(`/api/rolls/${roll.id}/skip-vote`)).ok()).toBeTruthy();

	// Hold back the script that holds Seek Answers' panel (found by its words, since chunk
	// names are hashes) until released, so bob's page has the roll before it has the card.
	let release!: () => void;
	const gate = new Promise<void>(r => (release = r));
	await bob.page.route('**/_app/immutable/**/*.js', async route => {
		const res = await route.fetch();
		const body = await res.text();
		if (body.includes('What have you learned?')) await gate;
		await route.fulfill({ response: res, body });
	});
	await bob.page.goto(`/table/${t.gameID}`);

	// Before the card: the roll draws itself, with the one heading.
	const region = bob.page.getByRole('region', { name: 'Dice roll' });
	await expect(region).toBeVisible({ timeout: 10_000 });
	await expect(bob.page.getByText('alice prepared Seek Answers')).toHaveCount(0);
	await expect(bob.page.locator('.step-heading')).toHaveCount(1);
	await expect(stepHeading(bob.page)).toContainText('aid or interfere');

	// Bob drafts a side.
	const interfere = region.getByRole('button', { name: 'Interfere', exact: true });
	await interfere.click();
	await expect(interfere).toHaveAttribute('aria-pressed', 'true');

	// The card arrives, and the roll moves into it between the finished lines and the body.
	release();
	await expect(bob.page.getByText('alice prepared Seek Answers')).toBeVisible({ timeout: 10_000 });
	await expect(region).toBeVisible();
	await above(bob.page.getByText('alice prepared Seek Answers'), region);
	// What bob picked is still picked, and there is still exactly one heading.
	await expect(region.getByRole('button', { name: 'Interfere', exact: true })).toHaveAttribute('aria-pressed', 'true');
	await expect(bob.page.locator('.step-heading')).toHaveCount(1);
});
