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

export interface Law {
	id: number;
	game_id: number;
	text: string;
	addendum: string | null;
	origin_plan_id: number | null;
	signatory_id: number | null;
	created_at: string;
	is_active: boolean;
	display_order: number;
}

export interface Rumor {
	id: number;
	game_id: number;
	text: string;
	target_asset_id: number | null;
	origin_plan_id: number | null;
	source_player_id: number | null;
	is_active: boolean;
	created_at: string;
	display_order: number;
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
	/** EC make + "messy": true once MakeChoice records the messy option. */
	messy_break_required?: boolean;
	/** EC make + "messy": true once the target has completed the messy break. */
	messy_break_done?: boolean;
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
	laws?: Law[];
	rumors?: Rumor[];
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

/**
 * Exchange Courtiers — messy break.
 * Called by the TARGET player after a make + "messy" outcome to tear a
 * marginalia on any asset. Must be called before CompletePlan is allowed.
 *
 * @param marginaliaID  The DB id of the marginalia to tear.
 */
export function messyBreak(planID: number, marginaliaID: number): Promise<{
	plan_id: number;
	marginalia_id: number;
	asset_id: number;
}> {
	return apiFetch(`/plans/${planID}/messy-break`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID }),
	});
}

// ── Plans (Phase 3 — Tier 1) ─────────────────────────────────────────────────
//
// Thin wrappers, one per endpoint in PHASE3_SPEC.md §New Endpoints — Tier 1.
// Response shapes are loose (plan_id + occasional extras) because backends
// mostly echo the plan and rely on WS for detailed state.

type PlanEcho = { plan_id: number } & Record<string, unknown>;

/** Seek Answers — break a marginalia on a target resource asset. */
export function breakResource(planID: number, assetID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-resource`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID, marginalia_id: marginaliaID }),
	});
}

/** Seek Answers — grant the preparer visibility into a secret-bearing asset. */
export function revealSecret(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/reveal-secret`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/**
 * Spread Rumors — break a marginalia.
 *
 * On make (preparer) the marginalia must belong to the plan's target asset
 * and `assetID` may be omitted. On mar (target-asset owner) `assetID` is
 * required and must be one of the preparer's assets.
 */
export function breakTarget(planID: number, marginaliaID: number, assetID?: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-target`, {
		method: 'POST',
		body: JSON.stringify({ marginalia_id: marginaliaID, asset_id: assetID ?? null }),
	});
}

/**
 * Spread Rumors — consent-based asset transfer.
 *
 * On make (preparer) the server transfers plan.target_asset_id; omit
 * `assetID`. On mar (target-asset owner) `assetID` must be one of the
 * preparer's assets, which is then transferred to the target-asset owner.
 * (Named `takeRumorAsset` to avoid collision with the asset-level `takeAsset`.)
 */
export function takeRumorAsset(planID: number, consent: boolean, assetID?: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/take-asset`, {
		method: 'POST',
		body: JSON.stringify({ consent, asset_id: assetID ?? null }),
	});
}

/** Spread Rumors — hide the rumor source behind a secret-bearing asset. */
export function hideSource(planID: number, secretAssetID: number, secretText: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/hide-source`, {
		method: 'POST',
		body: JSON.stringify({ secret_asset_id: secretAssetID, secret_text: secretText }),
	});
}

/** Chronicle Histories — add an artifact to the invoked list (pre-roll or via make option). */
export function invokeArtifact(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/invoke-artifact`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/** Chronicle Histories — break a marginalia on an invoked artifact. */
export function breakArtifact(planID: number, assetID: number, marginaliaID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/break-artifact`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID, marginalia_id: marginaliaID }),
	});
}

/**
 * Chronicle Histories — non-preparer (or preparer) submits a mar choice.
 * `asset_id` is required for break_artifact / invoke_another.
 */
export function marChoice(planID: number, choice: string, assetID?: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/mar-choice`, {
		method: 'POST',
		body: JSON.stringify({ choice, asset_id: assetID ?? null }),
	});
}

