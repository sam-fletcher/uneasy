// api.ts — typed wrappers around the Go API.
// All requests go to the same origin as the page (the Go server proxies
// everything through one port, so no CORS is needed).

// ── Types ────────────────────────────────────────────────────────────────────

export type GamePhase = 'lobby' | 'tone_setting' | 'prologue' | 'main_event' | 'shake_up' | 'ended';
export type ToneTopicStatus = 'default' | 'include' | 'avoid_detail' | 'never';
export type RankingCategory = 'power' | 'knowledge' | 'esteem';
export type AssetType = 'peer' | 'holding' | 'artifact' | 'resource';

export interface UserToken {
	display_name: string;
	created_at: string;
}

export interface Game {
	id: number;
	join_code: string;
	created_at: string;
	facilitator_id: number | null;
	phase: GamePhase;
	current_row: number;
	focus_player_id: number | null;
	ending_mode: string | null;
	dummy_token_mode: string;
}

export interface Player {
	id: number;
	game_id: number;
	display_name: string;
	joined_at: string;
	is_facilitator: boolean;
	token_color: string | null;
	seat_order: number | null;
}

export interface ToneTopic {
	id: number;
	game_id: number;
	topic: string;
	status: ToneTopicStatus;
}

export interface Ranking {
	id: number;
	game_id: number;
	player_id: number | null;
	category: RankingCategory;
	rank: number;
}

export interface Marginalium {
	id: number;
	asset_id: number;
	position: number;
	text: string;
	is_torn: boolean;
	torn_at: string | null;
	torn_by_id: number | null;
}

export interface Asset {
	id: number;
	game_id: number;
	owner_id: number;
	creator_id: number;
	asset_type: AssetType;
	name: string;
	is_main_character: boolean;
	is_leveraged: boolean;
	is_destroyed: boolean;
	created_at: string;
	destroyed_at: string | null;
	// Enriched by the API — always present in list/create/update responses.
	marginalia: Marginalium[];
}

export interface Secret {
	id: number;
	asset_id: number;
	author_id: number;
	text: string;
	is_revealed: boolean;
	revealed_at: string | null;
	created_at: string;
}

export interface ScenePost {
	id: number;
	game_id: number;
	row_number: number | null;
	plan_id: number | null;
	author_id: number;
	body: string;
	created_at: string;
}

export interface PresenceMember {
	id: number;
	display_name: string;
	online: boolean;
}

// ── API helpers ──────────────────────────────────────────────────────────────

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

// ── Identity ─────────────────────────────────────────────────────────────────

export function setIdentity(displayName: string): Promise<UserToken> {
	return apiFetch<UserToken>('/identity', {
		method: 'POST',
		body: JSON.stringify({ display_name: displayName })
	});
}

export function getIdentity(): Promise<{ display_name: string; player: Player | null }> {
	return apiFetch('/identity');
}

// ── Tables ───────────────────────────────────────────────────────────────────

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

// Full game state including phase-specific data.
export function getGameState(id: string | number): Promise<{
	game: Game;
	players: Player[];
	tone_topics?: ToneTopic[];
	rankings?: Ranking[];
}> {
	return apiFetch(`/tables/${id}/state`);
}

// ── Phase Transitions ────────────────────────────────────────────────────────

export function startToneSetting(gameID: string | number): Promise<{ phase: GamePhase }> {
	return apiFetch(`/tables/${gameID}/start-tone-setting`, { method: 'POST' });
}

export function startPrologue(gameID: string | number): Promise<{ phase: GamePhase }> {
	return apiFetch(`/tables/${gameID}/start-prologue`, { method: 'POST' });
}

export function startMainEvent(gameID: string | number): Promise<{
	phase: GamePhase;
	current_row: number;
	focus_player_id: number | null;
}> {
	return apiFetch(`/tables/${gameID}/start-main-event`, { method: 'POST' });
}

// ── Tone Setting ─────────────────────────────────────────────────────────────

export function listToneTopics(gameID: string | number): Promise<{ topics: ToneTopic[] }> {
	return apiFetch(`/tables/${gameID}/tone`);
}

export function updateToneTopic(
	gameID: string | number,
	topicID: number,
	status: ToneTopicStatus
): Promise<{ topic_id: number; status: ToneTopicStatus }> {
	return apiFetch(`/tables/${gameID}/tone/${topicID}`, {
		method: 'PUT',
		body: JSON.stringify({ status })
	});
}

