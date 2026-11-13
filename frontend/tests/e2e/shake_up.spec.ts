import { test, expect, type Page } from '@playwright/test';
import { cleanupGameAfterEach, tableTitle } from './helpers';
import { paint, playShakeUpToEnd, playerContext, resolveToken, seedGame } from './helpers/endgame';

// The Shake-Up and the end of the game, read at phone width.
//
// The first two specs play one spend through the real screens, as the three
// players would: announce, react, pay or abandon. They check the behaviour (the
// reaction gate, the cost floor, pay-or-abandon) and, with it, what the
// redesign ruled for the screen: an option is a chip and the pick is the bright
// gold border with no fill, a raised cost is a notice (a mark and coloured
// words) and not a tinted strip, the reaction buttons are plain buttons with no
// main one, and nothing is framed. The third plays a whole Shake-Up out to reach
// the ended screen, which the seed endpoint has no way to produce.

const track = cleanupGameAfterEach();

async function openShakeUp(browser: import('@playwright/test').Browser, tokens: number) {
  const seeded = await seedGame({
    phase: 'shake_up',
    players: ['alice', 'bob', 'carol'],
    shake_up_tokens: tokens,
    shake_up_step: 'spending',
  });
  track(seeded.game_id);
  const ctxs = Object.fromEntries(
    await Promise.all(
      ['alice', 'bob', 'carol'].map(async (n) => [n, await playerContext(browser, n)] as const),
    ),
  );
  // Whoever the server says announces first (reverse rank), and the other two react.
  const st = await (await ctxs.alice.request.get(`/api/tables/${seeded.game_id}/shake-up`)).json();
  const spenderName = seeded.players.find((p) => p.id === st.current_actor)!.display_name;
  const reactorNames = seeded.players.map((p) => p.display_name).filter((n) => n !== spenderName);
  const pages: Record<string, Page> = {};
  for (const n of Object.keys(ctxs)) {
    pages[n] = await ctxs[n].newPage();
    await pages[n].goto(`/table/${seeded.game_id}`);
  }
  const close = () => Promise.all(Object.values(ctxs).map((c) => c.close()));
  return { id: seeded.game_id, pages, spenderName, reactorNames, close };
}

test('a spend: announce, react, and pay the raised cost', async ({ browser }) => {
  const g = await openShakeUp(browser, 2);
  const spender = g.pages[g.spenderName];
  const [reactA, reactB] = g.reactorNames.map((n) => g.pages[n]);

  await expect(tableTitle(spender)).toContainText('Shake-Up');
  await expect(spender.getByRole('heading', { name: 'Announce a spend' })).toBeVisible();

  // The options are chips. Picking one gives it the bright gold border and its
  // words, in the same fill as the chips beside it: a pick is never a fill.
  const you = await resolveToken(spender, '--color-you');
  const take = spender.getByRole('button', { name: 'Take a peer asset.' });
  const bump = spender.getByRole('button', { name: /^Bump up a rank/ });
  await expect(take).toHaveAttribute('aria-pressed', 'false');
  await take.click();
  await expect(take).toHaveAttribute('aria-pressed', 'true');
  const picked = await paint(take);
  const other = await paint(bump);
  expect(picked.borderColor, 'the picked option wears bright gold').toBe(you);
  expect(other.borderColor, 'the others do not').not.toBe(you);
  expect(picked.background, 'the pick adds no fill').toBe(other.background);
  expect(parseFloat(picked.radius), 'a chip is fully rounded').toBeGreaterThan(20);
  expect((await take.boundingBox())!.height).toBeGreaterThanOrEqual(44);

  // Then the target, and the main button announces it.
  await spender.getByRole('checkbox').first().click();
  const announce = spender.getByRole('button', { name: /^Announce/ });
  await expect(announce).toBeEnabled();
  await announce.click();

  // Everyone sees the open spend, announced, with the cost so far.
  for (const page of [spender, reactA, reactB]) {
    await expect(page.getByRole('heading', { name: 'Open spend' })).toBeVisible();
    await expect(page.locator('.spend-phrase')).toContainText('announced: Take a peer asset');
  }
  await expect(spender.getByText('Others can still react')).toBeVisible();

  // The reactors get three plain buttons and no main one (each acts on the
  // spot, so none of them is "the" next step). The panel itself has no outline.
  await expect(reactA.getByRole('button', { name: /^\+1/ })).toBeEnabled();
  await expect(reactA.getByRole('button', { name: 'Let it stand' })).toBeEnabled();
  await expect(reactA.locator('.spend-panel .action-btn.primary')).toHaveCount(0);
  const panel = await paint(reactA.locator('.spend-panel'));
  expect(panel.borderWidth, 'the step has no outline').toBe('0px');
  expect(panel.background).toBe('rgba(0, 0, 0, 0)');

  // The cost can't go below 1. The −1 button is greyed through aria-disabled
  // (never opacity), still presses, and says why in a notice.
  const lower = reactA.getByRole('button', { name: /^−1/ });
  await expect(lower).toHaveAttribute('aria-disabled', 'true');
  expect(await lower.evaluate((el) => getComputedStyle(el).opacity), 'greyed, not faded').toBe('1');
  await lower.click({ force: true });
  const floorNotice = reactA.locator('.notice', { hasText: "The cost can't go below 1 token." });
  await expect(floorNotice).toBeVisible();
  await expect(floorNotice).toHaveAttribute('data-tone', 'warning');

  // One reactor raises the cost. Raising clears every pass, so both have to
  // let it stand before the spender can go on.
  await reactA.getByRole('button', { name: /^\+1/ }).click();
  await expect(spender.locator('.adjustment-history')).toContainText('raised the cost by 1');
  await reactB.getByRole('button', { name: 'Let it stand' }).click();
  await expect(spender.getByText('Others can still react')).toBeVisible();
  await reactA.getByRole('button', { name: 'Let it stand' }).click();

  // The raised cost is a notice (a mark and words in the warning colour), and
  // the spender chooses: pay the extra, or abandon.
  const raised = spender.locator('.notice', { hasText: 'The cost was raised to 2' });
  await expect(raised).toBeVisible();
  await expect(raised).toHaveAttribute('data-tone', 'warning');
  expect((await paint(raised)).background, 'a notice has no tint').toBe('rgba(0, 0, 0, 0)');
  const pay = spender.getByRole('button', { name: 'Pay 1 more token' });
  await expect(pay).toBeVisible();
  await expect(spender.getByRole('button', { name: 'Abandon' })).toBeVisible();
  // One main button in the step: Pay is it, Abandon is a plain one.
  await expect(spender.locator('.spend-panel .action-btn.primary')).toHaveCount(1);

  await pay.click();
  for (const page of [spender, reactA, reactB]) {
    await expect(page.getByRole('heading', { name: 'Open spend' })).toHaveCount(0);
  }
  await expect(spender.getByText('Your tokens:')).toContainText('0');
  await g.close();
});

