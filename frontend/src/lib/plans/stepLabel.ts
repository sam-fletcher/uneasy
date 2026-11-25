// stepLabel.ts
// What a plan calls the step it is on, for the open step's heading: the step alone,
// capitalised, under the who-line ("Waiting on otto" / "Your move"; see lib/stepHeading.ts).
//
// The server knows only that a plan is resolving (row-state kind `plan_resolving`),
// not which of the plan's own steps it is on. The plan's resolution data knows, so
// each plan registers one pure function here (plans/registry.ts) that reads it and
// answers with a phrase. It is a function and not something the panel reports for
// three reasons: the page's heading and the card's heading come from the same call
// and can't disagree; the label is right before a lazily-loaded panel has arrived
// (a panel is its own chunk); and it can be unit-tested without mounting anything.
//
// WORDING. A label is a lower-case phrase naming the step; the heading capitalises it
// and sets it under the who-line, so it never has to finish a sentence:
//
//     Your move                    Waiting on otto
//     Place the addendum           Place the addendum
//
// It is still the same words for every viewer and never speaks to the reader
// ("what have you learned?" would be wrong for the watcher), so what one player
// reads is what every other player reads. It never names the plan: the title bar
// and the card's first line ("otto prepared Seek Answers") already do.
// When several things are owed at once and no one of them is THE step, it names
// what they have in common ("carry out the picks"), not the first of them.

import type { DiceRoll, Plan } from '$lib/api';

export interface StepLabelInput {
	/** The plan the row state names. */
	plan: Plan;
	/** Every plan in the game (a Make Demands win is read off its demand). */
	plans: Plan[];
	/** The plan's OWN roll — the open or the last resolved one — or null. */
	roll: DiceRoll | null;
	/** The server's acting set for the row (`row_state.acting_player_ids`). A label
	 *  reads it only where one phase has two different asks and the set says which
	 *  is open (Propose Decree's council); never to name a player. */
	acting: number[];
}

/** A plan's answer. null means "I can't tell which step this is": the caller says
 *  UNKNOWN_STEP. Most often that is a state that lasts a moment (the server
 *  finishing a plan on its own) rather than one a player could be waiting in. */
export type StepLabelFn = (input: StepLabelInput) => string | null;

/** What the heading says when a plan genuinely cannot tell. */
export const UNKNOWN_STEP = 'the next step';

/** For a plan with no step of its own to name. */
export const noStepLabel: StepLabelFn = () => null;

/** `roll` if it belongs to `plan`, else null. A table can hold another plan's
 *  roll (a Make Demands roll beside the plan it targets), and a label must read
 *  only its own. */
export function ownRoll(plan: Plan, roll: DiceRoll | null | undefined): DiceRoll | null {
	return roll != null && roll.plan_id === plan.id ? roll : null;
}

/** How many times `option` appears among a plan's committed picks. */
export function countOf(options: readonly string[], option: string): number {
	return options.filter(o => o === option).length;
}

/**
 * The label for several sub-steps that are all open at once, in the order the panel
 * draws them. One owed → its own name. More than one → `many`, because the heading
 * is singular and naming the first would leave the actor thinking the rest were not
 * theirs to do. None owed → `none` (the step that follows them).
 */
export function oneOf(owed: readonly string[], many: string, none: string | null): string | null {
	if (owed.length === 0) return none;
	return owed.length === 1 ? owed[0] : many;
}
