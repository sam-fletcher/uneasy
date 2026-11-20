import { test, expect } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	NOTES, SECRET_MARKER, ALICE_SECRET, BOB_SECRET, ALICE_UNDERSIDE, BOB_UNDERSIDE, closeContextsAfterEach, seedLiaise,
	readableSecrets, openAll, pickCard, section, meetingBox, post, logBodies, liaiseData,
	type Who,
} from './helpers/clandestinely_liaise';

// Clandestinely Liaise — Secrets We Keep: each player writes the meeting on the
// underside of their own asset. The words are a SECRET, so the whole point of these
// specs is where they do not go: a live draft is relayed to every player as it is
// typed, so the box must never feed one, and nothing the server later says (a
// broadcast, the log, the plan) may carry the words either. Existence is public:
// which asset holds a secret, and who wrote it.
//
// Setup (assets, the delay reveal, resolving, advancing) runs over the API
// (helpers/clandestinely_liaise.ts); what the players do in the step is driven
// through the panel, at 375px, as alice (preparer), bob (partner) and carol (bystander).

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const PHONE = { width: 375, height: 812 };

test('the meeting is written, not defaulted: both boxes start empty, the preparer sees her note above hers, and the words reach no one else', async ({ browser }) => {
	const g = await seedLiaise(browser, 'keep', PHONE);
	track(g.gameID);

	// Everything each other page's socket is sent, from its first frame, so a leak by
	// broadcast is caught as well as one in a response — including the live drafts a
	// typing player would feed.
	const frames: Record<Who, string[]> = { alice: [], bob: [], carol: [] };
	const pages = await openAll(g, (who, page) => {
		page.on('websocket', ws => ws.on('framereceived', f => frames[who].push(String(f.payload))));
	});
	const { alice, bob, carol } = pages;
	const aliceSec = section(alice, 'Secrets we keep');
	const bobSec = section(bob, 'Secrets we keep');
	await expect(aliceSec).toBeVisible({ timeout: 10_000 });
	await expect(bobSec).toBeVisible({ timeout: 10_000 });
	const keep = (sec: typeof aliceSec) => sec.getByRole('button', { name: 'Keep this secret' });

	// Both boxes start EMPTY. The preparer's note is public, so prefilling it would let one
	// tap keep a "secret" the table has already read; it sits above her box as a reminder.
	// The partner wrote no note, so she sees no reminder.
	await expect(meetingBox(aliceSec)).toHaveValue('');
	await expect(aliceSec.getByText(`Your preparation note: “${NOTES}”`)).toBeVisible();
	await expect(meetingBox(bobSec)).toHaveValue('');
	await expect(bobSec.getByText('preparation note')).toHaveCount(0);
	// Each is told the line that goes in front: the other player and the peer they brought.
	await expect(aliceSec.getByText("“Met bob's Bob Confidant —” goes in front of what you write")).toBeVisible();
	await expect(bobSec.getByText("“Met alice's Alice Confidant —” goes in front of what you write")).toBeVisible();
	// What the table can and cannot read, said where the player writes.
	await expect(aliceSec.getByText('Only you can read it, unless someone looks at, takes or breaks this asset. The table sees which asset holds a secret, never what it says.')).toBeVisible();

	// Nothing to keep until both halves are there: an asset AND words.
	await expect(keep(aliceSec)).toBeDisabled();
	await pickCard(aliceSec, 'Alice Confidant');
	await expect(keep(aliceSec), 'an asset alone is not enough, even for the preparer').toBeDisabled();
	await expect(keep(bobSec)).toBeDisabled();
	await pickCard(bobSec, 'Bob Confidant');
	await expect(keep(bobSec), 'a blank box cannot be kept').toBeDisabled();
	await meetingBox(aliceSec).fill('   ');
	await expect(keep(aliceSec), 'whitespace is blank').toBeDisabled();

	await alice.screenshot({ path: test.info().outputPath('alice-before-writing.png') });
	await bob.screenshot({ path: test.info().outputPath('bob-before-writing.png') });
	await carol.screenshot({ path: test.info().outputPath('carol-bystander.png') });

	// Alice writes the meeting. Wait past the 150ms a draft would be sent after the last
	// change, then look at what the other two sockets were sent while she typed.
	await meetingBox(aliceSec).fill(ALICE_SECRET);
	await expect(keep(aliceSec)).toBeEnabled();
	await alice.waitForTimeout(800);
	for (const who of ['bob', 'carol'] as const) {
		expect(frames[who].join('\n'), `${who}'s socket, while alice types`).not.toContain(SECRET_MARKER);
		expect(frames[who].some(f => f.includes('"prepare_plan.draft"')), `a draft reached ${who}`).toBe(false);
	}

	await keep(aliceSec).click();
	await expect(alice.getByText("You've committed your secret. Waiting for bob")).toBeVisible();
	await expect
		.poll(() => frames.carol.some(f => f.includes('liaise.keep_secret_submitted')), { message: 'carol heard of the pick live' })
		.toBe(true);

	// Bob writes, and the phase moves on.
	await meetingBox(bobSec).fill(BOB_SECRET);
	await bob.waitForTimeout(800);
	for (const who of ['alice', 'carol'] as const) {
		expect(frames[who].join('\n'), `${who}'s socket, while bob types`).not.toContain(SECRET_MARKER);
	}
	await keep(bobSec).click();
	await expect(section(alice, 'Things we share')).toBeVisible({ timeout: 10_000 });

	// Each secret is what its author wrote behind the line naming who they met; only its
	// author reads it.
	expect(await readableSecrets(g, 'alice', g.peers.alice), 'alice reads her own').toEqual([ALICE_UNDERSIDE]);
	expect(await readableSecrets(g, 'bob', g.peers.bob), 'bob reads his own').toEqual([BOB_UNDERSIDE]);
	expect(await readableSecrets(g, 'bob', g.peers.alice), "bob cannot read alice's").toEqual([]);
	expect(await readableSecrets(g, 'alice', g.peers.bob), "alice cannot read bob's").toEqual([]);
	expect(await readableSecrets(g, 'carol', g.peers.alice), 'the bystander reads nothing').toEqual([]);
	expect(await readableSecrets(g, 'carol', g.peers.bob), 'the bystander reads nothing').toEqual([]);

	// And no one was sent the words, before or after: not a draft, not a broadcast, not
	// the log, not the plan, and no page shows them. Both secrets carry SECRET_MARKER,
	// so one check covers every socket, including each author's own.
	for (const who of ['alice', 'bob', 'carol'] as const) {
		expect(frames[who].join('\n'), `${who}'s socket`).not.toContain(SECRET_MARKER);
	}
	for (const who of ['alice', 'bob', 'carol'] as const) {
		expect((await logBodies(g, who)).join('\n'), `the log, as ${who} reads it`).not.toContain(SECRET_MARKER);
		expect(JSON.stringify(await liaiseData(g, who)), `the plan, as ${who} reads it`).not.toContain(SECRET_MARKER);
	}
	await expect(carol.locator('body')).not.toContainText(SECRET_MARKER);
	await expect(bob.locator('body')).not.toContainText(ALICE_SECRET);
});

