// Cross-component UI highlight state. Currently used by PlanPanel (writer)
// and PublicRecord (reader) so hovering/selecting a plan in the prep picker
// highlights its target row in the public record rail. Reusable for any
// future "draw the user's eye to row N" affordance.

import { writable } from 'svelte/store';

export const highlightedRow = writable<number | null>(null);
