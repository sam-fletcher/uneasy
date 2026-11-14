// sheetContext.ts — what the war's sheet tells the panel inside it.
//
// While a war owes a cost of battle or a surrender claim, the play area is taken
// over by that war's panel, and the sheet (opened from its band) holds the same
// panel. Two live copies of the payment form would be two places to act on one
// obligation, and under the sheet a player can't see the heading that says what
// is being asked. So the sheet says whether this war is being settled in the play
// area, and the panel then leaves the blocking forms there and says so in a line.
//
// Passed as a getter, so the panel reads it reactively.

import { getContext, setContext } from 'svelte';

type Getter = () => boolean;
const KEY = Symbol('warSheetSettledInPlay');

export function provideSettledInPlay(settledInPlay: Getter): void {
	setContext(KEY, settledInPlay);
}

/** True when this panel is in a war's sheet and the war is being settled in the play area. */
export function useSettledInPlay(): Getter {
	return getContext<Getter | undefined>(KEY) ?? (() => false);
}
