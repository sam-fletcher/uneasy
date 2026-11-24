import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
	createConnection,
	DISCONNECT_NOTICE_DELAY_MS,
	FOREGROUND_MIN_GAP_MS,
	HEARTBEAT_CHECK_MS,
	HEARTBEAT_GRACE_MS,
	HEARTBEAT_SILENCE_MS,
	type ConnectionStatus,
} from './ws';

// createConnection talks to four browser globals (WebSocket, window, document,
// location). The vitest env here is `node`, so stub all four. The fake socket
// exposes open()/fail() so a test can drive the lifecycle by hand instead of
// waiting on a real network.
class FakeWebSocket {
	static instances: FakeWebSocket[] = [];
	static readonly CONNECTING = 0;
	static readonly OPEN = 1;
	static readonly CLOSING = 2;
	static readonly CLOSED = 3;

	readyState = 0; // CONNECTING
	onopen: (() => void) | null = null;
	onmessage: ((e: { data: string }) => void) | null = null;
	onclose: (() => void) | null = null;
	onerror: (() => void) | null = null;

	constructor(public url: string) {
		FakeWebSocket.instances.push(this);
	}
	open() {
		this.readyState = 1;
		this.onopen?.();
	}
	/** Connection attempt failed, or an open socket dropped. */
	fail() {
		this.readyState = 3;
		this.onclose?.();
	}
	close() {
		this.fail();
	}
	send() {}
}

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

/** Number of sockets the module has created so far. */
const socketCount = () => FakeWebSocket.instances.length;
const latestSocket = () => FakeWebSocket.instances.at(-1)!;

function fireVisibility(state: 'visible' | 'hidden') {
	(doc.target as unknown as { visibilityState: string }).visibilityState = state;
	for (const fn of doc.listeners.get('visibilitychange') ?? []) {
		fn(new Event('visibilitychange'));
	}
}

/** Open the socket and let the resync promise settle. */
async function openAndSync() {
	latestSocket().open();
	await vi.waitFor(() => expect(resyncCalls).toBeGreaterThan(0));
	await Promise.resolve();
}

let resyncCalls = 0;

/** Advance time the way a healthy server does: the newest socket hears a
 *  heartbeat every 30s, so the silence watch has no reason to close it. Tests
 *  that wait a long while on a socket that is meant to stay up use this. */
async function advanceWithHeartbeats(ms: number) {
	for (let left = ms; left > 0; left -= 30_000) {
		await vi.advanceTimersByTimeAsync(Math.min(left, 30_000));
		latestSocket().onmessage?.({ data: '{"type":"heartbeat"}' });
	}
}

beforeEach(() => {
	FakeWebSocket.instances = [];
	resyncCalls = 0;
	vi.useFakeTimers();
	vi.spyOn(console, 'log').mockImplementation(() => {});

	doc = listenerStub({ visibilityState: 'visible' });
	win = listenerStub();

	vi.stubGlobal('WebSocket', FakeWebSocket);
	vi.stubGlobal('document', doc.target);
	vi.stubGlobal('window', win.target);
	vi.stubGlobal('location', { protocol: 'http:', host: 'localhost:5173' });
});

afterEach(() => {
	vi.useRealTimers();
	vi.unstubAllGlobals();
	vi.restoreAllMocks();
});

function connect() {
	return createConnection(7, () => {}, async () => { resyncCalls++; return true; });
}

