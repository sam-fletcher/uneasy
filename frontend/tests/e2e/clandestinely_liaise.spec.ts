import { test, expect, request as pwRequest, type BrowserContext, type Locator, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';
import * as H from './helpers/make_demands';
import {
	NOTES, SECRET_PREFIX, waitingBar, closeContextsAfterEach, seedLiaise, prepareLiaison, fetchPlan, fetchPlans, liaiseData,
	fetchAssets, asset, settleDelay, readableSecrets, openAll, pickCard, section, post, expectLogged, logBodies, currentRow, cap,
	type LGame, type Who,
} from './helpers/clandestinely_liaise';

// End-to-end for Clandestinely Liaise — specifically the "Secrets We Keep"
// hand-off, which had the same soft-lock bug class as Propose Duel: a
// two-sided submission step where one player's action wasn't pushed to the
// other, leaving the waiting party stuck on a stale panel.
//
// As with the duel spec, the *setup* (assets, the prep-time delay reveal,
// fast-forwarding the record to the plan's row, resolving, and advancing to
// the secrets phase) is driven over the API — it isn't the subject. The
// bug-prone part is the keep-secret loop: the preparer submits first, the
// partner submits second, and once both are in the liaison auto-advances to
// Things We Share — that transition must reach the preparer's panel live, with
// no manual reload. Pre-fix the hand-off never reached the waiting party at all.

interface PlanRow {
	id: number;
	plan_type: string;
	row_number: number | null;
	resolution_data: string | null;
}

async function fetchLiaisePlan(ctx: BrowserContext, gameID: number): Promise<PlanRow> {
	const res = await ctx.request.get(`/api/tables/${gameID}/plans`);
	expect(res.ok(), `GET plans failed: ${await res.text()}`).toBeTruthy();
	const { plans } = (await res.json()) as { plans: PlanRow[] };
	const cl = plans.find(p => p.plan_type === 'clandestinely_liaise');
	expect(cl, 'liaise plan not found').toBeTruthy();
	return cl!;
}

// Advance the public record to `targetRow` via the dev fast-forward, which runs
// the real advance (engrailed ranking updates, row.advanced posts) rather than
// writing current_row directly. There is no production row-advance route — see
// adr/FACILITATOR_POWERS_AUDIT.md. Mirrors the helper in propose_duel.spec.ts.
async function advanceToRow(gameID: number, targetRow: number, ctx: BrowserContext) {
	const res = await ctx.request.post('/api/dev/advance-row', {
		data: { game_id: gameID, advance_to: targetRow },
	});
	expect(res.ok(), `advance-row failed: ${await res.text()}`).toBeTruthy();
}

async function keepSecret(page: Page, assetName: string) {
	const sec = section(page, 'Secrets we keep');
	const card = sec.locator('.card', { hasText: assetName });
	await expect(card).toBeVisible();
	await card.getByRole('checkbox').click();
	await sec.getByRole('button', { name: 'Keep this secret' }).click();
}

const track = cleanupGameAfterEach();

test('clandestinely liaise: secrets-we-keep hand-off reaches the preparer live', async ({ browser }) => {
	// ── Seed a main_event game with alice (preparer) + bob (partner) ─────────
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: { phase: 'main_event', players: ['alice', 'bob'] },
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id, players } = await seedRes.json();
	track(game_id);
	const aliceID: number = players[0].id;
	const bobID: number = players[1].id;
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E });
	const bobCtx = await browser.newContext({ baseURL: E2E });
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');

	// ── Each player needs a peer: it bears the secret AND is the meeting peer ─
	const mkPeer = async (ctx: BrowserContext, name: string): Promise<number> => {
		const res = await ctx.request.post(`/api/tables/${game_id}/assets`, {
			data: { asset_type: 'peer', name, is_main_character: false, marginalia: ['trusted go-between'] },
		});
		expect(res.ok(), `create peer failed: ${await res.text()}`).toBeTruthy();
		return (await res.json()).asset.id as number;
	};
	const alicePeerID = await mkPeer(aliceCtx, 'Alice Confidant');
	const bobPeerID = await mkPeer(bobCtx, 'Bob Confidant');

	// ── Alice prepares the liaison with bob, naming both meeting peers;
	//    OnPrepare opens a delay reveal ──────────────────────────────────────
	const prep = await aliceCtx.request.post(`/api/tables/${game_id}/prepare-plan`, {
		data: {
			plan_type: 'clandestinely_liaise',
			target_player_id: bobID,
			preparer_peer_id: alicePeerID,
			partner_peer_id: bobPeerID,
			preparation_notes: 'A quiet word in the orangery.',
		},
	});
	expect(prep.ok(), `prepare failed: ${await prep.text()}`).toBeTruthy();

	// ── Both submit the delay reveal (face 1 → delay 1) so the plan lands ────
	const planBeforeReveal = await fetchLiaisePlan(aliceCtx, game_id);
	const liaise = JSON.parse(planBeforeReveal.resolution_data ?? '{}').liaise ?? {};
	const revealID: number = liaise.delay_reveal_id;
	expect(revealID, 'delay_reveal_id missing from resolution_data').toBeTruthy();
	for (const ctx of [aliceCtx, bobCtx]) {
		const res = await ctx.request.post(`/api/reveals/${revealID}/submit`, { data: { face: 1 } });
		expect(res.ok(), `reveal submit failed: ${await res.text()}`).toBeTruthy();
	}

	// ── Fast-forward to the plan's row, then resolve it (focus player) ───────
	const planAfterReveal = await fetchLiaisePlan(aliceCtx, game_id);
	expect(planAfterReveal.row_number, 'row_number not set after reveal').toBeTruthy();
	await advanceToRow(game_id, planAfterReveal.row_number!, aliceCtx);

	// The preparer (alice) resolves their own plan. Advancing onto the plan's
	// row auto-kicks-off resolution (plans.go), so this may 409 once the plan
	// is already 'resolving'. Either outcome means the liaison is now resolving.
	const resolveRes = await aliceCtx.request.post(`/api/plans/${planAfterReveal.id}/resolve`);
	if (!resolveRes.ok()) {
		expect(await resolveRes.text(), 'unexpected resolve failure').toContain('not in pending status');
	}

	// ── Advance together_at_last → secrets_we_keep (the preparer drives) ─────
	const advanceRes = await aliceCtx.request.post(`/api/plans/${planAfterReveal.id}/advance-liaise`);
	expect(advanceRes.ok(), `advance-liaise failed: ${await advanceRes.text()}`).toBeTruthy();

	// ── Open both players' tables; the resolving liaise panel auto-renders ───
	const alice = await aliceCtx.newPage();
	const bob = await bobCtx.newPage();
	await Promise.all([
		alice.goto(`/table/${game_id}`),
		bob.goto(`/table/${game_id}`),
	]);

	// Both land in the Secrets We Keep phase.
	await expect(section(alice, 'Secrets we keep')).toBeVisible({ timeout: 10_000 });
	await expect(section(bob, 'Secrets we keep')).toBeVisible({ timeout: 10_000 });

	// Preparer (alice) submits FIRST — this is the soft-lock ordering. Her own
	// refetch leaves her at 1/2 submitted, waiting on bob.
	await keepSecret(alice, 'Alice Confidant');
	await expect(section(alice, 'Secrets we keep').getByText(/Waiting for/)).toBeVisible();

	// Partner (bob) submits SECOND. With both secrets in, the server auto-advances
	// the liaison straight to Things We Share — there is no manual "Advance" click.
	// The phase-change broadcast must drive alice's panel refetch so her panel
	// moves to the next phase live, with no manual reload (the soft-lock this spec
	// guards). Both players land in Things We Share.
	await keepSecret(bob, 'Bob Confidant');
	await expect(section(alice, 'Things we share')).toBeVisible({ timeout: 10_000 });
	await expect(section(bob, 'Things we share')).toBeVisible({ timeout: 10_000 });

	await aliceCtx.close();
	await bobCtx.close();
});

