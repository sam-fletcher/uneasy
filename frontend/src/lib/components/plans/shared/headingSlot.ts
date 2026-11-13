// headingSlot.ts
// Who draws the open step's heading. MainEventView draws it at the top of the
// play surface BY DEFAULT, so no state can be left without one (a plan that is
// pending, a scene before it has loaded, a war's cost with no war on screen, a
// plan panel still loading, Make War's own panel). A surface that draws the
// heading itself, at the top of ITS open step (a plan's card, the preparation
// step, an open roll), CLAIMS it for as long as it does, and the default steps
// aside. The same pattern as the roll's claim (stepFrame.claimRoll): a claim is
// held while the claimer is on screen and let go when it goes.
//
// A claimer must claim only when it really draws a heading (it was given one),
// or a state would again show none.

import { getContext, setContext } from 'svelte';

export interface HeadingSlot {
	/** Hold the heading. Returns the function that lets go. */
	claim: () => () => void;
}

export const HEADING_SLOT_KEY = Symbol('headingSlot');

export function provideHeadingSlot(slot: HeadingSlot): void {
	setContext(HEADING_SLOT_KEY, slot);
}

/** The slot MainEventView provided, or null outside it (Shake-Up draws a roll with no heading). */
export function useHeadingSlot(): HeadingSlot | null {
	return getContext<HeadingSlot | undefined>(HEADING_SLOT_KEY) ?? null;
}
