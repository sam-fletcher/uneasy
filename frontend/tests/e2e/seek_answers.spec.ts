import { test, expect, request as pwRequest, type APIRequestContext, type Page } from '@playwright/test';
import { cleanupGameAfterEach } from './helpers';

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

const E2E = 'http://localhost:8090';

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

const track = cleanupGameAfterEach();

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
		},
	});
	expect(seedRes.ok(), `seed failed: ${await seedRes.text()}`).toBeTruthy();
	const { game_id } = await seedRes.json();
	track(game_id);
	await fixture.dispose();

	const aliceCtx = await browser.newContext({ baseURL: E2E });
	const bobCtx = await browser.newContext({ baseURL: E2E });
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
	const q2 = 'Then who carried your seal that night?';
	await reaskForm.getByRole('button', { name: 'bob' }).click();
	await reaskForm.locator('textarea').fill(q2);
	await reaskForm.getByRole('button', { name: 'Ask question' }).click();

	await expect(bob.getByText(q2)).toBeVisible({ timeout: 10_000 });
	const bobAnswer = bob.getByRole('button', { name: 'Answer', exact: true });
	await expect(bobAnswer).toBeVisible();
	await expect(bob.getByRole('button', { name: 'Veto (ask another)' })).toHaveCount(0);

	// ── Leg 4: bob answers. The sub-flow completes on alice's side live, so
	//    her "Complete plan" button appears with no reload. ──────────────────
	await bob.getByPlaceholder('Answer the question…').fill('My steward, Crane — I never left the hall.');
	await bobAnswer.click();

	const completeBtn = alice.getByRole('button', { name: 'Complete plan' });
	await expect(completeBtn).toBeVisible({ timeout: 10_000 });
	await completeBtn.click();

	// The plan resolves cleanly once completed.
	await expect.poll(async () => (await fetchSeekPlan(aliceCtx.request, game_id)).status, {
		timeout: 10_000,
	}).toBe('resolved');

	await aliceCtx.close();
	await bobCtx.close();
});
