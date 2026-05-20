// Per-session "Stay out of it" dismissals for Make War takeover.
//
// A non-participant who clicks "Stay out of it" on a pending Make War's
// delay-reveal box adds the plan ID here; MainEventView reads the set and
// lifts the war-box takeover so the player can see the rest of the play
// area again. Resets on reload — there's no server-side persistence.

import { writable } from 'svelte/store';

export const stayedOutOfWar = writable<Set<number>>(new Set());

export function stayOutOfWar(planID: number) {
	stayedOutOfWar.update(s => {
		if (s.has(planID)) return s;
		const next = new Set(s);
		next.add(planID);
		return next;
	});
}
