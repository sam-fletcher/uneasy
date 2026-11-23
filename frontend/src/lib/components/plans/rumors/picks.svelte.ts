// picks.svelte.ts
// What the actor has picked on Spread Rumors' make/mar list but not yet sent.
//
// It is made by the panel, not by any one step, because it has to outlive the steps
// that read it. The option picker, the "choose the assets to take" screen after it
// and the consent wait after that are three different steps, and the picker is not on
// screen for the last of them: a take-asset request leaves the counts standing, and
// when the victim disagrees the actor comes back to the picker with the other picks
// still in place. State owned by a step would be gone by then.
//
// `busy` is one flag for the three buttons that act on the picks (apply, request
// consent, answer a request), as it was when they all sat in one component.

export type TakeStep = 'options' | 'assets';

export class RumorPicks {
	/** How many times each option is picked (repeatable; the total is the dice result). */
	counts = $state<Record<string, number>>({
		break_target: 0, leverage_target: 0, take_asset: 0,
		hide_source: 0, reveal_source: 0,
	});
	/** Two-step apply: a take-asset pick first asks for the specific assets to take. */
	takeStep = $state<TakeStep>('options');
	takeSelected = $state<number[]>([]);
	busy = $state(false);

	total = $derived(Object.values(this.counts).reduce((a, b) => a + b, 0));
}
