// stepHeading.ts
// The open step's heading: the ONE place the game says, in words, what it is
// asking of whom. Two lines (Round 6, "Inside a step" in the decisions):
//
//     Your move · also ◆ alice          the who-line, 14px
//     Aid or interfere                  the heading, the step alone, 18px
//
// The who-line says "Your move" (bright gold) when the viewer is among the people
// the game waits on, naming anyone else owed beside them; otherwise "Waiting on"
// (faint) and the players' names in their colours, at most two, then "and others".
// The step label never has to finish a sentence: it is the step alone, capitalised.
//
// This replaces the page-level Waiting On bar. The facts come from the same
// derivation as before (lib/waitingOn.ts, which reads the server's
// acting_player_ids), so who the game waits on can't drift. The other places that
// show it do so through a mark, not words: the clock on a player's chip.
//
// Pure (no Svelte, no state) so the grammar is unit-tested on its own.

import type { Player } from '$lib/api';
import type { Waitee, WaitingOnState } from '$lib/waitingOn';

/** One name on the who-line: a player (drawn in their colour, with their shape) or
 *  plain words (a lobby's "1 more player to join", or "someone" for a player the
 *  roster can't find). */
export type WhoPart = { player: Player } | { text: string };

export interface StepHeading {
	/** The step alone, capitalised ("Aid or interfere"). Empty when the state has no
	 *  step to name; the who-line then stands as the heading. */
	label: string;
	/** The viewer is one of the people the game is waiting on. */
	yours: boolean;
	/** Who else is owed, for the who-line: every player but the viewer, at most
	 *  `MAX_NAMED` of them, then any free-text waitee. */
	who: WhoPart[];
	/** More players are owed than are named ("and others"). */
	more: boolean;
}

/** At most this many PLAYERS are named; the rest become "and others". */
export const MAX_NAMED = 2;

export const capitalise = (s: string): string => (s ? s.charAt(0).toUpperCase() + s.slice(1) : s);

/**
 * The heading for the step the game is on, or null when it is waiting on no
 * one (a beat the server ends by itself) — a step with nothing asked of anyone
 * has no "current ask" to state.
 *
 * When the viewer is among the waitees the line is theirs ("Your move") and
 * names the others owed beside them ("· also alice"): the heading must be
 * readable at a glance, and the colours tell a watcher whose pieces the step's
 * colours belong to.
 */
export function stepHeading(
	state: WaitingOnState,
	currentPlayerID: number | null,
	players: Player[],
): StepHeading | null {
	const waitees = state.waitees;
	if (waitees.length === 0) return null;

	const everyone = waitees.some(w => w.kind === 'everyone');
	const me = currentPlayerID != null
		&& waitees.some(w => w.kind === 'player' && w.playerID === currentPlayerID);
	const yours = everyone || me;

	// The other players owed: the viewer is never named on their own line.
	const owed: WhoPart[] = [];
	for (const w of waitees as Waitee[]) {
		if (w.kind !== 'player' || w.playerID === currentPlayerID) continue;
		const p = players.find(x => x.id === w.playerID);
		owed.push(p ? { player: p } : { text: 'someone' });
	}
	const more = owed.length > MAX_NAMED;
	const who: WhoPart[] = owed.slice(0, MAX_NAMED);
	// A label waitee ("1 more player to join") is free text, not a player: it is
	// said as it is written and never counted toward the cap.
	for (const w of waitees) if (w.kind === 'label') who.push({ text: w.text });

	// Not the viewer's and nobody nameable: still say it is someone else's.
	if (!yours && who.length === 0) who.push({ text: 'others' });

	const label = capitalise(state.stepLabel ?? '');
	return { label, yours, who, more };
}
