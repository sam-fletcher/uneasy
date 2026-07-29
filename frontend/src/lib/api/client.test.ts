import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { apiFetch, ApiError, OFFLINE_MESSAGE } from './client';

// apiFetch talks to two browser globals (fetch, window). The vitest env here
// is `node`, so stub both — same approach as ws.test.ts.

/** Build a Response-alike. `body` is sent verbatim, so a test can hand back
 *  the plain text and empty bodies the framework edges actually produce. */
function response(status: number, body: string): Response {
	return {
		ok: status >= 200 && status < 300,
		status,
		text: async () => body,
	} as Response;
}

/** Await a call expected to fail and hand back the ApiError, typed. Fails the
 *  test if it resolves instead — `.catch(e => e)` would type as `unknown`. */
async function rejection(p: Promise<unknown>): Promise<ApiError> {
	try {
		await p;
	} catch (e) {
		return e as ApiError;
	}
	throw new Error('expected the request to reject, but it resolved');
}

let dispatched: CustomEvent[];

beforeEach(() => {
	dispatched = [];
	vi.stubGlobal('window', {
		dispatchEvent: (e: CustomEvent) => {
			dispatched.push(e);
			return true;
		},
	});
	// Node has CustomEvent from v19; keep the stub explicit so the test does
	// not depend on which runtime vitest picked.
	if (typeof globalThis.CustomEvent === 'undefined') {
		vi.stubGlobal(
			'CustomEvent',
			class {
				type: string;
				detail: unknown;
				constructor(type: string, init?: { detail?: unknown }) {
					this.type = type;
					this.detail = init?.detail;
				}
			},
		);
	}
});

afterEach(() => vi.unstubAllGlobals());

function mockFetch(impl: () => Promise<Response>) {
	vi.stubGlobal('fetch', vi.fn(impl));
}

describe('apiFetch — success', () => {
	it('parses and returns a JSON body', async () => {
		mockFetch(async () => response(200, '{"id":7,"name":"Osric"}'));
		await expect(apiFetch('/thing')).resolves.toEqual({ id: 7, name: 'Osric' });
	});

	it('returns null for a 204 instead of throwing', async () => {
		// The regression that forced logout() and deletePushSubscription() to
		// bypass apiFetch entirely — and, because they then never checked
		// res.ok, to swallow their failures.
		mockFetch(async () => response(204, ''));
		await expect(apiFetch('/sessions', { method: 'DELETE' })).resolves.toBeNull();
	});

	it('prefixes /api and keeps the JSON content type', async () => {
		mockFetch(async () => response(200, '{}'));
		await apiFetch('/tables/3', { method: 'POST', body: '{"a":1}' });
		expect(fetch).toHaveBeenCalledWith('/api/tables/3', {
			headers: { 'Content-Type': 'application/json' },
			method: 'POST',
			body: '{"a":1}',
		});
	});
});

describe('apiFetch — error bodies', () => {
	it('surfaces the server message from a JSON error body', async () => {
		mockFetch(async () => response(409, '{"error":"you already own this asset"}'));
		await expect(apiFetch('/assets/1/take')).rejects.toThrow('you already own this asset');
	});

	it('carries the status and parsed body on the thrown ApiError', async () => {
		mockFetch(async () => response(403, '{"error":"not your turn"}'));
		const err = await rejection(apiFetch('/turn'));
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(403);
		expect(err.body).toEqual({ error: 'not your turn' });
	});

	it('replaces a plain-text 404 with copy naming the remedy', async () => {
		// chi answers an unknown /api path with text/plain "404 page not
		// found". Parsing that as JSON used to throw a SyntaxError whose text
		// reached the player verbatim.
		mockFetch(async () => response(404, '404 page not found\n'));
		const err = await rejection(apiFetch('/renamed-route'));
		expect(err.status).toBe(404);
		expect(err.message).toContain('Reload');
		expect(err.message).not.toMatch(/JSON|token/i);
	});

	it('handles a bodiless 405', async () => {
		// A wrong method on a real route returns 405 with no body at all;
		// JSON.parse('') throws "Unexpected end of JSON input".
		mockFetch(async () => response(405, ''));
		const err = await rejection(apiFetch('/accounts/me', { method: 'DELETE' }));
		expect(err.status).toBe(405);
		expect(err.message).toBe('Something went wrong (HTTP 405).');
	});

	it('gives 5xx its own copy, not the parser complaint', async () => {
		// chi's Recoverer sends a bare 500 on a panic; a platform 502 mid-
		// redeploy sends HTML.
		mockFetch(async () => response(502, '<html>Bad Gateway</html>'));
		const err = await rejection(apiFetch('/tables/1'));
		expect(err.status).toBe(502);
		expect(err.message).toBe('The server had a problem. Try again in a moment.');
	});

	it('falls back to status copy when the JSON body has an empty error', async () => {
		mockFetch(async () => response(400, '{"error":""}'));
		const err = await rejection(apiFetch('/thing'));
		expect(err.message).toBe('Something went wrong (HTTP 400).');
	});
});

describe('apiFetch — transport failure', () => {
	it('replaces the browser wording with one human sentence', async () => {
		// "Failed to fetch" (Chrome) / "Load failed" (Safari) used to reach the
		// player verbatim — the banner that started ERROR_HANDLING_PLAN.md.
		mockFetch(async () => {
			throw new TypeError('Failed to fetch');
		});
		const err = await rejection(apiFetch('/tables/1'));
		expect(err).toBeInstanceOf(ApiError);
		expect(err.message).toBe(OFFLINE_MESSAGE);
		expect(err.status).toBe(0);
	});

	it('treats a body that dies mid-read as a transport failure too', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(async () => ({
				ok: true,
				status: 200,
				text: async () => {
					throw new TypeError('network error');
				},
			})),
		);
		await expect(apiFetch('/tables/1')).rejects.toThrow(OFFLINE_MESSAGE);
	});
});

// The server still answers a plan preparation past row 13 with no endgame mode
// set with a structured 409 carrying `endgame_choice_required` / `modes`. It
// used to be dispatched as a window event driving a facilitator-only interrupt
// modal; that modal is retired (adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §7), because
// the row 7 → 8 table vote settles the mode before row 8 begins. The body shape
// stays covered, now as a plain error and nothing more.
describe('apiFetch — endgame_choice_required is an ordinary error', () => {
	it('throws the server message and dispatches nothing', async () => {
		mockFetch(async () =>
			response(
				409,
				JSON.stringify({
					error: 'plan would land past row 13, and the table has not settled how the game ends',
					endgame_choice_required: true,
					modes: ['smooth_landing', 'explosive_finale'],
				}),
			),
		);

		const err = await rejection(apiFetch('/tables/1/plans', { method: 'POST' }));

		expect(dispatched).toHaveLength(0);
		expect(err).toBeInstanceOf(ApiError);
		expect(err.status).toBe(409);
		expect(err.message).toBe(
			'plan would land past row 13, and the table has not settled how the game ends',
		);
		// The structured fields still reach a caller that wants them.
		expect(err.body).toMatchObject({ endgame_choice_required: true });
	});

	it('does not dispatch for an ordinary 409 either', async () => {
		mockFetch(async () => response(409, '{"error":"asset is already leveraged"}'));
		await apiFetch('/assets/1/leverage').catch(() => {});
		expect(dispatched).toHaveLength(0);
	});
});
