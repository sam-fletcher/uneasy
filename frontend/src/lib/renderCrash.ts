// renderCrash.ts — files a render crash that a <svelte:boundary> caught with the
// server (POST /api/render-crashes, handler/render_crash.go), so the owner can
// read it. boundaryError.ts only logs locally; the boundary's onerror calls
// both. This module imports $app/environment for the build id, which the
// Playwright suite can't load, so it can't sit in boundaryError.ts.
//
// Only a throw during rendering reaches here: that is all a boundary catches.
// Transport failures and 4xx/5xx responses from other calls are not reported.
// A failure of this report is swallowed, never logged and never re-reported,
// so a crash in the reporter can't turn into another crash.

import { version } from '$app/environment';
import { submitRenderCrash } from './api/support';

// Field caps. These mirror handler/render_crash.go. The server refuses an
// over-long field rather than cutting it, so the cut happens here.
export const RENDER_CRASH_LIMITS = {
	boundary: 200,
	message: 1000,
	stack: 8000,
	route: 500,
	build: 100,
} as const;

function clip(text: string, max: number): string {
	return text.length > max ? text.slice(0, max) : text;
}

/**
 * Sends one report for one caught render crash. Call it once per crash, from
 * the boundary's onerror. Never throws and never returns a promise to await.
 *
 * @param boundary  where the boundary is, as logBoundaryError labels it
 * @param error     the value the boundary caught
 * @param gameId    the game on screen, when the crash was on a table page
 */
export function reportRenderCrash(boundary: string, error: unknown, gameId?: number): void {
	try {
		const isError = error instanceof Error;
		// Trimmed before the check: the server trims too, so a whitespace-only message
		// would be refused as empty and the report lost.
		const message = String(isError ? error.message : error).trim() || '(no message)';
		submitRenderCrash({
			boundary: clip(boundary, RENDER_CRASH_LIMITS.boundary),
			message: clip(message, RENDER_CRASH_LIMITS.message),
			stack: isError && error.stack ? clip(error.stack, RENDER_CRASH_LIMITS.stack) : undefined,
			route: clip(typeof location === 'undefined' ? '' : location.pathname, RENDER_CRASH_LIMITS.route),
			build: clip(version, RENDER_CRASH_LIMITS.build),
			game_id: gameId,
		}).catch(() => {
			// The report failed (offline, logged out, server down). Say nothing:
			// the crash itself is already in the console from logBoundaryError.
		});
	} catch {
		// A value whose message or stack can't be read. The report is dropped
		// rather than thrown back into the boundary's handler.
	}
}
