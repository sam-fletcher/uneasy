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
  // Locally: the plain `list`. On CI (GitHub Actions sets CI=true): the same
  // list in the log, plus
  //  - `json`, a machine-readable report the nightly workflow reads to name
  //    the failing specs in the issue it files. It sits inside test-results/
  //    so one folder upload carries it together with the traces.
  //  - `github`, which turns each failure into an annotation on the run page.
  reporter: process.env.CI
    ? [['list'], ['json', { outputFile: 'test-results/report.json' }], ['github']]
    : 'list',
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
