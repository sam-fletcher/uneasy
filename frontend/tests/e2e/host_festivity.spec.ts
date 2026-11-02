import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedHFGame, startFestivity, openPages, act, startRoll, forceRoll, choose, rollAndChoose, optOut,
	festivity, festivityLog, expectLogLine, expectLogContaining, expectStatus, expectLocked,
	assetsOf, mainCharacter, addPeer, addMarginalia, planStatus, planResult, scoreRow,
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

	// The server agrees the make earned the host nothing: she has exactly the one
	// make for hosting, so the first host-choice lands and a second is refused.
	const first = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
	expect(first.status(), await first.text()).toBe(200);
	await expectLogContaining(g, planID, 'alice spread a new rumor at the event');
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
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
	await expectLogContaining(g, planID, 'alice spread a new rumor at the event');

	// She can spend exactly what she earned and no more.
	for (let i = 0; i < 2; i++) {
		const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
		expect(res.status(), await res.text()).toBe(200);
	}
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
	await expect(scoreRow(alice, 'alice')).toContainText('3 of 3 Makes taken');
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
	// (Positive first: carol's page must have rendered the insist before "nothing" means anything.)
	await expect(scoreRow(carol, 'alice')).toContainText('1 Mar to choose');
	await expect(scoreRow(bob, 'alice')).toContainText('1 Mar to choose');
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

	// The server refuses every other festivity action with the lock's own 409 —
	// including the host's.
	await expectLocked(await act(g, 'carol', planID, 'guest-roll', { action: 'roll' }));
	await expectLocked(await act(g, 'carol', planID, 'guest-roll', { action: 'opt_out' }));
	await expectLocked(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }));

	// The lock outlasts the dice: bob's roll has resolved but he hasn't chosen yet.
	await forceRoll(g, rollID, 'make');
	await expectLocked(await act(g, 'carol', planID, 'guest-roll', { action: 'opt_out' }));
	await expectLocked(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }));
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

test('the lock also covers insisting, settling a mar and challenging, and frees when the roller chooses', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	// Bob makes and insists at once: an unchosen mar waits on the host. Carol
	// makes and keeps her IOU.
	await bobMakes(g, planID);
	expect((await act(g, 'bob', planID, 'insist-host-mar')).status()).toBe(200);
	await rollAndChoose(g, 'carol', planID, 'make', { choice: 'spread_rumor', rumor_text: RUMOR });

	// Dave's roll is open: each of these would otherwise succeed.
	const daveRoll = await startRoll(g, 'dave', planID);
	await expectLocked(await act(g, 'carol', planID, 'insist-host-mar'));
	await expectLocked(await act(g, 'alice', planID, 'resolve-host-mar', { insister_id: g.ids.bob, mar_option: 'accept_duels' }));
	await expectLocked(await act(g, 'alice', planID, 'host-choice', { choice: 'challenge_duel', target_player_id: g.ids.carol }));
	await expectLocked(await act(g, 'carol', planID, 'challenge-duel', { target_player_id: g.ids.bob }));
	expect((await festivity(g, planID)).guest_ious).toEqual([g.ids.carol]);
	expect((await festivity(g, planID)).unchosen_host_mars).toEqual([g.ids.bob]);

	// Dave concludes; the same two actions now go through.
	await forceRoll(g, daveRoll, 'make');
	await choose(g, 'dave', planID, { choice: 'spread_rumor', rumor_text: RUMOR });
	expect((await act(g, 'carol', planID, 'insist-host-mar')).status()).toBe(200);
	const settle = await act(g, 'alice', planID, 'resolve-host-mar', { insister_id: g.ids.bob, mar_option: 'accept_duels' });
	expect(settle.status(), await settle.text()).toBe(200);
	// (The roller's own exemption on insisting can't be shown: a guest only
	// holds an IOU once their roll-and-choice has concluded.)
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
	// PROVISIONAL: that "all earned makes are taken before End" gate has not been
	// ruled on by the owner yet (the book lets the host take makes any time
	// before the event is over). If the gate is relaxed, drop this block.
	await expect(alice.getByText('0 of 3 taken')).toBeVisible();
	await expect(end).toBeDisabled();
	await expectStatus(await act(g, 'alice', planID, 'end-event'), 409);
	for (let i = 0; i < 3; i++) {
		const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
		expect(res.status(), await res.text()).toBe(200);
	}

	// Makes spent, but bob's mar is still owed. PROVISIONAL: the owner ruled the
	// guest's mar compulsory "for now" (audit Tier 3 #12, listed in principle 6's
	// register of rules-optional choices). If it becomes waivable, this pin and
	// the "IOU gating" half of the end gate change with it.
	await expect(scoreRow(alice, 'alice')).toContainText('3 of 3 Makes taken');
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
	await expect(scoreRow(bob, 'alice')).toContainText('1 Mar from guests');
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

test('the host spends an earned make on a duel; a guest bound by accept_duels cannot decline', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	await rollAndChoose(g, 'carol', planID, 'mar', { choice: 'accept_duels' }); // +1 host make, carol must accept
	const { alice, carol, dave } = await openPages(g, ['alice', 'carol', 'dave']);

	// First make: a challenge to dave, through the host's picker.
	await expect(alice.getByText('0 of 2 taken')).toBeVisible();
	await alice.getByRole('button', { name: 'Propose a duel' }).click();
	await alice.locator('.player-chip', { hasText: 'dave' }).click();
	await alice.getByRole('button', { name: 'Take this Make (2 left)' }).click();
	await expect.poll(async () => (await festivity(g, planID)).host_makes_taken).toEqual(['challenge_duel']);
	expect((await festivity(g, planID)).pending_challenge).toMatchObject({ challenger_id: g.ids.alice, target_id: g.ids.dave });
	await expectLogContaining(g, planID, 'alice threw down a challenge across the hall, calling out dave to a duel.');

	// Only the target is asked; everyone else sees the table paused.
	await expect(dave.getByRole('button', { name: 'Accept challenge' })).toBeEnabled();
	await expect(dave.getByRole('button', { name: 'Decline' })).toBeEnabled();
	await expect(carol.getByText('Awaiting the target')).toBeVisible();
	await expect(alice.getByText('Awaiting the target')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Accept challenge' })).toHaveCount(0);
	await expectStatus(await act(g, 'carol', planID, 'respond-challenge', { accept: false }), 403);

	await dave.getByRole('button', { name: 'Decline' }).click();
	await expectLogContaining(g, planID, "dave waved off alice's challenge, and the moment passed.");
	await expect.poll(async () => (await festivity(g, planID)).pending_challenge ?? null).toBeNull();

	// The make is spent whether or not it is accepted. Her second goes at carol,
	// who took accept_duels: the picker says so and she cannot decline.
	await alice.getByRole('button', { name: 'Propose a duel' }).click();
	await alice.locator('.player-chip', { hasText: 'carol' }).click();
	await expect(alice.getByText('This challenge must be accepted.')).toBeVisible();
	await alice.getByRole('button', { name: 'Take this Make (1 left)' }).click();
	await expect.poll(async () => (await festivity(g, planID)).host_makes_taken).toEqual(['challenge_duel', 'challenge_duel']);
	await expect(carol.getByRole('button', { name: 'Accept challenge' })).toBeEnabled();
	await expect(carol.getByRole('button', { name: 'Decline' })).toBeDisabled();
	await expectStatus(await act(g, 'carol', planID, 'respond-challenge', { accept: false }), 409);
	await expectStatus(await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR }), 409);
});

