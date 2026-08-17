import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ToneTopic, ToneTopicStatus } from '$lib/api';
import {
	TONE_CYCLE,
	nextToneStatus,
	acceptToneEcho,
	cycleToneStatus,
	type ToneWrite,
	type ToneWriteContext,
} from './toneWrites';

vi.mock('$lib/api', () => ({
	updateToneTopic: vi.fn(),
	listToneTopics: vi.fn(),
}));

import { updateToneTopic, listToneTopics } from '$lib/api';

const mockUpdate = vi.mocked(updateToneTopic);
const mockList = vi.mocked(listToneTopics);

// A context over a plain array, standing in for the page's $state runes.
function makeCtx(topics: ToneTopic[] = [topic(1, 'default')]) {
	const state = {
		topics,
		error: '',
		inFlight: new Map<number, ToneWrite>(),
	};
	const ctx: ToneWriteContext = {
		gameID: 7,
		setStatus: (topicID, status) => {
			state.topics = state.topics.map(t => (t.id === topicID ? { ...t, status } : t));
		},
		replaceTopics: (next) => { state.topics = next; },
		setError: (message) => { state.error = message; },
		inFlight: state.inFlight,
	};
	return {
		ctx,
		state,
		statusOf: (id: number) => state.topics.find(t => t.id === id)?.status,
	};
}

function topic(id: number, status: ToneTopicStatus): ToneTopic {
	return { id, game_id: 7, topic: `topic-${id}`, status };
}

/** A deferred promise, for holding a PUT open while further taps arrive. */
function deferred<T>() {
	let resolve!: (v: T) => void;
	let reject!: (e: unknown) => void;
	const promise = new Promise<T>((res, rej) => { resolve = res; reject = rej; });
	return { promise, resolve, reject };
}

beforeEach(() => {
	vi.resetAllMocks();
	mockUpdate.mockResolvedValue({ topic_id: 1, status: 'include' });
	mockList.mockResolvedValue({ topics: [] });
});

describe('nextToneStatus', () => {
	it('walks the cycle and wraps', () => {
		expect(TONE_CYCLE).toEqual(['default', 'include', 'avoid_detail', 'never']);
		expect(nextToneStatus('default')).toBe('include');
		expect(nextToneStatus('include')).toBe('avoid_detail');
		expect(nextToneStatus('avoid_detail')).toBe('never');
		expect(nextToneStatus('never')).toBe('default');
	});
});

describe('cycleToneStatus — optimistic paint', () => {
	it('paints before the request resolves', async () => {
		const { ctx, statusOf } = makeCtx();
		const gate = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(gate.promise);

		const run = cycleToneStatus(ctx, 1, 'default');
		expect(statusOf(1)).toBe('include'); // already painted, request still open

		gate.resolve({ topic_id: 1, status: 'include' });
		await run;
		expect(statusOf(1)).toBe('include');
	});

	it('clears a stale error on each tap', async () => {
		const { ctx, state } = makeCtx();
		state.error = 'previous failure';
		await cycleToneStatus(ctx, 1, 'default');
		expect(state.error).toBe('');
	});
});

describe('cycleToneStatus — coalescing', () => {
	it('sends only the status the player stopped on', async () => {
		const { ctx, statusOf } = makeCtx();
		const first = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(first.promise);

		// Tap 1 goes on the wire; taps 2 and 3 land while it is still open.
		const run = cycleToneStatus(ctx, 1, 'default');
		await cycleToneStatus(ctx, 1, 'include');
		await cycleToneStatus(ctx, 1, 'avoid_detail');
		expect(statusOf(1)).toBe('never');
		expect(mockUpdate).toHaveBeenCalledTimes(1);

		first.resolve({ topic_id: 1, status: 'include' });
		await run;

		// Two requests total for three taps: the one already on the wire, then
		// the value cycled to since. 'avoid_detail' is never sent.
		expect(mockUpdate).toHaveBeenCalledTimes(2);
		expect(mockUpdate.mock.calls.map(c => c[2])).toEqual(['include', 'never']);
		expect(statusOf(1)).toBe('never');
	});

	it('keeps different topics fully parallel', async () => {
		const { ctx } = makeCtx([topic(1, 'default'), topic(2, 'default')]);
		const held = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(held.promise);

		const run = cycleToneStatus(ctx, 1, 'default');
		await cycleToneStatus(ctx, 2, 'default'); // must not queue behind topic 1

		expect(mockUpdate).toHaveBeenCalledTimes(2);
		held.resolve({ topic_id: 1, status: 'include' });
		await run;
	});

	it('drains the in-flight entry once settled', async () => {
		const { ctx } = makeCtx();
		await cycleToneStatus(ctx, 1, 'default');
		expect(ctx.inFlight.size).toBe(0);
	});
});

