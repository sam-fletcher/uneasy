import { test, expect } from '@playwright/test';

test('dev-login + profile renders username', async ({ page, request }) => {
  const login = await request.post('/api/dev/login?username=alice');
  expect(login.ok()).toBeTruthy();

  // Carry the session cookie from `request` over to the browser context so
  // the page navigation is authenticated.
  const cookies = await request.storageState();
  await page.context().addCookies(cookies.cookies);

  await page.goto('/profile');
  // .first(): "alice" also appears in the roster pill of any table the
  // account belongs to, not just the profile header — strict mode needs a
  // single target.
  await expect(page.getByText('alice').first()).toBeVisible();
});
