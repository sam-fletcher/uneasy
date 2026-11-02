import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import {
	seedPD, prepareDecree, resolveDecree, resolvingDecree, openAll, act, actOK, forceRoll, decreeState,
	laws, assets, spareAsset, crown, expectLog, reloadAll, forceAndSettle, bankedDice, closeContextsAfterEach,
	type PDGame,
} from './helpers/propose_decree';

// Regression specs for what the 12-plan UX audit fixed in Propose Decree:
//   · the council meeting: the preparer finalizes the text and opens the debate,
//     lower-ranked players join (leveraging one asset) or decline, and the
//     signatory cannot close the debate until each has decided;
//   · passing the decree is automatic — no "pass the decree" click after the dice;
//   · a make: the signatory's addendum, then the preparer enacts and names the
//     resource (no placeholder asset); the law lands under Laws with its addendum;
//   · a mar: the council amends lowest power first, one at a time, and no
//     resource is created;
//   · the final law, amendments and addendum reach the action log (they once
//     didn't), and the "writing the law (being amended)" copy has its space;
//   · who sees what: the actor holds the form, everyone else a waiting line;
//   · a table with a reigning monarch: the monarch signs and sits on the
//     council at any power rank (never audited, ADR-007 Phase C).
//
// Whether the make resolves itself once the law is enacted is an open owner
// question (docs/work/questions/propose-decree-make-autoresolve.md), so no spec
// here asserts the plan's status after enact-law — the law and the resource are
// the outcome.
//
// Setup (seed, scene, prepare, fast-forward, forcing the dice, crowning a
// monarch via /api/dev/crown) goes over the API; the subject is the panel and
// the server's gates.
//
// Ranks: with `alice-first` (the default seed) alice is rank 1 — the lone council
// member and signatory — and bob and carol owe a join/decline. With `alice-last`
// bob is rank 1, carol rank 2, alice rank 3: everyone is seated and bob signs.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const LAW = 'No private armies within the capital.';

/** The Waiting On bar's text, as one player sees it. */
const waitingBar = (page: Page) => page.locator('p', { hasText: 'Waiting On:' }).first();

/** The proposed text under debate, as the panel shows it (not the prep note above it). */
const debated = (page: Page) =>
	page.locator('.choices-header', { hasText: 'Proposed decree being debated:' }).locator('+ p.plan-notes');

/** Tick a card in a CardPicker (assets are chosen by checkbox). */
async function pickCard(page: Page, name: string) {
	await page.locator('.card', { hasText: name }).getByRole('checkbox').click();
}

async function resolving(browser: Parameters<typeof seedPD>[0], opts: Parameters<typeof resolvingDecree>[1] = {}) {
	const r = await resolvingDecree(browser, opts);
	track(r.g.gameID);
	return r;
}

/** alice-first table: bob joins by leveraging a spare asset, carol declines, roll is called by alice. */
async function rolledDecree(browser: Parameters<typeof seedPD>[0]) {
	const { g, planID } = await resolving(browser);
	await actOK(g, 'alice', planID, 'start-debate', { text: LAW });
	const spare = await spareAsset(g, 'bob');
	await actOK(g, 'bob', planID, 'join-council', { asset_ids: [spare.id] });
	await actOK(g, 'carol', planID, 'decline-council');
	const { roll } = await actOK(g, 'alice', planID, 'call-roll');
	return { g, planID, rollID: roll.id as number, spare };
}

async function lawCount(g: PDGame) {
	return (await laws(g)).length;
}

