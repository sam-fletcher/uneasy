// Component-side half of the Back-button overlay dismissal described in
// overlayHistory.ts. Call it once, at component init, from anything that
// renders a surface covering the screen on mobile:
//
//     dismissOnBack(() => open, onClose);                 // always modal
//     dismissOnBack(() => expanded, close, () => !wide);  // overlay on phones,
//                                                         // docked column above
//
// `enabled` is what keeps the docked layouts out of it: the chat panel and the
// Public Record stop being overlays past their dock breakpoints (790 / 1070 —
// $lib/breakpoints), where there is no overlay to dismiss and Back should go
// on meaning "leave the page". Crossing a breakpoint while open releases the
// entry, which is the right answer in both directions.

import { pushState } from '$app/navigation';
import { page } from '$app/state';
import { untrack } from 'svelte';
import { createOverlayHistory, type OverlayHandle } from './overlayHistory';

// Shallow routing rather than raw history.pushState: SvelteKit's router keeps
// its own index in history.state and gets confused if entries appear behind
// its back, which would break real navigation, not just overlays.
const overlayHistory = createOverlayHistory({
	push: (depth) => pushState('', { overlayDepth: depth }),
	back: (count) => history.go(-count),
	href: () => location.href,
});

export function dismissOnBack(
	isOpen: () => boolean,
	close: () => void,
	enabled: () => boolean = () => true,
) {
	let handle: OverlayHandle | null = null;

	$effect(() => {
		const want = isOpen() && enabled();
		untrack(() => {
			if (want && handle === null) {
				handle = overlayHistory.acquire(close);
			} else if (!want && handle !== null) {
				overlayHistory.release(handle);
				handle = null;
			}
		});
	});

	// Mirror the router's view of the stack into the coordinator. Every caller
	// runs this, so with several overlay components mounted `observe` is called
	// once per mounted component per change — it's idempotent, and a watcher
	// per component is what lets the helper stand alone: HelpButton renders in
	// the global layout header, outside any table page that could host a single
	// shared watcher.
	$effect(() => {
		const depth = page.state.overlayDepth ?? 0;
		const href = page.url.href;
		untrack(() => overlayHistory.observe(depth, href));
	});

	// Unmounting with the overlay still open — the table shell tearing down
	// under a redirect — must not strand an owner the coordinator will keep
	// trying to reconcile.
	$effect(() => () => {
		if (handle !== null) {
			overlayHistory.release(handle);
			handle = null;
		}
	});
}
