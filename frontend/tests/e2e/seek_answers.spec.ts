import { test, expect, request as pwRequest, type APIRequestContext, type BrowserContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';
import { E2E } from './baseUrl';
import * as sa from './helpers/seek_answers';
import * as H from './helpers/make_demands';

// End-to-end for Seek Answers — specifically the "ask a player a question"
// sub-flow, which had the same soft-lock bug class as Propose Duel and
// Clandestinely Liaise: a two-sided step where one player's action wasn't
// pushed to the other, leaving the waiting party stuck on a stale panel.
//
// The question text lives in the plan's resolution_data. When the preparer
// asked, the handler only broadcast row_state — which the client uses solely
// to update the "Waiting On" bar, never to refetch the plan — so the target's
// answer/veto UI never appeared until a manual page reload. The fix has
// ask/veto/answer also broadcast plan.choice_applied (the standard cross-client
// "refetch this plan" nudge); this spec guards all three legs of that loop.
//
// As with the duel/liaise specs, the *setup* (prep, fast-forwarding the record
// to the plan's row, casting + resolving the dice, and committing the make/mar
// choice) is driven over the API — it isn't the subject and driving it through
// the UI would dwarf the part that matters. The interactive, bug-prone part —
// preparer asks → target answers/vetoes, each leg reaching the other side live
// with no reload — is driven entirely through the rendered panels for both
// players. Knowledge ranks are seeded so the target (bob) outranks the preparer
// (alice), which is what makes the first question vetoable.

interface PlanRow {
	id: number;
	plan_type: string;
	row_number: number | null;
	status: string;
}

interface RollRow {
	id: number;
	result: number | null;
	outcome: 'make' | 'mar' | null;
	stage?: string;
}

async function fetchSeekPlan(ctx: APIRequestContext, gameID: number): Promise<PlanRow> {
	const res = await ctx.get(`/api/tables/${gameID}/plans`);
	expect(res.ok(), `GET plans failed: ${await res.text()}`).toBeTruthy();
	const { plans } = (await res.json()) as { plans: PlanRow[] };
	const sa = plans.find(p => p.plan_type === 'seek_answers');
	expect(sa, 'seek_answers plan not found').toBeTruthy();
	return sa!;
}

// The preparer's ask form and the "remaining" sub-flow steps render as plain
// .plan-form blocks (no .choices-section wrapper, unlike the duel's pickers),
// so scope by the step's header text.
function planForm(page: Page, header: string) {
	return page.locator('.plan-form', { hasText: header });
}

interface AssetRow {
	id: number;
	owner_id: number;
	asset_type: string;
	is_destroyed: boolean;
	marginalia: { id: number; is_torn: boolean }[];
}

/**
 * Break as many of the preparer's own resources as the mar penalty demands, so
 * CanComplete stops blocking. A no-op on a make (the required count is 0). Each
 * resource may only be flawed once, and the server caps the requirement at the
 * eligible count, so one pass over the list is always enough.
 */
async function dischargeMarSelfFlaws(ctx: APIRequestContext, gameID: number, planID: number) {
	const planRes = await ctx.get(`/api/plans/${planID}`);
	expect(planRes.ok(), `GET plan failed: ${await planRes.text()}`).toBeTruthy();
	const { plan, resolution_data } = (await planRes.json()) as {
		plan: { preparer_id: number };
		resolution_data?: { seek_answers?: { mar_self_flaws_required?: number } };
	};
	const required = resolution_data?.seek_answers?.mar_self_flaws_required ?? 0;
	if (required === 0) return;

	const assetsRes = await ctx.get(`/api/tables/${gameID}/assets`);
	expect(assetsRes.ok(), `GET assets failed: ${await assetsRes.text()}`).toBeTruthy();
	const { assets } = (await assetsRes.json()) as { assets: AssetRow[] };
	const own = assets.filter(
		a => a.owner_id === plan.preparer_id && a.asset_type === 'resource' && !a.is_destroyed,
	);
	expect(own.length, 'the mar penalty is capped at the eligible count').toBeGreaterThanOrEqual(required);

	for (let i = 0; i < required; i++) {
		const intact = (own[i].marginalia ?? []).find(m => !m.is_torn);
		const res = await ctx.post(`/api/plans/${planID}/break-resource`, {
			// A blank resource names no marginalia — the break destroys it.
			data: { asset_id: own[i].id, marginalia_id: intact?.id ?? 0 },
		});
		expect(res.ok(), `self-flaw failed: ${await res.text()}`).toBeTruthy();
	}
}

const track = cleanupGameAfterEach();
H.cleanupAfterEach();

// Contexts are closed in an afterEach, so a failing assertion can't leak them.
const openContexts: BrowserContext[] = [];
test.afterEach(async () => {
	for (const c of openContexts.splice(0)) await c.close().catch(() => {});
});

test('seek answers: ask → veto → re-ask → answer each reach the other side live', async ({ browser }) => {
	// ── Seed a main_event game with alice (preparer) + bob (target) ──────────
	// The Seek Answers plan is seeded directly onto the current row so no
	// advance-row is needed: advancing fires the engrailed ranking update, which
	// (because alice holds a knowledge plan token) would promote alice to the top
	// of the knowledge track and defeat the rank setup below.
	//
	// Knowledge ranks: bob at rank 1, alice at rank 2 (lower number = higher
	// status), so bob outranks alice and may veto the first question. Difficulty
	// is alice's knowledge rank (2), but make/mar doesn't matter here — the option
	// list is identical and ask_question needs no asset target.
	const fixture = await pwRequest.newContext({ baseURL: E2E });
	const seedRes = await fixture.post('/api/dev/seed', {
		data: {
			phase: 'main_event',
			players: ['alice', 'bob'],
			current_row: 5,
			rankings: { knowledge: [1, 0] },
			plans: [{ preparer_idx: 0, plan_type: 'seek_answers', category: 'knowledge', row: 5, row_order: 0 }],
			// This spec asserts the blank-asset break branch (a resource with no
			// marginalia is destroyed outright), so it opts out of the seed's
			// default note. Everything else gets one, so break and tear
			// sub-flows have something to act on.
			starting_marginalia: false,
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id } = await seedRes.json();
	track(game_id);
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E });
	const bobCtx = await browser.newContext({ baseURL: E2E });
	openContexts.push(aliceCtx, bobCtx);
	await aliceCtx.request.post('/api/dev/login?username=alice');
	await bobCtx.request.post('/api/dev/login?username=bob');

	const planID = (await fetchSeekPlan(aliceCtx.request, game_id)).id;

	// ── Kick off resolution. Seek Answers opens the pre-roll step with no dice. ─
	const resolveRes = await aliceCtx.request.post(`/api/plans/${planID}/resolve`);
	expect(resolveRes.ok(), `resolve failed: ${await resolveRes.text()}`).toBeTruthy();

	// ── Pre-roll: alice restates her methods and casts the dice ──────────────
	const castRes = await aliceCtx.request.post(`/api/plans/${planID}/seek-cast-roll`, {
		data: { narration: 'I cross-referenced the ledgers and the seal looks forged.' },
	});
	expect(castRes.ok(), `cast-roll failed: ${await castRes.text()}`).toBeTruthy();
	const { roll: castRoll } = (await castRes.json()) as { roll: RollRow };

	// Resolve the roll over the API. Skipping the difficulty vote advances to the
	// leverage stage and, when nothing is leverageable, short-circuits straight to
	// a resolved roll. If it's still open (leverage stage), both participants ready
	// up without committing; the last ready triggers auto-resolution server-side.
	const skipRes = await aliceCtx.request.post(`/api/rolls/${castRoll.id}/skip-vote`);
	expect(skipRes.ok(), `skip-vote failed: ${await skipRes.text()}`).toBeTruthy();

	const readActiveRoll = async (): Promise<RollRow> => {
		const res = await aliceCtx.request.get(`/api/tables/${game_id}/rolls/active`);
		expect(res.ok(), `get active roll failed: ${await res.text()}`).toBeTruthy();
		return (await res.json()).roll as RollRow;
	};

	let roll = await readActiveRoll();
	// Ready up any still-unready participant until the roll resolves. Readying the
	// last one auto-resolves server-side, so a participant whose ready lands after
	// resolution gets a harmless "not leverage stage" — tolerate it and stop.
	for (const ctx of [aliceCtx, bobCtx]) {
		if (roll.result != null) break;
		const readyRes = await ctx.request.post(`/api/rolls/${castRoll.id}/ready`, {
			data: { is_ready: true },
		});
		if (!readyRes.ok()) {
			expect(await readyRes.text(), `ready failed: ${await readyRes.text()}`).toContain('leverage stage');
		}
		roll = await readActiveRoll();
	}
	// result drives how many options must be picked (a 2-die roll yields 1 or 2
	// distinct faces); outcome must match the make-choice body.
	expect(roll?.result, 'roll did not resolve').toBeTruthy();
	const result = roll.result!;
	const outcome = roll.outcome!;

	// Commit exactly one ask_question pick; fill any remaining picks (result is
	// 1 or 2) with declare_truth so the only sub-flow left for the UI is the
	// question we're testing.
	const choices = ['ask_question', ...Array(result - 1).fill('declare_truth')];
	const mcRes = await aliceCtx.request.post(`/api/plans/${planID}/make-choice`, {
		data: { result: outcome, choices },
	});
	expect(mcRes.ok(), `make-choice failed: ${await mcRes.text()}`).toBeTruthy();
	for (let i = 0; i < result - 1; i++) {
		const dtRes = await aliceCtx.request.post(`/api/plans/${planID}/declare-truth`, {
			data: { text: `An incidental truth (${i + 1}).` },
		});
		expect(dtRes.ok(), `declare-truth failed: ${await dtRes.text()}`).toBeTruthy();
	}
	// The dice are real, so this can land on a mar — which owes alice
	// (difficulty − result) self-flaws on her own resources before the plan may
	// complete. Discharge them over the API too; only the question loop below is
	// the subject. Her seeded resources are blank, which is a valid break target
	// (breaking destroys them outright — DRAFT_PEERS_AND_BLANK_ASSETS_PLAN D3),
	// so break-resource is posted with no marginalia_id.
	await dischargeMarSelfFlaws(aliceCtx.request, game_id, planID);

	// ── Open both players' tables; the resolving Seek Answers panel renders ───
	const alice = await aliceCtx.newPage();
	const bob = await bobCtx.newPage();
	await Promise.all([
		alice.goto(`/table/${game_id}`),
		bob.goto(`/table/${game_id}`),
	]);

	// Alice sees the ask form; bob waits on her.
	const askForm = planForm(alice, 'Ask a player a question');
	await expect(askForm).toBeVisible({ timeout: 10_000 });

	// ── Leg 1: alice asks bob. The question must reach bob's open page live. ──
	const q1 = 'Where were you on the night of the masque?';
	await askForm.getByRole('button', { name: 'bob' }).click();
	await askForm.locator('textarea').fill(q1);
	await askForm.getByRole('button', { name: 'Ask question' }).click();

	// Bob's answer/veto panel appears without a reload (the core regression).
	await expect(bob.getByText('alice asks you a question')).toBeVisible({ timeout: 10_000 });
	await expect(bob.getByText(q1)).toBeVisible();
	// `exact` to avoid matching the "Seek Answers" plan chip in the record rail.
	await expect(bob.getByRole('button', { name: 'Answer', exact: true })).toBeVisible();
	const vetoBtn = bob.getByRole('button', { name: 'Veto (ask another)' });
	await expect(vetoBtn).toBeVisible();
	// Alice's own panel flips to the waiting state.
	await expect(alice.getByText(/Waiting for bob to answer/)).toBeVisible();

	// ── Leg 2: bob vetoes. Alice must be re-prompted to ask again, live. ─────
	await vetoBtn.click();
	const reaskForm = planForm(alice, 'Ask a player a question');
	await expect(reaskForm.getByText(/Your last question was vetoed/)).toBeVisible({ timeout: 10_000 });

	// ── Leg 3: alice re-asks. The new question reaches bob live, and because
	//    the first formulation was already vetoed, this one is NOT vetoable. ──
	// The veto locks the target (book: "ask another in its stead"): the chip row
	// collapses to bob, pre-selected and not clickable, so there's nothing to pick.
	const bobChip = reaskForm.getByRole('button', { name: 'bob' });
	await expect(bobChip).toBeDisabled();
	await expect(reaskForm.locator('.player-chip')).toHaveCount(1);
	const q2 = 'Then who carried your seal that night?';
	await reaskForm.locator('textarea').fill(q2);
	await reaskForm.getByRole('button', { name: 'Ask question' }).click();

	await expect(bob.getByText(q2)).toBeVisible({ timeout: 10_000 });
	const bobAnswer = bob.getByRole('button', { name: 'Answer', exact: true });
	await expect(bobAnswer).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Veto (ask another)' })).toHaveCount(0);

	// ── Leg 4: bob answers. The answer was the last thing the plan owed, so it
	//    resolves itself live — alice is never handed a "Complete plan" button. ──
	await bob.getByPlaceholder('Answer the question…').fill('My steward, Crane — I never left the hall.');
	await bobAnswer.click();

	await expect.poll(async () => (await fetchSeekPlan(aliceCtx.request, game_id)).status, {
		timeout: 10_000,
	}).toBe('resolved');
	await expect(alice.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);

});

// ── What the 12-plan UX audit fixed in Seek Answers ──────────────────────────
//
//   - the Buffet ("What can happen?") is previewable by everyone before any dice,
//     and its mar copy says what a mar costs the preparer: one of their own
//     resources broken per point the roll fell short (#29)
//   - the make/mar picks run on the shared Stepper: capped per option at its live
//     target count, the exact total enforced, 44px targets (#29, audit finding 3)
//   - a mar committed with no picks is still a committed choice: the wait moves
//     on to the self-flaw penalty instead of falling back to "choosing options" (#27)
//   - hidden information: a secret the preparer learns never reaches a bystander,
//     through any response, the log, the live WebSocket, or the rendered page
//
// Setup (seed, forcing the dice, the sub-flow steps) goes over the API; the
// subject is what each player *sees and can do* in the rendered panels.

const SECRET = 'The ledger was burned by the steward, not the fire.';
const MESSAGE = 'I trace the seal back through three years of ledgers.';

test('seek answers pre-roll: preparer narrates, others wait, the Buffet is previewable by a bystander', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts);
	const { alice, bob, carol } = t;
	await sa.openTables(t);

	// ── Pre-roll: no dice yet. The preparer is asked; everyone else sees the same form greyed. ──
	await expect(alice.page.getByText('What have you learned?')).toBeVisible({ timeout: 10_000 });
	await expect(alice.page.getByRole('button', { name: 'Submit', exact: true })).toBeDisabled();
	for (const p of [bob, carol]) {
		await expect(p.page.getByText('alice is researching…')).toBeVisible({ timeout: 10_000 });
		await expect(p.page.getByPlaceholder('Restate your research methods')).toBeDisabled();
		await expect(p.page.getByRole('button', { name: 'Submit', exact: true })).toHaveCount(0);
	}

	// Only the preparer may cast the dice.
	const forged = await bob.ctx.request.post(`/api/plans/${t.planID}/seek-cast-roll`, { data: { narration: 'x' } });
	expect(forged.status(), 'only the preparer may cast').toBe(403);

	// ── The Buffet: collapsed by default, open to the bystander before any dice ──
	const toggle = carol.page.getByRole('button', { name: 'What can happen?' });
	await expect(toggle).toBeVisible();
	await expect(toggle).toHaveAttribute('aria-expanded', 'false');
	await toggle.click();
	await expect(toggle).toHaveAttribute('aria-expanded', 'true');
	await expect(carol.page.getByText('Pick options equal to the dice result (repeatable):')).toBeVisible();
	for (const opt of Object.values(sa.OPT)) {
		await expect(carol.page.getByText(opt).first()).toBeVisible();
	}
	// The mar tab carries the cost: the preparer's own resources, one per point short.
	await carol.page.getByRole('tab', { name: 'Mar' }).click();
	await expect(carol.page.getByText('the preparer also breaks one of their own resource assets for each point the roll fell short of the difficulty'))
		.toBeVisible();
	await expect(carol.page.getByRole('tab', { name: 'Mar' })).toHaveAttribute('aria-selected', 'true');

	// ── Alice casts from her panel; the narration reaches the log, the form goes away for all ──
	const roll = await sa.castFromUI(t, MESSAGE);
	expect(roll.result, 'dice are cast but not yet rolled out').toBeNull();
	for (const p of [alice, bob, carol]) {
		await expect(p.page.getByText('What have you learned?')).toHaveCount(0, { timeout: 10_000 });
	}
	await expect.poll(async () =>
		(await (await bob.ctx.request.get(`/api/tables/${t.gameID}/posts`)).text()).includes(MESSAGE),
		{ timeout: 10_000 }).toBe(true);
	// A second cast is refused once the dice exist.
	const again = await alice.ctx.request.post(`/api/plans/${t.planID}/seek-cast-roll`, { data: { narration: 'again' } });
	expect(again.status()).toBe(409);
});