test('preparation: the decree is written in the plan form and must not be blank', async ({ browser }) => {
	const g = await seedPD(browser, { viewport: { width: 375, height: 812 } });
	track(g.gameID);
	const scene = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/scenes`, {
		data: { location_custom: 'The Council Chamber', time_elapsed: 'days', present_peer_ids: [] },
	});
	expect(scene.ok(), await scene.text()).toBeTruthy();
	const ended = await g.ctx.alice.request.post(`/api/tables/${g.gameID}/end-scene`);
	expect(ended.ok(), await ended.text()).toBeTruthy();

	const alice = await g.ctx.alice.newPage();
	await alice.goto(`/table/${g.gameID}`);
	await alice.getByText('Propose Decree', { exact: true }).click();

	const prepare = alice.getByRole('button', { name: 'Prepare Plan' });
	await expect(prepare).toBeDisabled();
	await alice.getByPlaceholder('What law are you drafting?').fill('Salt may be traded freely.');
	await expect(prepare).toBeEnabled();
	await prepare.click();

	await expect
		.poll(async () => {
			const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`);
			const { plans } = (await res.json()) as { plans: { plan_type: string; preparation_notes: string }[] };
			return plans.find(p => p.plan_type === 'propose_decree')?.preparation_notes ?? null;
		})
		.toBe('Salt may be traded freely.');

	// And the server won't take a blank decree from anyone calling it directly.
	const blank = await g.ctx.bob.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: { plan_type: 'propose_decree', preparation_notes: '' },
	});
	expect(blank.status()).toBe(400);
});

test('council: text is finalized first, lower-ranked players join or decline, then the signatory may close', async ({ browser }) => {
	const { g, planID } = await resolving(browser);
	const { alice, bob, carol } = await openAll(g);

	// Kickoff: the preparer is seated at once as the sole member and signatory;
	// the others see the roster and are asked to join before the text is final.
	await expect(alice.getByText('Council (1) · Signatory: alice')).toBeVisible();
	await expect(bob.getByText('Council (1) · Signatory: alice')).toBeVisible();
	await expect(alice.getByText('Finalize your decree')).toBeVisible();
	await expect(bob.getByText('alice is finalizing the decree…')).toBeVisible();
	await expect(bob.getByText('Join the council?')).toBeVisible();
	await expect(carol.getByText('Join the council?')).toBeVisible();
	await expect(bob.getByText('Proposed decree being debated:')).toHaveCount(0);
	await expect(bob.getByRole('button', { name: 'Open the debate' })).toHaveCount(0);
	for (const who of ['bob', 'carol']) {
		await expect(waitingBar({ bob, carol }[who as 'bob' | 'carol'])).toContainText('alice');
	}

	// Nobody but the preparer opens the debate; a blank text is refused.
	expect((await act(g, 'bob', planID, 'start-debate', { text: LAW })).status()).toBe(403);
	expect((await act(g, 'alice', planID, 'start-debate', { text: '  ' })).status()).toBe(400);

	await alice.getByRole('button', { name: 'Open the debate' }).click();
	// The proposed law now shows to everyone, live.
	await expect(bob.getByText('Proposed decree being debated:')).toBeVisible();
	for (const page of [alice, bob, carol]) await expect(debated(page)).toHaveText(`"${LAW}"`);
	expect((await act(g, 'alice', planID, 'start-debate', { text: LAW })).status()).toBe(409);

	// The signatory can't close the debate while anyone is undecided.
	const endDebate = alice.getByRole('button', { name: 'End the debate' });
	await expect(endDebate).toBeDisabled();
	await expect(alice.getByText('Waiting on bob, carol to decide if they want to join the council.')).toBeVisible();
	expect((await act(g, 'alice', planID, 'call-roll')).status()).toBe(409);
	// Only the signatory may call it at all.
	expect((await act(g, 'bob', planID, 'call-roll')).status()).toBe(403);

	// The Waiting On bar names the undecided joiners, not the signatory (who can't act yet).
	await expect(waitingBar(bob)).toContainText('carol');
	await expect(waitingBar(alice)).toContainText('bob');
	await expect(waitingBar(alice)).not.toContainText('You');

	// bob joins by leveraging one asset, through the picker.
	const spare = await spareAsset(g, 'bob');
	expect(await bankedDice(g, 'bob')).toHaveLength(0);
	await pickCard(bob, spare.name);
	await bob.getByRole('button', { name: 'Join', exact: true }).click();
	await expect(alice.getByText('Council (2) · Signatory: alice')).toBeVisible();
	await expect(alice.getByText('Waiting on carol to decide if they want to join the council.')).toBeVisible();
	await expect
		.poll(async () => (await assets(g)).find(a => a.id === spare.id)?.is_leveraged)
		.toBe(true);
	// ...and the asset becomes one die to help or interfere on the decree's roll.
	expect((await bankedDice(g, 'bob')).map(d => d.source)).toEqual(['decree']);
	await expect(waitingBar(alice)).not.toContainText('bob');
	await expect(waitingBar(alice)).toContainText('carol');

	// carol declines: it buys nothing, but it unblocks the table and is shown.
	await carol.getByRole('button', { name: 'Decline' }).click();
	await expect(carol.getByText('You declined to join the council.')).toBeVisible();
	await expect(alice.getByText('Declined: carol')).toBeVisible();
	await expect(endDebate).toBeEnabled();

	// A decision is made once.
	expect((await act(g, 'bob', planID, 'join-council', { asset_ids: [spare.id] })).status()).toBe(409);
	expect((await act(g, 'carol', planID, 'join-council', { asset_ids: [spare.id] })).status()).toBe(409);
	expect((await act(g, 'carol', planID, 'decline-council')).status()).toBe(409);
	expect((await act(g, 'alice', planID, 'join-council', { asset_ids: [spare.id] })).status()).toBe(409);
	expect((await act(g, 'alice', planID, 'decline-council')).status()).toBe(409);

	await endDebate.click();
	// The roll is called: a second call is refused.
	await expect.poll(async () => (await act(g, 'alice', planID, 'call-roll')).status()).toBe(409);

	await expectLog(alice, [
		'opened the council debate on the proposed decree',
		'bob leveraged into the council',
		'carol declined to join the council',
		'alice declared the debate over and calls for the dice roll',
	]);
});

