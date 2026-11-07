import { E2E } from './baseUrl';

/**
 * Preflight for the whole Playwright run: is the app answering at all?
 *
 * Without this, a dead or broken stack fails the suite slowly and noisily —
 * every spec waits out its 30s timeout, so a full run spends the better part
 * of an hour saying "the app returned 500". One request up front turns that
 * into a single message in about a second.
 *
 * Passes silently when the base URL returns 2xx. Throws (which aborts the run
 * before any spec starts) when it is unreachable, hangs, or returns non-2xx.
 */

/** Long enough for Vite's first, cold compile of the page; short enough to fail fast. */
const TIMEOUT_MS = 15_000;

/** The first non-blank line of a response body, trimmed and capped. */
function firstLine(body: string): string {
  const line = body.split('\n').map((l) => l.trim()).find((l) => l !== '') ?? '';
  return line.length > 200 ? `${line.slice(0, 200)}…` : line;
}

/** An Error whose message is the whole report: no stack trace to scroll past. */
function abort(message: string): Error {
  const err = new Error(message);
  err.stack = message;
  return err;
}

/**
 * Checks `base` and throws a one-message Error if the app isn't healthy.
 * `ownServer` is true when E2E_BASE_URL points at a server the caller started
 * (a worktree's), so the docker hints would be wrong.
 */
export async function preflight(base: string, ownServer: boolean): Promise<void> {
  const startHint = ownServer
    ? 'E2E_BASE_URL is set, so this run targets your own server: start it on that address (see the test-worktree skill).'
    : 'Start the dev stack: docker compose up -d';
  const brokenHint = ownServer
    ? "Check that server's own output for the error."
    : 'Restart the Vite container: docker restart uneasy-frontend-1 (its log: docker logs --tail 30 uneasy-frontend-1).';

  let res: Response;
  try {
    res = await fetch(base, {
      headers: { Accept: 'text/html' },
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch (e) {
    const timedOut = e instanceof Error && (e.name === 'TimeoutError' || e.name === 'AbortError');
    // A hang means something is listening but wedged; a refusal means nothing is.
    const why = timedOut ? `did not answer within ${TIMEOUT_MS / 1000}s` : 'is not reachable (connection refused)';
    throw abort(`e2e preflight failed: ${base} ${why}. No spec was run.\n${timedOut ? brokenHint : startHint}\n`);
  }

  if (!res.ok) {
    const body = firstLine(await res.text().catch(() => ''));
    throw abort(
      `e2e preflight failed: ${base} returned HTTP ${res.status} ${res.statusText}`.trimEnd() +
        `${body ? `\n  ${body}` : ''}\n` +
        `No spec was run: every page would fail the same way, one slow timeout at a time.\n${brokenHint}\n`,
    );
  }
}

export default async function globalSetup(): Promise<void> {
  await preflight(E2E, !!process.env.E2E_BASE_URL);
}
