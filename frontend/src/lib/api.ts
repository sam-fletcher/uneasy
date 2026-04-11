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

export interface SceneEntry {
	id: number;
	game_id: number;
	row_number: number;
	author_id: number;
	body: string;
	created_at: string;
}

export interface DiceRoll {
	id: number;
	game_id: number;
	plan_id: number | null;
	row_number: number | null;
	is_shake_up: boolean;
	actor_id: number;
	difficulty: number;
	adjusted_difficulty: number | null;
	result: number | null;
	outcome: 'make' | 'mar' | null;
	created_at: string;
	resolved_at: string | null;
}

export interface DiceRollDie {
	id: number;
	roll_id: number;
	player_id: number;
	is_interference: boolean;
	leveraged_asset_id: number | null;
	face: number | null;
	is_cancelled: boolean;
}

export interface DifficultyVote {
	roll_id: number;
	player_id: number;
	vote: 'yea' | 'nay';
	voted_at: string;
}

export type PlanType = 'exchange_courtiers' | 'make_introductions' | 'spread_propaganda'
	| 'make_demands' | 'propose_decree' | 'make_war' | 'seek_answers'
	| 'chronicle_histories' | 'clandestinely_liaise' | 'spread_rumors'
	| 'propose_duel' | 'host_festivity';

export interface Plan {
	id: number;
	game_id: number;
	plan_type: PlanType;
	category: RankingCategory;
	preparer_id: number;
	target_player_id: number | null;
	target_asset_id: number | null;
	row_number: number;
	row_order: number;
	prepared_at_row: number;
	status: 'pending' | 'resolving' | 'resolved' | 'cancelled';
	result: 'make' | 'mar' | null;
	resolved_at: string | null;
	preparation_notes: string | null;
	resolution_data: string | null;
}

/** Parsed resolution_data JSON embedded in a Plan. */
export interface PlanResolutionData {
	peer_count?: number;
	fair_trade_asset_id?: number | null;
	fair_trade_accepted?: boolean | null;
	choices?: string[];
}

/** Response shape from GET /api/plans/:id. */
export interface PlanDetail {
	plan: Plan;
	difficulty: number;
	resolution_data: PlanResolutionData;
}

/** One eligibility entry from GET /api/tables/:id/plan-eligibility. */
export interface EligiblePlan {
	plan_type: PlanType;
	category: RankingCategory;
	delay: number;
	target_row: number;
}

export interface IneligiblePlan {
	plan_type: PlanType;
	category: RankingCategory;
	reason: string;
}

/** One row of the public record as returned by GET /api/tables/:id/record. */
export interface RecordRow {
	row_number: number;
	entries: SceneEntry[];
	plans: Plan[];
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

// ── Public Record ─────────────────────────────────────────────────────────────

export function getFullRecord(gameID: string | number): Promise<{ rows: RecordRow[] }> {
	return apiFetch(`/tables/${gameID}/record`);
}

export function createSceneEntry(
	gameID: string | number,
	rowNumber: number,
	body: string
): Promise<{ entry: SceneEntry }> {
	return apiFetch(`/tables/${gameID}/rows/${rowNumber}/summary`, {
		method: 'POST',
		body: JSON.stringify({ body })
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

// ── Turn structure (Phase 2d) ─────────────────────────────────────────────────

/** Focus player signals the scene is over. Broadcasts scene.ended to all clients. */
export function endScene(gameID: string | number): Promise<{ row_number: number }> {
	return apiFetch(`/tables/${gameID}/end-scene`, { method: 'POST' });
}

/**
 * Focus player refreshes up to current_row leveraged assets.
 * Pass an empty array to take the "refresh nothing" action.
 */
export function refreshAssets(
	gameID: string | number,
	assetIDs: number[]
): Promise<{ refreshed: number[] }> {
	return apiFetch(`/tables/${gameID}/refresh-assets`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs })
	});
}

/**
 * Advance current_row by 1. Handles engrailed line detection and the
 * transition to ended when row 13 completes. Sets next focus player.
 */
export function advanceRow(gameID: string | number): Promise<{
	row_number?: number;
	crossed_engrailed?: boolean;
	phase?: GamePhase;
}> {
	return apiFetch(`/tables/${gameID}/advance-row`, { method: 'POST' });
}

/** Pass the focus marker to the next player by seat order (within-row). */
export function passFocus(gameID: string | number): Promise<{
	focus_player_id: number;
	focus_player_name: string;
}> {
	return apiFetch(`/tables/${gameID}/pass-focus`, { method: 'POST' });
}

// ── Dice Rolls (Phase 2e) ─────────────────────────────────────────────────────

/**
 * Get the active (unresolved) dice roll for a game, if any.
 * Returns null in the roll field if there is no active roll.
 */
export function getActiveRollForGame(gameID: string | number): Promise<{
	roll: DiceRoll | null;
	dice: DiceRollDie[];
	votes: DifficultyVote[];
}> {
	return apiFetch(`/tables/${gameID}/rolls/active`);
}

/** Create a new dice roll for the current row. The caller becomes the actor. */
export function createRoll(
	gameID: string | number,
	difficulty: number
): Promise<{ roll: DiceRoll }> {
	return apiFetch(`/tables/${gameID}/rolls`, {
		method: 'POST',
		body: JSON.stringify({ difficulty })
	});
}

/** Get full roll state — roll, all dice, and current votes. */
export function getRoll(rollID: number): Promise<{
	roll: DiceRoll;
	dice: DiceRollDie[];
	votes: DifficultyVote[];
}> {
	return apiFetch(`/rolls/${rollID}`);
}

/** Leverage one of your assets to add a die to an open roll. */
export function leverageRoll(
	rollID: number,
	assetID: number
): Promise<{ die: DiceRollDie }> {
	return apiFetch(`/rolls/${rollID}/leverage`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID })
	});
}

