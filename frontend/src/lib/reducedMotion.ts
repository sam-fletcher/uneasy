// reducedMotion.ts — the one place JS asks whether the reader wants motion.
//
// CSS owns as much of this as it can: an animation that lives in a stylesheet
// gets its guard in the same file, next to the keyframes
// (shared/jumpPulse.css, shared/choicePip.css). This module exists for the
// part CSS cannot reach — `scrollIntoView`, whose smoothness is a JS argument,
// not a property.
//
// The distinction the guard draws (adr/PROLOGUE_UX_ROUND2_PLAN.md §3c, owner's
// ruling): `reduce` removes *motion*, not *feedback*. A guarded interaction
// still scrolls, still changes state, still shows its confirmation — it just
// arrives instead of travelling. Suppressing the scroll as well would take away
// the positional feedback the prologue's motion beat exists to add.

/**
 * Whether the reader has asked for reduced motion right now.
 *
 * Read at call time rather than cached: the setting is a live media query
 * (macOS and iOS both flip it without a reload), and every caller is a
 * one-shot event handler, so there is nothing to subscribe to. Safe during
 * SSR, where there is no matchMedia and no motion either.
 */
export function prefersReducedMotion(): boolean {
	if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
	return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}

/** The `behavior` to hand `scrollIntoView` / `scrollTo`: the reader still gets
 *  taken to the element, they just get there instantly. */
export function scrollBehavior(): ScrollBehavior {
	return prefersReducedMotion() ? 'auto' : 'smooth';
}
