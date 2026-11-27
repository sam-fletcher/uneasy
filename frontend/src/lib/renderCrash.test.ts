import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// $app/environment is SvelteKit runtime, stubbed here so the build id is known.
// The vitest env is `node`, so there is no window: `location` is stubbed per test.
vi.mock('$app/environment', () => ({ version: 'build-42' }));

import { RENDER_CRASH_LIMITS, reportRenderCrash } from './renderCrash';

/** The JSON body of the Nth fetch call. */
function sentBody(fetchMock: ReturnType<typeof vi.fn>, n = 0): Record<string, unknown> {
	const init = fetchMock.mock.calls[n][1] as RequestInit;
	return JSON.parse(init.body as string);
}

describe('reportRenderCrash', () => {
	let fetchMock: ReturnType<typeof vi.fn>;

	beforeEach(() => {
		fetchMock = vi.fn(async () => new Response(JSON.stringify({ id: 1 }), { status: 201 }));
		vi.stubGlobal('fetch', fetchMock);
		vi.stubGlobal('location', { pathname: '/table/7' });
	});

	afterEach(() => {
		vi.unstubAllGlobals();
		vi.restoreAllMocks();
	});

	it('makes one POST to /api/render-crashes per crash it is handed', () => {
		const error = new Error('each_key_duplicate');

		reportRenderCrash('table phase view (main_event)', error, 7);

		expect(fetchMock).toHaveBeenCalledTimes(1);
		const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
		expect(url).toBe('/api/render-crashes');
		expect(init.method).toBe('POST');
	});

	it('sends one call per crash: two crashes make two calls, not a retry of either', async () => {
		reportRenderCrash('table phase view (lobby)', new Error('a'));
		reportRenderCrash('table phase view (lobby)', new Error('b'));

		expect(fetchMock).toHaveBeenCalledTimes(2);
		await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(2));
	});

	it('sends the boundary, message, stack, route, build and game of the crash', () => {
		const error = new Error('each_key_duplicate');

		reportRenderCrash('table phase view (main_event)', error, 7);

		expect(sentBody(fetchMock)).toEqual({
			boundary: 'table phase view (main_event)',
			message: 'each_key_duplicate',
			stack: error.stack,
			route: '/table/7',
			build: 'build-42',
			game_id: 7,
		});
	});

	it('leaves the game out when the crash is not on a table page', () => {
		reportRenderCrash('somewhere', new Error('x'));

		expect(sentBody(fetchMock)).not.toHaveProperty('game_id');
	});

	it('takes a thrown value that is not an Error: its text is the message and there is no stack', () => {
		reportRenderCrash('somewhere', 'a bare string');

		const body = sentBody(fetchMock);
		expect(body.message).toBe('a bare string');
		expect(body).not.toHaveProperty('stack');
	});

	it('names the message when the thrown value has none, or only whitespace (the server would refuse it as empty)', () => {
		reportRenderCrash('somewhere', '');
		reportRenderCrash('somewhere', new Error('   \n\t '));

		expect(sentBody(fetchMock, 0).message).toBe('(no message)');
		expect(sentBody(fetchMock, 1).message).toBe('(no message)');
	});

	it('cuts each field to the limit the server enforces, so the server never refuses a report for length', () => {
		const error = new Error('m'.repeat(5000));
		error.stack = 's'.repeat(20000);

		reportRenderCrash('b'.repeat(999), error);

		const body = sentBody(fetchMock);
		expect((body.boundary as string).length).toBe(RENDER_CRASH_LIMITS.boundary);
		expect((body.message as string).length).toBe(RENDER_CRASH_LIMITS.message);
		expect((body.stack as string).length).toBe(RENDER_CRASH_LIMITS.stack);
	});

	it('reports an empty route when there is no window to read it from (the server requires a route field)', () => {
		vi.unstubAllGlobals();
		vi.stubGlobal('fetch', fetchMock);

		reportRenderCrash('somewhere', new Error('x'));

		expect(sentBody(fetchMock).route).toBe('');
	});

	it('swallows a failed report: no throw, no console output, nothing re-reported', async () => {
		fetchMock.mockImplementation(async () => {
			throw new TypeError('Failed to fetch');
		});
		const err = vi.spyOn(console, 'error').mockImplementation(() => {});
		const warn = vi.spyOn(console, 'warn').mockImplementation(() => {});

		expect(() => reportRenderCrash('somewhere', new Error('x'))).not.toThrow();
		await vi.waitFor(() => expect(fetchMock).toHaveBeenCalledTimes(1));
		await new Promise(resolve => setTimeout(resolve, 0));

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(err).not.toHaveBeenCalled();
		expect(warn).not.toHaveBeenCalled();
	});

	it('swallows a refusal from the server (a logged-out tab gets 401)', async () => {
		fetchMock.mockImplementation(async () => new Response(JSON.stringify({ error: 'log in first' }), { status: 401 }));
		const err = vi.spyOn(console, 'error').mockImplementation(() => {});

		expect(() => reportRenderCrash('somewhere', new Error('x'))).not.toThrow();
		await new Promise(resolve => setTimeout(resolve, 0));

		expect(fetchMock).toHaveBeenCalledTimes(1);
		expect(err).not.toHaveBeenCalled();
	});
});
