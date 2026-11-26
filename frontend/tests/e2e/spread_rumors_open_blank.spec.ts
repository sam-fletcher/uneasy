import { test, expect } from './fixtures';
import { cleanupGameAfterEach } from './helpers';
import { seedWarGame, runScene, closeContextsAfterEach } from './helpers/make_war';

// An open rumor may be left unwritten at preparation (book p.59: the text is asked for then only
// "if you'd like to keep it secret"); it is stated at the pre-roll, which still needs it. A secret
// rumor always has words, so its form still waits for them. Driven on a phone, as alice (preparing)
// and carol (watching). Set SHOTS=<dir> to keep 375px screenshots of each step.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const PHONE = { width: 375, height: 2200 };
const shots = process.env.SHOTS;

test('an open rumor is prepared blank and stated at the pre-roll; a secret one still needs its text', async ({ browser }) => {
	const g = await seedWarGame(browser, PHONE);
	track(g.gameID);
	await runScene(g);

	const alice = await g.ctx.alice.newPage();
	const carol = await g.ctx.carol.newPage();
	await alice.goto(`/table/${g.gameID}`);
	await carol.goto(`/table/${g.gameID}`);

	await alice.getByText('Spread Rumors', { exact: true }).click();
	await alice.getByRole('button', { name: 'bob', exact: true }).first().click();
	await alice.locator('button', { hasText: "bob's holding" }).getByRole('checkbox').click();

	const prepare = alice.getByRole('button', { name: 'Prepare Plan' });
	// Open and blank: nothing to write yet, so nothing stops her.
	await expect(alice.getByRole('textbox', { name: 'Rumor', exact: true })).toHaveValue('');
	await expect(prepare).toBeEnabled();
	await alice.waitForTimeout(600); // the watcher's mirror is fed by a draft sent shortly after a change
	await expect(carol.getByText('alice is preparing the rumor.')).toBeVisible({ timeout: 10_000 });
	if (shots) {
		await alice.screenshot({ path: `${shots}/rumor-prep-open-blank-alice.png`, fullPage: true });
		await carol.screenshot({ path: `${shots}/rumor-prep-open-blank-carol.png`, fullPage: true });
	}

	// Secret: the words are required, so the button waits for them (and for a holder).
	await alice.getByText('Keep it secret for now').click();
	await expect(prepare).toBeDisabled();
	await alice.locator('button', { hasText: "alice's artifact" }).getByRole('checkbox').click();
	await expect(prepare).toBeDisabled();
	await alice.getByRole('textbox', { name: 'Rumor', exact: true }).fill('She waters her wine.');
	await expect(prepare).toBeEnabled();
	await alice.getByRole('textbox', { name: 'Rumor', exact: true }).fill('');
	await expect(prepare).toBeDisabled();
	if (shots) await alice.screenshot({ path: `${shots}/rumor-prep-secret-blank-alice.png`, fullPage: true });

	// Back to open, and prepare it blank.
	await alice.getByText('Keep it secret for now').click();
	await expect(prepare).toBeEnabled();
	await prepare.click();
	await expect(alice.getByText('Prepare Plan')).toHaveCount(0, { timeout: 10_000 });

	const plans = await (await g.ctx.alice.request.get(`/api/tables/${g.gameID}/plans`)).json();
	const plan = (plans.plans ?? plans).find((p: { plan_type: string }) => p.plan_type === 'spread_rumors');
	expect(plan, 'the blank open rumor was prepared').toBeTruthy();

	// Fast-forward to its resolution: the pre-roll opens with the rumor field empty.
	const adv = await g.ctx.alice.request.post('/api/dev/advance-row', { data: { plan_id: plan.id } });
	expect(adv.ok(), `advance-row failed: ${await adv.text()}`).toBeTruthy();
	await alice.reload();
	await carol.reload();
	const rumorField = alice.getByRole('textbox', { name: 'The rumor', exact: true });
	const how = alice.getByRole('textbox', { name: 'How you are spreading it', exact: true });
	const cast = alice.getByRole('button', { name: 'Post & open the roll' });
	await expect(rumorField).toHaveValue('');
	await expect(alice.getByText('Starts from your preparation note')).toHaveCount(0);
	await how.fill('Over every tavern table.');
	await expect(cast, 'the rumor is still required before the roll').toBeDisabled();
	if (shots) {
		await alice.screenshot({ path: `${shots}/rumor-preroll-blank-alice.png`, fullPage: true });
		await carol.screenshot({ path: `${shots}/rumor-preroll-blank-carol.png`, fullPage: true });
	}
	await rumorField.fill('The Duchess waters her wine.');
	await expect(cast).toBeEnabled();
	await cast.click();
	await expect(alice.getByText('The Duchess waters her wine.').first()).toBeVisible({ timeout: 10_000 });
});