test('mars the host settles herself: break_self tears her main character; a fallen-out peer rejoins broken at the end', async ({ browser }) => {
	const { g, planID } = await newFestivity(browser);
	// Alice's main character needs a second marginalia (so one tear doesn't
	// destroy it), and she needs a non-main peer with two (so the rejoin break
	// tears rather than destroys).
	const mc = await mainCharacter(g, 'alice');
	await addMarginalia(g, 'alice', mc.id, 'Quick to anger.');
	const peerID = await addPeer(g, 'alice', 'Lord Quill', ['Fond of cards.', 'Owes everyone.']);

	await bobMakes(g, planID);
	await rollAndChoose(g, 'carol', planID, 'make', { choice: 'spread_rumor', rumor_text: RUMOR });
	await optOut(g, 'dave', planID);                     // +1 host make: two to take
	expect((await act(g, 'bob', planID, 'insist-host-mar')).status()).toBe(200);
	expect((await act(g, 'carol', planID, 'insist-host-mar')).status()).toBe(200);
	const { alice, bob } = await openPages(g, ['alice', 'bob']);

	// The insisting guest can't pick or supply the host's asset; the host does.
	await expect(alice.getByText('bob insists you take a Mar')).toBeVisible();
	await expect(alice.getByText('2 mars to settle')).toBeVisible();
	await expect(scoreRow(bob, 'alice')).toContainText('2 Mars to choose');

	// break_self: the host picks which marginalia on her own main character tears.
	const quick = mc.marginalia.length ? mc.marginalia[0].id : 0;
	const tear = await act(g, 'alice', planID, 'resolve-host-mar',
		{ insister_id: g.ids.bob, mar_option: 'break_self', marginalia_id: quick });
	expect(tear.status(), await tear.text()).toBe(200);
	await expect.poll(async () => {
		const m = (await mainCharacter(g, 'alice')).marginalia.find(x => x.id === quick);
		return m?.is_torn;
	}).toBe(true);
	expect((await mainCharacter(g, 'alice')).is_destroyed).toBe(false);

	// disagreement: her peer goes to the centre, still hers; a main character can't.
	await expectStatus(await act(g, 'alice', planID, 'resolve-host-mar',
		{ insister_id: g.ids.carol, mar_option: 'disagreement', asset_id: mc.id }), 400);
	const fall = await act(g, 'alice', planID, 'resolve-host-mar',
		{ insister_id: g.ids.carol, mar_option: 'disagreement', asset_id: peerID });
	expect(fall.status(), await fall.text()).toBe(200);
	await expect.poll(async () => (await festivity(g, planID)).centered_asset_ids).toEqual([peerID]);
	await expectLogContaining(g, planID, 'fell out with their peer **Lord Quill**');
	await expect(alice.getByText('Available peers to join a retinue:')).toBeVisible();
	await expect(alice.getByRole('listitem').filter({ hasText: 'Lord Quill' })).toBeVisible();

	// Nobody takes him. Ending the event sends him home broken (torn, not destroyed).
	for (let i = 0; i < 2; i++) {
		const res = await act(g, 'alice', planID, 'host-choice', { choice: 'spread_rumor', rumor_text: RUMOR });
		expect(res.status(), await res.text()).toBe(200);
	}
	const end = await act(g, 'alice', planID, 'end-event');
	expect(end.status(), await end.text()).toBe(200);
	await expectLogContaining(g, planID, '**Lord Quill** rejoined alice, broken by the falling-out.');
	const quill = (await assetsOf(g)).find(a => a.id === peerID)!;
	expect(quill.owner_id).toBe(g.ids.alice);
	expect(quill.is_destroyed).toBe(false);
	expect(quill.marginalia.filter(m => m.is_torn)).toHaveLength(1);
});
