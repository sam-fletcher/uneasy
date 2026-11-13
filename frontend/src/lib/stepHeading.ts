// stepHeading.ts
// The open step's heading: the ONE place the game says, in words, what it is
// asking of whom. "Your move — aid or interfere" when the viewer is among the
// people the game waits on, "Waiting on otto — the roll" when it is someone else.
//
// This replaces the page-level Waiting On bar. The facts come from the same
// derivation as before (lib/waitingOn.ts, which reads the server's
// acting_player_ids), so who the game waits on can't drift; what changed is
// where it is said and the grammar it is said in. The other places that show
// it do so through a mark, not words: the clock on a player's chip.
//
// Pure (no Svelte, no state) so the grammar is unit-tested on its own.

import type { Player } from '$lib/api';
import type { Waitee, WaitingOnState } from '$lib/waitingOn';

export interface StepHeading {
	/** The whole heading, ready to print. */
	text: string;
	/** The viewer is one of the people the game is waiting on. */
	yours: boolean;
}

/** More than this many names stop being a list and become a count. */
const MAX_NAMED = 3;

const join = (parts: string[]): string =>
	parts.length <= 1 ? (parts[0] ?? '') : `${parts.slice(0, -1).join(', ')} and ${parts[parts.length - 1]}`;

function namesOf(waitees: Waitee[], players: Player[]): string {
	const names: string[] = [];
	for (const w of waitees) {
		if (w.kind === 'player') names.push(players.find(p => p.id === w.playerID)?.display_name ?? 'someone');
		else if (w.kind === 'label') names.push(w.text);
	}
	if (names.length > MAX_NAMED) return `${names.length} players`;
	return join(names);
}

/**
 * The heading for the step the game is on, or null when it is waiting on no
 * one (a beat the server ends by itself) — a step with nothing asked of anyone
 * has no "current ask" to state.
 *
 * When the viewer is among the waitees the heading is theirs and names no one
 * else: the clocks on the other chips already say who else is being waited on,
 * and the heading is about what the viewer must do.
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

	const lead = yours ? 'Your move' : `Waiting on ${namesOf(waitees, players) || 'others'}`;
	return { yours, text: state.stepLabel ? `${lead} — ${state.stepLabel}` : lead };
}
