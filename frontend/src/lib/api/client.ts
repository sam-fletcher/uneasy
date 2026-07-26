// api/client.ts — the shared fetch helper. All same-origin; the Go server
// proxies everything through one port, so no CORS is needed.
//
// Every message this file produces can end up in front of a player verbatim:
// the app-wide idiom is `catch (e) { xError = e.message }`. So the job here is
// to make sure `message` is always a sentence somebody wrote on purpose.
// Three things used to leak through and don't any more:
//
//  1. Transport failures. `fetch` rejects with the browser's own wording —
//     "Failed to fetch" (Chrome), "Load failed" (Safari) — which told players
//     nothing and was the banner that started adr/ERROR_HANDLING_PLAN.md.
//  2. Non-JSON error bodies. Every *handler* error is proper {"error": …}
//     JSON (handler/respond.go), but the framework edges are not: chi answers
//     an unknown /api path with text/plain "404 page not found" and a wrong
//     method with an empty 405, chi's Recoverer sends a bare 500 on a panic,
//     and a platform 502 mid-redeploy is HTML. Parsing before checking res.ok
//     turned all of those into a raw SyntaxError ("Unexpected token 'p'…").
//  3. The status code. It used to be discarded, so callers that needed it
//     pattern-matched the server's prose instead (MakeWarPanel's /no war/i).
//     ApiError carries it.

/** Thrown by every apiFetch failure. `status` is the HTTP status, or 0 when
 *  the request never reached the server at all. `message` is safe to show. */
export class ApiError extends Error {
	readonly status: number;
	/** The parsed JSON body, when there was one. Null for non-JSON/empty. */
	readonly body: unknown;

	constructor(message: string, status: number, body: unknown = null) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
		this.body = body;
	}
}

/** The request never reached the server: offline, a dead pooled connection
 *  after a redeploy, or the server mid-restart. */
export const OFFLINE_MESSAGE =
	'Could not reach the server — check your connection and try again.';

/** Fallback copy for a failure whose body carried no {"error": …} to show.
 *  Deliberately generic about "the server", not "the table" — apiFetch serves
 *  accounts and profile too. */
function messageForStatus(status: number): string {
	// The realistic way a 404 reaches a player is a tab left open across a
	// redeploy calling a route the new build renamed, so name the remedy.
	if (status === 404) {
		return 'Could not find that — the page may be out of date. Reload to get the current version.';
	}
	if (status >= 500) return 'The server had a problem. Try again in a moment.';
	return `Something went wrong (HTTP ${status}).`;
}

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
	let res: Response;
	let text: string;
	try {
		res = await fetch(`/api${path}`, {
			headers: { 'Content-Type': 'application/json' },
			...init
		});
		// Read the body as text before deciding anything about it. A body that
		// dies mid-read is a transport failure like any other, so it belongs in
		// this try.
		text = await res.text();
	} catch {
		throw new ApiError(OFFLINE_MESSAGE, 0);
	}

	// A 204 (logout, delete-push-subscription) and a bodiless 405 both land
	// here as ''. JSON.parse('') throws, so guard on emptiness rather than
	// leaning on the catch.
	let body: unknown = null;
	if (text !== '') {
		try {
			body = JSON.parse(text);
		} catch {
			// Non-JSON body — a framework edge, not a handler. Leave body null
			// so the status decides the message.
			body = null;
		}
	}

	if (!res.ok) {
		const parsed = body as { error?: unknown; endgame_choice_required?: unknown; modes?: unknown } | null;

		// Plan preparation past row 13 with no endgame mode set returns a
		// structured 409 instead of a plain error. Dispatch a window event
		// so the table page can show a mode picker, then throw normally so
		// the calling component still sees the failure.
		if (parsed?.endgame_choice_required) {
			window.dispatchEvent(
				new CustomEvent('uneasy:endgame_choice_required', {
					detail: { modes: parsed.modes ?? [] }
				})
			);
		}

		const msg =
			typeof parsed?.error === 'string' && parsed.error !== ''
				? parsed.error
				: messageForStatus(res.status);
		throw new ApiError(msg, res.status, body);
	}

	// Success with an empty body (204) returns null; the handful of callers in
	// that shape are all typed Promise<void>.
	return body as T;
}
