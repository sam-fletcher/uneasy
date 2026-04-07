// api.ts — typed wrappers around the Go API.
// All requests go to the same origin as the page (the Go server proxies
// everything through one port, so no CORS is needed).

export interface UserToken {
	display_name: string;
	created_at: string;
}

export interface Game {
	id: number;
	join_code: string;
	created_at: string;
	facilitator_id: number | null;
}

export interface Player {
	id: number;
	game_id: number;
	display_name: string;
	joined_at: string;
	is_facilitator: boolean;
}

export interface Post {
	id: number;
	game_id: number;
	author_id: number;
	body: string;
	created_at: string;
}

export interface PresenceMember {
	id: number;
	display_name: string;
	online: boolean;
}

// ── API helpers ───────────────────────────────────────────────────────────────

async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
	const res = await fetch(`/api${path}`, {
		headers: { 'Content-Type': 'application/json' },
		...init
	});
	const body = await res.json();
	if (!res.ok) {
		throw new Error(body.error ?? `HTTP ${res.status}`);
	}
	return body as T;
}

// ── Identity ──────────────────────────────────────────────────────────────────

export function setIdentity(displayName: string): Promise<UserToken> {
	return apiFetch<UserToken>('/identity', {
		method: 'POST',
		body: JSON.stringify({ display_name: displayName })
	});
}

export function getIdentity(): Promise<{ display_name: string; player: Player | null }> {
	return apiFetch('/identity');
}

// ── Tables ────────────────────────────────────────────────────────────────────

export function createTable(): Promise<{ game: Game; player: Player }> {
	return apiFetch('/tables', { method: 'POST' });
}

export function joinTable(joinCode: string): Promise<{ game: Game; player: Player }> {
	return apiFetch('/tables/join', {
		method: 'POST',
		body: JSON.stringify({ join_code: joinCode })
	});
}

export function getTable(id: string | number): Promise<{ game: Game; players: Player[] }> {
	return apiFetch(`/tables/${id}`);
}

// ── Posts ─────────────────────────────────────────────────────────────────────

export function listPosts(
	gameID: string | number,
	afterID?: number
): Promise<{ posts: Post[] }> {
	const query = afterID != null ? `?after=${afterID}` : '';
	return apiFetch(`/tables/${gameID}/posts${query}`);
}

export function createPost(gameID: string | number, body: string): Promise<{ post: Post }> {
	return apiFetch(`/tables/${gameID}/posts`, {
		method: 'POST',
		body: JSON.stringify({ body })
	});
}
