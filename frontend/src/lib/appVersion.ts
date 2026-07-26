// appVersion.ts — notice when the server is serving a newer build than the
// one this tab is running.
//
// A tab left open across a deploy keeps the OLD build's chunk hashes baked
// into its JS; nothing re-derives them. Those files are gone from the new
// build, and the Go server's SPA fallback answers any unknown path with
// index.html at 200 text/html (cmd/server/main.go, setupFrontend) — so a
// stale chunk never 404s, it arrives as HTML and dies at parse time.
// SvelteKit hard-reloads when a *navigation* hits that, so route changes
// self-heal; what breaks is in-place lazy loading (hover preloads, deferred
// components), which fails silently in the console.
//
// SvelteKit already ships the mechanism: every build writes _app/version.json,
// and `updated.check()` fetches it and flips `updated.current` on a mismatch.
// What we deliberately don't use is `kit.version.pollInterval`, which would
// set a timer — the same cost mistake as the notifications ticker, waking the
// stack for tabs nobody is looking at (see the refresh-on-return note in
// routes/profile/+page.svelte). Checking on "a human just came back" costs
// one request for a static embedded file, touches no DB, and fires at exactly
// the moment a stale tab is about to be used.
//
// In dev this is inert by design: SvelteKit's create_updated_store short-
// circuits `check()` to `false` under DEV, so the banner only ever appears
// against a real production build.

import { updated } from '$app/state';

// Version ids change at most once per deploy, so a returning player never
// needs a fresh answer more often than this. Absorbs the visibilitychange /
// focus overlap (both fire on most tab switches) without extra plumbing.
const CHECK_MIN_INTERVAL_MS = 60_000;

let lastCheckAt = 0;
let inFlight = false;

/**
 * Ask the server whether a newer build exists, at most once per
 * CHECK_MIN_INTERVAL_MS. Flips `updated.current` on a mismatch; never throws.
 */
export async function checkForNewVersion(): Promise<void> {
	// `updated.current` is monotonic — once the server has moved on, it stays
	// moved on, and the only cure is a reload. Re-asking would just confirm
	// what the banner is already telling the player.
	if (inFlight || updated.current) return;

	const now = Date.now();
	if (now - lastCheckAt < CHECK_MIN_INTERVAL_MS) return;
	lastCheckAt = now;
	inFlight = true;
	try {
		await updated.check();
	} catch {
		// Offline, or the server is mid-restart — which is when this is most
		// likely to fail and least likely to matter. The next return retries.
	} finally {
		inFlight = false;
	}
}

/**
 * Check for a new build whenever the player comes back to the app. Returns an
 * unsubscribe function.
 *
 * visibilitychange covers tab switches; focus covers app switches that leave
 * the tab "visible" — the same pair routes/profile/+page.svelte uses, for the
 * same reason.
 */
export function watchForNewVersion(): () => void {
	// A page that just loaded is current by definition. Seed the floor so
	// returning to a tab you only opened seconds ago doesn't spend a request.
	lastCheckAt = Date.now();

	const onVisibility = () => {
		if (document.visibilityState === 'visible') void checkForNewVersion();
	};
	const onFocus = () => void checkForNewVersion();

	document.addEventListener('visibilitychange', onVisibility);
	window.addEventListener('focus', onFocus);
	return () => {
		document.removeEventListener('visibilitychange', onVisibility);
		window.removeEventListener('focus', onFocus);
	};
}

/** Test seam: reset the module-level throttle between cases. */
export function __resetVersionCheckThrottle(): void {
	lastCheckAt = 0;
	inFlight = false;
}
