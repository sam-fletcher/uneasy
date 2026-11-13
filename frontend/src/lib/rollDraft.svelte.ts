// rollDraft.svelte.ts
// What the viewer has picked on the open roll but not yet committed: their side
// (aid or interfere), the assets they mean to leverage and the banked dice they
// mean to spend. Nothing is sent until they press the button, so until then the
// picks live only here.
//
// They are kept OUT of DiceRollPanel on purpose. The roll is drawn in one of two
// places (inside the plan's card, or alone on the page), and moves from the one
// to the other when the card appears (a plan panel still loading when the roll
// opened) or when a panel remounts its card mid-roll. A component's own state does
// not survive that move; this does, because it is keyed by the roll. A different
// roll starts a fresh draft, and a commit clears the picks.
//
// One draft at a time is enough: a game has at most one open roll, and a tab shows
// one table.

import type { RollIntent } from '$lib/api';

class RollDraft {
	rollId = $state(-1);
	intent = $state<RollIntent | null>(null);
	assetIds = $state<Set<number>>(new Set());
	bankedIds = $state<Set<number>>(new Set());

	/** Start (or keep) the draft for this roll. The same roll keeps what was picked. */
	begin(rollId: number): void {
		if (this.rollId === rollId) return;
		this.rollId = rollId;
		this.intent = null;
		this.assetIds = new Set();
		this.bankedIds = new Set();
	}

	/** The picks have been sent (or dropped): the side stays, the dice picked do not. */
	clearPicks(): void {
		this.assetIds = new Set();
		this.bankedIds = new Set();
	}
}

export const rollDraft = new RollDraft();