test('a tab from before the text box still finishes the liaison, and the prep note it gets sits under the same header', async ({ browser }) => {
	const g = await seedLiaise(browser, 'keep');
	track(g.gameID);
	const url = `/api/plans/${g.planID}/keep-secret`;

	// A blank box is refused, and refused whole: nothing is kept, the seat stays open.
	await post(g, 'alice', url, { asset_id: g.peers.alice, text: '' }, 400);
	await post(g, 'alice', url, { asset_id: g.peers.alice, text: '   ' }, 400);
	expect((await liaiseData(g)).kept_secrets ?? [], 'a refused box kept nothing').toEqual([]);
	expect(await readableSecrets(g, 'alice', g.peers.alice)).toEqual([]);

	// Old tabs send an asset and no text at all.
	await post(g, 'alice', url, { asset_id: g.peers.alice }, 200);
	await post(g, 'bob', url, { asset_id: g.peers.bob }, 200);

	// The prep note stands in for the words, under the same header as a written secret: the
	// other player and the peer they brought, never the holder.
	expect(await readableSecrets(g, 'alice', g.peers.alice)).toEqual([`Met bob's Bob Confidant — ${NOTES}`]);
	expect(await readableSecrets(g, 'bob', g.peers.bob)).toEqual([`Met alice's Alice Confidant — ${NOTES}`]);
	expect((await liaiseData(g)).phase, 'both seats in: the liaison moves on').toBe('things_we_share');
});

test("a refetch of the plan while a player is typing never wipes the box", async ({ browser }) => {
	const g = await seedLiaise(browser, 'keep', PHONE);
	track(g.gameID);
	const { alice } = await openAll(g);
	const aliceSec = section(alice, 'Secrets we keep');
	await expect(aliceSec).toBeVisible({ timeout: 10_000 });

	// Alice ticks her asset and is half-way through writing the meeting.
	const halfWritten = 'We met in the orangery, and she was already';
	await pickCard(aliceSec, 'Alice Confidant');
	await meetingBox(aliceSec).fill(halfWritten);

	// Bob keeps his secret. The server tells every client, and alice's page fetches the plan
	// again and swaps in a new plan object, as it does on every liaison event.
	const refetched = alice.waitForResponse(r =>
		r.request().method() === 'GET' && r.url().endsWith(`/api/plans/${g.planID}`));
	await post(g, 'bob', `/api/plans/${g.planID}/keep-secret`, { asset_id: g.peers.bob, text: BOB_SECRET }, 200);
	await refetched;
	await alice.waitForTimeout(500); // the new plan reaches the panel

	// Her words and her tick survive it, and the note reminder is still there.
	await expect(meetingBox(aliceSec)).toHaveValue(halfWritten);
	await expect(aliceSec.getByText(`Your preparation note: “${NOTES}”`)).toBeVisible();
	await expect(aliceSec.locator('.card', { hasText: 'Alice Confidant' }).getByRole('checkbox')).toBeChecked();
	await expect(aliceSec.getByRole('button', { name: 'Keep this secret' })).toBeEnabled();
});