// ── Plans (Phase 3 — Tier 2) ─────────────────────────────────────────────────

/** Propose Decree — join the council by leveraging one or more assets. */
export function joinCouncil(planID: number, assetIDs: number[]): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-council`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/** Propose Decree — the current signatory closes the council and triggers the roll. */
export function callRoll(planID: number): Promise<{ plan_id: number; roll?: DiceRoll }> {
	return apiFetch(`/plans/${planID}/call-roll`, { method: 'POST' });
}

/** Propose Decree — signatory sets the addendum text after a make. */
export function setAddendum(planID: number, addendum: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/set-addendum`, {
		method: 'POST',
		body: JSON.stringify({ addendum }),
	});
}

/** Clandestinely Liaise — focus player advances to the next phase. */
export function advanceLiaise(planID: number): Promise<{ plan_id: number; phase: string }> {
	return apiFetch(`/plans/${planID}/advance-liaise`, { method: 'POST' });
}

/** Clandestinely Liaise (phase 2) — commit a secret-bearing asset. */
export function keepSecret(planID: number, assetID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/keep-secret`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/** Clandestinely Liaise (phase 3) — submit a "Things We Share" choice. */
export function shareChoice(
	planID: number,
	body: { choice: string; target_asset_id?: number | null; die_face?: number | null }
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/share-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Clandestinely Liaise (phase 4) — submit re-delay die face (0 = cancel). */
export function redelayReveal(planID: number, face: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/redelay-reveal`, {
		method: 'POST',
		body: JSON.stringify({ face }),
	});
}

/** Spend a banked die on this roll. */
export function useBankedDie(rollID: number, bankedDieID: number): Promise<{ roll_id: number }> {
	return apiFetch(`/rolls/${rollID}/use-banked-die`, {
		method: 'POST',
		body: JSON.stringify({ banked_die_id: bankedDieID }),
	});
}

// ── Simultaneous reveals (shared) ────────────────────────────────────────────

export type RevealType = 'make_war_delay' | 'liaise_delay' | 'liaise_redelay';

export interface SimultaneousRevealEntry {
	player_id: number;
	/** Null until the reveal completes; then populated for every participant. */
	face: number | null;
	revealed_at: string | null;
}

export interface SimultaneousReveal {
	id: number;
	game_id: number;
	plan_id: number | null;
	reveal_type: RevealType;
	is_complete: boolean;
	result_delay: number | null;
	entries: SimultaneousRevealEntry[];
}

/** Submit a die face for a simultaneous reveal. */
export function submitReveal(revealID: number, face: number): Promise<{ reveal_id: number; is_complete: boolean }> {
	return apiFetch(`/reveals/${revealID}/submit`, {
		method: 'POST',
		body: JSON.stringify({ face }),
	});
}

/** Fetch reveal state. Faces stay hidden until every participant has submitted. */
export function getReveal(revealID: number): Promise<SimultaneousReveal> {
	return apiFetch(`/reveals/${revealID}`);
}

// ── Plans (Phase 3 — Tier 3) ─────────────────────────────────────────────────

// Propose Duel.

/** Propose Duel — elect a peer as champion (omit asset_id to fight yourself). */
export function electChampion(planID: number, assetID: number | null): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/elect-champion`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID }),
	});
}

/** Propose Duel — simultaneously reveal stake count (1..1+status). */
export function stakeReveal(planID: number, count: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/stake-reveal`, {
		method: 'POST',
		body: JSON.stringify({ count }),
	});
}

/** Propose Duel — select the specific assets to stake. */
export function selectStakes(planID: number, assetIDs: number[]): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/select-stakes`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/** Propose Duel — declarer picks a stake and declares high or low. */
export function boutDeclare(planID: number, stakeID: number, declaration: 'high' | 'low'): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/bout-declare`, {
		method: 'POST',
		body: JSON.stringify({ stake_id: stakeID, declaration }),
	});
}