describe('createConnection reconnect backoff', () => {
	it('doubles the delay while reconnects keep failing', async () => {
		const conn = connect();
		await openAndSync();
		expect(socketCount()).toBe(1);

		latestSocket().fail();
		// First retry is 1s away — not sooner.
		vi.advanceTimersByTime(999);
		expect(socketCount()).toBe(1);
		vi.advanceTimersByTime(1);
		expect(socketCount()).toBe(2);

		// That attempt fails: the next one waits 2s, then 4s.
		latestSocket().fail();
		vi.advanceTimersByTime(1999);
		expect(socketCount()).toBe(2);
		vi.advanceTimersByTime(1);
		expect(socketCount()).toBe(3);

		latestSocket().fail();
		vi.advanceTimersByTime(3999);
		expect(socketCount()).toBe(3);
		vi.advanceTimersByTime(1);
		expect(socketCount()).toBe(4);

		conn.disconnect();
	});

	it('reconnects immediately when the page becomes visible', async () => {
		const conn = connect();
		await openAndSync();

		// Let the backoff climb to a 4s wait.
		latestSocket().fail();
		vi.advanceTimersByTime(1000);
		latestSocket().fail();
		vi.advanceTimersByTime(2000);
		latestSocket().fail();
		expect(socketCount()).toBe(3);

		// The player comes back before that 4s elapses.
		fireVisibility('visible');
		expect(socketCount()).toBe(4);

		// The pre-empted timer must not also fire — that would leak a second
		// socket for one disconnect.
		vi.advanceTimersByTime(10_000);
		expect(socketCount()).toBe(4);

		conn.disconnect();
	});

	it('resets the backoff, so a later drop retries in 1s again', async () => {
		const conn = connect();
		await openAndSync();

		latestSocket().fail();
		vi.advanceTimersByTime(1000);
		latestSocket().fail();
		vi.advanceTimersByTime(2000);
		latestSocket().fail();
		fireVisibility('visible');
		await openAndSync();

		// Backoff is back to its floor rather than the 8s it had grown to.
		latestSocket().fail();
		const before = socketCount();
		vi.advanceTimersByTime(1000);
		expect(socketCount()).toBe(before + 1);

		conn.disconnect();
	});

	it('does nothing when the socket is healthy or the page is hidden', async () => {
		const conn = connect();
		await openAndSync();

		// Open socket: becoming visible must not open a second one.
		fireVisibility('visible');
		expect(socketCount()).toBe(1);

		// Hidden: never reconnect for a player who isn't looking.
		latestSocket().fail();
		fireVisibility('hidden');
		expect(socketCount()).toBe(1);

		conn.disconnect();
	});

	it('stops reconnecting after disconnect()', async () => {
		const conn = connect();
		await openAndSync();

		latestSocket().fail();
		conn.disconnect();

		// Neither the pending timer nor a visibility event may revive it.
		vi.advanceTimersByTime(30_000);
		fireVisibility('visible');
		expect(socketCount()).toBe(1);
		expect(doc.listeners.get('visibilitychange')?.size ?? 0).toBe(0);
	});
});

// ── Telling the player the table has gone quiet ───────────────────────────────
//
// "Live" is an open socket whose resync has settled. The page shows a notice
// from the first 'down' to the next 'live', so these tests pin when each fires:
// not for a blip, after the threshold for a real outage, and only once the
// board is current again.

/** A connection that records every status it reports. `resync` lets a test hold
 *  the snapshot fetch open to look at the moment between "socket open" and
 *  "board current". */
function watched(resync: () => Promise<boolean> = async () => true) {
	const statuses: ConnectionStatus[] = [];
	const conn = createConnection(
		7,
		() => {},
		async () => {
			resyncCalls++;
			return resync();
		},
		{ onStatus: (s) => statuses.push(s) },
	);
	return { conn, statuses };
}

/** Open the newest socket and let its resync settle. Unlike openAndSync this
 *  works for the second and third socket too (it does not wait on the
 *  cumulative resyncCalls). */
async function openAndSettle() {
	latestSocket().open();
	await vi.advanceTimersByTimeAsync(0);
}

describe('createConnection status: the notice threshold', () => {
	it('sits between "two retries have had their chance" and "still news to a player"', () => {
		// After a drop the retries fire at 1s and 3s (1s, then 2s of backoff), and a
		// successful one then needs another second or two for the resync. Below ~5s
		// the notice would flash for blips the second retry heals; far above it the
		// notice is not telling anyone anything they could use. The behavioural
		// tests below pin each edge; this keeps the constant from drifting out of
		// the band by itself.
		expect(DISCONNECT_NOTICE_DELAY_MS).toBeGreaterThanOrEqual(4000);
		expect(DISCONNECT_NOTICE_DELAY_MS).toBeLessThanOrEqual(10_000);
	});

	it('says nothing while the connection is healthy', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();
		await advanceWithHeartbeats(60_000);
		expect(statuses).toEqual([]);
		conn.disconnect();
	});

	it('stays silent for a blip that heals on the first retry', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();

		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(1000); // the 1s retry fires...
		await openAndSettle(); // ...and the table is back

		// Well past the threshold: the drop's timer must have been cancelled, or
		// it would fire here and flash a notice for a connection that is fine.
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS * 2);
		expect(statuses).toEqual([]);
		conn.disconnect();
	});

	it('stays silent when the second retry is the one that works', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();

		latestSocket().fail(); // t=0
		await vi.advanceTimersByTimeAsync(1000); // retry 1 at t=1s
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(2000); // retry 2 at t=3s
		await openAndSettle();

		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS * 2);
		expect(statuses).toEqual([]);
		conn.disconnect();
	});

	it('reports down when the drop has lasted the threshold, not a moment before', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();

		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS - 1);
		expect(statuses).toEqual([]);
		await vi.advanceTimersByTimeAsync(1);
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('measures from the drop, not from the latest failed retry', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();

		latestSocket().fail(); // t=0
		await vi.advanceTimersByTimeAsync(1000); // retry at 1s fails
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(2000); // retry at 3s fails
		latestSocket().fail();
		// Re-arming on each failure would push 'down' out to 3s + 5s. It is due
		// at 5s from the drop, two seconds from here.
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS - 3000);
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('reports down once per outage, however many retries fail', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();

		latestSocket().fail();
		for (let i = 0; i < 8; i++) {
			await vi.advanceTimersByTimeAsync(30_000);
			latestSocket().fail();
		}
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('counts a first connect that never succeeds as an outage too', async () => {
		// Page opened with the server unreachable: the board is whatever was cached.
		const { conn, statuses } = watched();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS);
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('does not flash on a normal first load', async () => {
		const { conn, statuses } = watched();
		// Handshake and the first resync take a second or two, well inside the delay.
		await vi.advanceTimersByTimeAsync(1500);
		await openAndSettle();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS * 2);
		expect(statuses).toEqual([]);
		conn.disconnect();
	});
});

