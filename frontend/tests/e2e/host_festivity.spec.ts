import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedHFGame, startFestivity, openPages, act, startRoll, forceRoll, choose, rollAndChoose, optOut,
	festivity, festivityLog, expectLogLine, expectStatus, planStatus, planResult, scoreRow,
	closeContextsAfterEach, type HFGame, type Name,
} from './helpers/host_festivity';

// Regression specs for what the 12-plan UX audit and the festivity overhaul
// settled in Host Festivity:
//   · the host never rolls; every other player is a guest who rolls or opts out;
//   · a guest's mar or opt-out earns the HOST a make (the host's spoils, counted,
//     not owed to that guest) — a guest's own make does not;
//   · a guest who rolls a make holds an IOU: they can insist that the host take
//     a mar, and it is the HOST who chooses which (audit Q13);
//   · one roll-and-choice at a time, locked server-side across dice AND choice;
//   · the host ends the event through a gated end-event route, and the generic
//     "complete" path is closed;
//   · who sees what: the host's make picker and End button, a guest's roll
//     buttons and insist button, the scorecard for everyone.
//
// Setup (seed, prepare, rolls, forcing dice) goes over the API; the subject is
// what each player is shown and what the server allows. The "earns a make" log
// line's severity is an open owner question (docs/work/questions/
// minor-severity-logs.md), so the log is read over the API and only its text is
// asserted — never the severity, which the chat's default filter would hide.
//
// Alice is the host; she sits at esteem rank 4, so a guest's roll is
// difficulty 2 and /api/dev/force-roll can reach a make with two dice.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const RUMOR = 'It is said the host watered the wine.';

async function newFestivity(browser: Parameters<typeof seedHFGame>[0]) {
	const g = await seedHFGame(browser);
	track(g.gameID);
	const planID = await startFestivity(g);
	return { g, planID };
}

/** Bob rolls a make and takes the spread-a-rumor option: he now holds an IOU. */
async function bobMakes(g: HFGame, planID: number) {
	await rollAndChoose(g, 'bob', planID, 'make', { choice: 'spread_rumor', rumor_text: RUMOR });
}

const hostMakes = (page: Page, text: string) => expect(page.getByText(text)).toBeVisible();