// ── What the 12-plan UX audit fixed in Clandestinely Liaise ──────────────────
// Three players at 375px: alice prepares, bob is the partner, carol is the
// bystander. Setup runs over the API (helpers/clandestinely_liaise.ts); what a
// player does inside the liaison is driven through the panel.

closeContextsAfterEach();

test('prep: bad meeting peers are refused, then the delay reveal is shared with the bystander and sets the row', async ({ browser }) => {
	const g = await seedLiaise(browser, 'none');
	track(g.gameID);
	const assets = await fetchAssets(g);
	const aliceArtifact = assets.find(a => a.name === "alice's artifact")!.id;
	const carolPeer = assets.find(a => a.name === "carol's main character")!.id;
	const prep = (data: Record<string, unknown>) => g.ctx.alice.request.post(`/api/tables/${g.gameID}/prepare-plan`, {
		data: {
			plan_type: 'clandestinely_liaise', target_player_id: g.ids.bob,
			preparer_peer_id: g.peers.alice, partner_peer_id: g.peers.bob, preparation_notes: NOTES, ...data,
		},
	});

	const REFUSED = 400;
	expect((await prep({ target_player_id: g.ids.alice })).status(), 'liaising with yourself').toBe(REFUSED);
	expect((await prep({ target_player_id: null })).status(), 'no partner').toBe(REFUSED);
	expect((await prep({ preparer_peer_id: null })).status(), 'no own meeting peer').toBe(REFUSED);
	expect((await prep({ partner_peer_id: null })).status(), 'no partner meeting peer').toBe(REFUSED);
	expect((await prep({ partner_peer_id: g.peers.alice })).status(), "partner's peer must be the partner's").toBe(REFUSED);
	expect((await prep({ partner_peer_id: carolPeer })).status(), "a third player's peer").toBe(REFUSED);
	expect((await prep({ preparer_peer_id: aliceArtifact })).status(), 'a non-peer asset').toBe(REFUSED);
	expect((await fetchPlans(g)).filter(p => p.plan_type === 'clandestinely_liaise'), 'no refused attempt left a plan behind').toHaveLength(0);

	const startRow = await currentRow(g);
	g.planID = await prepareLiaison(g);
	const pages = await openAll(g);

	// The pair settle the time; everyone can read who is meeting whom.
	await expect(pages.alice.getByText('Pick a die face to set the delay:')).toBeVisible({ timeout: 10_000 });
	await expect(pages.bob.getByText('Pick a die face to set the delay:')).toBeVisible();
	await expect(pages.carol.getByText('alice and bob are settling on a time…')).toBeVisible();
	await expect(pages.carol.locator('.cl-meeting-peers')).toContainText("alice's Alice Confidant meets bob's Bob Confidant");
	await expect(pages.carol.getByRole('button', { name: 'Submit' })).toHaveCount(0);
	// The bystander cannot reveal a face either.
	const revealID = (await liaiseData(g)).delay_reveal_id as number;
	await post(g, 'carol', `/api/reveals/${revealID}/submit`, { face: 3 }, 403);

	// Alice reveals in the panel; her side locks and she waits on bob.
	await pages.alice.getByRole('button', { name: 'Pick 2' }).click();
	await pages.alice.getByRole('button', { name: 'Submit', exact: true }).click();
	await expect(pages.alice.getByText('Waiting on 1 other: bob')).toBeVisible();
	await expect(pages.alice.getByRole('button', { name: 'Pick 2' })).toBeDisabled();
	expect((await fetchPlan(g)).row_number, 'no row until both faces are in').toBeNull();

	// Bob's face completes the reveal: delay = ceil((2 + 5) / 2) = 4 rows ahead.
	await pages.bob.getByRole('button', { name: 'Pick 5' }).click();
	await pages.bob.getByRole('button', { name: 'Submit', exact: true }).click();
	await expect.poll(async () => (await fetchPlan(g)).row_number, { timeout: 10_000 }).not.toBeNull();
	expect((await fetchPlan(g)).row_number, 'delay ceil((2 + 5) / 2) = 4 rows past the start row').toBe(startRow + 4);
});