export function addToneTopic(
	gameID: string | number,
	topic: string
): Promise<{ topic: ToneTopic }> {
	return apiFetch(`/tables/${gameID}/tone`, {
		method: 'POST',
		body: JSON.stringify({ topic })
	});
}

// ── Rankings ─────────────────────────────────────────────────────────────────

export function getRankings(gameID: string | number): Promise<{ rankings: Ranking[] }> {
	return apiFetch(`/tables/${gameID}/rankings`);
}

export function setRankings(
	gameID: string | number,
	rankings: Array<{ player_id: number | null; category: RankingCategory; rank: number }>
): Promise<{ rankings: Ranking[] }> {
	return apiFetch(`/tables/${gameID}/rankings`, {
		method: 'PUT',
		body: JSON.stringify({ rankings })
	});
}

export function setSeats(
	gameID: string | number,
	seats: Array<{ player_id: number; seat_order: number }>
): Promise<void> {
	return apiFetch(`/tables/${gameID}/seats`, {
		method: 'PUT',
		body: JSON.stringify({ seats })
	});
}

// ── Scene Posts ──────────────────────────────────────────────────────────────

export function listScenePosts(
	gameID: string | number,
	rowNumber: number,
	opts?: { planID?: number; afterID?: number }
): Promise<{ posts: ScenePost[] }> {
	const params = new URLSearchParams();
	if (opts?.planID != null) params.set('plan_id', String(opts.planID));
	if (opts?.afterID != null) params.set('after', String(opts.afterID));
	const query = params.toString() ? `?${params}` : '';
	return apiFetch(`/tables/${gameID}/rows/${rowNumber}/posts${query}`);
}

export function createScenePost(
	gameID: string | number,
	rowNumber: number,
	body: string,
	planID?: number
): Promise<{ post: ScenePost }> {
	return apiFetch(`/tables/${gameID}/rows/${rowNumber}/posts`, {
		method: 'POST',
		body: JSON.stringify({ body, plan_id: planID ?? null })
	});
}

// ── Assets ───────────────────────────────────────────────────────────────────

export function listAssets(gameID: string | number): Promise<{ assets: Asset[] }> {
	return apiFetch(`/tables/${gameID}/assets`);
}

export function createAsset(
	gameID: string | number,
	params: {
		asset_type: AssetType;
		name: string;
		is_main_character?: boolean;
		marginalia?: string[];
	}
): Promise<{ asset: Asset }> {
	return apiFetch(`/tables/${gameID}/assets`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

export function updateAsset(
	assetID: number,
	params: { name?: string; is_main_character?: boolean }
): Promise<{ asset: Asset }> {
	return apiFetch(`/assets/${assetID}`, {
		method: 'PUT',
		body: JSON.stringify(params)
	});
}

// ── Marginalia ────────────────────────────────────────────────────────────────

export function addMarginalia(
	assetID: number,
	text: string
): Promise<{ marginalia: Marginalium }> {
	return apiFetch(`/assets/${assetID}/marginalia`, {
		method: 'POST',
		body: JSON.stringify({ text })
	});
}

export function updateMarginalia(
	assetID: number,
	position: number,
	text: string
): Promise<{ marginalia: Marginalium }> {
	return apiFetch(`/assets/${assetID}/marginalia/${position}`, {
		method: 'PUT',
		body: JSON.stringify({ text })
	});
}

export function tearMarginalia(
	assetID: number,
	position: number
): Promise<{ torn: boolean; destroyed: boolean }> {
	return apiFetch(`/assets/${assetID}/marginalia/${position}`, {
		method: 'DELETE'
	});
}

// ── Leverage / Refresh / Take ─────────────────────────────────────────────────

export function leverageAsset(assetID: number): Promise<{ leveraged: boolean }> {
	return apiFetch(`/assets/${assetID}/leverage`, { method: 'POST' });
}

export function refreshAsset(assetID: number): Promise<{ leveraged: boolean }> {
	return apiFetch(`/assets/${assetID}/refresh`, { method: 'POST' });
}

export function takeAsset(assetID: number): Promise<{ asset: Asset }> {
	return apiFetch(`/assets/${assetID}/take`, { method: 'POST' });
}

// ── Secrets ───────────────────────────────────────────────────────────────────

export function writeSecret(assetID: number, text: string): Promise<{ secret: Secret }> {
	return apiFetch(`/assets/${assetID}/secrets`, {
		method: 'POST',
		body: JSON.stringify({ text })
	});
}

export function getSecrets(assetID: number): Promise<{ secrets: Secret[] }> {
	return apiFetch(`/assets/${assetID}/secrets`);
}
