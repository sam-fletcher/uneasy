import { defineConfig, devices } from '@playwright/test';
import { E2E } from './tests/e2e/baseUrl';

export default defineConfig({
  testDir: './tests/e2e',
  // Aborts the whole run at once, with one message, if the app is down or
  // returning errors (instead of ~150 specs each timing out at 30s).
  globalSetup: './tests/e2e/globalSetup.ts',
  fullyParallel: false,
  forbidOnly: !!process.env.CI,
  retries: 0,
  workers: 1,
  reporter: 'list',
  use: {
    baseURL: E2E,
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'] },
    },
  ],
});