async function keepSecretInPanel(page: Page, assetName: string) {
	const sec = section(page, 'Secrets we keep');
	await pickCard(sec, assetName);
	await sec.getByRole('button', { name: 'Keep this secret' }).click();
}

async function chooseShare(page: Page, option: string) {
	const sec = section(page, 'Things we share');
	await sec.getByRole('button', { name: option, exact: true }).click();
	return sec;
}

const submitShare = (sec: Locator) => sec.getByRole('button', { name: 'Submit share choice' }).click();

/** Everything in the action log, joined, as `who` reads it. */
async function logText(g: LGame, who: Who) {
	return (await logBodies(g, who)).join('\n');
}

test('secret meeting: the secret text never reaches a bystander, even written after their page opened', async ({ browser }) => {
	const g = await seedLiaise(browser, 'keep');
	track(g.gameID);
	// Everything carol's open page receives over the WebSocket, so a leak by broadcast
	// is caught as well as one in a response.
	const carolFrames: string[] = [];
	const pages = await openAll(g, (who, page) => {
		if (who === 'carol') page.on('websocket', ws => ws.on('framereceived', f => carolFrames.push(String(f.payload))));
	});
	const { alice, bob, carol } = pages;
	const assets = await fetchAssets(g, 'carol');
	const aliceConfidant = g.peers.alice;
	const bobConfidant = g.peers.bob;
	expect(assets.find(a => a.id === aliceConfidant)!.secret_count).toBe(0);

	// Positive first: the bystander's table is live and shows the liaison under way.
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible({ timeout: 10_000 });
	await expect(section(alice, 'Secrets we keep')).toBeVisible();
	const framesBefore = carolFrames.length;

	// Alice nominates her Confidant (written while carol's page is open).
	await keepSecretInPanel(alice, 'Alice Confidant');
	await expect(alice.getByText("You've committed your secret. Waiting for bob")).toBeVisible();

	// What is public: that she entrusted a secret, and that her Confidant now bears one.
	await expectLogged(g, "entrusted the meeting's secret to", 'carol');
	await expect
		.poll(async () => (await asset(g, aliceConfidant, 'carol')).secret_count, { message: 'secret existence is public' })
		.toBe(1);
	// ...and the live socket carried the submission to carol's page (so the leak checks below can bite).
	await expect
		.poll(() => carolFrames.slice(framesBefore).some(f => f.includes('liaise.keep_secret_submitted')),
			{ message: 'carol received the keep-secret event live' })
		.toBe(true);
	// The Waiting On bar moved to the one pick still owed, on the actor's page and the bystander's.
	await expect(waitingBar(carol)).toHaveText(/Waiting On:\s*bob\s*$/);
	await expect(waitingBar(alice)).toHaveText(/Waiting On:\s*bob\s*$/);

	// What is not: the text. Only its author can read it; neither the partner nor the bystander can.
	const aliceReads = await readableSecrets(g, 'alice', aliceConfidant);
	expect(aliceReads).toHaveLength(1);
	expect(aliceReads[0]).toContain(SECRET_PREFIX);
	expect(await readableSecrets(g, 'bob', aliceConfidant), 'the partner cannot read it yet').toEqual([]);
	expect(await readableSecrets(g, 'carol', aliceConfidant), 'the bystander cannot read it').toEqual([]);

	// Which asset she picked is masked from everyone but her until both have picked.
	// This follows current behaviour and is in tension with the public log line above,
	// which names the holder: see questions/liaise-secret-holder-public.md. Whichever way
	// that is ruled, one of the two stops being asserted.
	// The mask runs at three read sites (plan list, one plan, the public record).
	const pickAt = async (site: 'list' | 'one' | 'record', who: Who) => {
		let liaise: any;
		if (site === 'list') liaise = await liaiseData(g, who);
		else if (site === 'one') {
			const res = await g.ctx[who].request.get(`/api/plans/${g.planID}`);
			expect(res.status(), `${who} GET plan`).toBe(200);
			liaise = (await res.json()).resolution_data.liaise;
		} else {
			const res = await g.ctx[who].request.get(`/api/tables/${g.gameID}/record`);
			expect(res.status(), `${who} GET record`).toBe(200);
			const found: any[] = [];
			const walk = (n: any) => {
				if (Array.isArray(n)) n.forEach(walk);
				else if (n && typeof n === 'object') {
					if (n.plan_type === 'clandestinely_liaise' && n.id === g.planID) found.push(n);
					Object.values(n).forEach(walk);
				}
			};
			walk(await res.json());
			expect(found.length, 'the liaison is on the record').toBeGreaterThan(0);
			const rd = typeof found[0].resolution_data === 'string' ? JSON.parse(found[0].resolution_data) : found[0].resolution_data;
			liaise = rd.liaise;
		}
		return (liaise.kept_secrets as { player_id: number; asset_id: number }[])
			.find(k => k.player_id === g.ids.alice)?.asset_id;
	};
	for (const site of ['list', 'one', 'record'] as const) {
		expect(await pickAt(site, 'alice'), `${site}: the picker sees her own pick`).toBe(aliceConfidant);
		expect(await pickAt(site, 'bob'), `${site}: the partner sees only that she has picked`).toBe(0);
		expect(await pickAt(site, 'carol'), `${site}: the bystander sees only that she has picked`).toBe(0);
	}

	// Bob's pick completes the phase; both picks become public like the rest of the plan.
	await keepSecretInPanel(bob, 'Bob Confidant');
	await expect(section(alice, 'Things we share')).toBeVisible({ timeout: 10_000 });
	await expect(section(bob, 'Things we share')).toBeVisible();
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible();
	for (const site of ['list', 'one', 'record'] as const) {
		expect(await pickAt(site, 'carol'), `${site}: unmasked once the phase moves on`).toBe(aliceConfidant);
	}

	// keep-secret broadcasts no secret.created, so alice's open page still counts no
	// secret on bob's Confidant and would offer her nothing to look at. Reload to get
	// past it; the live path is the fixme below (liaise-keep-secret-not-broadcast.md).
	await alice.reload();

	// Things We Share: alice reads bob's secret; bob leverages alice's Confidant instead.
	const aliceShare = await chooseShare(alice, "Look at partner's secrets");
	await pickCard(aliceShare, 'Bob Confidant');
	await submitShare(aliceShare);
	const bobShare = await chooseShare(bob, "Leverage partner's asset, bank a die");
	await pickCard(bobShare, 'Alice Confidant');
	await submitShare(bobShare);
	await expectLogged(g, 'looked at the secrets of', 'carol');

	// Alice may now read bob's secret; bob still cannot read hers, and carol reads nothing.
	await expect.poll(async () => (await readableSecrets(g, 'alice', bobConfidant)).length).toBe(1);
	expect((await readableSecrets(g, 'alice', bobConfidant))[0]).toContain(SECRET_PREFIX);
	expect(await readableSecrets(g, 'bob', aliceConfidant), 'bob did not choose to look').toEqual([]);
	expect(await readableSecrets(g, 'carol', aliceConfidant)).toEqual([]);
	expect(await readableSecrets(g, 'carol', bobConfidant)).toEqual([]);
	const visible = await g.ctx.carol.request.get(`/api/tables/${g.gameID}/secrets/visible`);
	expect(visible.status()).toBe(200);
	expect(await visible.text(), 'the bystander has no visible secrets').not.toContain(SECRET_PREFIX);

	// Nothing the bystander can read anywhere carries the secret's text.
	expect(await logText(g, 'carol'), 'action log').not.toContain(SECRET_PREFIX);
	expect(JSON.stringify(await fetchPlans(g, 'carol')), 'plans').not.toContain(SECRET_PREFIX);
	expect(carolFrames.join('\n'), 'WebSocket frames').not.toContain(SECRET_PREFIX);
	await expect(carol.locator('body')).not.toContainText(SECRET_PREFIX);
});