describe('createConnection status: coming back', () => {
	async function downConnection(resync?: () => Promise<boolean>) {
		const w = watched(resync);
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS);
		expect(w.statuses).toEqual(['down']);
		return w;
	}

	it('reports live when a later retry works', async () => {
		const { conn, statuses } = await downConnection();
		await vi.advanceTimersByTimeAsync(30_000); // some retry fires (the table is still failing)
		await openAndSettle();
		expect(statuses).toEqual(['down', 'live']);
		conn.disconnect();
	});

	it('waits for the resync: an open socket alone is not a current board', async () => {
		let release!: () => void;
		const gate = new Promise<boolean>((resolve) => (release = () => resolve(true)));
		// Hold only the resync that follows the outage.
		let held = false;
		const { conn, statuses } = await downConnection(() => (held ? gate : Promise.resolve(true)));
		held = true;

		await vi.advanceTimersByTimeAsync(30_000);
		latestSocket().open();
		await vi.advanceTimersByTimeAsync(0);
		// Socket is open, the snapshot has not landed: the notice stays.
		expect(statuses).toEqual(['down']);

		release();
		await vi.advanceTimersByTimeAsync(0);
		expect(statuses).toEqual(['down', 'live']);
		conn.disconnect();
	});

	it('does not report live when the socket dropped while the resync was running', async () => {
		let release!: () => void;
		const gate = new Promise<boolean>((resolve) => (release = () => resolve(true)));
		let held = false;
		const { conn, statuses } = await downConnection(() => (held ? gate : Promise.resolve(true)));
		held = true;

		await vi.advanceTimersByTimeAsync(30_000);
		latestSocket().open();
		await vi.advanceTimersByTimeAsync(0);
		latestSocket().fail(); // lost again before the snapshot arrived
		release();
		await vi.advanceTimersByTimeAsync(0);

		// A late resync for a dead socket must not clear the notice.
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('reports a second outage after a recovery, with a fresh delay', async () => {
		const { conn, statuses } = await downConnection();
		await vi.advanceTimersByTimeAsync(30_000);
		await openAndSettle();
		expect(statuses).toEqual(['down', 'live']);

		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS - 1);
		expect(statuses).toEqual(['down', 'live']);
		await vi.advanceTimersByTimeAsync(1);
		expect(statuses).toEqual(['down', 'live', 'down']);
		conn.disconnect();
	});

	it('reports live when the page becoming visible reconnects at once', async () => {
		const { conn, statuses } = await downConnection();
		fireVisibility('visible');
		await openAndSettle();
		expect(statuses).toEqual(['down', 'live']);
		conn.disconnect();
	});

	it('reports nothing after disconnect(), even with the timer pending', async () => {
		const { conn, statuses } = watched();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS - 1);
		conn.disconnect(); // the player left the page
		await vi.advanceTimersByTimeAsync(60_000);
		expect(statuses).toEqual([]);
	});
});