test('join-council: exactly one owned, un-leveraged asset; nobody at or above the preparer', async ({ browser }) => {
	const { g, planID } = await resolving(browser);
	await actOK(g, 'alice', planID, 'start-debate', { text: LAW });
	const mine = await spareAsset(g, 'bob');
	const hers = await spareAsset(g, 'alice');

	expect((await act(g, 'bob', planID, 'join-council', { asset_ids: [] })).status()).toBe(400);
	expect((await act(g, 'bob', planID, 'join-council', { asset_ids: [mine.id, 999999999] })).status()).toBe(400);
	expect((await act(g, 'bob', planID, 'join-council', { asset_ids: [999999999] })).status()).toBe(404);
	expect((await act(g, 'bob', planID, 'join-council', { asset_ids: [hers.id] })).status()).toBe(403);

	// Nothing was spent by the refusals.
	expect((await assets(g)).find(a => a.id === mine.id)?.is_leveraged).toBe(false);
	await actOK(g, 'bob', planID, 'join-council', { asset_ids: [mine.id] });
	expect((await decreeState(g, planID)).signatory_player_ids).toContain(g.ids.bob);
});

test('make: the signatory\'s addendum, then the preparer names the resource; the law lands under Laws', async ({ browser }) => {
	const { g, planID, rollID, spare } = await rolledDecree(browser);
	await forceAndSettle(g, planID, rollID, 'make');
	// The joiner's unspent council die doesn't outlive the decree's roll.
	expect(await bankedDice(g, 'bob')).toHaveLength(0);
	const { alice, bob, carol } = await openAll(g);

	// Passing the decree is automatic: straight to the addendum, no "pass" gate.
	await expect(alice.getByText("Signatory's Addendum")).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Pass the decree' })).toHaveCount(0);
	await expect(bob.getByText('Waiting for alice to place the addendum…')).toBeVisible();
	await expect(carol.getByText('Waiting for alice to place the addendum…')).toBeVisible();
	await expect(bob.getByText('The decree passed — writing the law.')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Place addendum' })).toHaveCount(0);
	await expect(waitingBar(bob)).toContainText('alice');

	// The steps are ordered, and only the right person may take each.
	expect((await act(g, 'alice', planID, 'enact-law', { resource_name: 'Salt Tax', resource_marginalia: ['Hated by the guilds'] })).status()).toBe(409);
	expect((await act(g, 'bob', planID, 'set-addendum', { connector: 'and', addendum: 'x' })).status()).toBe(403);
	expect((await act(g, 'alice', planID, 'set-addendum', { addendum: 'salt is exempt' })).status()).toBe(400);
	expect((await act(g, 'alice', planID, 'amend-decree', { text: 'x' })).status()).toBe(409);
	expect(await lawCount(g)).toBe(1); // the seed's sample law only: nothing enacted yet

	// The addendum, through the form: pick "but", write the rider.
	await alice.getByRole('button', { name: 'but', exact: true }).click();
	await alice.getByPlaceholder('a simple phrase. (optional)').fill('salt is exempt');
	await alice.getByRole('button', { name: 'Place addendum' }).click();

	// Now the preparer enacts; the others wait on her. The enact button stays
	// disabled until the resource has a name and a marginalia.
	const enact = alice.getByRole('button', { name: 'Enact the law' });
	await expect(enact).toBeDisabled();
	await expect(bob.getByText('Waiting for alice to enact the law…')).toBeVisible();
	await expect(carol.getByRole('button', { name: 'Enact the law' })).toHaveCount(0);
	expect((await act(g, 'bob', planID, 'enact-law', { resource_name: 'Salt Tax', resource_marginalia: ['x'] })).status()).toBe(403);
	expect((await act(g, 'alice', planID, 'enact-law', { resource_marginalia: ['x'] })).status()).toBe(400);
	expect((await act(g, 'alice', planID, 'enact-law', { resource_name: 'Salt Tax' })).status()).toBe(400);
	expect(await lawCount(g)).toBe(1);

	await alice.getByPlaceholder('Name your new asset…').fill('The Salt Tax');
	await alice.getByPlaceholder('A description, a function, a fun fact, or something more poetic…').fill('Hated by the guilds');
	await expect(enact).toBeEnabled();
	await enact.click();

	// Outcome: the law is on the books with its addendum and the signatory; the
	// resource exists, and belongs to the preparer — not to the signatory or council.
	await expect.poll(() => lawCount(g)).toBe(2);
	const law = (await laws(g)).find(l => l.origin_plan_id === planID)!;
	expect(law.text).toBe(LAW);
	expect(law.addendum).toBe('but salt is exempt');
	expect(law.signatory_id).toBe(g.ids.alice);
	const resource = (await assets(g)).find(a => a.name === 'The Salt Tax')!;
	expect(resource.asset_type).toBe('resource');
	expect(resource.owner_id).toBe(g.ids.alice);
	// Joining cost bob his asset; the decree itself took nothing else.
	expect((await assets(g)).find(a => a.id === spare.id)?.owner_id).toBe(g.ids.bob);
	// Re-enacting is refused.
	expect((await act(g, 'alice', planID, 'enact-law', { resource_name: 'Again', resource_marginalia: ['x'] })).status()).toBe(409);
	expect(await lawCount(g)).toBe(2);

	// The action log carries the assembled law and the named resource, in order
	// (the audit found only "a decree passed" there).
	await expectLog(carol, [
		'declared the debate over',
		'The decree passed. It takes effect once alice places the addendum.',
		'placed the signatory\'s addendum at the end of the law',
		'but salt is exempt',
		'The decree was enacted, signed by alice',
		`${LAW} but salt is exempt`,
		'The Salt Tax',
		'Hated by the guilds',
	]);

	// Where players read laws: the Laws drawer shows text, addendum and bylines to
	// a bystander as well.
	await carol.getByRole('button', { name: 'Open laws' }).click();
	const drawer = carol.locator('.laws-rumors .lr-item', { hasText: LAW });
	await expect(drawer).toBeVisible();
	await expect(drawer.getByText('…but salt is exempt')).toBeVisible();
	await expect(drawer.getByText('Proposed by')).toBeVisible();
	await expect(drawer.getByText('Signed by')).toBeVisible();
});

test('mar: the council amends lowest power first, one at a time; the law is enacted with no resource', async ({ browser }) => {
	const { g, planID } = await resolving(browser, { powerOrder: 'alice-last' });
	const { alice, bob, carol } = await openAll(g);

	// Everyone outranks the preparer, so everyone is seated and nobody is asked
	// to decide; bob (rank 1) signs.
	await expect(alice.getByText('Council (3) · Signatory: bob')).toBeVisible();
	await expect(carol.getByText('Join the council?')).toHaveCount(0);
	expect((await act(g, 'carol', planID, 'join-council', { asset_ids: [1] })).status()).toBe(403);
	expect((await act(g, 'carol', planID, 'decline-council')).status()).toBe(403);

	await actOK(g, 'alice', planID, 'start-debate', { text: LAW });
	// With nobody left to decide, the signatory's "End the debate" is live at once.
	await expect(bob.getByRole('button', { name: 'End the debate' })).toBeEnabled();
	expect((await act(g, 'alice', planID, 'call-roll')).status()).toBe(403);
	await bob.getByRole('button', { name: 'End the debate' }).click();
	// Force the roll the click opened to a mar.
	let rollID = 0;
	await expect
		.poll(async () => {
			const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/rolls/active`);
			const body = (await res.json()) as { roll?: { id: number } | null };
			rollID = body.roll?.id ?? 0;
			return rollID;
		})
		.toBeGreaterThan(0);
	await forceAndSettle(g, planID, rollID, 'mar');
	await reloadAll([alice, bob, carol]); // the auto-applied outcome isn't pushed live (see the fixme below)

	// Lowest power first: carol (rank 2), then bob (rank 1). The preparer doesn't amend.
	await expect.poll(async () => (await decreeState(g, planID)).amendment_order).toEqual([g.ids.carol, g.ids.bob]);
	await expect(carol.getByText('Amend the law (your turn)')).toBeVisible();
	await expect(alice.getByText('The council is amending the law (2 to go). Next: carol.')).toBeVisible();
	await expect(bob.getByText('The council is amending the law (2 to go). Next: carol.')).toBeVisible();
	await expect(alice.getByText('The decree passed — writing the law (being amended).')).toBeVisible();
	await expect(waitingBar(alice)).toContainText('carol');
	await expect(bob.getByRole('button', { name: 'Submit amendment' })).toHaveCount(0);

	// Out of turn, and from the preparer: refused.
	expect((await act(g, 'bob', planID, 'amend-decree', { text: 'bob jumps the queue' })).status()).toBe(409);
	expect((await act(g, 'alice', planID, 'skip-amend')).status()).toBe(409);
	expect((await act(g, 'bob', planID, 'set-addendum', { connector: 'and', addendum: 'early' })).status()).toBe(409);
	expect((await act(g, 'alice', planID, 'enact-law', {})).status()).toBe(409);

	// carol rewrites the body (the editor starts from the current text).
	const AMENDED = 'No private armies within the capital, save the Queen\'s guard.';
	const editor = carol.getByPlaceholder('The amended law…');
	await expect(editor).toHaveValue(LAW);
	await editor.fill(AMENDED);
	await carol.getByRole('button', { name: 'Submit amendment' }).click();

	// Then bob, who works from carol's version, and leaves it be.
	await expect(bob.getByPlaceholder('The amended law…')).toHaveValue(AMENDED);
	await expect(alice.getByText('The council is amending the law (1 to go). Next: bob.')).toBeVisible();
	await bob.getByRole('button', { name: 'Leave unchanged' }).click();

	// bob signs: an addendum, then alice enacts — on a mar there's no resource form.
	await bob.getByPlaceholder('a simple phrase. (optional)').fill('the Queen consents');
	await bob.getByRole('button', { name: 'Place addendum' }).click();
	await expect(alice.getByText('The law above takes effect (no resource on a mar).')).toBeVisible();
	await expect(carol.getByText('Waiting for alice to enact the law…')).toBeVisible();
	await expect(alice.getByPlaceholder('The amended law…')).toHaveCount(0);

	// NOTE: "Enact the law" on a mar is a click-to-agree gate that the audit flagged
	// (high) and that is still open; if it is removed this spec changes here.
	const resourcesBefore = (await assets(g)).filter(a => a.asset_type === 'resource').length;
	await alice.getByRole('button', { name: 'Enact the law' }).click();

	await expect.poll(() => lawCount(g)).toBe(2);
	const law = (await laws(g)).find(l => l.origin_plan_id === planID)!;
	expect(law.text).toBe(AMENDED);
	expect(law.addendum).toBe('and the Queen consents');
	expect(law.signatory_id).toBe(g.ids.bob);
	expect((await assets(g)).filter(a => a.asset_type === 'resource').length).toBe(resourcesBefore);

	// The log has each hand's contribution and the assembled law.
	await expectLog(bob, [
		'The decree passed but was marred',
		AMENDED,
		'left the decree\'s text unchanged',
		'the Queen consents',
		'The amended decree was enacted, signed by bob',
		`${AMENDED} and the Queen consents`,
	]);
});

test('monarch: the crown-bearer signs and sits on the council even at the lowest power rank', async ({ browser }) => {
	// carol is power rank 3 and alice (rank 1) prepares; with no throne alice would sign.
	const g = await seedPD(browser);
	track(g.gameID);
	await crown(g, 'carol');
	const planID = await prepareDecree(g, LAW);
	await resolveDecree(g, planID);
	const { alice, bob, carol } = await openAll(g);

	// carol is seated without leveraging and holds the pen; bob (below alice, no
	// crown) still owes a decision.
	const pd = await decreeState(g, planID);
	expect(pd.signatory_id).toBe(g.ids.carol);
	expect([...pd.signatory_player_ids].sort()).toEqual([g.ids.alice, g.ids.carol].sort());
	await expect(alice.getByText('Council (2) · Signatory: carol')).toBeVisible();
	await expect(carol.getByText('carol (signatory)')).toBeVisible();
	await expect(carol.getByText('Join the council?')).toHaveCount(0);
	await expect(bob.getByText('Join the council?')).toBeVisible();
	// A seated monarch can't also leverage in; alice is a higher power but not the signatory.
	expect((await act(g, 'carol', planID, 'join-council', { asset_ids: [(await spareAsset(g, 'carol')).id] })).status()).toBe(409);

	await actOK(g, 'alice', planID, 'start-debate', { text: LAW });
	await expect(carol.getByRole('button', { name: 'End the debate' })).toBeDisabled();
	await expect(carol.getByText('Waiting on bob to decide if they want to join the council.')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'End the debate' })).toHaveCount(0);
	await bob.getByRole('button', { name: 'Decline' }).click();
	expect((await act(g, 'alice', planID, 'call-roll')).status()).toBe(403);
	await expect(carol.getByRole('button', { name: 'End the debate' })).toBeEnabled();
	await carol.getByRole('button', { name: 'End the debate' }).click();

	let rollID = 0;
	await expect
		.poll(async () => {
			const res = await g.ctx.alice.request.get(`/api/tables/${g.gameID}/rolls/active`);
			rollID = ((await res.json()) as { roll?: { id: number } | null }).roll?.id ?? 0;
			return rollID;
		})
		.toBeGreaterThan(0);
	await forceAndSettle(g, planID, rollID, 'make');
	await reloadAll([alice, bob, carol]);

	// The monarch signs; the preparer enacts and keeps the resource.
	await expect(carol.getByText("Signatory's Addendum")).toBeVisible();
	await expect(alice.getByText('Waiting for carol to place the addendum…')).toBeVisible();
	expect((await act(g, 'alice', planID, 'set-addendum', { connector: 'and', addendum: 'alice signs' })).status()).toBe(403);
	await carol.getByPlaceholder('a simple phrase. (optional)').fill('by the crown\'s leave');
	await carol.getByRole('button', { name: 'Place addendum' }).click();
	await expect(alice.getByRole('button', { name: 'Enact the law' })).toBeVisible();
	await actOK(g, 'alice', planID, 'enact-law', { resource_name: 'Crown Charter', resource_marginalia: ['Sealed in wax'] });

	await expect.poll(() => lawCount(g)).toBe(2);
	const law = (await laws(g)).find(l => l.origin_plan_id === planID)!;
	expect(law.signatory_id).toBe(g.ids.carol);
	expect(law.addendum).toBe('and by the crown\'s leave');
	expect((await assets(g)).find(a => a.name === 'Crown Charter')?.owner_id).toBe(g.ids.alice);
	await expectLog(bob, ['The decree was enacted, signed by carol']);
});

test('phone width: the join and decline buttons are real tap targets and nothing overflows', async ({ browser }) => {
	const { g } = await resolving(browser, { viewport: { width: 375, height: 812 } });
	const { alice, bob, carol } = await openAll(g);
	await expect(bob.getByText('Join the council?')).toBeVisible();

	// Join only appears once an asset is picked, so measure it after ticking one.
	await pickCard(bob, (await spareAsset(g, 'bob')).name);
	for (const name of ['Join', 'Decline']) {
		const box = await bob.getByRole('button', { name, exact: true }).boundingBox();
		expect(box!.height, `${name} is too short to tap`).toBeGreaterThanOrEqual(44);
	}
	await expect(alice.getByRole('button', { name: 'Open the debate' })).toBeVisible();
	const box = await alice.getByRole('button', { name: 'Open the debate' }).boundingBox();
	expect(box!.height).toBeGreaterThanOrEqual(44);
	for (const page of [alice, bob, carol]) {
		const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
		expect(overflow).toBeLessThanOrEqual(0);
	}
});

// BUG (docs/work/next/auto-applied-choice-not-broadcast.md): Propose Decree
// applies its outcome the instant the dice land, but that is not pushed to open
// panels, so the signatory stays on the debate step until they reload. Un-fixme
// this when the fix lands.
test.fixme('make: the signatory is handed the addendum live, with no reload', async ({ browser }) => {
	const { g, rollID } = await rolledDecree(browser);
	const { alice, bob } = await openAll(g);
	await expect(alice.getByText('Dice roll in progress…')).toBeVisible();
	await forceRoll(g, rollID, 'make');
	await expect(alice.getByText("Signatory's Addendum")).toBeVisible();
	await expect(bob.getByText('Waiting for alice to place the addendum…')).toBeVisible();
});

// The audit found the finalized text vanishing once the roll started, leaving
// only the stale prep draft. It is on screen through the roll now; pin that,
// using a finalized text that differs from the prep draft.
test('roll: the finalized decree text, not the prep draft, is on screen while the dice are open', async ({ browser }) => {
	const FINAL = 'No private armies, and no sworn banners.';
	const { g, planID } = await resolving(browser, { notes: 'No private armies.' });
	await actOK(g, 'alice', planID, 'start-debate', { text: FINAL });
	await actOK(g, 'bob', planID, 'decline-council');
	await actOK(g, 'carol', planID, 'decline-council');
	await actOK(g, 'alice', planID, 'call-roll');
	const { bob } = await openAll(g);
	await expect(bob.getByText('Dice roll in progress…')).toBeVisible();
	await expect(bob.getByText(FINAL).first()).toBeVisible();
});