test('outcomes: break and gift land, both parting 0 resolves a make, and the bystander only watches', async ({ browser }) => {
	const g = await seedLiaise(browser, 'share');
	track(g.gameID);
	const { alice, bob, carol } = await openAll(g);
	const artifact = (await fetchAssets(g)).find(a => a.name === "alice's artifact")!;
	expect(artifact.owner_id).toBe(g.ids.alice);

	// The bystander watches: a wait line and no controls.
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible({ timeout: 10_000 });
	await expect(carol.getByRole('button', { name: 'Submit share choice' })).toHaveCount(0);
	await expect(section(alice, 'Things we share')).toBeVisible();

	// Alice breaks bob's meeting peer; her only marginalia there is its last, so she is warned.
	const aliceShare = await chooseShare(alice, "Break partner's meeting peer");
	await expect(aliceShare.getByText('Break Bob Confidant')).toBeVisible();
	await pickCard(aliceShare, 'Bob Confidant');
	await expect(aliceShare.locator('.res-warning')).toBeVisible();
	await submitShare(aliceShare);
	await expect(alice.getByText("You've submitted. Waiting for your partner")).toBeVisible();
	await expect(waitingBar(bob)).toHaveText(/Waiting On:\s*You\s*$/);
	await expect(waitingBar(carol)).toHaveText(/Waiting On:\s*bob\s*$/);
	// Picks stay hidden until both are in.
	expect((await asset(g, g.peers.bob, 'carol')).is_destroyed, 'nothing applied before both picks').toBe(false);

	// Bob takes a gift: only non-peer assets of alice's are on offer.
	const bobShare = await chooseShare(bob, 'Take a gift from partner');
	await expect(bobShare.locator('.card', { hasText: "alice's artifact" })).toBeVisible();
	await expect(bobShare.locator('.card', { hasText: 'Alice Confidant' })).toHaveCount(0);
	await pickCard(bobShare, "alice's artifact");
	await submitShare(bobShare);

	// Both picks apply together and the liaison moves on to the parting question.
	for (const page of [alice, bob]) {
		await expect(section(page, 'When will I see you again?')).toBeVisible({ timeout: 10_000 });
	}
	await expect.poll(async () => (await asset(g, g.peers.bob, 'carol')).is_destroyed).toBe(true);
	expect((await asset(g, artifact.id, 'carol')).owner_id, 'the gift changed hands').toBe(g.ids.bob);
	expect((await asset(g, g.peers.alice, 'carol')).is_destroyed, "alice's own Confidant is untouched").toBe(false);
	await expectLogged(g, 'from their partner as a gift.', 'carol');
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible();

	// Both reveal 0: no follow-up, and the plan resolves as a make by itself.
	for (const page of [alice, bob]) {
		await page.getByRole('button', { name: 'Pick 0' }).click();
		await page.getByRole('button', { name: 'Submit', exact: true }).click();
	}
	await expect.poll(async () => (await fetchPlan(g, 'carol')).status, { timeout: 10_000 }).toBe('resolved');
	const plan = await fetchPlan(g, 'carol');
	expect(plan.result).toBe('make');
	expect((await liaiseData(g, 'carol')).phase).toBe('done');
	expect((await fetchPlans(g)).filter(p => p.plan_type === 'clandestinely_liaise'), 'no follow-up scheduled').toHaveLength(1);

	// The log tells it in order: parting, then the shared resolution line.
	await expectLogged(g, 'The clandestine meeting came to an end.', 'carol');
	const log = await logBodies(g, 'carol');
	const parted = log.findIndex(b => b.includes('The pair parted ways with no future meeting planned.'));
	const ended = log.findIndex(b => b.includes('The clandestine meeting came to an end.'));
	expect(parted, 'the parting line is logged').toBeGreaterThanOrEqual(0);
	expect(ended, 'the resolution line follows it').toBeGreaterThan(parted);
	expect(log.some(b => b.includes('Clandestinely Liaise succeeded')), 'the custom line replaces the tautology').toBe(false);

	// Nobody is left owing the liaison anything; the bystander's wait line is gone.
	await expect(carol.getByText('A clandestine liaison is underway.')).toHaveCount(0);
	await expect(alice.getByText('When will I see you again?')).toHaveCount(0);
});