describe('createConnection retryNow', () => {
	it('skips the wait: connects at once, and the backoff starts over', async () => {
		const { conn } = watched();
		await openAndSettle();

		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(1000);
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(2000);
		latestSocket().fail(); // now waiting 4s for the next attempt
		const before = socketCount();

		void conn.retryNow();
		expect(socketCount()).toBe(before + 1);

		// The timer it pre-empted must not also fire (a second socket for one
		// drop)...
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(999);
		expect(socketCount()).toBe(before + 1);
		// ...and the backoff is back at 1s, not the 8s it had grown to.
		await vi.advanceTimersByTimeAsync(1);
		expect(socketCount()).toBe(before + 2);
		conn.disconnect();
	});

	it('resolves when the table is live again', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().fail();

		let settled = false;
		void conn.retryNow().then(() => (settled = true));
		await vi.advanceTimersByTimeAsync(0);
		expect(settled).toBe(false); // attempt still in flight

		await openAndSettle();
		expect(settled).toBe(true);
		conn.disconnect();
	});

	it('resolves when the attempt fails, so "Trying…" does not stick on a dead network', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().fail();

		let settled = false;
		void conn.retryNow().then(() => (settled = true));
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(0);
		expect(settled).toBe(true);
		conn.disconnect();
	});

	it('does not double an attempt that is already in flight', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(1000); // retry fires: the new socket is CONNECTING
		const before = socketCount();

		void conn.retryNow();
		expect(socketCount()).toBe(before);
		conn.disconnect();
	});

	it('restarts a retry loop that has died, instead of waiting for an event that will not come', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(1000); // retry fires: socket #2 is CONNECTING
		// The browser fails that socket without ever firing `close` (a blocked
		// connection): nothing schedules another retry.
		latestSocket().readyState = FakeWebSocket.CLOSED;
		await vi.advanceTimersByTimeAsync(60_000);
		const stuckAt = socketCount();
		expect(stuckAt).toBe(2);

		let settled = false;
		void conn.retryNow().then(() => (settled = true));
		expect(socketCount()).toBe(stuckAt + 1); // it started a new attempt itself
		await openAndSettle();
		expect(settled).toBe(true);
		conn.disconnect();
	});

	it('leaves a socket that is closing alone: its close event will schedule the retry', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().readyState = FakeWebSocket.CLOSING;
		const before = socketCount();
		void conn.retryNow();
		expect(socketCount()).toBe(before);
		conn.disconnect();
	});

	it('resolves at once when the table is already live', async () => {
		const { conn } = watched();
		await openAndSettle();
		const before = socketCount();
		await expect(conn.retryNow()).resolves.toBeUndefined();
		expect(socketCount()).toBe(before);
		conn.disconnect();
	});

	it('is released by disconnect(), not left hanging', async () => {
		const { conn } = watched();
		await openAndSettle();
		latestSocket().fail();

		let settled = false;
		void conn.retryNow().then(() => (settled = true));
		conn.disconnect();
		await vi.advanceTimersByTimeAsync(0);
		expect(settled).toBe(true);
	});
});

// ── A resync that does not land ───────────────────────────────────────────────
//
// onResync says whether the snapshot landed. The page's loadGameState swallows
// its own errors (so a background refresh never blanks the board), which means
// that without that answer a reconnect whose fetch failed would look exactly
// like one that worked: the notice would clear over a stale board and, with the
// socket staying open, nothing would ever resync again. That is the case a tab
// waking from sleep or a redeploy hits (the first fetch over a dead pooled
// connection fails).

/** A connection whose resyncs the test steers: `results` is consumed one per
 *  resync (an Error rejects, a boolean resolves), then it keeps returning the
 *  last. `received` collects what reaches the page's message handler. */
function steered(results: Array<boolean | Error | Promise<boolean>>) {
	const statuses: ConnectionStatus[] = [];
	const received: string[] = [];
	let n = 0;
	const conn = createConnection(
		7,
		(msg) => received.push(msg.type),
		async () => {
			resyncCalls++;
			const next = results[Math.min(n++, results.length - 1)];
			if (next instanceof Error) throw next;
			return next;
		},
		{ onStatus: (s) => statuses.push(s) },
	);
	return { conn, statuses, received };
}

const sendEvent = (type: string) =>
	latestSocket().onmessage?.({ data: JSON.stringify({ type, payload: {} }) });

