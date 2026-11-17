// Loading a simultaneous reveal for SimultaneousRevealInput.
//
// The widget's reveal id comes from the plan's resolution data, and that id is
// cleared when the plan finishes. A pick in flight can outlive it: the viewer
// submits their face, the last reveal completes, the plan resolves and the panel
// drops the id, and then the widget's own follow-up refresh reads the prop, finds
// it null, and asks for `GET /api/reveals/null`. This is the one place the widget
// fetches from, so "no id, no request" is decided here rather than at each of
// its call sites (mount, a WebSocket nudge, the end of a submit).
import { getReveal, type SimultaneousReveal } from '$lib/api';

/** The reveal with this id, or null when there is no id to ask for (no request is made). */
export function loadReveal(
	revealID: number | null | undefined,
	fetchReveal: (id: number) => Promise<SimultaneousReveal> = getReveal,
): Promise<SimultaneousReveal | null> {
	if (revealID == null) return Promise.resolve(null);
	return fetchReveal(revealID);
}