test('outcomes: update rewrites the meeting peer, and a parting delay schedules the next meeting', async ({ browser }) => {
	const g = await seedLiaise(browser, 'share');
	track(g.gameID);
	const { alice, carol } = await openAll(g);
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible({ timeout: 10_000 });

	const aliceShare = await chooseShare(alice, "Update partner's meeting peer");
	await pickCard(aliceShare, 'Bob Confidant');
	// Submit stays shut until the replacement text is written.
	await expect(aliceShare.getByRole('button', { name: 'Submit share choice' })).toBeDisabled();
	await aliceShare.getByPlaceholder('The rewritten marginalia…').fill('owes the crown a favour');
	await submitShare(aliceShare);
	await post(g, 'bob', `/api/plans/${g.planID}/share-choice`, {
		choice: 'look_at_secret', target_asset_id: g.peers.alice,
	}, 200);

	await expect(section(alice, 'When will I see you again?')).toBeVisible({ timeout: 10_000 });
	const bobPeer = await asset(g, g.peers.bob, 'carol');
	expect(bobPeer.marginalia.map(m => m.text)).toEqual(['owes the crown a favour']);
	expect(bobPeer.is_destroyed).toBe(false);
	await expectLogged(g, "rewrote a marginalia on their partner's peer", 'carol');

	// Faces 1 and 2 → ceil(1.5) = 2 rows after the row the liaison is resolving on.
	const row = await currentRow(g);
	const planBefore = await fetchPlans(g);
	await post(g, 'alice', `/api/plans/${g.planID}/redelay-reveal`, { face: 1 }, 200);
	await post(g, 'bob', `/api/plans/${g.planID}/redelay-reveal`, { face: 2 }, 200);
	await expect.poll(async () => (await fetchPlan(g, 'carol')).status, { timeout: 10_000 }).toBe('resolved');
	expect((await fetchPlan(g, 'carol')).result).toBe('make');

	const liaisons = (await fetchPlans(g, 'carol')).filter(p => p.plan_type === 'clandestinely_liaise');
	expect(planBefore.filter(p => p.plan_type === 'clandestinely_liaise')).toHaveLength(1);
	expect(liaisons, 'a second meeting is on the record').toHaveLength(2);
	const next = liaisons.find(p => p.id !== g.planID)!;
	expect(next.status).toBe('pending');
	expect(next.row_number).toBe(row + 2);
	await expectLogged(g, 'The pair arranged to meet again.', 'carol');
	await expectLogged(g, 'The clandestine meeting came to an end.', 'carol');
});