describe('createConnection: a resync that does not land', () => {
	it('does not report live, and closes the socket so the loop resyncs again', async () => {
		const { conn, statuses } = steered([true, false, true]);
		await openAndSettle(); // first load: fine
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS);
		expect(statuses).toEqual(['down']);

		// A retry reaches the server, but its fetch fails.
		await vi.advanceTimersByTimeAsync(30_000);
		const failing = latestSocket();
		await openAndSettle();
		expect(statuses, 'a socket whose resync failed is not a live table').toEqual(['down']);
		expect(failing.readyState, 'it is closed so the loop retries').toBe(FakeWebSocket.CLOSED);

		// The loop connects and resyncs again, and that one lands.
		const before = socketCount();
		await vi.advanceTimersByTimeAsync(30_000);
		expect(socketCount()).toBe(before + 1);
		await openAndSettle();
		expect(statuses).toEqual(['down', 'live']);
		conn.disconnect();
	});

	it('treats a resync that rejects the same way', async () => {
		const { conn, statuses } = steered([true, new Error('Failed to fetch'), true]);
		await openAndSettle();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS);
		await vi.advanceTimersByTimeAsync(30_000);
		const failing = latestSocket();
		await openAndSettle();
		expect(statuses).toEqual(['down']);
		expect(failing.readyState).toBe(FakeWebSocket.CLOSED);
		conn.disconnect();
	});

	it('does not let an open socket reset the backoff before its resync lands', async () => {
		// A server that accepts sockets but fails the fetch. If opening reset the
		// delay to 1s, this would be retried every second for as long as it lasted.
		const { conn } = steered([false]);
		await openAndSettle(); // socket 1 opens, resync fails, it is closed
		let sockets = socketCount();

		for (const wait of [1000, 2000, 4000]) {
			await vi.advanceTimersByTimeAsync(wait - 1);
			expect(socketCount(), `no new socket before ${wait}ms`).toBe(sockets);
			await vi.advanceTimersByTimeAsync(1);
			expect(socketCount(), `a new socket at ${wait}ms`).toBe(sockets + 1);
			sockets = socketCount();
			await openAndSettle(); // it opens, its resync fails, it is closed
		}
		conn.disconnect();
	});

	it('still lets `ready` settle when the first resync fails, so the page can show its error', async () => {
		const { conn } = steered([false]);
		let ready = false;
		void conn.ready.then(() => (ready = true));
		await openAndSettle();
		expect(ready).toBe(true);
		conn.disconnect();
	});

	it('discards the events held during a failed resync instead of applying them to the old board', async () => {
		let release!: (landed: boolean) => void;
		const gate = new Promise<boolean>((resolve) => (release = resolve));
		const { conn, received } = steered([gate]);
		latestSocket().open();
		await vi.advanceTimersByTimeAsync(0);
		sendEvent('law.enacted'); // arrives while the snapshot is in flight: held
		expect(received).toEqual([]);

		release(false); // the snapshot never landed
		await vi.advanceTimersByTimeAsync(0);
		expect(received, 'newer than a snapshot we never got').toEqual([]);
		conn.disconnect();
	});

	it('still replays the held events after a resync that lands', async () => {
		let release!: (landed: boolean) => void;
		const gate = new Promise<boolean>((resolve) => (release = resolve));
		const { conn, received } = steered([gate]);
		latestSocket().open();
		await vi.advanceTimersByTimeAsync(0);
		sendEvent('law.enacted');
		release(true);
		await vi.advanceTimersByTimeAsync(0);
		expect(received).toEqual(['law.enacted']);
		conn.disconnect();
	});
});

describe('createConnection: a socket that has been replaced', () => {
	/** The state "Try now" rescues: the table is down, a retry's socket has been
	 *  failed by the browser (readyState CLOSED), but its `close` event has not
	 *  fired yet, or never will. */
	async function deadRetryAwaitingItsEvent() {
		await openAndSettle();
		latestSocket().fail(); // a real drop: the table is down, a retry is pending
		await vi.advanceTimersByTimeAsync(1000); // the retry connects socket 2
		const old = latestSocket();
		old.readyState = FakeWebSocket.CLOSED;
		return old;
	}

	it('cannot schedule a retry beside the live socket that replaced it', async () => {
		const { conn, statuses } = steered([true]);
		const old = await deadRetryAwaitingItsEvent();

		void conn.retryNow(); // the loop is dead, so this connects socket 3 at once
		await openAndSettle(); // ...and it is live
		const live = socketCount();

		old.onclose?.(); // the old socket's close event finally arrives
		await advanceWithHeartbeats(60_000);
		expect(socketCount(), 'no second retry, no second live socket').toBe(live);
		expect(statuses, 'and the live table is not marked down').toEqual([]);
		conn.disconnect();
	});

	it('is closed, not synced, if it opens after being replaced', async () => {
		const { conn } = steered([true]);
		const old = await deadRetryAwaitingItsEvent();
		void conn.retryNow();
		const before = resyncCalls;

		old.open(); // late `open` from the replaced socket
		await vi.advanceTimersByTimeAsync(0);
		expect(resyncCalls, 'it does not run a resync of its own').toBe(before);
		expect(old.readyState).toBe(FakeWebSocket.CLOSED);
		conn.disconnect();
	});

	it("cannot flip the new socket's buffering when its own resync finishes", async () => {
		let release1!: (landed: boolean) => void;
		let release2!: (landed: boolean) => void;
		const gate1 = new Promise<boolean>((resolve) => (release1 = resolve));
		const gate2 = new Promise<boolean>((resolve) => (release2 = resolve));
		const { conn, received } = steered([gate1, gate2]);

		latestSocket().open(); // socket 1, resync 1 in flight
		await vi.advanceTimersByTimeAsync(0);
		latestSocket().fail(); // it drops mid-resync
		await vi.advanceTimersByTimeAsync(1000);
		latestSocket().open(); // socket 2, resync 2 in flight
		await vi.advanceTimersByTimeAsync(0);

		release1(true); // the OLD resync finishes first
		await vi.advanceTimersByTimeAsync(0);
		sendEvent('law.enacted'); // still ahead of socket 2's snapshot: must be held
		expect(received).toEqual([]);

		release2(true);
		await vi.advanceTimersByTimeAsync(0);
		expect(received).toEqual(['law.enacted']);
		conn.disconnect();
	});
});

