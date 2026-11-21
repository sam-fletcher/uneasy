import { test, expect, type Page } from './fixtures';
import { cleanupGameAfterEach } from './helpers';
import {
	seedWarGame, warAtCostRow, frontPayer, openAll, runScene, closeContextsAfterEach,
} from './helpers/make_war';

// The house checkbox (shared/Checkbox.svelte): a drawn box over a real, hidden
// <input type="checkbox">, its words beside it, and what ticking does in words
// underneath. Its two users are driven here on a phone: Make War's surrender
// row (an explanation that used to live in a `title`, and a lock with a reason)
// and Spread Rumors' "Keep it secret for now" (and the inert mirror a watcher
// sees). The look itself is held by Checkbox.test.ts and checkboxGuard.test.ts.

const track = cleanupGameAfterEach();
closeContextsAfterEach();

const PHONE = { width: 375, height: 1500 };

/** The drawn box beside a checkbox's words: the input's next sibling. */
const drawnBox = (page: Page, name: string) =>
	page.getByRole('checkbox', { name, exact: true }).locator('xpath=following-sibling::*[1]');

test('surrender: tapping the words ticks it; the explanation is on screen; peace locks it and says why', async ({ browser }) => {
	const g = await seedWarGame(browser, PHONE);
	track(g.gameID);
	await warAtCostRow(g);
	expect(await frontPayer(g)).toBe('bob');
	const bob = (await openAll(g)).bob;
	await bob.getByRole('group', { name: 'Your cost of battle' }).getByRole('button', { name: 'alice', exact: true }).click();
	await bob.getByRole('button', { name: 'Break an asset' }).click();

	const surrender = bob.getByRole('checkbox', { name: 'Surrender after this payment' });
	const explanation = bob.getByText(/After this payment is recorded you are marked surrendered/);
	const reason = bob.getByText("Doesn't apply when proposing peace.");

	// What it does is words on the screen, not a tooltip a phone can't show.
	await expect(explanation).toBeVisible();
	await expect(reason).toHaveCount(0);
	await expect(surrender.locator('xpath=ancestor::label')).not.toHaveAttribute('title', /.*/);
	await expect(surrender).not.toBeChecked();
	await expect(surrender).toHaveAccessibleDescription(/each opposing opponent who hasn't surrendered/);

	// A tap on the words ticks the box (the row is the control), and the box shows it.
	const box = drawnBox(bob, 'Surrender after this payment');
	const resting = await box.evaluate((el) => getComputedStyle(el).boxShadow);
	await bob.getByText('Surrender after this payment').click();
	await expect(surrender).toBeChecked();
	expect(await box.evaluate((el) => getComputedStyle(el).boxShadow), 'a ticked box takes the gold ring').not.toBe(resting);
	await expect(box.locator('svg')).toBeVisible(); // the tick

	// Space toggles it from the keyboard, too.
	await surrender.focus();
	await bob.keyboard.press('Space');
	await expect(surrender).not.toBeChecked();
	await bob.keyboard.press('Space');
	await expect(surrender).toBeChecked();

	// Proposing peace has nothing to surrender from: locked, cleared, and it says why.
	await bob.getByRole('button', { name: 'Propose peace' }).first().click();
	await expect(surrender).toBeDisabled();
	await expect(surrender).not.toBeChecked();
	await expect(reason).toBeVisible();
	await expect(surrender).toHaveAccessibleDescription(/Doesn't apply when proposing peace/);
	await expect(bob.getByText('Surrender after this payment')).toHaveCSS('color', 'rgb(153, 150, 137)'); // faint, not faded
	expect(await bob.getByText('Surrender after this payment').evaluate((el) => getComputedStyle(el.closest('label')!).opacity)).toBe('1');

	// Back to a payment: unlocked, and the reason goes.
	await bob.getByRole('button', { name: 'Break an asset' }).click();
	await expect(surrender).toBeEnabled();
	await expect(reason).toHaveCount(0);
});

test('keep it secret: the preparer ticks it by its words; a watcher\'s mirror shows it ticked and does not respond', async ({ browser }) => {
	const g = await seedWarGame(browser, PHONE);
	track(g.gameID);
	await runScene(g);
	const { alice, bob } = await openAll(g);

	await alice.getByText('Spread Rumors', { exact: true }).click();
	await alice.getByRole('button', { name: 'bob', exact: true }).first().click();

	const aliceBox = alice.getByRole('checkbox', { name: 'Keep it secret for now' });
	await expect(aliceBox).not.toBeChecked();
	await expect(alice.getByText(/Others won't see the text until you spread it/)).toBeVisible();
	await alice.getByText('Keep it secret for now').click();
	await expect(aliceBox).toBeChecked();
	await expect(alice.getByText('Hide the rumor under:', { exact: true })).toBeVisible();

	// The watcher's copy follows the draft, but nothing on it can be pressed.
	const bobBox = bob.getByRole('checkbox', { name: 'Keep it secret for now' });
	await expect(bobBox).toBeChecked({ timeout: 10_000 });
	await expect(bobBox).toBeDisabled();
	await bob.getByText('Keep it secret for now').click({ force: true });
	await expect(bobBox).toBeChecked();
	await expect(bob.getByText('Hidden under:')).toBeVisible();
});