test('guards: only the pair act, in order, once, and only on each other', async ({ browser }) => {
	const g = await seedLiaise(browser, 'together');
	track(g.gameID);
	const url = (route: string) => `/api/plans/${g.planID}/${route}`;
	const assets = await fetchAssets(g);
	const idOf = (name: string) => assets.find(a => a.name === name)!.id;

	// Together at Last: later steps are not open yet; only the preparer advances.
	await post(g, 'alice', url('keep-secret'), { asset_id: g.peers.alice }, 409);
	await post(g, 'alice', url('share-choice'), { choice: 'leverage_partner', target_asset_id: g.peers.bob }, 409);
	await post(g, 'alice', url('redelay-reveal'), { face: 1 }, 409);
	await post(g, 'bob', url('advance-liaise'), {}, 403);
	await post(g, 'carol', url('advance-liaise'), {}, 403);
	await post(g, 'alice', url('advance-liaise'), {}, 200);
	await post(g, 'alice', url('advance-liaise'), {}, 409); // not again: both secrets must be in first

	// Secrets We Keep: seat-holders only, their own assets only, once each.
	await post(g, 'carol', url('keep-secret'), { asset_id: idOf("carol's artifact") }, 403);
	await post(g, 'alice', url('keep-secret'), { asset_id: g.peers.bob }, 403);
	await post(g, 'alice', url('keep-secret'), {}, 400);
	await post(g, 'alice', url('share-choice'), { choice: 'leverage_partner', target_asset_id: g.peers.bob }, 409);
	await post(g, 'alice', url('keep-secret'), { asset_id: idOf("alice's resource") }, 200);
	await post(g, 'alice', url('keep-secret'), { asset_id: g.peers.alice }, 409);
	expect((await asset(g, g.peers.alice, 'carol')).secret_count, 'the refused duplicate wrote nothing').toBe(0);
	expect((await asset(g, idOf("alice's resource"), 'carol')).secret_count).toBe(1);
	await post(g, 'bob', url('keep-secret'), { asset_id: g.peers.bob }, 200);

	// Things We Share: a spectator cannot pick; every option aims at the partner's assets.
	const share = (who: Who, data: Record<string, unknown>, status: number) =>
		post(g, who, url('share-choice'), data, status);
	await share('carol', { choice: 'leverage_partner', target_asset_id: g.peers.bob }, 403);
	await share('alice', { choice: 'bogus' }, 400);
	await share('alice', { choice: 'leverage_partner' }, 400);
	await share('alice', { choice: 'leverage_partner', target_asset_id: idOf("alice's artifact") }, 403);
	await share('alice', { choice: 'leverage_partner', target_asset_id: idOf("carol's artifact") }, 403);
	await share('alice', { choice: 'take_gift', target_asset_id: g.peers.bob }, 400); // a gift is not a peer
	await share('alice', { choice: 'break_peer', target_asset_id: idOf("bob's artifact") }, 400); // not a peer
	await share('alice', { choice: 'break_peer', target_asset_id: idOf("bob's main character"), target_marginalia_id: 1 }, 400); // not the meeting peer
	await share('alice', { choice: 'break_peer', target_asset_id: g.peers.bob }, 400); // needs a marginalia
	const bobConfidantMarg = (await asset(g, g.peers.bob)).marginalia[0].id;
	await share('alice', { choice: 'update_peer', target_asset_id: g.peers.bob, target_marginalia_id: bobConfidantMarg }, 400); // needs text
	await share('alice', { choice: 'leverage_partner', target_asset_id: idOf("bob's holding") }, 200);

	// The pair's second pick completes the phase; the picks are final from then on, and
	// the parting reveal is theirs alone.
	await share('bob', { choice: 'leverage_partner', target_asset_id: idOf("alice's holding") }, 200);
	await share('alice', { choice: 'leverage_partner', target_asset_id: idOf("bob's artifact") }, 409);
	await post(g, 'carol', url('redelay-reveal'), { face: 1 }, 403);
	await post(g, 'alice', url('redelay-reveal'), { face: 7 }, 400);
	await post(g, 'alice', url('redelay-reveal'), { face: 3 }, 200);
	await post(g, 'bob', url('redelay-reveal'), { face: 3 }, 200);
	expect((await fetchPlan(g, 'carol')).status).toBe('resolved');
	await post(g, 'alice', url('redelay-reveal'), { face: 3 }, 409); // nothing left to reveal
});

