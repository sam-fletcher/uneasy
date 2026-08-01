import { describe, it, expect, vi } from 'vitest';
import { planSync, createOverlayHistory, type HistoryAdapter } from './overlayHistory';

const TABLE = 'https://uneasy.test/table/7';
const PROFILE = 'https://uneasy.test/profile';

/** Drives a coordinator against a fake history stack. `schedule` is manual so
 *  tests can put several calls in one "tick" the way a click handler does, then
 *  flush once — that batching is the whole point of the microtask. */
function harness() {
	const calls: string[] = [];
	const pending: (() => void)[] = [];
	let depth = 0;
	let href = TABLE;
	let pushThrows = false;

	const adapter: HistoryAdapter = {
		push: (d) => {
			if (pushThrows) throw new Error('router not initialized');
			calls.push(`push:${d}`);
			depth = d;
		},
		back: (n) => {
			calls.push(`back:${n}`);
			depth = Math.max(0, depth - n);
		},
		href: () => href,
	};

	const oh = createOverlayHistory(adapter, (fn) => pending.push(fn));

	return {
		oh,
		calls,
		/** Run every queued microtask (and anything they queue in turn). */
		flush() {
			let guard = 0;
			while (pending.length) {
				if (++guard > 50) throw new Error('sync did not settle');
				pending.shift()!();
			}
		},
		/** The router reporting where the stack actually ended up. */
		settle() {
			oh.observe(depth, href);
		},
		/** The player pressing Back: one entry off, then the router reports. */
		pressBack() {
			depth = Math.max(0, depth - 1);
			oh.observe(depth, href);
		},
		navigateTo(to: string) {
			href = to;
			depth = 0;
		},
		breakPush() {
			pushThrows = true;
		},
		get depth() {
			return depth;
		},
	};
}

describe('planSync', () => {
	it('pushes the shortfall when more overlays are open than entries held', () => {
		expect(planSync(1, 0)).toEqual({ kind: 'push', count: 1 });
		expect(planSync(2, 0)).toEqual({ kind: 'push', count: 2 });
	});

	it('steps back over the surplus when overlays have closed', () => {
		expect(planSync(0, 1)).toEqual({ kind: 'back', count: 1 });
		expect(planSync(1, 3)).toEqual({ kind: 'back', count: 2 });
	});

	it('leaves the stack alone when the two already agree', () => {
		expect(planSync(0, 0)).toBeNull();
		expect(planSync(2, 2)).toBeNull();
	});
});

