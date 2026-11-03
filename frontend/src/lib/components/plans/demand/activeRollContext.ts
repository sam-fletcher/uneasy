import { getContext, setContext } from 'svelte';
import type { DiceRoll } from '$lib/api';

export const ACTIVE_ROLL_KEY = Symbol('activeRoll');

/** PlanPanel publishes the page's active roll as a getter, so the demand
 *  overlay — nested several components down inside a per-plan panel — can tell
 *  whether the roll its forms act on is still open without every panel having
 *  to thread a prop through. */
export function provideActiveRoll(get: () => DiceRoll | null) {
	setContext(ACTIVE_ROLL_KEY, get);
}

/** The getter provideActiveRoll set, or one that answers null when nothing did. */
export function useActiveRoll(): () => DiceRoll | null {
	return getContext<(() => DiceRoll | null) | undefined>(ACTIVE_ROLL_KEY) ?? (() => null);
}