test('seek answers mar: Stepper picks, the bystander sees the committed state, and no secret reaches them', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts);
	const { alice, bob, carol } = t;

	// Everything carol's open page receives over the WebSocket, so a secret leaked by a
	// broadcast is caught as well as one in a response.
	const carolFrames: string[] = [];
	carol.page.on('websocket', ws => ws.on('framereceived', f => carolFrames.push(String(f.payload))));
	await sa.openTables(t);

	const assets = await sa.listAssets(alice.ctx.request, t.gameID);
	const bobHolding = assets.find(a => a.asset_type === 'holding' && a.name.startsWith('bob'))!;
	const bobResource = assets.find(a => a.asset_type === 'resource' && a.name.startsWith('bob'))!;
	const aliceResource = assets.find(a => a.asset_type === 'resource' && a.name.startsWith('alice'))!;
	const carolResource = assets.find(a => a.asset_type === 'resource' && a.name.startsWith('carol'))!;

	// Bob writes a secret on his holding once every table is open, so the live event has
	// watchers. Existence is public; the text is not.
	const wrote = await bob.ctx.request.post(`/api/assets/${bobHolding.id}/secrets`, { data: { text: SECRET } });
	expect(wrote.ok(), `write secret: ${await wrote.text()}`).toBeTruthy();

	const roll = await sa.castFromUI(t, MESSAGE);
	expect(roll.difficulty).toBeGreaterThanOrEqual(3);
	// A mar at result 2: two picks, and the preparer owes (difficulty − 2) self-flaws.
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: 2 });

	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });
	await expect(alice.page.getByText('must also describe a flaw in (break) your own resource assets')).toBeVisible();
	// Bystanders get the same options greyed out, a wait line, and nothing to press.
	for (const p of [bob, carol]) {
		await expect(p.page.getByText('alice is choosing make/mar options…')).toBeVisible({ timeout: 10_000 });
		await expect(p.page.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
		await expect(sa.more(p.page, sa.OPT.declareTruth)).toBeDisabled();
		await expect(p.page.getByText('alice must also describe a flaw')).toBeVisible();
	}

	// ── The shared Stepper: capped per option, exact total, 44px targets ──
	const a = alice.page;
	const apply = a.getByRole('button', { name: 'Apply choices' });
	const total = a.locator('.choices-note', { hasText: 'Total picks:' });
	await expect(apply).toBeDisabled();
	await expect(total).toContainText('0 / 2');
	const box = await sa.more(a, sa.OPT.declareTruth).boundingBox();
	expect(box!.width).toBeGreaterThanOrEqual(44);
	expect(box!.height).toBeGreaterThanOrEqual(44);

	// Only bob's holding carries a secret alice can't read, so "learn the secrets" has one
	// target; a second pick is shut with the reason, though the budget has room.
	await sa.more(a, sa.OPT.revealSecret).click();
	await expect(total).toContainText('1 / 2');
	await expect(sa.more(a, sa.OPT.revealSecret)).toBeDisabled();
	await expect(a.getByText('all targets picked')).toBeVisible();
	await expect(apply).toBeDisabled();
	// A declare-truth is uncapped, so it can fill the budget; then every + shuts.
	await sa.more(a, sa.OPT.declareTruth).click();
	await expect(total).toContainText('2 / 2');
	await expect(sa.more(a, sa.OPT.declareTruth)).toBeDisabled();
	await expect(sa.more(a, sa.OPT.askQuestion)).toBeDisabled();
	await expect(apply).toBeEnabled();
	// Stepping down reopens it, and swapping a pick for another keeps the total exact.
	await sa.fewer(a, sa.OPT.declareTruth).click();
	await expect(total).toContainText('1 / 2');
	await expect(apply).toBeDisabled();
	// Breaking a resource has two targets (bob's and carol's: never the preparer's own on a mar).
	await sa.more(a, sa.OPT.breakResource).click();
	await expect(total).toContainText('2 / 2');
	await expect(apply).toBeEnabled();

	await apply.click();

	// ── Committed: the picks persist in the panel for every viewer; the wait moves on ──
	// "Choices applied:" exists only once the commit is made (the greyed picker never shows it).
	for (const p of [alice, bob, carol]) {
		const applied = p.page.locator('.choices-summary', { hasText: 'Choices applied:' });
		await expect(applied).toBeVisible({ timeout: 10_000 });
		await expect(applied.getByText(`${sa.OPT.revealSecret} × 1`)).toBeVisible();
		await expect(applied.getByText(`${sa.OPT.breakResource} × 1`)).toBeVisible();
		await expect(p.page.locator('.choices-note', { hasText: 'Total picks:' })).toHaveCount(0);
	}
	for (const p of [bob, carol]) {
		await expect(p.page.getByText('alice is resolving Seek Answers…')).toBeVisible();
		await expect(p.page.getByText('alice is choosing make/mar options…')).toHaveCount(0);
		await expect(p.page.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
		await expect(p.page.getByRole('button', { name: 'Reveal secrets' })).toHaveCount(0);
	}
	await expect(alice.page.getByRole('button', { name: 'Reveal secrets' })).toBeVisible();
	const plan = await sa.getPlan(alice.ctx.request, t.planID);
	// Alice owns one resource, so a penalty of (difficulty − 2) is capped at one (ruled: correct as built).
	expect(plan.resolution_data?.seek_answers?.mar_self_flaws_required, 'penalty capped at the one eligible resource')
		.toBe(Math.min(roll.difficulty - 2, 1));

	// ── The steps, in the order the panel lists them ──
	const reveal = await alice.ctx.request.post(`/api/plans/${t.planID}/reveal-secret`, { data: { asset_id: bobHolding.id } });
	expect(reveal.ok(), `reveal-secret: ${await reveal.text()}`).toBeTruthy();
	// Learned by alice: she can read it now, and the table is told which asset, not what.
	await expect.poll(async () => (await alice.ctx.request.get(`/api/assets/${bobHolding.id}/secrets`)).text(),
		{ timeout: 10_000 }).toContain(SECRET);
	const brk = await alice.ctx.request.post(`/api/plans/${t.planID}/break-resource`, {
		data: { asset_id: bobResource.id, marginalia_id: bobResource.marginalia[0].id },
	});
	expect(brk.ok(), `break-resource: ${await brk.text()}`).toBeTruthy();

	// A bystander's open panel hears about these steps live: the "Resolved so far" list
	// names the asset to every viewer, and never what was written under it.
	for (const p of [bob, carol]) {
		const list = p.page.locator('.resolved-so-far');
		await expect(list.getByText("Learned the secrets of bob's holding")).toBeVisible({ timeout: 10_000 });
		// The destroyed resource stays in the live asset list (flagged), so the line
		// names it, as a reload does.
		await expect(list.getByText(`Broke ${bobResource.name}`)).toBeVisible();
	}

	// The penalty is the one thing left owed: not resolved, alice is the one waited on.
	expect(await sa.planStatus(alice.ctx.request, t.planID)).toBe('resolving');
	await expect(carol.page.getByText('alice is resolving Seek Answers…')).toBeVisible();
	await expect(a.getByText('Describe a flaw in your own resource assets')).toBeVisible();
	await expect(a.getByText('The plan marred by 1')).toBeVisible();
	const pen = await alice.ctx.request.post(`/api/plans/${t.planID}/break-resource`, {
		data: { asset_id: aliceResource.id, marginalia_id: aliceResource.marginalia[0].id },
	});
	expect(pen.ok(), `penalty: ${await pen.text()}`).toBeTruthy();

	// Nothing else is owed: the plan resolves itself, with no "Complete plan" to click.
	await expect.poll(() => sa.planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	await expect(a.getByRole('button', { name: 'Complete plan' })).toHaveCount(0);
	const after = await sa.listAssets(alice.ctx.request, t.gameID);
	const broken = (id: number) => {
		const x = after.find(y => y.id === id)!;
		return x.is_destroyed || x.marginalia[0].is_torn;
	};
	expect(broken(bobResource.id), "bob's resource broken").toBe(true);
	expect(broken(aliceResource.id), "alice's own resource broken").toBe(true);
	expect(broken(carolResource.id), "carol's resource, never picked, is untouched").toBe(false);

	// ── Hidden information ──
	const aliceSees = await sa.everythingVisibleTo(alice.ctx.request, t, [bobHolding.id]);
	expect(aliceSees.includes(SECRET), 'the learner reads the secret').toBe(true);
	const carolSees = await sa.everythingVisibleTo(carol.ctx.request, t, [bobHolding.id]);
	// The leak paths were exercised: the secret's creation and the grant to alice both
	// went out over carol's socket, as events without the text.
	expect(carolFrames.some(f => f.includes('secret.created')), 'secret.created reached carol').toBe(true);
	expect(carolFrames.some(f => f.includes('secret.visibility_grant')), 'the grant reached carol').toBe(true);
	// carol may read the secrets endpoint but gets none: the exact status and an empty list.
	const carolRead = await carol.ctx.request.get(`/api/assets/${bobHolding.id}/secrets`);
	expect(carolRead.status()).toBe(200);
	expect((await carolRead.json()).secrets).toEqual([]);
	// (booleans, so a failure doesn't dump every response into the report)
	expect(carolFrames.join('\n').includes(SECRET), 'secret text broadcast to a bystander').toBe(false);
	expect(carolSees.includes(SECRET), 'secret text reached a bystander').toBe(false);
	expect(carolSees, "the secret's existence is public").toContain('"secret_count":1');
	await carol.page.reload();
	// The reloaded board shows the asset that carries the secret, with its public count of 1.
	await expect(carol.page.getByRole('button', { name: "bob's holding Holding 1" })).toBeVisible({ timeout: 10_000 });
	expect((await carol.page.content()).includes(SECRET), "secret text rendered in a bystander's page").toBe(false);
	// The log tells the table what happened to the board, never what was written.
	const posts = await (await carol.ctx.request.get(`/api/tables/${t.gameID}/posts`)).text();
	expect(posts).toContain('Seek Answers marred');
	expect(posts).toContain('learned the secrets of');
});

test('seek answers mar: a zero-pick commit is a committed choice, so the wait moves to the self-flaw', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts);
	const { alice, bob, carol } = t;
	await sa.openTables(t);

	const roll = await sa.castFromUI(t, MESSAGE);
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: 1 });
	await expect(alice.page.getByText('choose exactly 1')).toBeVisible({ timeout: 10_000 });
	for (const p of [bob, carol]) {
		await expect(p.page.getByText('alice is choosing make/mar options…')).toBeVisible({ timeout: 10_000 });
	}
	const aliceID = (await sa.getPlan(alice.ctx.request, t.planID)).plan.preparer_id;

	// A hand-built commit with nothing picked (the picker never offers one).
	const res = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: [] },
	});
	expect(res.ok(), `zero-pick commit: ${await res.text()}`).toBeTruthy();

	// The choice is recorded though it lists nothing, and the penalty is still owed:
	// the plan stays open with alice, and only alice, waited on. (difficulty − 1 is owed;
	// alice has a single resource, so it is capped at one.)
	const body = await sa.getPlan(alice.ctx.request, t.planID);
	expect(body.resolution_data?.choice_submitted).toBe(true);
	expect(body.resolution_data?.make_mar_choices ?? []).toHaveLength(0);
	expect(body.plan.status).toBe('resolving');
	expect(body.resolution_data?.seek_answers?.mar_self_flaws_required).toBe(1);
	await expect.poll(() => sa.actingIDs(bob.ctx.request, t.gameID), { timeout: 10_000 })
		.toMatchObject({ ids: [aliceID] });

	// Discharging the penalty is then the last thing owed, and the plan resolves on it. That
	// only happens because the commit counts (autoCompleteIfDone refuses an uncommitted plan).
	const assets = await sa.listAssets(alice.ctx.request, t.gameID);
	const mine = assets.find(a => a.asset_type === 'resource' && a.name.startsWith('alice'))!;
	const pen = await alice.ctx.request.post(`/api/plans/${t.planID}/break-resource`, {
		data: { asset_id: mine.id, marginalia_id: mine.marginalia[0].id },
	});
	expect(pen.ok(), `penalty: ${await pen.text()}`).toBeTruthy();
	await expect.poll(() => sa.planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
});

