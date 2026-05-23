// Per-session "Stay out of it" choices for Make War delay reveals.
//
// During a Make War delay reveal, non-participants see three options in
// the WarStatus panel: Join Side 1, Join Side 2, or Stay out of it. The
// first two are real game actions (the player becomes a participant);
// "Stay out of it" is purely a communication device — it tells the player
// they have the option to remain a bystander, and clicking it hides the
// three buttons in favour of a "You're staying out of it" indicator so
// the choice feels deliberate.
//
// Game-flow note: a Stay-out doesn't gate anything. The delay reveal
// completes when every reveal entry (named participants + volunteer
// joiners) has a die face submitted. Players who clicked Stay-out and
// players who simply never clicked anything are treated identically by
// the reveal-completion logic — see openDelayRevealPlan in row_state.go
// and the reveal handler in reveals.go.
//
// This store is intentionally per-session (no server-side persistence):
// the play-area panel itself remains visible to every player regardless,
// so an across-reload reset just re-shows the three options. The shared
// view stays consistent; only this player's input affordance differs.

import { writable } from 'svelte/store';

export const warStayOuts = writable<Set<number>>(new Set());

export function stayOutOfWar(planID: number) {
	warStayOuts.update(s => {
		if (s.has(planID)) return s;
		const next = new Set(s);
		next.add(planID);
		return next;
	});
}
