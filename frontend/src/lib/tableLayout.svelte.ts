// tableLayout.svelte.ts — how many columns the table page is drawing right now.
//
// The table's three panes show as one, two or three columns depending on the
// viewport (lib/breakpoints.ts). Most of that is CSS, but the page also has to
// know which it is in: a phone gets a tab bar of three tabs, two columns get a
// tab bar of two, and three columns get none; a sheet covers the whole screen
// on a phone and one column above it. So the breakpoints are read once, here,
// and everything else reads `tableLayout.mode`.
//
// It is initialised when the module loads, before the first render, so the
// first paint is already the right layout and nothing flips on mount (the app
// has no server rendering, so `window` is there).

import { threeColumnQuery, twoColumnQuery } from '$lib/breakpoints';
import type { LayoutMode } from '$lib/components/table/panes';

export type { LayoutMode };

function read(): LayoutMode {
	if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return 'phone';
	if (window.matchMedia(threeColumnQuery).matches) return 'three';
	if (window.matchMedia(twoColumnQuery).matches) return 'two';
	return 'phone';
}

/** Reactive. Read `tableLayout.mode`; never assign it. */
export const tableLayout = $state<{ mode: LayoutMode }>({ mode: read() });

/** Start following the viewport. Call once from the table page's onMount and
 *  call the returned function on the way out. It re-reads on mount as well, in
 *  case the window changed between this module loading and the page mounting. */
export function watchTableLayout(): () => void {
	const queries = [window.matchMedia(twoColumnQuery), window.matchMedia(threeColumnQuery)];
	const sync = () => {
		tableLayout.mode = read();
	};
	sync();
	for (const q of queries) q.addEventListener('change', sync);
	return () => {
		for (const q of queries) q.removeEventListener('change', sync);
	};
}