test('seek answers make: a question and its answer are told to the whole table, live, not just the target', async ({ browser }) => {
	// Unlike a secret, a question's answer is public by design: the log and the panel's
	// "Resolved so far" give it to every viewer. The bystander's open page hears the
	// question when it is asked (plan.choice_applied) and the answer when it lands.
	const t = await sa.setUp(browser, track, openContexts, [0, 1, 2]);
	const { alice, bob, carol } = t;
	await sa.openTables(t);
	const roll = await sa.castFromUI(t, MESSAGE);
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'make', result: 2 });
	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });
	const commit = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'make', choices: ['declare_truth', 'ask_question'] },
	});
	expect(commit.ok(), `commit: ${await commit.text()}`).toBeTruthy();
	const dt = await alice.ctx.request.post(`/api/plans/${t.planID}/declare-truth`, { data: { text: 'The seal is warm.' } });
	expect(dt.ok(), `declare-truth: ${await dt.text()}`).toBeTruthy();

	const Q = 'Who paid for the forged seal?';
	const A = "The treasurer, out of the orphans' fund.";
	const ask = await alice.ctx.request.post(`/api/plans/${t.planID}/ask-question`, { data: { target_id: t.ids.bob, question: Q } });
	expect(ask.ok(), `ask: ${await ask.text()}`).toBeTruthy();
	// Carol, who is not asked, still reads the question and who owes the answer.
	await expect(carol.page.getByText("Waiting for bob to answer alice's question…")).toBeVisible({ timeout: 10_000 });
	await expect(carol.page.getByText(Q)).toBeVisible();
	await expect(carol.page.getByRole('button', { name: 'Answer', exact: true })).toHaveCount(0);
	await expect(bob.page.getByRole('button', { name: 'Answer', exact: true })).toBeVisible({ timeout: 10_000 });
	// Bob ranks below alice on knowledge here, so he has no veto: not offered, and refused.
	await expect(bob.page.getByRole('button', { name: 'Veto (ask another)' })).toHaveCount(0);
	const veto = await bob.ctx.request.post(`/api/plans/${t.planID}/veto-question`);
	expect(veto.status()).toBe(409);

	// Nobody but the target may answer.
	const wrong = await carol.ctx.request.post(`/api/plans/${t.planID}/answer-question`, { data: { answer: 'me' } });
	expect(wrong.status()).toBe(403);
	const ans = await bob.ctx.request.post(`/api/plans/${t.planID}/answer-question`, { data: { answer: A } });
	expect(ans.ok(), `answer: ${await ans.text()}`).toBeTruthy();

	await expect.poll(() => sa.planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
	await expect.poll(async () =>
		(await (await carol.ctx.request.get(`/api/tables/${t.gameID}/posts`)).text()).includes(A),
		{ timeout: 10_000 }).toBe(true);
});

