import { test, expect } from '@playwright/test';

test('dev-login + profile renders username', async ({ page, request }) => {
  await request.post('/api/dev/reset');
  const login = await request.post('/api/dev/login?username=alice');
  expect(login.ok()).toBeTruthy();

  // Carry the session cookie from `request` over to the browser context so
  // the page navigation is authenticated.
  const cookies = await request.storageState();
  await page.context().addCookies(cookies.cookies);

  await page.goto('/profile');
  await expect(page.getByText('alice')).toBeVisible();
});