// Pinned for liaise-keep-secret-not-broadcast.md: keep-secret writes the secret
// directly and, unlike the plain write-secret route, broadcasts no secret.created,
// so the other player's open page never learns the asset now bears a secret.
test.fixme('secret meeting: the partner can look at a freshly kept secret without reloading', async ({ browser }) => {
	const g = await seedLiaise(browser, 'keep');
	track(g.gameID);
	const { alice } = await openAll(g);
	await expect(section(alice, 'Secrets we keep')).toBeVisible({ timeout: 10_000 });

	await post(g, 'bob', `/api/plans/${g.planID}/keep-secret`, { asset_id: g.peers.bob }, 200);
	await post(g, 'alice', `/api/plans/${g.planID}/keep-secret`, { asset_id: g.peers.alice }, 200);
	await expect(section(alice, 'Things we share')).toBeVisible({ timeout: 10_000 });

	const sec = await chooseShare(alice, "Look at partner's secrets");
	await expect(sec.locator('.card', { hasText: 'Bob Confidant' })).toBeVisible();
});

test('parting: one player revealing 0 cancels the follow-up for both', async ({ browser }) => {
	const g = await seedLiaise(browser, 'redelay');
	track(g.gameID);
	const { carol } = await openAll(g);
	await expect(section(carol, 'When will I see you again?')).toHaveCount(0); // a bystander is shown no reveal
	await expect(carol.getByText('A clandestine liaison is underway.')).toBeVisible({ timeout: 10_000 });

	await post(g, 'alice', `/api/plans/${g.planID}/redelay-reveal`, { face: 3 }, 200);
	expect((await fetchPlan(g, 'carol')).status, 'one face in: still open').toBe('resolving');
	await post(g, 'bob', `/api/plans/${g.planID}/redelay-reveal`, { face: 0 }, 200);

	await expect.poll(async () => (await fetchPlan(g, 'carol')).status, { timeout: 10_000 }).toBe('resolved');
	expect((await fetchPlan(g, 'carol')).result).toBe('make');
	expect((await fetchPlans(g)).filter(p => p.plan_type === 'clandestinely_liaise'), 'no follow-up').toHaveLength(1);
	await expectLogged(g, 'The pair parted ways with no future meeting planned.', 'carol');
	expect(await logText(g, 'carol')).not.toContain('The pair arranged to meet again.');
	await expectLogged(g, 'The clandestine meeting came to an end.', 'carol');
});

