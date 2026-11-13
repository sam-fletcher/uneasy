// The two viewport breakpoints of the layout width system
// (docs/STYLE_GUIDE.md "Layout widths"). Every matchMedia in the app composes
// from these constants. CSS `@media` can't read variables, so the same literals
// also appear in the few stylesheets that need one (the sheet, the modal
// shell); layoutTokens.test.ts keeps every occurrence in sync.
//
// The table has three panes (Public Record, Play, Chat). A wide window lays them
// side by side; a narrow one shows fewer at a time and switches with tabs. Every
// pane is drawn at phone width, so a breakpoint is simply the width at which
// another phone-width pane fits:
//
//     phone        one pane at a time, a tab bar at the foot
//     two columns  Chat docked on the right; the left column switches between
//                  Public Record and Play with two tabs at its foot
//     three        Public Record, Play and Chat side by side, no tabs

/** The narrowest a column is drawn: the narrowest mainstream phone. */
export const COLUMN_MIN_PX = 360;

/** The widest a column is drawn: the widest mainstream phone. Past this a
 *  column stops growing and the slack becomes margin. */
export const COLUMN_MAX_PX = 440;

/** The gap between columns, and the edge padding beside the outer two. */
export const GUTTER_PX = 8;

/** Two columns fit: 8 + 360 + 8 + 360 + 8, an upright iPad mini. */
export const TWO_COLUMN_PX = 744;

/** Three columns fit: 8 + 360 + 8 + 360 + 8 + 360 + 8, an iPad on its side. */
export const THREE_COLUMN_PX = 1112;

export const twoColumnQuery = `(min-width: ${TWO_COLUMN_PX}px)`;
export const threeColumnQuery = `(min-width: ${THREE_COLUMN_PX}px)`;
