// Back-button dismissal for full-screen overlays.
//
// Every surface that covers the screen on mobile — the chat sheet, the Public
// Record, all eight RetinueSheets, the prologue's claim modal — holds a
// history entry while it's open, so the Android/iOS Back gesture closes it.
// Without this, Back on /table/[id] unmounts the whole game shell: it drops
// the WebSocket, the scroll position and any half-typed message, which is the
// opposite of what "dismiss the thing covering my screen" should do. It
// matters more than usual here because the manifest is `display: standalone`
// — an installed player has no browser chrome, so the system Back gesture is
// their only navigation affordance.
//
// This module is the coordinator. Components talk to it through
// `dismissOnBack` in dismissOnBack.svelte.ts; it is split out, and takes its
// history access as an adapter, so the reconciliation below can be unit
// tested without a browser or a router.
//
// The whole model is one number: how many overlay entries are on the stack
// (App.PageState.overlayDepth). Owners register when they open and deregister
// when they close, and a microtask later we reconcile stack against owners:
//
//     more owners than entries  → push the difference
//     fewer owners than entries → step back over the difference
//     equal                     → leave the stack alone
//
// Batching into a microtask is what makes hand-offs free. When one surface
// closes and another opens in the same click — HelpButton's Help → Feedback
// swap, or opening a header panel while the chat sheet is up, which the table
// page does deliberately to keep one full-screen surface at a time — the
// owner count never changes, so no history churn happens at all and Back
// still returns to the table instead of reopening what was just dismissed.
// Doing it eagerly instead would push before the pending back() had landed
// and leave the two counts permanently out of step.

/** Where the coordinator's side effects go. The browser implementation
 *  (shallow routing + window.history) is wired up in dismissOnBack.svelte.ts;
 *  tests pass a fake. */
export interface HistoryAdapter {
	/** Push one overlay entry, recording `depth` in the page state. */
	push(depth: number): void;
	/** Step back over `count` entries. */
	back(count: number): void;
	/** Current document href, used to notice a real navigation. */
	href(): string;
}

export type SyncPlan =
	| { kind: 'push'; count: number }
	| { kind: 'back'; count: number }
	| null;

/** Reconcile the number of open overlays against the number of history
 *  entries we're holding for them. `null` means the two already agree. */
export function planSync(owners: number, entries: number): SyncPlan {
	if (owners > entries) return { kind: 'push', count: owners - entries };
	if (owners < entries) return { kind: 'back', count: entries - owners };
	return null;
}

/** An open overlay's claim on a history entry. Opaque to callers. */
export interface OverlayHandle {
	readonly close: () => void;
}

export interface OverlayHistory {
	/** Claim an entry for a newly-opened overlay. `close` is invoked if the
	 *  entry later disappears because the player pressed Back. */
	acquire(close: () => void): OverlayHandle;
	/** Give an entry back because the overlay closed some other way (its own
	 *  close button, the backdrop, Escape, or the component unmounting). */
	release(handle: OverlayHandle): void;
	/** Report the router's current overlay depth and href. Idempotent. */
	observe(depth: number, href: string): void;
	/** Test seam. */
	readonly ownerCount: number;
}

export function createOverlayHistory(
	adapter: HistoryAdapter,
	schedule: (fn: () => void) => void = queueMicrotask,
): OverlayHistory {
	/** Open overlays, oldest first. Back closes the last one. */
	const owners: OverlayHandle[] = [];
	/** Our belief about page.state.overlayDepth, refreshed by observe(). */
	let entries = 0;
	/** The href the entries belong to, captured when we push. */
	let anchorHref: string | null = null;
	let scheduled = false;
	/** A back() we asked for hasn't landed yet; don't plan against a stale
	 *  `entries` until the router tells us where we ended up. */
	let backPending = false;
	/** A push that threw takes the whole feature offline rather than leaving
	 *  the counts skewed. Overlays keep working; they just lose Back. */
	let broken = false;

	function requestSync() {
		if (scheduled) return;
		scheduled = true;
		schedule(() => {
			scheduled = false;
			sync();
		});
	}

	function sync() {
		if (broken || backPending) return;
		const plan = planSync(owners.length, entries);
		if (plan === null) return;

		if (plan.kind === 'push') {
			try {
				for (let i = 0; i < plan.count; i++) adapter.push(entries + 1 + i);
			} catch {
				// Shallow routing threw — most likely the router wasn't ready.
				// Stop touching history entirely; observe() goes quiet too, so
				// nothing can mistake a depth-0 stack for a dismissal.
				broken = true;
				entries = 0;
				anchorHref = null;
				return;
			}
			entries += plan.count;
			anchorHref = adapter.href();
			return;
		}

		// Stepping back is only safe while we're still on the page that owns
		// the entries. If a real navigation happened since (the table redirects
		// to /profile, or to the login page when auth lapses) our entries are
		// buried behind it, and going back would undo the navigation instead of
		// closing an overlay. Drop the bookkeeping and leave history alone.
		if (anchorHref !== null && adapter.href() !== anchorHref) {
			entries = 0;
			anchorHref = null;
			return;
		}

		backPending = true;
		adapter.back(plan.count);
	}

	return {
		acquire(close) {
			const handle: OverlayHandle = { close };
			owners.push(handle);
			requestSync();
			return handle;
		},

		release(handle) {
			const i = owners.indexOf(handle);
			// Already gone: observe() dropped it when the entry vanished, and
			// this is the owning component reacting to its own close callback.
			// The stack is settled, so there's nothing to reconcile.
			if (i === -1) return;
			owners.splice(i, 1);
			requestSync();
		},

		observe(depth, href) {
			if (broken) return;

			// How many entries *we were holding* have gone. This has to be
			// measured against `entries`, not against owners.length: an owner
			// that has acquired but whose push is still queued holds no entry
			// yet, and comparing it to the stack would read as a dismissal. A
			// modal mounted already-open (the prologue claim ledger) acquires
			// and observes in the same flush, so it hits that gap every time.
			const removed = Math.max(0, entries - depth);

			// A drop we asked for is a close that already happened, not a
			// dismissal to act on. Without this an overlay opened in the gap
			// between release() and the pop landing — tapping a header panel
			// straight after closing the chat sheet — would be closed again by
			// the previous overlay's own back(). Re-syncing below re-pushes for
			// it instead.
			const wasOurs = backPending;
			// Adopting a depth *above* ours (the player pressing Forward back
			// into an entry whose overlay is long closed) leaves the stack one
			// entry over, which the sync below spends. Cancelling a Forward is
			// the lesser evil against leaving an entry whose Back does nothing
			// visible, and Forward doesn't exist at all in an installed PWA.
			entries = depth;
			backPending = false;
			if (depth === 0 && href !== anchorHref) anchorHref = null;

			// Entries the stack no longer holds were dismissed by Back. Close
			// their owners, newest first. Each close() calls release(), which
			// no-ops because we popped the handle first.
			if (!wasOurs) {
				for (let i = 0; i < removed && owners.length > 0; i++) {
					owners.pop()?.close();
				}
			}
			requestSync();
		},

		get ownerCount() {
			return owners.length;
		},
	};
}
