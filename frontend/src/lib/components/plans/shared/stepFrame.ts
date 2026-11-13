// stepFrame.ts
// What PlanPanel hands down to the plan's card so it can lay the plan's steps
// out in order: the roll step slots in between what is finished and what is
// open, and the open step's heading sits where the open step starts.
//
// It travels by context because the card is drawn by whichever of the twelve
// plan panels is on screen, several components below PlanPanel, and none of
// them should have to thread it through. A getter (not a value), like
// demand/activeRollContext, so a card always reads the current state.
//
// Outside a PlanPanel (the war drawer, a delay reveal drawn by the page) there
// is no frame: a card then draws only its own lines, and whoever mounted it
// draws the heading and the roll.

import { getContext, setContext, type Snippet } from 'svelte';
import type { StepHeading } from '$lib/stepHeading';

export interface StepFrame {
	/** The roll as a step, or null when no roll is on the table. The card draws
	 *  it after its finished lines (open or finished: it knows which). */
	roll: Snippet | null;
	/** The roll is still open, so it carries the heading itself. */
	rollOpen: boolean;
	/** The open step's heading: the current ask. Null when nothing is asked. */
	heading: StepHeading | null;
	/** A card calls this while it is on screen, so the page knows the roll is
	 *  drawn inside it and doesn't draw it a second time. The returned function
	 *  lets go. */
	claimRoll: () => () => void;
}

/** Exported so a test can provide a frame without mounting a PlanPanel. */
export const STEP_FRAME_KEY = Symbol('stepFrame');

export function provideStepFrame(get: () => StepFrame): void {
	setContext(STEP_FRAME_KEY, get);
}

/** The frame PlanPanel provided, or null when there isn't one. */
export function useStepFrame(): (() => StepFrame) | null {
	return getContext<(() => StepFrame) | undefined>(STEP_FRAME_KEY) ?? null;
}
