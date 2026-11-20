import { test, expect } from './fixtures';
import { cleanupGameAfterEach } from './helpers';
import { seedWarGame, runScene, closeContextsAfterEach } from './helpers/make_war';

// Spread Rumors preparation: a rumor's text is never streamed to the table as it is typed.
// While the preparer fills in the form, every other player's read-only mirror is fed a live
// draft over the socket, so the draft is as public as anything the server says. The rumor can
// be kept secret, and the box that says so can be ticked after the text is typed, by which
// time every watcher would already have read it. Showing words as they are typed is the least
// important thing a watcher's screen does, so the draft carries the target, the secret flag
// and the holder, and never the text. The text reaches the table when the plan is prepared.
//
// Driven on a phone, as alice (preparing) and carol (watching), by what carol's own socket is
// sent. The server also strips the text for a tab still running an older client
// (hub/prepare_plan_draft_test.go); this spec drives the current client end to end.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const PHONE = { width: 375, height: 1500 };
const RUMOR = 'The Duchess waters her own wine with hemlock.';
const MORE = ' She keeps the cellar key on a chain.';

test("a rumor's text is never sent to a watcher: typed first, ticked after, or unticked again", async ({ browser }) => {
	const g = await seedWarGame(browser, PHONE);
	track(g.gameID);
	await runScene(g);

	// Open the pages by hand: carol's socket must be listened to from its first frame.
	const alice = await g.ctx.alice.newPage();
	const carol = await g.ctx.carol.newPage();
	const drafts: string[] = [];
	carol.on('websocket', ws => ws.on('framereceived', f => {
		const s = typeof f.payload === 'string' ? f.payload : f.payload.toString();
		if (s.includes('"prepare_plan.draft"')) drafts.push(s);
	}));
	await alice.goto(`/table/${g.gameID}`);
	await carol.goto(`/table/${g.gameID}`);

	await alice.getByText('Spread Rumors', { exact: true }).click();
	await alice.getByRole('button', { name: 'bob', exact: true }).first().click();

	const aliceBox = alice.getByRole('textbox', { name: 'Rumor:' });
	const carolBox = carol.getByRole('textbox', { name: 'Rumor:' });
	const writing = carol.getByText('alice is writing the rumor.');
	const keptSecret = carol.getByText('Kept secret for now. Only alice can read it.');

	// Typing an open rumor: carol is told it is being written, and is given no box to read it in.
	await aliceBox.fill(RUMOR);
	await alice.waitForTimeout(800); // a draft is sent 150ms after the last change
	await expect(writing).toBeVisible({ timeout: 10_000 });
	await expect(carolBox).toHaveCount(0);

	// Ticking the box after typing: the line changes, and the text was never there to wipe.
	await alice.getByText('Keep it secret for now').click();
	await expect(keptSecret).toBeVisible({ timeout: 10_000 });
	await expect(writing).toHaveCount(0);

	// More typing while ticked, then unticking again: the text is still not sent.
	await aliceBox.fill(RUMOR + MORE);
	await alice.waitForTimeout(800);
	await alice.getByText('Keep it secret for now').click();
	await expect(writing).toBeVisible({ timeout: 10_000 });
	await expect(keptSecret).toHaveCount(0);
	await expect(carolBox).toHaveCount(0);
	await alice.waitForTimeout(800);

	// Every draft carol was sent, in every state, is free of the rumor. The flag still flows.
	expect(drafts.some(f => f.includes('"keep_secret":true')), 'a draft saying the rumor is secret').toBe(true);
	expect(drafts.some(f => f.includes('"keep_secret":false')), 'a draft saying it is not').toBe(true);
	for (const frame of drafts) {
		expect(frame).not.toContain('hemlock');
		expect(frame).not.toContain('cellar');
		expect(frame).not.toContain('"notes"');
	}
});
