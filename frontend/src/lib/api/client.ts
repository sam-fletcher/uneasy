// api/client.ts — the shared fetch helper. All same-origin; the Go server
// proxies everything through one port, so no CORS is needed.

export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`/api${path}`, {
		headers: { 'Content-Type': 'application/json' },
		...init
	});
	const body = await res.json();
	if (!res.ok) {
		// Plan preparation past row 13 with no endgame mode set returns a
		// structured 409 instead of a plain error. Dispatch a window event
		// so the table page can show a mode picker, then throw normally so
		// the calling component still sees the failure.
		if (body && body.endgame_choice_required) {
			window.dispatchEvent(
				new CustomEvent('uneasy:endgame_choice_required', {
					detail: { modes: body.modes ?? [] }
				})
			);
		}
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}
	return body as T;
}
