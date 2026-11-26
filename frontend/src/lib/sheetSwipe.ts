// sheetSwipe.ts — drag a bottom sheet down to dismiss it (RetinueSheet's
// gesture). Every sheet in the app comes through RetinueSheet, so this is the
// only place that listens for it.
//
// WHY TOUCH EVENTS, NOT POINTER EVENTS. A sheet's body scrolls, and the same
// finger has to be able to scroll it *or* drag the whole sheet. With pointer
// events that hand-over has to be declared up front through `touch-action`,
// and iOS Safari can't switch from scrolling to dragging partway through a
// gesture. Touch events can: a non-passive `touchmove` listener may call
// `preventDefault()` at the moment the drag is claimed. The listener has to be
// `{ passive: false }` for that call to count.
//
// WHEN A DRAG IS CLAIMED. Decided once per gesture, at the first move past a
// few pixels of slop: the finger is going mostly downward AND the body is
// scrolled to the very top (the body, and any scrolling region under the
// finger). Anything else (moving up, moving sideways, a body
// scrolled partway) is left alone for the rest of that gesture, so the body
// scrolls as normal, and a long scroll-up flick that happens to reach the top
// doesn't carry on into closing the sheet; the owner lifts and swipes again.
// The deciding move has to happen before the browser starts scrolling, which is
// why the slop is smaller than any browser's own.
//
// Never started on a text field: dragging there selects text or moves the
// caret, and the feedback forms and Tones have them.

import { prefersReducedMotion } from '$lib/reducedMotion';

/** Pixels of movement before the gesture is classified. Under every browser's
 *  own scroll slop, so the classification lands before scrolling begins. */
export const SLOP_PX = 5;
/** Past this share of the sheet's height, letting go closes it. */
export const DISMISS_FRACTION = 0.25;
/** …or a flick faster than this (px per ms) closes it whatever the distance. */
export const FLICK_PX_PER_MS = 0.5;
/** Only the last stretch of the drag counts towards the flick speed. */
const VELOCITY_WINDOW_MS = 100;
const EXIT_MS = 180;
const SPRING_MS = 200;

/** Elements a drag must never start on. */
const TEXT_FIELD = 'input, textarea, select, [contenteditable]:not([contenteditable="false"])';

/** Whether letting go at `dy` pixels down, moving at `velocity` px/ms, closes
 *  a sheet `height` pixels tall. Pure, so the thresholds are unit-testable. */
export function shouldDismiss(dy: number, height: number, velocity: number): boolean {
	if (dy <= 0) return false;
	return dy > height * DISMISS_FRACTION || velocity > FLICK_PX_PER_MS;
}

/** Decide whether the first real movement of a touch is a drag of the sheet:
 *  mostly downward, with the body at the top. `scrollTop` can be negative on
 *  iOS (rubber-banding), hence `<= 0`. */
export function claimsDrag(dx: number, dy: number, scrollTop: number): boolean {
	return dy > 0 && dy > Math.abs(dx) && scrollTop <= 0;
}

export interface SwipeOptions {
	/** Read at the start of each touch, not once: the sheet's mode changes with the window. */
	enabled: () => boolean;
	onClose: () => void;
	/** The scrim, faded in step with the drag. */
	scrim: () => HTMLElement | null;
}

type Sample = { t: number; y: number };

/** The largest `scrollTop` among the vertical scrollers between `from` and the
 *  sheet (inclusive of `from`, exclusive of `root`). A sheet can scroll in more
 *  than one place: its own body, or a region inside its content (Help's tab
 *  panel has a fixed height and scrolls itself, leaving the body at 0). A drag
 *  may only claim the gesture when all of them are at the top. */
export function scrolledAmount(from: Element | null, root: HTMLElement): number {
	let most = 0;
	for (let el = from; el && el !== root; el = el.parentElement) {
		if (!(el instanceof HTMLElement) || el.scrollTop <= 0) continue;
		const overflowY = getComputedStyle(el).overflowY;
		if ((overflowY === 'auto' || overflowY === 'scroll') && el.scrollHeight > el.clientHeight) {
			most = Math.max(most, el.scrollTop);
		}
	}
	return most;
}