// Make Demands' perform_steps winner stands in for the preparer on the Liaise panel
// and the preparer is locked out (the audit's Liaise fix, 031e43c). Alice prepares a
// liaison with carol and bob demands against it, so bob is the winner, alice the
// locked-out preparer and carol the partner. The retarget winner's wait on a Liaise
// is an open question (questions/make-demands-retarget-wait-duel-liaise.md) and is
// not asserted.
//
// Pinned for make-demands-on-liaise-never-rolls.md: the demand never gets its roll
// (a liaison's difficulty is 0), so the test stops at rollDemand. With that one
// failure patched out locally the rest of the body passes, and fails if the
// stand-in seat is removed. Un-fix it when the roll is fixed.
test.fixme('perform_steps: the demand winner acts for the preparer, who is locked out of every step', async ({ browser }) => {
	// Alice outranks bob on the power track, as in the Make Demands specs: a demand's
	// difficulty is its target's plus the rank gap, and a liaison's own is 0.
	const g = await seedLiaise(browser, 'none', undefined, { current_row: 1, rankings: { power: [1, 0, 2], knowledge: [0, 1, 2] } });
	track(g.gameID);
	// The partner is carol this time, so she needs a meeting peer (bob's is unused).
	const res = await g.ctx.carol.request.post(`/api/tables/${g.gameID}/assets`, {
		data: { asset_type: 'peer', name: 'Carol Confidant', is_main_character: false, marginalia: ['trusted go-between'] },
	});
	expect(res.ok(), `create peer failed: ${await res.text()}`).toBeTruthy();
	const carolPeer: number = (await res.json()).asset.id;
	g.planID = await prepareLiaison(g, { target_player_id: g.ids.carol, partner_peer_id: carolPeer });
	const revealID = (await liaiseData(g)).delay_reveal_id as number;
	// Faces 4 and 4: the liaison lands on row 5, as in the Make Demands specs.
	await post(g, 'alice', `/api/reveals/${revealID}/submit`, { face: 4 }, 200);
	await post(g, 'carol', `/api/reveals/${revealID}/submit`, { face: 4 }, 200);

	const f: H.MDFixture = {
		gameID: g.gameID,
		api: { alice: g.ctx.alice.request, bob: g.ctx.bob.request, carol: g.ctx.carol.request },
		ids: g.ids,
		targetID: g.planID,
		demandID: 0,
		row: (await fetchPlan(g)).row_number!,
	};
	await H.prepareDemand(f);
	await H.reachRow(f).catch(() => {});
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['perform_steps', 'control_leverage'], alice: ['keep_assets', 'keep_or_change_target'] });
	await H.completeDemand(f);
	await H.startTarget(f);
	await expect.poll(async () => (await fetchPlan(g)).status, { message: 'the liaison is resolving' }).toBe('resolving');
	await post(g, 'alice', `/api/plans/${g.planID}/advance-liaise`, {}, 200);

	const { alice, bob, carol } = await openAll(g);
	const url = (route: string) => `/api/plans/${g.planID}/${route}`;
	const bobSec = section(bob, 'Secrets we keep');
	await expect(bobSec).toBeVisible({ timeout: 10_000 });

	// The winner is offered alice's side; alice is told who nominates for her and gets no form.
	await expect(bobSec.getByText("You hold alice's side of this meeting")).toBeVisible();
	await expect(section(alice, 'Secrets we keep').getByText('bob is nominating on your behalf.')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Keep this secret' })).toHaveCount(0);
	await expect(section(carol, 'Secrets we keep').getByRole('button', { name: 'Keep this secret' })).toBeVisible();
	await post(g, 'alice', url('keep-secret'), { asset_id: g.peers.alice }, 403);

	// The partner nominates; the table now waits on whoever holds the preparer's seat, not alice.
	await post(g, 'carol', url('keep-secret'), { asset_id: carolPeer }, 200);
	await expect(waitingBar(alice)).toHaveText(/Waiting On:\s*bob\s*$/);
	await expect(waitingBar(carol)).toHaveText(/Waiting On:\s*bob\s*$/);

	// Bob nominates one of alice's assets, recorded under alice's seat.
	await keepSecretInPanel(bob, 'Alice Confidant');
	await expect(section(alice, 'Things we share')).toBeVisible({ timeout: 10_000 });
	expect((await liaiseData(g, 'alice')).kept_secrets.map((k: { player_id: number }) => k.player_id).sort())
		.toEqual([g.ids.alice, g.ids.carol].sort());
	expect((await asset(g, g.peers.alice, 'carol')).secret_count, "the secret sits on alice's asset").toBe(1);

	// Things We Share: the winner picks for alice's seat, aimed at alice's partner; alice cannot.
	await expect(section(alice, 'Things we share').getByText('bob is making your pick for you.')).toBeVisible();
	await post(g, 'alice', url('share-choice'), { choice: 'leverage_partner', target_asset_id: carolPeer }, 403);
	const bobShare = await chooseShare(bob, "Leverage partner's asset, bank a die");
	await pickCard(bobShare, 'Carol Confidant');
	await submitShare(bobShare);
	await post(g, 'carol', url('share-choice'), { choice: 'leverage_partner', target_asset_id: g.peers.alice }, 200);
	await expect(section(bob, 'When will I see you again?')).toBeVisible({ timeout: 10_000 });

	// The parting reveal is the winner's for alice's seat too; the liaison then resolves as a make.
	await post(g, 'alice', url('redelay-reveal'), { face: 0 }, 403);
	await post(g, 'bob', url('redelay-reveal'), { face: 0 }, 200);
	await post(g, 'carol', url('redelay-reveal'), { face: 0 }, 200);
	await expect.poll(async () => (await fetchPlan(g, 'alice')).status, { timeout: 10_000 }).toBe('resolved');
	expect((await fetchPlan(g, 'alice')).result).toBe('make');
});
