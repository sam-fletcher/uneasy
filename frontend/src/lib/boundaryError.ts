// What a <svelte:boundary> does with the error it catches.
//
// A boundary with a `failed` snippet but no `onerror` swallows the throw: the
// player sees the fallback, and the developer sees nothing, not even in the
// console. The nightly e2e failure of 2026-11-08 (a duplicated war id throwing
// each_key_duplicate inside the table's boundary) read as a stale waiting-on
// bar until a CDP debugger hook dug the real error out.
//
// This only logs, locally. Reporting a render crash to the server is a separate
// question waiting on the owner (docs/work/questions/render-crash-report.md).

/** Prefix on every line this writes, so a console capture (a Playwright
 *  listener, a bug report) can pick the lines out. */
export const BOUNDARY_ERROR_PREFIX = '[uneasy:boundary]';

/**
 * Logs a boundary's caught error to the console, with the prefix above and
 * where in the app it happened. `onerror` hands over only the error (and a
 * reset function): Svelte 5 attaches its "in Foo.svelte" component trail only
 * to an error that escapes every boundary, so there's no component stack to
 * log. The error is passed as an object so the console prints its own stack,
 * which in a dev build names the throwing component's file.
 */
export function logBoundaryError(where: string, error: unknown): void {
	console.error(`${BOUNDARY_ERROR_PREFIX} ${where}`, error);
}