test('seek answers make: a pick with no valid target is carried by the forfeit hatch, and the plan resolves', async ({ browser }) => {
	// No asset at this table holds a secret, so "learn the secrets" has nothing to act on. The
	// picker shuts that option ("no valid targets"); a stale client can still commit it, and
	// then the step offers the way out instead of wedging the row.
	const t = await sa.setUp(browser, track, openContexts, [0, 1, 2]);
	const { alice, carol } = t;
	await sa.openTables(t);
	const roll = await sa.castFromUI(t, MESSAGE);
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'make', result: 2 });
	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });
	await expect(sa.more(alice.page, sa.OPT.revealSecret)).toBeDisabled();
	await expect(alice.page.getByText('no valid targets')).toBeVisible();

	const commit = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'make', choices: ['reveal_secret', 'reveal_secret'] },
	});
	expect(commit.ok(), `commit: ${await commit.text()}`).toBeTruthy();

	await expect(alice.page.getByText('No asset\'s secrets remain to learn')).toBeVisible({ timeout: 10_000 });
	await expect(carol.page.getByText('alice is resolving Seek Answers…')).toBeVisible({ timeout: 10_000 });
	await expect(carol.page.getByRole('button', { name: 'Skip — no valid targets' })).toHaveCount(0);
	expect(await sa.planStatus(alice.ctx.request, t.planID)).toBe('resolving');
	// Only the preparer may forfeit.
	const stolen = await carol.ctx.request.post(`/api/plans/${t.planID}/seek-forfeit-step`, { data: { step: 'reveal_secret' } });
	expect(stolen.status()).toBe(403);
	await alice.page.getByRole('button', { name: 'Skip — no valid targets' }).click();
	await expect.poll(() => sa.planStatus(alice.ctx.request, t.planID), { timeout: 10_000 }).toBe('resolved');
});

