/**
 * Where the e2e specs point. Defaults to the docker `server-e2e` container
 * (main's code); set E2E_BASE_URL to test a worktree's own server instead
 * (see the test-worktree skill). playwright.config.ts reads this too, so the
 * config's `request`/`page` fixtures and the specs' own contexts agree.
 */
export const E2E = process.env.E2E_BASE_URL ?? 'http://localhost:8090';