describe('createOverlayHistory', () => {
	it('pushes one entry when an overlay opens', () => {
		const h = harness();
		h.oh.acquire(vi.fn());
		h.flush();
		expect(h.calls).toEqual(['push:1']);
	});

	it('steps back when the overlay closes by its own button', () => {
		const h = harness();
		const close = vi.fn();
		const handle = h.oh.acquire(close);
		h.flush();
		h.settle();
		h.flush();

		h.oh.release(handle);
		h.flush();

		expect(h.calls).toEqual(['push:1', 'back:1']);
		// The pop landing must not re-fire the overlay's own close callback.
		h.settle();
		h.flush();
		expect(close).not.toHaveBeenCalled();
		expect(h.oh.ownerCount).toBe(0);
	});

	it('closes the overlay when the player presses Back', () => {
		const h = harness();
		const close = vi.fn();
		h.oh.acquire(close);
		h.flush();
		h.settle();
		h.flush();

		h.pressBack();
		h.flush();

		expect(close).toHaveBeenCalledTimes(1);
		expect(h.oh.ownerCount).toBe(0);
		// Nothing further owed — the entry is already gone.
		expect(h.calls).toEqual(['push:1']);
	});

	it('leaves history untouched when one surface hands off to another', () => {
		// HelpButton's Help → Feedback swap, and the table page closing the chat
		// sheet as a header panel opens: both happen in a single click handler.
		const h = harness();
		const closeA = vi.fn();
		const closeB = vi.fn();
		const a = h.oh.acquire(closeA);
		h.flush();
		h.settle();
		h.flush();
		h.calls.length = 0;

		h.oh.release(a);
		h.oh.acquire(closeB);
		h.flush();

		expect(h.calls).toEqual([]);
		expect(h.depth).toBe(1);

		// One Back closes the surface that's actually up, and returns to the
		// table rather than reopening the one that handed off.
		h.pressBack();
		h.flush();
		expect(closeB).toHaveBeenCalledTimes(1);
		expect(closeA).not.toHaveBeenCalled();
		expect(h.depth).toBe(0);
	});

	it('re-pushes for an overlay opened while our own back is still in flight', () => {
		const h = harness();
		const closeA = vi.fn();
		const closeB = vi.fn();
		const a = h.oh.acquire(closeA);
		h.flush();
		h.settle();
		h.flush();
		h.calls.length = 0;

		// Close, then open again a tick later — before the pop has landed.
		h.oh.release(a);
		h.flush();
		expect(h.calls).toEqual(['back:1']);

		h.oh.acquire(closeB);
		h.flush();
		// Still nothing: planning against a stale depth would double-push.
		expect(h.calls).toEqual(['back:1']);

		h.settle();
		h.flush();
		expect(h.calls).toEqual(['back:1', 'push:1']);
		// B must survive the pop that belonged to A.
		expect(closeB).not.toHaveBeenCalled();
		expect(h.oh.ownerCount).toBe(1);
	});

	it('closes only the top overlay when two are stacked', () => {
		const h = harness();
		const closeA = vi.fn();
		const closeB = vi.fn();
		h.oh.acquire(closeA);
		h.oh.acquire(closeB);
		h.flush();
		expect(h.calls).toEqual(['push:1', 'push:2']);
		h.settle();
		h.flush();

		h.pressBack();
		h.flush();
		expect(closeB).toHaveBeenCalledTimes(1);
		expect(closeA).not.toHaveBeenCalled();
		expect(h.oh.ownerCount).toBe(1);
	});

	it('survives an overlay that acquires before its first observe', () => {
		// A modal mounted already-open (the prologue claim ledger) runs its
		// acquire and its first observe in the same flush, before the queued
		// push has run. That instant — one owner, zero entries — must not read
		// as a dismissal, or the modal closes the moment it opens.
		const h = harness();
		const close = vi.fn();
		h.oh.acquire(close);
		h.settle(); // observe(0) while the push is still queued
		expect(close).not.toHaveBeenCalled();

		h.flush();
		h.settle();
		h.flush();
		expect(h.calls).toEqual(['push:1']);
		expect(close).not.toHaveBeenCalled();
		expect(h.oh.ownerCount).toBe(1);

		// And it still dismisses normally afterwards.
		h.pressBack();
		h.flush();
		expect(close).toHaveBeenCalledTimes(1);
	});

	it('does not step back across a real navigation', () => {
		// The table redirects to /profile (or the login page) with a sheet open.
		// Our entry is buried behind the navigation now; going back would undo
		// the redirect instead of closing anything.
		const h = harness();
		const handle = h.oh.acquire(vi.fn());
		h.flush();
		h.settle();
		h.flush();
		h.calls.length = 0;

		h.navigateTo(PROFILE);
		h.oh.release(handle);
		h.flush();

		expect(h.calls).toEqual([]);
	});

	it('goes quiet when shallow routing is unavailable', () => {
		// Overlays must keep working — they just lose Back integration. The
		// danger is the opposite: a depth that never leaves 0 reading as a
		// dismissal and closing every overlay the moment it opens.
		const h = harness();
		h.breakPush();
		const close = vi.fn();
		h.oh.acquire(close);
		h.flush();
		h.settle();
		h.flush();

		expect(h.calls).toEqual([]);
		expect(close).not.toHaveBeenCalled();
		expect(h.oh.ownerCount).toBe(1);
	});
});
