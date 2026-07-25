import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { createConnection } from './ws';

// createConnection talks to four browser globals (WebSocket, window, document,
// location). The vitest env here is `node`, so stub all four. The fake socket
// exposes open()/fail() so a test can drive the lifecycle by hand instead of
// waiting on a real network.
class FakeWebSocket {
	static instances: FakeWebSocket[] = [];
	static readonly OPEN = 1;

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

beforeEach(() => {
	FakeWebSocket.instances = [];
	resyncCalls = 0;
	vi.useFakeTimers();
	vi.spyOn(console, 'log').mockImplementation(() => {});

	doc = listenerStub({ visibilityState: 'visible' });
	const win = listenerStub();

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
	return createConnection(7, () => {}, async () => { resyncCalls++; });
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