test('the host never rolls: guests get Roll and Opt out, the host gets the make picker and End', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	const { alice, bob, carol } = await openPages(g, ['alice', 'bob', 'carol']);

	// Host: introduces the event, holds the free make earned for hosting, and
	// can't wind it down yet. No roll controls at all.
	await expect(alice.getByText('As host: Introduce the event in the chat.')).toBeVisible();
	await hostMakes(alice, '0 of 1 taken');
	await expect(alice.getByRole('button', { name: 'End the event' })).toBeDisabled();
	await expect(alice.getByRole('button', { name: 'Opt out' })).toHaveCount(0);
	await expect(alice.getByRole('button', { name: 'Roll · difficulty' })).toHaveCount(0);
	await expect(scoreRow(alice, 'alice')).toContainText('Working the room');
	await expect(scoreRow(alice, 'alice')).toContainText('0 of 1 Make taken');

	// Guests: roll at the host's standing, or opt out. Neither sees the host's controls.
	for (const guest of [bob, carol]) {
		await expect(guest.getByText('Make your entrance in chat')).toBeVisible();
		await expect(guest.getByRole('button', { name: 'Roll · difficulty 2' })).toBeEnabled();
		await expect(guest.getByRole('button', { name: 'Opt out' })).toBeEnabled();
		await expect(guest.getByText('Your extra Makes')).toHaveCount(0);
		await expect(guest.getByRole('button', { name: 'End the event' })).toHaveCount(0);
		await expect(scoreRow(guest, 'bob')).toContainText('Yet to make their mark');
	}

	// The server holds the same line: the host can neither roll nor opt out nor
	// choose as a guest, and guests can't take host makes or end the event.
	await expectStatus(await act(g, 'alice', planID, 'guest-roll', { action: 'roll' }), 403);
	await expectStatus(await act(g, 'alice', planID, 'guest-roll', { action: 'opt_out' }), 403);
	await expectStatus(await act(g, 'alice', planID, 'guest-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 403);
	await expectStatus(await act(g, 'bob', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 403);
	await expectStatus(await act(g, 'bob', planID, 'end-event'), 403);
	expect((await festivity(g, planID)).outcomes).toEqual({ [String(g.ids.alice)]: 'host' });
});

test('a guest rolls a make through the UI: the choice lands, they hold an IOU, the host gains no make', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	const { alice, bob, carol } = await openPages(g, ['alice', 'bob', 'carol']);

	await bob.getByRole('button', { name: 'Roll · difficulty 2' }).click();
	await expect.poll(async () => Object.keys((await festivity(g, planID)).guest_roll_ids ?? {})).toEqual([String(g.ids.bob)]);
	await expect(bob.getByText('Rolling… resolve the dice above.')).toBeVisible();

	// The dice land on a make; bob is offered the make list.
	const rollID = (await festivity(g, planID)).guest_roll_ids![String(g.ids.bob)];
	await forceRoll(g, rollID, 'make');
	await expect(bob.getByText('✓ Make')).toBeVisible();
	await bob.getByRole('button', { name: 'Spread a new rumor' }).click();
	await bob.getByPlaceholder('What does the rumor say?').fill(RUMOR);
	await bob.getByRole('button', { name: 'Submit choice' }).click();

	await expect.poll(async () => (await festivity(g, planID)).outcomes?.[String(g.ids.bob)]).toBe('make');
	expect((await festivity(g, planID)).guest_ious).toEqual([g.ids.bob]);

	// Everyone reads the verdict off the scorecard; bob still holds something over the host.
	for (const page of [alice, bob, carol]) {
		await expect(scoreRow(page, 'bob')).toContainText('Made a good impression');
		await expect(scoreRow(page, 'bob')).toContainText('spread a rumor');
		await expect(scoreRow(page, 'bob')).toContainText('still holds something over the host');
	}
	// A guest's make earns the guest a mar to inflict, not the host a make.
	await expect(alice.getByText('0 of 1 taken')).toBeVisible();
	// Only the make-roller is offered the insist button.
	await expect(bob.getByRole('button', { name: 'Insist the host take a Mar' })).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Insist the host take a Mar' })).toHaveCount(0);
	await expect(alice.getByRole('button', { name: 'Insist the host take a Mar' })).toHaveCount(0);
	// Once chosen, a guest has had their turn.
	await expect(bob.getByRole('button', { name: 'Roll · difficulty 2' })).toHaveCount(0);
	await expectStatus(await act(g, 'bob', planID, 'guest-roll', { action: 'opt_out' }), 409);
});

test('a mar or an opt-out earns the host a make, and the host spends them as spoils', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	const { alice, carol } = await openPages(g, ['alice', 'carol']);

	await optOut(g, 'carol', planID);
	await expectLogLine(g, planID, 'carol opted out of rolling — alice earns a make.');
	await expect(alice.getByText('0 of 2 taken')).toBeVisible();
	await expect(scoreRow(carol, 'carol')).toContainText('Kept to themselves');

	await rollAndChoose(g, 'dave', planID, 'mar', { choice: 'accept_duels' });
	await expectLogLine(g, planID, "dave's mar earns alice a make.");
	await expect(alice.getByText('0 of 3 taken')).toBeVisible();
	await expect(scoreRow(alice, 'dave')).toContainText('Slipped up');

	// The host takes one through the UI: the make is hers, tied to no guest.
	await alice.getByRole('button', { name: 'Spread a new rumor' }).click();
	await alice.getByLabel('Rumor text:').fill(RUMOR);
	await alice.getByRole('button', { name: 'Take this Make (3 left)' }).click();
	await expect.poll(async () => (await festivity(g, planID)).host_makes_taken).toEqual(['spread_rumor']);
	await expect(alice.getByText('1 of 3 taken')).toBeVisible();
	await expect(scoreRow(alice, 'alice')).toContainText('1 of 3 Makes taken');

	// She can spend exactly what she earned and no more.
	for (let i = 0; i < 2; i++) {
		const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
		expect(res.status(), await res.text()).toBe(200);
	}
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
	await expect(alice.getByText('Your extra Makes')).toHaveCount(0);
});