/** Actor opens a difficulty vote; broadcasts to all players. */
export function callVote(rollID: number): Promise<{ roll_id: number }> {
	return apiFetch(`/rolls/${rollID}/call-vote`, { method: 'POST' });
}

/** Submit a difficulty vote (yea increases difficulty, nay decreases it). */
export function voteOnRoll(rollID: number, vote: 'yea' | 'nay'): Promise<{
	vote: string;
	adjusted_difficulty?: number;
}> {
	return apiFetch(`/rolls/${rollID}/vote`, {
		method: 'POST',
		body: JSON.stringify({ vote })
	});
}

/** Actor or facilitator closes the leverage window and rolls all dice. */
export function closeLeverage(rollID: number): Promise<{
	roll: DiceRoll;
	dice: DiceRollDie[];
	cancelled_dice: DiceRollDie[];
}> {
	return apiFetch(`/rolls/${rollID}/close-leverage`, { method: 'POST' });
}

// ── Plans (Phase 2f) ──────────────────────────────────────────────────────────

/** List all plans for a game. */
export function listPlans(gameID: string | number): Promise<{ plans: Plan[] }> {
	return apiFetch(`/tables/${gameID}/plans`);
}

/** Check which Phase 2 plans the current player is eligible to prepare. */
export function getPlanEligibility(gameID: string | number): Promise<{
	eligible: EligiblePlan[];
	ineligible: IneligiblePlan[];
}> {
	return apiFetch(`/tables/${gameID}/plan-eligibility`);
}

/** Prepare a plan (focus player only). */
export function preparePlan(
	gameID: string | number,
	params: {
		plan_type: PlanType;
		target_player_id?: number | null;
		target_asset_id?: number | null;
		peer_count?: number;
		preparation_notes?: string | null;
	}
): Promise<{ plan: Plan }> {
	return apiFetch(`/tables/${gameID}/prepare-plan`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

/** Get full plan details including computed difficulty and resolution state. */
export function getPlan(planID: number): Promise<PlanDetail> {
	return apiFetch(`/plans/${planID}`);
}

/** Begin resolution of a pending plan on the current row. */
export function resolvePlan(planID: number): Promise<{
	plan_id: number;
	roll?: DiceRoll;
}> {
	return apiFetch(`/plans/${planID}/resolve`, { method: 'POST' });
}

/**
 * Exchange Courtiers fair trade step.
 *
 * - Target player offers a peer:  { action: 'offer', offered_asset_id: X }
 * - Preparer accepts the offer:   { action: 'accept' }
 * - Preparer declines (roll):     { action: 'decline' }
 */
export function fairTrade(
	planID: number,
	body: { action: 'offer'; offered_asset_id: number }
		| { action: 'accept' }
		| { action: 'decline' }
): Promise<{ plan_id: number; roll?: DiceRoll; result?: string }> {
	return apiFetch(`/plans/${planID}/fair-trade`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

/** Record make/mar option choices after the dice roll resolves. */
export function makeChoice(
	planID: number,
	result: 'make' | 'mar',
	choices: string[]
): Promise<{ plan_id: number; result: string; choices: string[] }> {
	return apiFetch(`/plans/${planID}/make-choice`, {
		method: 'POST',
		body: JSON.stringify({ result, choices })
	});
}

/** Mark the plan as resolved (after choices applied). */
export function completePlan(planID: number): Promise<{
	plan_id: number;
	result: 'make' | 'mar';
}> {
	return apiFetch(`/plans/${planID}/complete`, { method: 'POST' });
}