// BUG (docs/work/next/seek-answers-zero-pick-panel-reverts.md): the panel decides "choices
// made" from the picked options alone (choicesDone = existingChoices.length > 0), not from
// the committed marker the server sets, so after a zero-pick mar commit everyone is shown
// the picker again and the preparer is never offered the self-flaw penalty. Un-fixme this
// when it is fixed.
test.fixme('seek answers mar: after a zero-pick commit the panels move on to the self-flaw', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts);
	const { alice, bob, carol } = t;
	await sa.openTables(t);
	const roll = await sa.castFromUI(t, MESSAGE);
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'mar', result: 1 });
	await expect(alice.page.getByText('choose exactly 1')).toBeVisible({ timeout: 10_000 });
	const res = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: [] },
	});
	expect(res.ok(), `zero-pick commit: ${await res.text()}`).toBeTruthy();

	await expect(alice.page.getByText('Describe a flaw in your own resource assets')).toBeVisible({ timeout: 10_000 });
	for (const p of [bob, carol]) {
		await expect(p.page.getByText('alice is resolving Seek Answers…')).toBeVisible({ timeout: 10_000 });
		await expect(p.page.getByText('alice is choosing make/mar options…')).toHaveCount(0);
	}
});

// BUG (docs/work/next/seek-answers-choice-before-cast.md): make-choice is accepted before the
// pre-roll step is done. A "make" commits with no dice at all (and the picks play out); a
// "mar" dies in a 500 ("could not load roll for mar penalty") instead of a 4xx. Spread
// Propaganda refuses both with a 409. Un-fixme this when it is fixed.
test.fixme('seek answers pre-roll: a choice before the dice are cast is refused with a 4xx', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts);
	const mar = await t.alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'mar', choices: ['declare_truth'] },
	});
	expect(mar.status(), 'a mar before the pre-roll').toBe(409);
	const make = await t.alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'make', choices: ['declare_truth'] },
	});
	expect(make.status(), 'a make before the pre-roll').toBe(409);
});