/** Svelte action for the `.sheet` element. */
export function swipeToDismiss(node: HTMLElement, options: SwipeOptions) {
	let opts = options;
	let startX = 0;
	let startY = 0;
	let originY = 0;
	let target: Element | null = null; // where the touch began
	let tracking = false; // a one-finger touch is down and still undecided or claimed
	let claimed = false;
	let busy = false; // an exit or spring-back is animating; ignore new touches
	let samples: Sample[] = [];
	let dy = 0;

	function setDrag(offset: number) {
		dy = offset;
		node.style.transform = `translateY(${offset}px)`;
		const scrim = opts.scrim();
		if (scrim) scrim.style.opacity = String(Math.max(0, 1 - offset / Math.max(node.offsetHeight, 1)));
	}

	function clearInline() {
		node.style.transition = '';
		node.style.transform = '';
		node.style.animation = '';
		const scrim = opts.scrim();
		if (scrim) {
			scrim.style.transition = '';
			scrim.style.opacity = '';
			scrim.style.animation = '';
		}
		dy = 0;
	}

	/** Animate to `offset` (or jump, under reduced motion), then run `done`. */
	function settle(offset: number, scrimOpacity: number, ms: number, done: () => void) {
		const scrim = opts.scrim();
		if (prefersReducedMotion()) {
			// The sheet followed the finger, which is the user's own motion; the
			// arrival is the part that gets skipped.
			node.style.transform = `translateY(${offset}px)`;
			if (scrim) scrim.style.opacity = String(scrimOpacity);
			done();
			return;
		}
		busy = true;
		let finished = false;
		const finish = () => {
			if (finished) return;
			finished = true;
			node.removeEventListener('transitionend', onEnd);
			done();
		};
		const onEnd = (e: TransitionEvent) => {
			if (e.target === node && e.propertyName === 'transform') finish();
		};
		node.addEventListener('transitionend', onEnd);
		node.style.transition = `transform ${ms}ms ease-out`;
		if (scrim) scrim.style.transition = `opacity ${ms}ms ease-out`;
		node.style.transform = `translateY(${offset}px)`;
		if (scrim) scrim.style.opacity = String(scrimOpacity);
		// transitionend doesn't fire when nothing changed (a drag released at 0).
		setTimeout(finish, ms + 80);
	}

	function springBack() {
		settle(0, 1, SPRING_MS, () => {
			clearInline();
			busy = false;
		});
	}

	function dismiss() {
		// Past the sheet's own height plus its shadow, so no sliver is left.
		settle(node.offsetHeight + 48, 0, EXIT_MS, () => {
			// onClose only after the exit, so Back/history sees the same call the
			// × button makes.
			opts.onClose();
			// If the owner of `open` declined to close, the sheet is still here
			// and must not stay stranded off-screen.
			requestAnimationFrame(() => {
				if (node.isConnected) clearInline();
				busy = false;
			});
		});
	}

	function onTouchStart(e: TouchEvent) {
		tracking = false;
		claimed = false;
		if (busy || !opts.enabled() || e.touches.length !== 1) return;
		if (e.target instanceof Element && e.target.closest(TEXT_FIELD)) return;
		const t = e.touches[0];
		target = e.target instanceof Element ? e.target : null;
		startX = t.clientX;
		startY = t.clientY;
		samples = [{ t: performance.now(), y: t.clientY }];
		tracking = true;
	}

	function onTouchMove(e: TouchEvent) {
		if (!tracking) return;
		if (e.touches.length !== 1) {
			// A second finger (a pinch) cancels a drag in progress.
			tracking = false;
			if (claimed) springBack();
			claimed = false;
			return;
		}
		const t = e.touches[0];
		if (!claimed) {
			const mx = t.clientX - startX;
			const my = t.clientY - startY;
			if (Math.abs(mx) < SLOP_PX && Math.abs(my) < SLOP_PX) return;
			const scrollTop = scrolledAmount(target, node);
			if (!claimsDrag(mx, my, scrollTop)) {
				tracking = false; // leave this gesture to the browser
				return;
			}
			claimed = true;
			originY = t.clientY;
			// An entrance animation still running would override the transform.
			node.style.animation = 'none';
			const scrim = opts.scrim();
			if (scrim) scrim.style.animation = 'none';
		}
		if (e.cancelable) e.preventDefault();
		const now = performance.now();
		samples.push({ t: now, y: t.clientY });
		while (samples.length > 2 && now - samples[0].t > VELOCITY_WINDOW_MS) samples.shift();
		setDrag(Math.max(0, t.clientY - originY));
	}

	function onTouchEnd(e: TouchEvent) {
		if (!tracking) return;
		tracking = false;
		if (!claimed) return;
		claimed = false;
		// The drag already moved things; don't let the lift also click what's under it.
		if (e.cancelable) e.preventDefault();
		const first = samples[0];
		const last = samples[samples.length - 1];
		const span = last.t - first.t;
		const velocity = span > 0 ? (last.y - first.y) / span : 0;
		if (shouldDismiss(dy, node.offsetHeight, velocity)) dismiss();
		else springBack();
	}

	function onTouchCancel() {
		if (!tracking) return;
		tracking = false;
		if (claimed) {
			claimed = false;
			springBack();
		}
	}

	node.addEventListener('touchstart', onTouchStart, { passive: true });
	node.addEventListener('touchmove', onTouchMove, { passive: false });
	node.addEventListener('touchend', onTouchEnd, { passive: false });
	node.addEventListener('touchcancel', onTouchCancel, { passive: true });

	return {
		update(next: SwipeOptions) {
			opts = next;
		},
		destroy() {
			node.removeEventListener('touchstart', onTouchStart);
			node.removeEventListener('touchmove', onTouchMove);
			node.removeEventListener('touchend', onTouchEnd);
			node.removeEventListener('touchcancel', onTouchCancel);
		},
	};
}