// ── Coming back to the foreground ─────────────────────────────────────────────
//
// A tab with a live socket used to do nothing when it became visible, so it
// never looked at the player's "seen" markers again: open Laws on one device and
// the dot stayed up on the phone in your pocket. The page now supplies
// `onForeground`, and these tests pin when it is called: on a live connection,
// for both a tab switch and a window gaining focus, never while a reconnect is
// already going to read everything, and not more than once per burst.

function pressFocus() {
	for (const fn of win.listeners.get('focus') ?? []) fn(new Event('focus'));
}

function foregrounded(onForeground?: () => void | Promise<void>) {
	let calls = 0;
	const conn = createConnection(
		7,
		() => {},
		async () => {
			resyncCalls++;
			return true;
		},
		{
			onForeground: () => {
				calls++;
				return onForeground?.();
			},
		},
	);
	return { conn, calls: () => calls };
}

describe('createConnection: the page returns to the foreground', () => {
	it('calls onForeground when the tab becomes visible on a live connection', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();
		expect(calls()).toBe(0);

		fireVisibility('visible');
		expect(calls()).toBe(1);
		expect(socketCount(), 'it needs no new socket').toBe(1);
		conn.disconnect();
	});

	it('calls it when the window gains focus: a second window never changes visibility', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();

		pressFocus();
		expect(calls()).toBe(1);
		conn.disconnect();
	});

	it('is not called when the tab is hidden', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();

		fireVisibility('hidden');
		expect(calls()).toBe(0);
		conn.disconnect();
	});

	it('is not called before the first resync has landed: that fetch reads the markers itself', async () => {
		let release!: (landed: boolean) => void;
		const gate = new Promise<boolean>((resolve) => (release = resolve));
		let calls = 0;
		const conn = createConnection(7, () => {}, () => gate, { onForeground: () => { calls++; } });
		latestSocket().open(); // open, but the snapshot is still in flight
		await vi.advanceTimersByTimeAsync(0);

		fireVisibility('visible');
		pressFocus();
		expect(calls).toBe(0);

		release(true);
		await vi.advanceTimersByTimeAsync(0);
		await vi.advanceTimersByTimeAsync(FOREGROUND_MIN_GAP_MS);
		fireVisibility('visible');
		expect(calls).toBe(1);
		conn.disconnect();
	});

	it('is not called while a reconnect is pending: the reconnect resync reads everything', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();
		latestSocket().fail(); // dropped; a retry is 1s away

		fireVisibility('visible');
		expect(socketCount(), 'visible reconnects at once, as before').toBe(2);
		expect(calls(), 'a second fetch of the same markers would only race the resync').toBe(0);

		await openAndSettle();
		expect(calls()).toBe(0);
		conn.disconnect();
	});

	it('is not called on a socket that is down and not yet retrying', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();
		latestSocket().fail();
		await vi.advanceTimersByTimeAsync(1000); // retry fires, socket 2 is CONNECTING

		fireVisibility('visible');
		pressFocus();
		expect(calls()).toBe(0);
		expect(socketCount(), 'it must not leak a second socket').toBe(2);
		conn.disconnect();
	});

	it('is called once for the visibility-and-focus pair one return produces', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();

		fireVisibility('visible');
		pressFocus();
		expect(calls()).toBe(1);
		conn.disconnect();
	});

	it('is called again after the burst window, not only the first time', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();

		fireVisibility('visible');
		await vi.advanceTimersByTimeAsync(FOREGROUND_MIN_GAP_MS - 1);
		fireVisibility('visible');
		expect(calls(), 'still inside the window').toBe(1);

		await vi.advanceTimersByTimeAsync(1);
		fireVisibility('visible');
		expect(calls()).toBe(2);
		conn.disconnect();
	});

	it('is not called after disconnect(), and drops its window listener', async () => {
		const { conn, calls } = foregrounded();
		await openAndSettle();

		conn.disconnect();
		fireVisibility('visible');
		pressFocus();
		expect(calls()).toBe(0);
		expect(win.listeners.get('focus')?.size ?? 0).toBe(0);
	});

	it('survives a callback that throws or rejects: the connection carries on', async () => {
		const unhandled: unknown[] = [];
		const onUnhandled = (reason: unknown) => unhandled.push(reason);
		process.on('unhandledRejection', onUnhandled);
		try {
			const { conn, calls } = foregrounded(() => {
				throw new Error('sync');
			});
			await openAndSettle();
			expect(() => fireVisibility('visible')).not.toThrow();
			expect(calls()).toBe(1);
			conn.disconnect();

			const second = foregrounded(() => Promise.reject(new Error('async')));
			await openAndSettle();
			expect(() => fireVisibility('visible')).not.toThrow();
			await vi.advanceTimersByTimeAsync(0);
			// Node reports an unhandled rejection a tick after the microtasks drain.
			await new Promise<void>((resolve) => process.nextTick(resolve));
			await new Promise<void>((resolve) => process.nextTick(resolve));
			expect(second.calls()).toBe(1);
			expect(unhandled, 'a rejection from the callback must not escape').toEqual([]);
			second.conn.disconnect();
		} finally {
			process.off('unhandledRejection', onUnhandled);
		}
	});

	it('works without a callback: the option is optional', async () => {
		const conn = createConnection(7, () => {}, async () => true);
		await openAndSettle();
		expect(() => fireVisibility('visible')).not.toThrow();
		expect(() => pressFocus()).not.toThrow();
		conn.disconnect();
	});
});