// break-resource, reveal-secret and declare-truth save their progress; a bystander
// sees each step land live, with no reload.
test('seek answers make: a bystander sees each step land live, with no reload', async ({ browser }) => {
	const t = await sa.setUp(browser, track, openContexts, [0, 1, 2]);
	const { alice, carol } = t;
	await sa.openTables(t);
	const roll = await sa.castFromUI(t, MESSAGE);
	await sa.forceRoll(alice.ctx.request, roll.id, { outcome: 'make', result: 2 });
	await expect(alice.page.getByText('choose exactly 2')).toBeVisible({ timeout: 10_000 });
	const assets = await sa.listAssets(alice.ctx.request, t.gameID);
	const bobResource = assets.find(a => a.asset_type === 'resource' && a.name.startsWith('bob'))!;
	const commit = await alice.ctx.request.post(`/api/plans/${t.planID}/make-choice`, {
		data: { result: 'make', choices: ['break_resource', 'declare_truth'] },
	});
	expect(commit.ok(), `commit: ${await commit.text()}`).toBeTruthy();
	await expect(carol.page.getByText('Break a resource (1 remaining)')).toBeVisible({ timeout: 10_000 });

	const brk = await alice.ctx.request.post(`/api/plans/${t.planID}/break-resource`, {
		data: { asset_id: bobResource.id, marginalia_id: bobResource.marginalia[0].id },
	});
	expect(brk.ok(), `break-resource: ${await brk.text()}`).toBeTruthy();
	await expect(carol.page.locator('.resolved-so-far').getByText(`Broke ${bobResource.name}`)).toBeVisible({ timeout: 10_000 });
	await expect(carol.page.getByText('Break a resource (1 remaining)')).toHaveCount(0);
});

