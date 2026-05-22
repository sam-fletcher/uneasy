// Cross-component state for the header "War" button + drawer.
// MainEventView (writer) publishes separate pending- and active-war counts
// and renders the drawer. The page-level header (reader) shows the button
// when either count is non-zero, picks its colour from the mix (yellow =
// only pending, red = only active, orange = both), and toggles
// `warDrawerOpen` on click.

import { writable } from 'svelte/store';

export const warDrawerOpen = writable(false);
// Wars planned but not yet started (origin plan still 'pending').
export const pendingWarCount = writable(0);
// Wars currently underway (origin plan resolved, war row still 'active').
export const activeWarCount = writable(0);
