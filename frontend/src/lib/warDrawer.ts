// Cross-component state for the header "War" button + drawer.
// MainEventView (writer) publishes the count of non-ended wars and renders
// the drawer. The page-level header (reader) shows the button when there's
// at least one active war and toggles `warDrawerOpen` on click.

import { writable } from 'svelte/store';

export const warDrawerOpen = writable(false);
export const activeWarCount = writable(0);