test('a make-roller insists; the host, not the guest, chooses the mar', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	await bobMakes(g, planID);
	const { alice, bob, carol } = await openPages(g, ['alice', 'bob', 'carol']);

	// Only an IOU holder can insist, and only once.
	await expectStatus(await act(g, 'carol', planID, 'insist-host-mar'), 403);
	await expectStatus(await act(g, 'alice', planID, 'insist-host-mar'), 403);
	// The insist carries no option: the guest can't pick the mar.
	await bob.getByRole('button', { name: 'Insist the host take a Mar' }).click();
	await expect.poll(async () => (await festivity(g, planID)).unchosen_host_mars).toEqual([g.ids.bob]);
	await expectLogLine(g, planID, 'bob insisted that alice take a mar — alice chooses which.');
	await expectStatus(await act(g, 'bob', planID, 'insist-host-mar'), 403);
	await expect(bob.getByRole('button', { name: 'Insist the host take a Mar' })).toHaveCount(0);

	// The host is handed the choice of all four; nobody else is.
	await expect(alice.getByText('bob insists you take a Mar')).toBeVisible();
	for (const label of ['A rumor spreads about you', 'You must accept all duels', 'Break yourself']) {
		await expect(alice.getByRole('button', { name: label })).toBeEnabled();
	}
	// (Falling out with a peer is on offer too, but locked: the host has no peer to lose.)
	await expect(alice.getByRole('button', { name: 'A peer considers leaving' })).toBeDisabled();
	await expect(bob.getByText('insists you take a Mar')).toHaveCount(0);
	await expect(carol.getByRole('button', { name: 'Take this Mar' })).toHaveCount(0);
	await expectStatus(await act(g, 'bob', planID, 'resolve-host-mar', { insister_id: g.ids.bob, mar_option: 'accept_duels' }), 403);
	await expectStatus(await act(g, 'carol', planID, 'resolve-host-mar', { insister_id: g.ids.bob, mar_option: 'accept_duels' }), 403);

	await alice.getByRole('button', { name: 'You must accept all duels' }).click();
	await alice.getByRole('button', { name: 'Take this Mar' }).click();
	await expect.poll(async () => (await festivity(g, planID)).host_mar_insists).toEqual(['accept_duels']);
	expect((await festivity(g, planID)).unchosen_host_mars ?? []).toEqual([]);
	expect((await festivity(g, planID)).guest_ious ?? []).toEqual([]);
	await expectLogLine(g, planID,
		"alice chose to accept any duel challenge for the rest of the event in answer to bob's insistence.");

	// The scorecard now says the IOU is spent, and the host's mar is counted.
	await expect(scoreRow(carol, 'bob')).toContainText('got the better of the host');
	await expect(scoreRow(carol, 'alice')).toContainText('1 Mar from guests');
	await expect(alice.getByText('insists you take a Mar')).toHaveCount(0);
});