test('a spend the spender cannot afford is abandoned, and says so', async ({ browser }) => {
  // One token each: announcing burns the spender's, so a raise by anyone cannot be paid.
  const g = await openShakeUp(browser, 1);
  const spender = g.pages[g.spenderName];
  const [reactA, reactB] = g.reactorNames.map((n) => g.pages[n]);

  // A bump option needs no target.
  await spender.getByRole('button', { name: /^Bump up a rank/ }).click();
  await spender.getByRole('button', { name: /^Announce/ }).click();
  await expect(reactA.getByRole('heading', { name: 'Open spend' })).toBeVisible();

  await reactA.getByRole('button', { name: /^\+1/ }).click();
  await reactB.getByRole('button', { name: 'Let it stand' }).click();

  // The loss is a notice in the danger colour, and Abandon is the only way on.
  const forced = spender.locator('.notice', { hasText: 'The spend must be abandoned' });
  await expect(forced).toBeVisible();
  await expect(forced).toHaveAttribute('data-tone', 'danger');
  await expect(spender.getByRole('button', { name: /^Pay/ })).toHaveCount(0);
  await spender.getByRole('button', { name: 'Abandon' }).click();
  await expect(spender.getByRole('heading', { name: 'Open spend' })).toHaveCount(0);
  await g.close();
});

test('the ended screen lists the final standings, a shape and a name each', async ({ browser }) => {
  const seeded = await seedGame({
    phase: 'shake_up',
    players: ['alice', 'bob', 'carol'],
    shake_up_tokens: 0,
    shake_up_step: 'rolling',
  });
  track(seeded.game_id);
  const ctxs = Object.fromEntries(
    await Promise.all(
      ['alice', 'bob', 'carol'].map(async (n) => [n, await playerContext(browser, n)] as const),
    ),
  );
  await playShakeUpToEnd((n) => ctxs[n].request, seeded.game_id, seeded.players);

  const page = await ctxs.alice.newPage();
  await page.goto(`/table/${seeded.game_id}`);
  await expect(tableTitle(page)).toHaveText('Game over');
  await expect(page.getByRole('heading', { name: 'Final Rankings' })).toBeVisible();

  // Three standings, one per track, each with every seated player once and in
  // rank order. Dummy slots hold ranks but aren't anyone, so none is listed.
  const tracks = page.locator('.standings .track');
  await expect(tracks).toHaveCount(3);
  await expect(tracks.locator('h4')).toHaveText(['power', 'knowledge', 'esteem']);
  for (let i = 0; i < 3; i++) {
    const rows = tracks.nth(i).locator('li');
    await expect(rows).toHaveCount(3);
    const ranks = (await rows.locator('.rank').allTextContents()).map((t) => parseInt(t, 10));
    expect(ranks, 'best rank first').toEqual([...ranks].sort((a, b) => a - b));
    // Every row carries the player's shape beside the name.
    await expect(rows.locator('svg.player-shape')).toHaveCount(3);
    const names = (await rows.locator('.name').allTextContents()).sort();
    expect(names).toEqual(['alice', 'bob', 'carol']);
  }

  // The standings are lists with rules between the rows, not boxes.
  const standings = await paint(page.locator('.standings'));
  expect(standings.borderWidth).toBe('0px');
  expect(standings.background).toBe('rgba(0, 0, 0, 0)');

  await Promise.all(Object.values(ctxs).map((c) => c.close()));
});