/** Propose Duel — responder picks their stake; server resolves the bout. */
export function boutRespond(planID: number, stakeID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/bout-respond`, {
		method: 'POST',
		body: JSON.stringify({ stake_id: stakeID }),
	});
}

// Host Festivity.

/** Host Festivity — join as a guest. */
export function joinFestivity(planID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-festivity`, { method: 'POST' });
}

/** Host Festivity — guest commits to rolling or opts out. */
export function guestRoll(planID: number, action: 'roll' | 'opt_out'): Promise<{ plan_id: number; roll?: DiceRoll }> {
	return apiFetch(`/plans/${planID}/guest-roll`, {
		method: 'POST',
		body: JSON.stringify({ action }),
	});
}

/** Host Festivity — guest submits their make/mar choice after rolling. */
export function guestChoice(
	planID: number,
	body: { choice: string } & Record<string, unknown>
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/guest-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Host Festivity — host submits a make choice on behalf of a mar/opt-out guest. */
export function hostChoice(
	planID: number,
	body: { target_player_id: number; choice: string } & Record<string, unknown>
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/host-choice`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Host Festivity — challenge a guest to a duel; spawns a Propose Duel plan. */
export function challengeDuel(planID: number, targetPlayerID: number): Promise<{ plan_id: number; duel_plan_id: number }> {
	return apiFetch(`/plans/${planID}/challenge-duel`, {
		method: 'POST',
		body: JSON.stringify({ target_player_id: targetPlayerID }),
	});
}

// Make War.

/** Make War — uninvited player joins a side before the delay reveal completes. */
export function joinWar(planID: number, side: 1 | 2): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-war`, {
		method: 'POST',
		body: JSON.stringify({ side }),
	});
}

/** Make War — submit die face for the delay reveal. */
export function warReveal(planID: number, face: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/war-reveal`, {
		method: 'POST',
		body: JSON.stringify({ face }),
	});
}

/** Make War — pay the cost of battle (break asset, leverage two, surrender, negotiate). */
export function payBattleCost(
	planID: number,
	body: { choice: string; asset_id?: number | null } & Record<string, unknown>
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/pay-battle-cost`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Make War — propose peace terms. */
export function proposePeace(planID: number, terms: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/propose-peace`, {
		method: 'POST',
		body: JSON.stringify({ terms }),
	});
}

/** Make War — accept proposed peace terms. */
export function acceptPeace(planID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/accept-peace`, { method: 'POST' });
}

/** Make War — reject proposed peace terms. */
export function rejectPeace(planID: number): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/reject-peace`, { method: 'POST' });
}

// Make Demands.

/** Make Demands — pick a draft option on your turn in the alternating draft. */
export function draftChoice(planID: number, option: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/draft-choice`, {
		method: 'POST',
		body: JSON.stringify({ option }),
	});
}

/**
 * Make Demands — target places a counter Make Demands.
 * Pass `targetPlanID = null` to attach the counter to the target's next prepared plan.
 */
export function counterDemand(planID: number, targetPlanID: number | null): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/counter-demand`, {
		method: 'POST',
		body: JSON.stringify({ target_plan_id: targetPlanID }),
	});
}

// ── Laws & rumors ────────────────────────────────────────────────────────────

export function listLaws(gameID: number): Promise<{ laws: Law[] }> {
	return apiFetch(`/tables/${gameID}/laws`);
}

export function updateLaw(
	lawID: number,
	patch: { text: string; addendum?: string | null }
): Promise<{ law: Law }> {
	return apiFetch(`/laws/${lawID}`, {
		method: 'PATCH',
		body: JSON.stringify(patch),
	});
}

export function listRumors(gameID: number): Promise<{ rumors: Rumor[] }> {
	return apiFetch(`/tables/${gameID}/rumors`);
}

export function updateRumor(rumorID: number, text: string): Promise<{ rumor: Rumor }> {
	return apiFetch(`/rumors/${rumorID}`, {
		method: 'PATCH',
		body: JSON.stringify({ text }),
	});
}
