import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

// $app/state is SvelteKit runtime — stubbed wholesale so these stay pure-TS
// unit tests (the vitest env here is `node`). `current` is mutable so a test
// can put the module into the "already known stale" state.
const { mockUpdated } = vi.hoisted(() => ({
	mockUpdated: { current: false, check: vi.fn<() => Promise<boolean>>() },
}));
vi.mock('$app/state', () => ({ updated: mockUpdated }));

import { checkForNewVersion, watchForNewVersion, __resetVersionCheckThrottle } from './appVersion';

const CHECK_MIN_INTERVAL_MS = 60_000;

type Listeners = Map<string, Set<EventListener>>;
function listenerStub(extra: Record<string, unknown> = {}) {
	const listeners: Listeners = new Map();
	return {
		listeners,
		target: {
			...extra,
			addEventListener(type: string, fn: EventListener) {
				if (!listeners.has(type)) listeners.set(type, new Set());
				listeners.get(type)!.add(fn);
			},
			removeEventListener(type: string, fn: EventListener) {
				listeners.get(type)?.delete(fn);
			},
		},
	};
}

let doc: ReturnType<typeof listenerStub>;
let win: ReturnType<typeof listenerStub>;

function fire(on: ReturnType<typeof listenerStub>, type: string) {
	for (const fn of on.listeners.get(type) ?? []) fn(new Event(type));
}
function setVisibility(state: 'visible' | 'hidden') {
	(doc.target as unknown as { visibilityState: string }).visibilityState = state;
}

beforeEach(() => {
	vi.useFakeTimers();
	__resetVersionCheckThrottle();
	mockUpdated.current = false;
	mockUpdated.check.mockReset().mockResolvedValue(false);

	doc = listenerStub({ visibilityState: 'visible' });
	win = listenerStub();
	vi.stubGlobal('document', doc.target);
	vi.stubGlobal('window', win.target);
});

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

describe('watchForNewVersion', () => {
	it('checks when the player returns to the tab', async () => {
		watchForNewVersion();
		// The watcher seeds the throttle so a just-loaded page doesn't spend a
		// request; a real return happens long after that floor.
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);

		fire(doc, 'visibilitychange');
		await vi.advanceTimersByTimeAsync(0);
		expect(mockUpdated.check).toHaveBeenCalledTimes(1);
	});

	it('checks on window focus too (app switches that leave the tab visible)', async () => {
		watchForNewVersion();
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);

		fire(win, 'focus');
		await vi.advanceTimersByTimeAsync(0);
		expect(mockUpdated.check).toHaveBeenCalledTimes(1);
	});

	it('ignores a visibilitychange that hid the tab', async () => {
		watchForNewVersion();
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);

		setVisibility('hidden');
		fire(doc, 'visibilitychange');
		await vi.advanceTimersByTimeAsync(0);
		expect(mockUpdated.check).not.toHaveBeenCalled();
	});

	it('does not check on a page that just loaded', async () => {
		watchForNewVersion();
		fire(doc, 'visibilitychange');
		fire(win, 'focus');
		await vi.advanceTimersByTimeAsync(0);
		expect(mockUpdated.check).not.toHaveBeenCalled();
	});

	it('removes both listeners on teardown', () => {
		const stop = watchForNewVersion();
		expect(doc.listeners.get('visibilitychange')?.size).toBe(1);
		expect(win.listeners.get('focus')?.size).toBe(1);

		stop();
		expect(doc.listeners.get('visibilitychange')?.size ?? 0).toBe(0);
		expect(win.listeners.get('focus')?.size ?? 0).toBe(0);
	});
});

describe('checkForNewVersion', () => {
	it('collapses the visibilitychange/focus overlap into one request', async () => {
		__resetVersionCheckThrottle();
		await checkForNewVersion();
		await checkForNewVersion();
		expect(mockUpdated.check).toHaveBeenCalledTimes(1);
	});

	it('checks again once the floor has elapsed', async () => {
		__resetVersionCheckThrottle();
		await checkForNewVersion();
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);
		await checkForNewVersion();
		expect(mockUpdated.check).toHaveBeenCalledTimes(2);
	});

	it('stops asking once the server is known to have moved on', async () => {
		__resetVersionCheckThrottle();
		mockUpdated.current = true;
		await checkForNewVersion();
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);
		await checkForNewVersion();
		expect(mockUpdated.check).not.toHaveBeenCalled();
	});

	it('swallows a failed check — the server may be mid-restart', async () => {
		__resetVersionCheckThrottle();
		mockUpdated.check.mockRejectedValue(new TypeError('Failed to fetch'));
		await expect(checkForNewVersion()).resolves.toBeUndefined();

		// And the failure must not wedge the in-flight guard shut.
		mockUpdated.check.mockResolvedValue(false);
		await vi.advanceTimersByTimeAsync(CHECK_MIN_INTERVAL_MS + 1);
		await checkForNewVersion();
		expect(mockUpdated.check).toHaveBeenCalledTimes(2);
	});
});
