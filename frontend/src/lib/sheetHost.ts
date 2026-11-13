// sheetHost.ts — which column a sheet rises over.
//
// From two columns up, a sheet takes its column's width and opens over the column
// that launched it, leaving the others visible and usable: Tones opens over Chat,
// the war's panel over Play, and a sheet opened from the title bar or the player
// row (Help, a player's retinue) over Play, or over the left column when there
// are two. On a phone there is one column, so a sheet covers the screen.
//
// A sheet can't tell which column it is in from the DOM, because the page
// renders some sheets at its top level and the columns are siblings, so the
// answer travels by context: a column provides its own element for everything
// inside it, and the page provides the "primary" column for the title bar and
// the sheets it renders itself. RetinueSheet asks for it when it opens and
// measures it. A sheet with no host (the profile page, the landing page) covers
// the viewport, which is the old behaviour.

import { getContext, setContext } from 'svelte';

/** Returns the element the sheet should cover, or null for the whole viewport.
 *  A function, not an element, because the page's answer depends on the layout
 *  and on which pane is showing, so it must be asked when the sheet opens. */
export type SheetHost = () => HTMLElement | null;

const KEY = Symbol('sheetHost');

export function provideSheetHost(host: SheetHost): void {
	setContext(KEY, host);
}

export function useSheetHost(): SheetHost | undefined {
	return getContext<SheetHost | undefined>(KEY);
}