// ── The silence watch ────────────────────────────────────────────────────────
// A socket the browser still calls open but that has gone dead never fires
// `close`. The server sends a heartbeat every 30s; the page closes a socket it
// has heard nothing on for HEARTBEAT_SILENCE_MS, and the ordinary close
// handling takes over. The fake socket's `close()` fires `close` at once.

function hear(sock: FakeWebSocket, type = 'row_state.changed') {
	sock.onmessage?.({ data: JSON.stringify({ type, payload: {} }) });
}

describe('createConnection: a silent socket', () => {
	it('is closed after the silence limit and then reconnects', async () => {
		const { conn, statuses } = watched();
		await openAndSettle();
		const first = latestSocket();

		// Just short of the limit: nothing happens.
		await vi.advanceTimersByTimeAsync(HEARTBEAT_SILENCE_MS - 1_000);
		expect(first.readyState).toBe(1);

		// The check runs every HEARTBEAT_CHECK_MS, so one more interval is enough.
		await vi.advanceTimersByTimeAsync(1_000 + HEARTBEAT_CHECK_MS);
		expect(first.readyState).toBe(3);

		// The ordinary drop follows: a retry, and the notice after its delay.
		await vi.advanceTimersByTimeAsync(1_000);
		expect(socketCount()).toBe(2);
		await vi.advanceTimersByTimeAsync(DISCONNECT_NOTICE_DELAY_MS);
		expect(statuses).toEqual(['down']);
		conn.disconnect();
	});

	it('is kept open by a message, which restarts the count', async () => {
		const { conn } = watched();
		await openAndSettle();
		const sock = latestSocket();

		await vi.advanceTimersByTimeAsync(40_000);
		hear(sock);
		await vi.advanceTimersByTimeAsync(40_000);
		expect(sock.readyState).toBe(1);

		// Silence counted from that message, not from the open.
		await vi.advanceTimersByTimeAsync(10_000);
		expect(sock.readyState).toBe(3);
		conn.disconnect();
	});

	it('counts the heartbeat as a message, without passing it on', async () => {
		const seen: string[] = [];
		const conn = createConnection(7, (m) => seen.push(m.type), async () => true);
		await openAndSettle();
		const sock = latestSocket();

		// A heartbeat every 30s, as the server sends: the socket lives on.
		for (let i = 0; i < 6; i++) {
			await vi.advanceTimersByTimeAsync(30_000);
			hear(sock, 'heartbeat');
		}
		expect(sock.readyState).toBe(1);
		expect(seen).toEqual([]);

		// Other events still reach the page.
		hear(sock, 'row_state.changed');
		expect(seen).toEqual(['row_state.changed']);
		conn.disconnect();
	});

	it('does not buffer a heartbeat that arrives during a resync', async () => {
		const seen: string[] = [];
		let finishResync!: (landed: boolean) => void;
		const conn = createConnection(
			7,
			(m) => seen.push(m.type),
			() => new Promise<boolean>((resolve) => { finishResync = resolve; }),
		);
		latestSocket().open();
		hear(latestSocket(), 'heartbeat');
		hear(latestSocket(), 'row_state.changed');
		finishResync(true);
		await vi.advanceTimersByTimeAsync(0);
		expect(seen).toEqual(['row_state.changed']);
		conn.disconnect();
	});

	it('is not checked while the tab is hidden', async () => {
		const { conn } = watched();
		await openAndSettle();
		const sock = latestSocket();

		fireVisibility('hidden');
		await vi.advanceTimersByTimeAsync(10 * 60_000);
		expect(sock.readyState).toBe(1);
		expect(socketCount()).toBe(1);
		conn.disconnect();
	});

	it('gets a grace period on return, then is closed and reconnected if still silent', async () => {
		const { conn } = watched();
		await openAndSettle();
		const sock = latestSocket();

		fireVisibility('hidden');
		// +2s so the return falls between two of the 5s checks, not on one.
		await vi.advanceTimersByTimeAsync(10 * 60_000 + 2_000);
		fireVisibility('visible');

		// Not closed at once: a frozen phone's queued frames may be about to land.
		expect(sock.readyState).toBe(1);
		await vi.advanceTimersByTimeAsync(HEARTBEAT_GRACE_MS - 1_000);
		expect(sock.readyState).toBe(1);

		// Nothing came: closed right as the grace ends (not up to a tick later),
		// and the returning player gets the reconnect without the backoff wait.
		await vi.advanceTimersByTimeAsync(1_000);
		expect(sock.readyState).toBe(3);
		await vi.advanceTimersByTimeAsync(1_000);
		expect(socketCount()).toBe(2);
		conn.disconnect();
	});

	it('is kept open if a frame arrives during the grace period', async () => {
		const { conn } = watched();
		await openAndSettle();
		const sock = latestSocket();

		fireVisibility('hidden');
		await vi.advanceTimersByTimeAsync(10 * 60_000);
		fireVisibility('visible');

		// The frames queued during the freeze are delivered just after the event.
		await vi.advanceTimersByTimeAsync(100);
		hear(sock, 'heartbeat');
		await vi.advanceTimersByTimeAsync(HEARTBEAT_GRACE_MS * 3);
		expect(sock.readyState).toBe(1);
		expect(socketCount()).toBe(1);
		conn.disconnect();
	});

	it('counts the open itself as the first thing heard', async () => {
		const { conn } = watched();
		// The socket sits CONNECTING for a long while, then opens: the silence is
		// measured from the open, not from when the attempt began.
		await vi.advanceTimersByTimeAsync(60_000);
		await openAndSettle();
		await vi.advanceTimersByTimeAsync(HEARTBEAT_SILENCE_MS - 1_000);
		expect(latestSocket().readyState).toBe(1);
		conn.disconnect();
	});

	it('is left alone on return if something was heard while hidden', async () => {
		const { conn } = watched();
		await openAndSettle();
		const sock = latestSocket();

		fireVisibility('hidden');
		await vi.advanceTimersByTimeAsync(10 * 60_000);
		hear(sock, 'heartbeat');
		fireVisibility('visible');

		expect(sock.readyState).toBe(1);
		expect(socketCount()).toBe(1);
		conn.disconnect();
	});

	it('is not watched after disconnect()', async () => {
		const { conn } = watched();
		await openAndSettle();
		conn.disconnect();
		await vi.advanceTimersByTimeAsync(10 * 60_000);
		expect(socketCount()).toBe(1);
	});

	it('survives a close event that arrives late, once the watch has dropped it', async () => {
		const { conn } = watched();
		await openAndSettle();
		const first = latestSocket();
		// A half-open socket that closes slowly: make close() do nothing, so only
		// the watch's own handling moves things along.
		first.close = () => {};

		await vi.advanceTimersByTimeAsync(HEARTBEAT_SILENCE_MS + HEARTBEAT_CHECK_MS);
		await vi.advanceTimersByTimeAsync(1_000);
		expect(socketCount()).toBe(2);

		// The browser finally reports the old socket closed: it must not schedule a
		// second retry beside the new socket.
		first.fail();
		await vi.advanceTimersByTimeAsync(60_000);
		expect(socketCount()).toBe(2);
		conn.disconnect();
	});
});
