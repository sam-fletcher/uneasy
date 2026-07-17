// The two viewport breakpoints of the layout width system
// (docs/STYLE_GUIDE.md "Layout widths"; derivations and decision history in
// adr/LAYOUT_WIDTHS_PLAN.md). Every matchMedia in the app composes from
// these constants. CSS `@media` can't read variables, so the same literals
// also appear in the shell stylesheets (table +page.svelte, ChatPanel,
// PublicRecord); layoutTokens.test.ts keeps every occurrence in sync.

/** Chat docks as a right column: 44 rail + 8 + 360 main + 8 + 360 chat + 8. */
export const CHAT_DOCK_PX = 790;

/** The Public Record's width — the overlay and the docked panel are the same
 *  token. Frozen by eye (adr/LAYOUT_WIDTHS_PLAN.md; safe retune range
 *  246–316). To retune: change this constant, then let layoutTokens.test.ts
 *  point you at the two CSS mirrors (PublicRecord's `.expanded` width and
 *  the table page's record grid column). Above 288, RECORD_DOCK_PX must be
 *  re-derived too — the test enforces dock ≥ width + 752. */
export const RECORD_WIDTH_PX = 280;

/** Record rail/overlay becomes a permanent panel:
 *  8 + 280 record + 8 + 360 main + 8 + 360 chat + 8 = 1032, pinned round. */
export const RECORD_DOCK_PX = 1040;

export const chatDockQuery = `(min-width: ${CHAT_DOCK_PX}px)`;
export const recordDockQuery = `(min-width: ${RECORD_DOCK_PX}px)`;