test('one roll at a time: while a roll-and-choice is open, nobody else may act — dice or choice', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	const rollID = await startRoll(g, 'bob', planID);
	const { alice, carol } = await openPages(g, ['alice', 'carol']);

	// A bystander is told who they are waiting on and can't roll or opt out.
	await expect(carol.getByText('Waiting for bob to finish their roll…')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Roll · difficulty 2' })).toBeDisabled();
	await expect(carol.getByRole('button', { name: 'Opt out' })).toBeDisabled();

	// The server refuses every other festivity action with 409 — including the host's.
	await expectStatus(await act(g, 'carol', planID, 'guest-roll', { action: 'roll' }), 409);
	await expectStatus(await act(g, 'carol', planID, 'guest-roll', { action: 'opt_out' }), 409);
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);

	// The lock outlasts the dice: bob's roll has resolved but he hasn't chosen yet.
	await forceRoll(g, rollID, 'make');
	await expectStatus(await act(g, 'carol', planID, 'guest-roll', { action: 'opt_out' }), 409);
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
	await expect(carol.getByText('Waiting for bob to finish their roll…')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'End the event' })).toBeDisabled();

	// Bob concludes; the table is free again.
	await choose(g, 'bob', planID, { choice: 'spread_rumor', rumor_text: RUMOR });
	await expect(carol.getByText('Waiting for bob to finish their roll…')).toHaveCount(0);
	await expect(carol.getByRole('button', { name: 'Roll · difficulty 2' })).toBeEnabled();
	await optOut(g, 'carol', planID);
	const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
	expect(res.status(), await res.text()).toBe(200);
});

test('the host ends the event only when everything is settled; generic complete is closed', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	const { alice, bob, carol } = await openPages(g, ['alice', 'bob', 'carol']);
	const end = alice.getByRole('button', { name: 'End the event' });

	// Guests haven't chosen: gated, in the UI and on the server. The generic
	// complete route can't reach the roster, so it is refused outright.
	await expect(end).toBeDisabled();
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);
	await expectStatus(await act(g, 'alice', planID, 'complete'), 409);

	await bobMakes(g, planID);              // bob: a make, so an IOU
	await optOut(g, 'carol', planID);       // +1 host make
	await rollAndChoose(g, 'dave', planID, 'mar', { choice: 'accept_duels' }); // +1 host make

	// Every guest has chosen, but the host still has three makes to take.
	await expect(alice.getByText('0 of 3 taken')).toBeVisible();
	await expect(end).toBeDisabled();
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);
	for (let i = 0; i < 3; i++) {
		const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
		expect(res.status(), await res.text()).toBe(200);
	}

	// Makes spent, but bob's mar is still owed: the guest can't waive it (the
	// owner ruled this compulsory), so the event stays open.
	await expect(alice.getByText('Your extra Makes')).toHaveCount(0);
	await expect(end).toBeDisabled();
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);

	// Bob insists; until the host has chosen and settled the mar, still gated.
	await act(g, 'bob', planID, 'insist-host-mar').then(r => expect(r.status()).toBe(200));
	await expect(alice.getByText('bob insists you take a Mar')).toBeVisible();
	await expect(end).toBeDisabled();
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);

	await alice.getByRole('button', { name: 'You must accept all duels' }).click();
	await alice.getByRole('button', { name: 'Take this Mar' }).click();

	// Now it can end. Only the host is offered the button; a guest is refused.
	await expect(end).toBeEnabled();
	await expect(bob.getByRole('button', { name: 'End the event' })).toHaveCount(0);
	await expectStatus(await act(g, 'bob', planID, 'end-event'), 403);
	await end.click();

	await expect.poll(() => planStatus(g, planID)).toBe('resolved');
	expect(await planResult(g, planID)).toBe('make');
	// The table is told it wound down, the panel clears for everyone, and the
	// record moves on to the next scene.
	await expect
		.poll(async () => (await festivityLog(g, planID, false)).includes('The festivity drew to a close.'))
		.toBe(true);
	for (const page of [alice, bob, carol]) {
		await expect(page.getByText('The Talk of the Event')).toHaveCount(0);
		await expect(page.getByRole('button', { name: 'End the event' })).toHaveCount(0);
		await expect(page.getByText('Set the scene')).toBeVisible();
	}
	// Nothing further is accepted.
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);
	await expectStatus(await act(g, 'carol', planID, 'guest-roll', { action: 'opt_out' }), 409);
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
});