describe('acceptToneEcho', () => {
	it('accepts echoes for topics with nothing in flight', () => {
		const inFlight = new Map<number, ToneWrite>();
		expect(acceptToneEcho(inFlight, 1, 'never')).toBe(true);
	});

	it('drops this client\'s own echo, including a coalesced earlier step', () => {
		const inFlight = new Map<number, ToneWrite>([
			[1, { sent: 'never', seen: new Set<ToneTopicStatus>(['include', 'never']) }],
		]);
		expect(acceptToneEcho(inFlight, 1, 'include')).toBe(false); // stale step
		expect(acceptToneEcho(inFlight, 1, 'never')).toBe(false);   // current step
		expect(inFlight.get(1)?.sawForeignEcho).toBeUndefined();
	});

	it('flags a foreign edit landing mid-flight', () => {
		const inFlight = new Map<number, ToneWrite>([
			[1, { sent: 'include', seen: new Set<ToneTopicStatus>(['include']) }],
		]);
		expect(acceptToneEcho(inFlight, 1, 'avoid_detail')).toBe(false);
		expect(inFlight.get(1)?.sawForeignEcho).toBe(true);
	});
});

describe('cycleToneStatus — echo suppression end to end', () => {
	it('does not walk the tile backwards when its own echo lands', async () => {
		const { ctx, statusOf } = makeCtx();
		const first = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(first.promise);

		const run = cycleToneStatus(ctx, 1, 'default');   // sends 'include'
		await cycleToneStatus(ctx, 1, 'include');          // queues 'avoid_detail'
		expect(statusOf(1)).toBe('avoid_detail');

		// The broadcast for tap 1 arrives while tap 2 is still queued. This is
		// the regression: applying it would repaint the tile 'include'.
		expect(acceptToneEcho(ctx.inFlight, 1, 'include')).toBe(false);
		expect(statusOf(1)).toBe('avoid_detail');

		first.resolve({ topic_id: 1, status: 'include' });
		await run;
		expect(statusOf(1)).toBe('avoid_detail');
	});

	it('refetches once when a foreign edit was dropped mid-flight', async () => {
		const { ctx, statusOf } = makeCtx();
		const first = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(first.promise);
		mockList.mockResolvedValue({ topics: [topic(1, 'never')] });

		const run = cycleToneStatus(ctx, 1, 'default');
		acceptToneEcho(ctx.inFlight, 1, 'avoid_detail'); // another player's edit

		first.resolve({ topic_id: 1, status: 'include' });
		await run;

		expect(mockList).toHaveBeenCalledTimes(1);
		expect(statusOf(1)).toBe('never');
	});

	it('does not refetch when every dropped echo was its own', async () => {
		const { ctx } = makeCtx();
		const first = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(first.promise);

		const run = cycleToneStatus(ctx, 1, 'default');
		acceptToneEcho(ctx.inFlight, 1, 'include');

		first.resolve({ topic_id: 1, status: 'include' });
		await run;

		expect(mockList).not.toHaveBeenCalled();
	});
});

describe('cycleToneStatus — failure', () => {
	it('reports the error and resyncs from the server', async () => {
		const { ctx, state, statusOf } = makeCtx();
		mockUpdate.mockRejectedValueOnce(new Error('tones are locked'));
		mockList.mockResolvedValue({ topics: [topic(1, 'default')] });

		await cycleToneStatus(ctx, 1, 'default');

		expect(state.error).toBe('tones are locked');
		expect(statusOf(1)).toBe('default'); // optimistic paint corrected
		expect(ctx.inFlight.size).toBe(0);
	});

	it('abandons queued taps behind a failure', async () => {
		const { ctx } = makeCtx();
		const first = deferred<{ topic_id: number; status: ToneTopicStatus }>();
		mockUpdate.mockReturnValueOnce(first.promise);
		mockList.mockResolvedValue({ topics: [topic(1, 'default')] });

		const run = cycleToneStatus(ctx, 1, 'default');
		await cycleToneStatus(ctx, 1, 'include'); // queues 'avoid_detail'

		first.reject(new Error('offline'));
		await run;

		// Only the first PUT was attempted; the resync is the truth now.
		expect(mockUpdate).toHaveBeenCalledTimes(1);
		expect(mockList).toHaveBeenCalledTimes(1);
	});

	it('keeps the optimistic value when the resync also fails', async () => {
		const { ctx, statusOf } = makeCtx();
		mockUpdate.mockRejectedValueOnce(new Error('offline'));
		mockList.mockRejectedValue(new Error('offline'));

		await cycleToneStatus(ctx, 1, 'default');

		expect(statusOf(1)).toBe('include');
		expect(ctx.inFlight.size).toBe(0);
	});
});
