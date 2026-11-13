// chatView.ts — what the chat feed says about a plan's resolution, as plain data.
//
// ChatPanel draws a plan's resolution as one tappable header row (the plan's
// mark, its name, the preparer's shape, a status word, a caret) with its lines
// behind a thin rule. The words and the choices behind that row are decided
// here rather than in the markup so they can be tested without a DOM:
//
//   - which mark: the flag, or war's barbute for Make War;
//   - the plan's name;
//   - the status word and what colour (if any) it may wear;
//   - the label on the "new messages" divider.
//
// Pure: no Svelte, no fetches.

import type { PlanType } from '$lib/api';
import { PLAN_SHORT } from '$lib/components/plans/shared';
import type { PlanOutcome } from '$lib/chatFeed';

/** A status word and the colour class it may take. `neutral` is plain secondary
 *  text: a state ("Resolving") or a result nobody won or lost. */
export interface ResolutionStatus {
	word: string;
	tone: 'neutral' | 'success' | 'danger';
}

/**
 * Plans the book gives no Make or Mar: Make War and Clandestinely Liaise end
 * another way, though the app stores "make" when they do and the log says "Make
 * War succeeded.". A status word of Make would claim a result the game doesn't
 * have, so these two show none once they are over.
 */
const NO_MAKE_OR_MAR: ReadonlySet<PlanType> = new Set(['make_war', 'clandestinely_liaise']);

/**
 * The status word for a plan's header. While there is no terminal post it is
 * "Resolving" (in secondary text, as every plain state is); once there is one,
 * Make in green or Mar in red, "Fell through" and "No effect" in plain text,
 * because nothing was won or lost. `planType` may be null (the plan list hasn't
 * loaded): then Make and Mar are shown as they come.
 */
export function resolutionStatus(
	outcome: PlanOutcome | null,
	planType: PlanType | null
): ResolutionStatus | null {
	if (outcome == null) return { word: 'Resolving', tone: 'neutral' };
	const noMakeMar = planType != null && NO_MAKE_OR_MAR.has(planType);
	switch (outcome) {
		case 'make':
			return noMakeMar ? null : { word: 'Make', tone: 'success' };
		case 'mar':
			return noMakeMar ? null : { word: 'Mar', tone: 'danger' };
		case 'cancelled':
			return { word: 'Fell through', tone: 'neutral' };
		case 'other':
			return { word: 'No effect', tone: 'neutral' };
	}
}

/** The mark a plan's header wears: the flag, or war's barbute. One mark means
 *  war everywhere, and Make War is the plan that is a war. */
export function planMarkFamily(planType: PlanType | null): 'plan' | 'war' {
	return planType === 'make_war' ? 'war' : 'plan';
}

/**
 * The plan's name for its header. The plan list knows it for certain; failing
 * that, the `plan.resolving` post always reads "<Name> is resolving.", so the
 * name is what's left when that ending comes off. Posts are plain text here,
 * never log markup.
 */
export function planHeaderName(planType: PlanType | null, resolvingBody: string): string {
	if (planType != null && PLAN_SHORT[planType]) return PLAN_SHORT[planType];
	return resolvingBody.replace(/\s+is resolving\.?\s*$/i, '').trim() || resolvingBody;
}

/** The unread divider's words: the count lives here and not on the tab. */
export function unreadDividerLabel(count: number): string {
	return `${count} new`;
}

/** What a screen reader hears for the unread divider. */
export function unreadDividerAria(count: number): string {
	return count === 1 ? '1 new message' : `${count} new messages`;
}