// Make Demands' perform_steps winner drives the choices in the preparer's stead, but this
// panel still gives the preparer a live picker she cannot submit. Same bug as
// make_demands.spec.ts pins; see docs/work/next/perform-steps-panels-gate-on-preparer.md.
test.fixme('seek answers under perform_steps: the preparer sees a wait line, not a picker', async ({ browser }) => {
	const f = await H.seedMD({ target: 'seek_answers' });
	await H.prepareDemand(f);
	await H.reachRow(f);
	await H.rollDemand(f, 'make');
	await H.draftWants(f, { bob: ['perform_steps', 'keep_assets'], alice: ['control_leverage', 'keep_or_change_target'] });
	await H.completeDemand(f);
	await H.startTarget(f);
	const cast = await H.call(f.api.alice, 'post', `/api/plans/${f.targetID}/seek-cast-roll`, { narration: 'I read the ledgers.' });
	await H.call(f.api.alice, 'post', `/api/rolls/${cast.roll.id}/skip-vote`);
	await H.call(f.api.alice, 'post', '/api/dev/force-roll', { roll_id: cast.roll.id, result: 2 });
	const { alice } = await H.openPages(browser, f);
	await expect(alice.getByText('bob is choosing make/mar options…')).toBeVisible();
	await expect(alice.getByRole('button', { name: 'Apply choices' })).toHaveCount(0);
});
