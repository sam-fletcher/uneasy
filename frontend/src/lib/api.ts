// api.ts — typed wrappers around the Go API.
// All requests go to the same origin as the page (the Go server proxies
// everything through one port, so no CORS is needed).

// ── Types ────────────────────────────────────────────────────────────────────

export type GamePhase = 'lobby' | 'tone_setting' | 'prologue' | 'main_event' | 'shake_up' | 'ended';
export type ToneTopicStatus = 'default' | 'include' | 'avoid_detail' | 'never';
export type RankingCategory = 'power' | 'knowledge' | 'esteem';
export type AssetType = 'peer' | 'holding' | 'artifact' | 'resource';

export interface Account {
	id: number;
	username: string;
	email: string | null;
}

export interface MyTable {
	game_id: number;
	join_code: string;
	is_facilitator: boolean;
	joined_at: string;
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
	prologue_ranking_step: PrologueRankingStep | null;
	shake_up_category: string | null;
	shake_up_step: number | null;
}

export type PrologueRankingStep =
	| 'declare_power'
	| 'place_set_asides_power'
	| 'declare_knowledge'
	| 'place_set_asides_knowledge'
	| 'declare_esteem'
	| 'place_set_asides_esteem'
	| 'extra_peers';

export type PrologueSheetType = 'titles' | 'hailing_from' | 'laws_rumors';

export interface PrologueCard {
	suit: 'C' | 'D' | 'S' | 'H';
	value: string;
}

export interface PrologueChoice {
	name: string;
	description: string;
	cards: [PrologueCard, PrologueCard];
}

export interface PrologueSheet {
	type: PrologueSheetType;
	display_name: string;
	choice_asset_type: 'Artifact' | 'Holding' | 'Resource';
	choices: PrologueChoice[];
}

export interface PrologueClaim {
	sheet_type: PrologueSheetType;
	choice_name: string;
	player_id: number;
	turn_number: number;
}

export interface PlayerCardRow {
	id: number;
	game_id: number;
	player_id: number;
	card_suit: 'C' | 'D' | 'S' | 'H';
	card_value: string;
}

export interface Player {
	id: number;
	game_id: number;
	account_id: number;
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

/**
 * One entry in the unified chat feed. Two shapes:
 *
 * - **Player message**: `author_id != null`, `system_code == null`,
 *   `severity == 0`. Always shown regardless of severity threshold.
 * - **System post**: `author_id == null`, `system_code != null`, `severity > 0`.
 *   Severity is an integer scale (see SEVERITY in lib/severity.ts);
 *   `system_code` identifies the event class (`row.advanced`,
 *   `scene.started`, `plan.prepared`, etc.) and indexes the schema of
 *   `system_data`.
 */
export interface ChatPost {
	id: number;
	game_id: number;
	row_number: number | null;
	plan_id: number | null;
	scene_id: number | null;
	author_id: number | null;
	body: string;
	created_at: string;
	severity: number;
	system_code: string | null;
	system_data: unknown;
	/** Asset the author is speaking as for this message; null = OOC / system. */
	speaking_as_asset_id: number | null;
}

/** @deprecated use ChatPost — retained briefly for incremental migration. */
export type ScenePost = ChatPost;

export interface SceneEntry {
	id: number;
	game_id: number;
	row_number: number;
	author_id: number;
	body: string;
	created_at: string;
}

export type RollStage = 'decide_vote' | 'voting' | 'leverage' | 'resolved';

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
	stage: RollStage;
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
	cancelled_by_die_id: number | null;
}

/**
 * A vote view as returned by GET /rolls and /rolls/active. During the
 * voting stage, other players' vote values are server-redacted and the
 * `vote` field is omitted (`voted` is true regardless).
 */
export interface VoteView {
	roll_id: number;
	player_id: number;
	voted: true;
	vote?: 1 | -1;
}

export type RollIntent = 'aid' | 'interfere';

export interface RollParticipant {
	roll_id: number;
	player_id: number;
	intent: RollIntent | null;
	is_ready: boolean;
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
	/** Null while a variable-delay plan (Make War / Clandestinely Liaise) is
	 *  awaiting its simultaneous delay reveal. The reveal closing assigns the
	 *  real row and broadcasts plan.prepared again. */
	row_number: number | null;
	row_order: number;
	prepared_at_row: number;
	status: 'pending' | 'resolving' | 'resolved' | 'cancelled';
	result: 'make' | 'mar' | null;
	resolved_at: string | null;
	preparation_notes: string | null;
	resolution_data: string | null;
	/** Set on a Make Demands plan to point at the plan being demanded against. */
	targeted_plan_id: number | null;
}

/** One entry in ResolutionData.make_mar_choices. Mirrors game.Choice
 * (uneasy/game/plan.go). Entries from the generic POST /make-choice
 * endpoint leave player_id null; per-plan handlers (Chronicle) set it. */
export interface Choice {
	player_id?: number | null;
	option: string;
}

/** One player's keep-secret submission in Clandestinely Liaise's
 * "Secrets We Keep" phase. Mirrors game.KeptSecret. */
export interface KeptSecret {
	player_id: number;
	asset_id: number;
}

// Per-plan typed resolution_data views. Each plan's view lives in
// $lib/plans/resolutionData/<name>.ts and is re-exported here so the
// shared ResolutionData interface can reference it.
export type { LiaiseResolutionData } from '$lib/plans/resolutionData/liaise';
import type { LiaiseResolutionData } from '$lib/plans/resolutionData/liaise';
export type { SpreadPropagandaResolutionData } from '$lib/plans/resolutionData/spread_propaganda';
import type { SpreadPropagandaResolutionData } from '$lib/plans/resolutionData/spread_propaganda';
export type { SpreadRumorsResolutionData } from '$lib/plans/resolutionData/spread_rumors';
import type { SpreadRumorsResolutionData } from '$lib/plans/resolutionData/spread_rumors';
export type { MakeDemandsResolutionData } from '$lib/plans/resolutionData/make_demands';
import type { MakeDemandsResolutionData } from '$lib/plans/resolutionData/make_demands';
export type { ProposeDecreeResolutionData } from '$lib/plans/resolutionData/propose_decree';
import type { ProposeDecreeResolutionData } from '$lib/plans/resolutionData/propose_decree';
export type { MakeIntroductionsResolutionData } from '$lib/plans/resolutionData/make_introductions';
import type { MakeIntroductionsResolutionData } from '$lib/plans/resolutionData/make_introductions';
export type { ExchangeCourtiersResolutionData } from '$lib/plans/resolutionData/exchange_courtiers';
import type { ExchangeCourtiersResolutionData } from '$lib/plans/resolutionData/exchange_courtiers';
export type { ChronicleHistoriesResolutionData } from '$lib/plans/resolutionData/chronicle_histories';
import type { ChronicleHistoriesResolutionData } from '$lib/plans/resolutionData/chronicle_histories';
export type { DuelResolutionData, DuelPhase } from '$lib/plans/resolutionData/propose_duel';
import type { DuelResolutionData } from '$lib/plans/resolutionData/propose_duel';
export type { MakeWarResolutionData } from '$lib/plans/resolutionData/make_war';
import type { MakeWarResolutionData } from '$lib/plans/resolutionData/make_war';
export type { FestivityResolutionData, FestivityPhase } from '$lib/plans/resolutionData/host_festivity';
import type { FestivityResolutionData } from '$lib/plans/resolutionData/host_festivity';

/** Mirrors game.ResolutionData (uneasy/game/plan.go). All fields optional —
 * only the ones relevant to a given plan type are populated. */
export interface ResolutionData {
	// ── Exchange Courtiers ──
	// All EC-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/exchange_courtiers.ts.
	exchange_courtiers?: ExchangeCourtiersResolutionData;

	// ── Make Introductions ──
	// All MI-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/make_introductions.ts.
	make_introductions?: MakeIntroductionsResolutionData;

	// ── Spread Propaganda ──
	// All SP-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/spread_propaganda.ts. Read via
	// parseSpreadPropagandaData(plan) for an ergonomic always-non-nil view.
	spread_propaganda?: SpreadPropagandaResolutionData;

	// ── Make/Mar choices ──
	// Written by POST /api/plans/:id/make-choice and by per-plan handlers
	// (e.g. Chronicle) for per-player make/mar entries. Plan-specific
	// pre-roll state belongs on per-plan typed fields, not here.
	//
	// Entries from the generic endpoint have player_id == null. Per-plan
	// handlers (Chronicle) set player_id to the submitting player.
	make_mar_choices?: Choice[];

	// ── Spread Rumors ──
	// All SR-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/spread_rumors.ts.
	spread_rumors?: SpreadRumorsResolutionData;

	// ── Chronicle Histories ──
	// All CH-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/chronicle_histories.ts.
	chronicle_histories?: ChronicleHistoriesResolutionData;

	// ── Propose Decree ──
	// All PD-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/propose_decree.ts.
	propose_decree?: ProposeDecreeResolutionData;

	// ── Clandestinely Liaise ──
	// All Liaise-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/liaise.ts. Read via parseLiaiseData(plan)
	// for an ergonomic always-non-nil view.
	liaise?: LiaiseResolutionData;

	// ── Propose Duel ──
	// All duel-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/propose_duel.ts.
	duel?: DuelResolutionData;

	// ── Host Festivity ──
	// All HF-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/host_festivity.ts.
	festivity?: FestivityResolutionData;

	// ── Make War ──
	// All MW-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/make_war.ts.
	make_war?: MakeWarResolutionData;

	// ── Make Demands ──
	// All MD-specific state lives on the nested struct; see
	// $lib/plans/resolutionData/make_demands.ts.
	make_demands?: MakeDemandsResolutionData;
}

/** Response shape from GET /api/plans/:id. */
export interface PlanDetail {
	plan: Plan;
	difficulty: number;
	resolution_data: ResolutionData;
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

// ── Accounts & sessions ─────────────────────────────────────────────────────

export function createAccount(body: {
	username: string;
	code: string;
	email?: string | null;
}): Promise<Account> {
	return apiFetch<Account>('/accounts', {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function login(username: string, code: string): Promise<Account> {
	return apiFetch<Account>('/sessions', {
		method: 'POST',
		body: JSON.stringify({ username, code })
	});
}

export async function logout(): Promise<void> {
	await fetch('/api/sessions', { method: 'DELETE' });
}

export async function getMe(): Promise<Account | null> {
	const res = await fetch('/api/accounts/me');
	if (res.status === 401) return null;
	if (!res.ok) throw new Error(`HTTP ${res.status}`);
	return (await res.json()) as Account;
}

export function updateMe(patch: {
	username?: string;
	email?: string | null;
	code?: string;
}): Promise<Account> {
	return apiFetch<Account>('/accounts/me', {
		method: 'PATCH',
		body: JSON.stringify(patch)
	});
}

export function listMyTables(): Promise<{ tables: MyTable[] }> {
	return apiFetch('/accounts/me/tables');
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

// RowStateKind names the rulebook step (or pre-step gate) a main-event row
// is currently in. Authoritative — computed server-side from plans, scenes,
// wars, and reveals. See model/row_state.go for the precedence chain.
export type RowStateKind =
	| 'phase_not_main_event'
	| 'await_surrender_claim'
	| 'await_battle_cost'
	| 'await_delay_reveal'
	| 'plan_resolving'
	| 'plan_pending'
	| 'await_demand_counter'
	| 'await_demand_draft_pick'
	| 'await_festivity_guest_turn'
	| 'await_festivity_challenge_response'
	| 'await_duel_staking'
	| 'await_duel_bout'
	| 'scene_active'
	| 'post_scene_action'
	| 'scene_setting';

export interface RowState {
	kind: RowStateKind;
	plan_id?: number | null;
	scene_id?: number | null;
	war_id?: number | null;
	claim_id?: number | null;
	/** Player whose action the row is blocked on for sub-phase kinds that
	 *  override plan_resolving (await_demand_counter,
	 *  await_festivity_guest_turn, await_festivity_challenge_response). */
	acting_player_id?: number | null;
}

// Full game state including phase-specific data.
export function getGameState(id: string | number): Promise<{
	game: Game;
	players: Player[];
	tone_topics?: ToneTopic[];
	rankings?: Ranking[];
	laws?: Law[];
	rumors?: Rumor[];
	current_prologue_player_id?: number | null;
	/** Authoritative row-state in main_event phase. Absent in other phases. */
	row_state?: RowState;
}> {
	return apiFetch(`/tables/${id}/state`);
}

// ── Phase Transitions ────────────────────────────────────────────────────────

export function startPrologue(gameID: string | number): Promise<{ phase: GamePhase }> {
	return apiFetch(`/tables/${gameID}/start-prologue`, { method: 'POST' });
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


// ── Phase 4c: Shake-Up ───────────────────────────────────────────────────────

export type ShakeUpCategory = 'esteem' | 'knowledge' | 'power';

export interface ShakeUpOptionInfo {
	Key: string;
	Category: ShakeUpCategory;
	Description: string;
	NeedsAsset: boolean;
	BumpsTrack: string;
}

export interface ShakeUpSpend {
	id: number;
	game_id: number;
	player_id: number;
	category: ShakeUpCategory;
	option_key: string;
	target_asset_id: number | null;
	target_player_id: number | null;
	base_cost: number;
	final_cost: number | null;
	committed_at: string | null;
	applied: boolean;
	created_at: string;
}

export interface ShakeUpAdjustmentRow {
	id: number;
	spend_id: number;
	player_id: number;
	adjustment: number;
	created_at: string;
}

export interface ShakeUpTokensRow {
	id: number; // player id
	shake_up_tokens: number;
}

export function getShakeUp(gameID: string | number): Promise<{
	phase: string;
	shake_up_category: ShakeUpCategory | null;
	shake_up_step: number | null;
	tokens: ShakeUpTokensRow[];
	options: ShakeUpOptionInfo[] | null;
	open_spend?: { spend: ShakeUpSpend; adjustments: ShakeUpAdjustmentRow[] };
}> {
	return apiFetch(`/tables/${gameID}/shake-up`);
}

export function shakeUpRoll(
	gameID: string | number,
	result: number
): Promise<{ tokens: number }> {
	return apiFetch(`/tables/${gameID}/shake-up/roll`, {
		method: 'POST',
		body: JSON.stringify({ result })
	});
}

export function shakeUpSpend(
	gameID: string | number,
	body: { option_key: string; target_asset_id?: number; target_player_id?: number }
): Promise<{ spend: ShakeUpSpend }> {
	return apiFetch(`/tables/${gameID}/shake-up/spend`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function shakeUpAdjust(
	gameID: string | number,
	spendID: number,
	adjustment: 1 | -1
): Promise<{ adjustment: ShakeUpAdjustmentRow }> {
	return apiFetch(`/tables/${gameID}/shake-up/adjust`, {
		method: 'POST',
		body: JSON.stringify({ spend_id: spendID, adjustment })
	});
}

export function shakeUpCommit(
	gameID: string | number,
	spendID: number
): Promise<{ spend_id: number; final_cost: number }> {
	return apiFetch(`/tables/${gameID}/shake-up/commit`, {
		method: 'POST',
		body: JSON.stringify({ spend_id: spendID })
	});
}

// ── Phase 4d: endgame mode selection ─────────────────────────────────────────

export type EndgameMode = 'smooth_landing' | 'explosive_finale';

export function setEndgameMode(
	gameID: string | number,
	mode: EndgameMode
): Promise<{ mode: EndgameMode }> {
	return apiFetch(`/tables/${gameID}/endgame`, {
		method: 'POST',
		body: JSON.stringify({ mode })
	});
}

// ── Phase 4b: structured prologue ────────────────────────────────────────────

export function getPrologueSheets(
	gameID: string | number
): Promise<{
	sheets: PrologueSheet[];
	claims: PrologueClaim[];
	current_player_id: number | null;
	turn_number: number;
}> {
	return apiFetch(`/tables/${gameID}/prologue/sheets`);
}

export function getPrologueCards(
	gameID: string | number
): Promise<{ cards: PlayerCardRow[] }> {
	return apiFetch(`/tables/${gameID}/prologue/cards`);
}

export interface PrologueCardAssetText {
	suit: string;
	value: string;
	text: string;
}

export function choosePrologue(
	gameID: string | number,
	body: {
		sheet_type: PrologueSheetType;
		choice_name: string;
		asset_text: string;
		marginalium_text?: string;
		law_or_rumor_text?: string;
		card_assets: PrologueCardAssetText[];
	}
): Promise<{ sheet_type: PrologueSheetType; choice_name: string; turn_number: number }> {
	return apiFetch(`/tables/${gameID}/prologue/choose`, {
		method: 'POST',
		body: JSON.stringify(body)
	});
}

export function getPrologueCardSuggestions(
	gameID: string | number,
	suit: string,
): Promise<{ suggestions: string[]; asset_type: string }> {
	return apiFetch(`/tables/${gameID}/prologue/card-suggestions?suit=${encodeURIComponent(suit)}`);
}

export function declareHearts(
	gameID: string | number,
	count: number
): Promise<{ track: string; count: number }> {
	return apiFetch(`/tables/${gameID}/prologue/declare-hearts`, {
		method: 'POST',
		body: JSON.stringify({ count })
	});
}

export function finalizeTrackRanking(
	gameID: string | number
): Promise<{
	track: string;
	ranked: number[];
	set_aside: number[];
	next_step: PrologueRankingStep | '';
}> {
	return apiFetch(`/tables/${gameID}/prologue/finalize-ranking`, { method: 'POST' });
}

export function placePrologueSetAsides(
	gameID: string | number,
	ordering: number[]
): Promise<{ track: string; next_step: PrologueRankingStep | '' }> {
	return apiFetch(`/tables/${gameID}/prologue/place-set-asides`, {
		method: 'POST',
		body: JSON.stringify({ ordering })
	});
}

export function createExtraPeer(
	gameID: string | number,
	titleName: string,
	peerText: string,
): Promise<{ asset: Asset }> {
	return apiFetch(`/tables/${gameID}/prologue/extra-peer`, {
		method: 'POST',
		body: JSON.stringify({ title_name: titleName, peer_text: peerText })
	});
}

// ── Phase 4b: max-commitment prologue ranking ────────────────────────────────

export type PrologueTrack = 'power' | 'knowledge' | 'esteem';

export interface CommittedHeart {
	player_id: number;
	track: PrologueTrack;
	card_id: number;
	value: string;
	suit: 'H';
}

export interface TrackDone {
	player_id: number;
	track: PrologueTrack;
	done: boolean;
}

export interface ExtraPeer {
	player_id: number;
	title_name: string;
	asset_id: number;
}

export interface PrologueRankingState {
	committed: CommittedHeart[];
	done: TrackDone[];
	extra_peers: ExtraPeer[];
}

export function getPrologueRankingState(
	gameID: string | number
): Promise<PrologueRankingState> {
	return apiFetch(`/tables/${gameID}/prologue/ranking-state`);
}

export function commitTrackHearts(
	gameID: string | number,
	track: PrologueTrack,
	cardIDs: number[]
): Promise<{ track: PrologueTrack; card_ids: number[] }> {
	return apiFetch(`/tables/${gameID}/prologue/committed-hearts`, {
		method: 'POST',
		body: JSON.stringify({ track, card_ids: cardIDs })
	});
}

export function setPrologueDone(
	gameID: string | number,
	track: PrologueTrack,
	done: boolean
): Promise<{ track: PrologueTrack; done: boolean }> {
	return apiFetch(`/tables/${gameID}/prologue/done`, {
		method: 'POST',
		body: JSON.stringify({ track, done })
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

// ── Chat (unified game-wide feed) ────────────────────────────────────────────

export function listGamePosts(
	gameID: string | number,
	opts?: { afterID?: number }
): Promise<{ posts: ChatPost[] }> {
	const query = opts?.afterID != null ? `?after=${opts.afterID}` : '';
	return apiFetch(`/tables/${gameID}/posts${query}`);
}

export function createPlayerPost(
	gameID: string | number,
	body: string,
	opts?: { speakingAsAssetID?: number | null }
): Promise<{ post: ChatPost }> {
	const payload: Record<string, unknown> = { body };
	if (opts?.speakingAsAssetID) {
		payload.speaking_as_asset_id = opts.speakingAsAssetID;
	}
	return apiFetch(`/tables/${gameID}/posts`, {
		method: 'POST',
		body: JSON.stringify(payload)
	});
}

// ── Scenes ───────────────────────────────────────────────────────────────────

export type TimeElapsed =
	| 'moments'
	| 'hours'
	| 'days'
	| 'weeks'
	| 'flashback'
	| 'simultaneous';

export interface Scene {
	id: number;
	game_id: number;
	row_number: number;
	focus_player_id: number;
	location_holding_id: number | null;
	location_custom: string | null;
	time_elapsed: TimeElapsed;
	time_note: string | null;
	prompt: string;
	resolved_plan_id: number | null;
	started_at: string;
	ended_at: string | null;
}

export interface ScenePeerView {
	peer_asset_id: number;
	/** null = unclaimed focus-player peer (a non-focus player can take over). */
	controller_player_id: number | null;
}

export interface SceneResponse {
	scene: Scene | null;
	peers: ScenePeerView[];
}

/**
 * Ephemeral broadcast of the focus player's in-flight scene-setup
 * selections. Mirrors model.SceneSetupDraftPayload. Not persisted; consumed
 * by SceneSetupForm in read-only mode so non-focus players can see what's
 * being chosen as it happens.
 */
export interface SceneSetupDraft {
	player_id: number;
	holding_id: number | null;
	custom_location: string;
	time_elapsed: string;
	time_note: string;
	present_peer_ids: number[];
}

/**
 * Ephemeral broadcast of the focus player's currently-highlighted plan
 * card during the post-scene "prepare a plan" step. Mirrors
 * model.PreparePlanDraftPayload. Not persisted. plan_type is "" when the
 * focus player has nothing selected.
 */
export interface PreparePlanDraft {
	player_id: number;
	plan_type: string;
	/**
	 * Opaque per-plan-type snapshot of the prep form. The shape is owned
	 * by the plan's panel component; consumers cast based on plan_type.
	 * May be undefined/null when only a card is highlighted but no fields
	 * have been touched yet.
	 */
	prep?: Record<string, unknown> | null;
}

export function getActiveScene(gameID: string | number): Promise<SceneResponse> {
	return apiFetch(`/tables/${gameID}/scenes/active`);
}

export function createScene(
	gameID: string | number,
	params: {
		location_holding_id?: number | null;
		location_custom?: string;
		time_elapsed: TimeElapsed;
		time_note?: string;
		present_peer_ids: number[];
	}
): Promise<SceneResponse> {
	return apiFetch(`/tables/${gameID}/scenes`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

export function claimScenePeer(
	gameID: string | number,
	sceneID: number,
	peerAssetID: number
): Promise<{ scene_id: number; peer_asset_id: number; controller_id: number }> {
	return apiFetch(`/tables/${gameID}/scenes/${sceneID}/claim-peer`, {
		method: 'POST',
		body: JSON.stringify({ peer_asset_id: peerAssetID })
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
	params: { name?: string; is_main_character?: boolean; tear_position?: number }
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

/** All secrets in this game that the caller can see (one query for the retinue). */
export function getVisibleSecrets(gameID: string | number): Promise<{ secrets: Secret[] }> {
	return apiFetch(`/tables/${gameID}/secrets/visible`);
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

// ── Dice Rolls (Phase 2e + stage machine) ─────────────────────────────────────

export interface ActiveRollPayload {
	roll: DiceRoll | null;
	dice: DiceRollDie[];
	votes: VoteView[];
	participants: RollParticipant[];
}

/** Get the active (unresolved) dice roll for a game, if any. */
export function getActiveRollForGame(gameID: string | number): Promise<ActiveRollPayload> {
	return apiFetch(`/tables/${gameID}/rolls/active`);
}

/**
 * Create a new dice roll. The caller specifies the actor explicitly. If a
 * scene_id or plan_id is provided, the server cross-validates the actor
 * against the scene's focus_player_id / plan's preparer_id.
 */
export function createRoll(
	gameID: string | number,
	params: { actor_id: number; difficulty: number; scene_id?: number; plan_id?: number }
): Promise<{ roll: DiceRoll }> {
	return apiFetch(`/tables/${gameID}/rolls`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

/** Get full roll state — roll, dice, redacted votes, participants. */
export function getRoll(rollID: number): Promise<{
	roll: DiceRoll;
	dice: DiceRollDie[];
	votes: VoteView[];
	participants: RollParticipant[];
}> {
	return apiFetch(`/rolls/${rollID}`);
}

/** Leverage one of your assets to add a die to the active roll. */
export function leverageRoll(
	rollID: number,
	assetID: number
): Promise<{ die: DiceRollDie }> {
	return apiFetch(`/rolls/${rollID}/leverage`, {
		method: 'POST',
		body: JSON.stringify({ asset_id: assetID })
	});
}

/** Actor opens a difficulty vote (decide_vote → voting). */
export function callVote(rollID: number): Promise<{ roll_id: number }> {
	return apiFetch(`/rolls/${rollID}/call-vote`, { method: 'POST' });
}

/** Actor skips the difficulty vote (decide_vote → leverage). */
export function skipVote(rollID: number): Promise<{ roll_id: number }> {
	return apiFetch(`/rolls/${rollID}/skip-vote`, { method: 'POST' });
}

/** Submit a difficulty vote: +1 (harder) or -1 (easier). Hidden ballot. */
export function voteOnRoll(rollID: number, vote: 1 | -1): Promise<{
	vote: number;
	adjusted_difficulty?: number;
}> {
	return apiFetch(`/rolls/${rollID}/vote`, {
		method: 'POST',
		body: JSON.stringify({ vote })
	});
}

/** Non-actor sets their intent. Locks once they commit any die. */
export function setRollIntent(rollID: number, intent: RollIntent): Promise<{ intent: RollIntent }> {
	return apiFetch(`/rolls/${rollID}/intent`, {
		method: 'POST',
		body: JSON.stringify({ intent })
	});
}

/** Toggle ready. Setting ready=true when last unready triggers auto-resolve. */
export function setRollReady(rollID: number, isReady: boolean): Promise<{ is_ready: boolean }> {
	return apiFetch(`/rolls/${rollID}/ready`, {
		method: 'POST',
		body: JSON.stringify({ is_ready: isReady })
	});
}

/** Legacy: actor/facilitator closes leverage. Not surfaced in the new UI. */
export function closeLeverage(rollID: number): Promise<{ roll_id: number }> {
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
		target_plan_id?: number | null;
		peer_count?: number;
		enemy_player_ids?: number[];
		duel_type?: 'arms' | 'wits';
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

/**
 * Make Introductions — create a single peer during the pre-roll naming step.
 * Called once per peer until peer_count peers exist; then call
 * finalizeIntroductionsPeers to create the dice roll.
 */
export function createIntroductionsPeer(
	planID: number,
	params: { name: string; marginalia?: string[] }
): Promise<{ plan_id: number; asset: Asset; created_peer_ids: number[] }> {
	return apiFetch(`/plans/${planID}/create-peer`, {
		method: 'POST',
		body: JSON.stringify(params)
	});
}

/**
 * Make Introductions — finalize the pre-roll naming step. Creates the
 * dice roll; resolution proceeds normally from there. Returns the roll.
 */
export function finalizeIntroductionsPeers(planID: number): Promise<{
	plan_id: number;
	roll: DiceRoll;
}> {
	return apiFetch(`/plans/${planID}/finalize-peers`, { method: 'POST' });
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

export interface BankedDie {
	id: number;
	game_id: number;
	player_id: number;
	source: string;
	created_at: string;
	used_at: string | null;
	used_roll_id: number | null;
}

/** List the calling player's unspent banked dice in this game. */
export function listBankedDice(gameID: string | number): Promise<{ dice: BankedDie[] }> {
	return apiFetch(`/tables/${gameID}/banked-dice`);
}

/** Spend a banked die on this roll. Direction follows the player's intent. */
export function useBankedDie(rollID: number, bankedDieID: number): Promise<{ die: DiceRollDie; banked_die_id: number }> {
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

/** A staked asset as seen by a caller. `hidden_die` is null for stakes the
 * caller doesn't own unless the stake has already been resolved in a bout. */
export interface DuelStake {
	id: number;
	plan_id: number;
	player_id: number;
	asset_id: number;
	is_resolved: boolean;
	is_winner: boolean | null;
	hidden_die: number | null;
}

export interface DuelBout {
	id: number;
	plan_id: number;
	bout_number: number;
	declarer_id: number;
	declarer_stake_id: number;
	responder_id: number;
	responder_stake_id: number | null;
	declaration: 'high' | 'low' | null;
	declarer_die: number | null;
	responder_die: number | null;
	winner_id: number | null;
	is_match: boolean;
	created_at: string;
	resolved_at: string | null;
}

export interface DuelStateResponse {
	plan_id: number;
	stakes: DuelStake[];
	bouts: DuelBout[];
}

/** Propose Duel — current stakes + bout history visible to the caller. */
export function getDuelState(planID: number): Promise<DuelStateResponse> {
	return apiFetch(`/plans/${planID}/duel-state`);
}

/** Propose Duel — elect a peer as champion. Pass null to fight yourself. */
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

/** Propose Duel — select the specific assets to stake. Response includes the
 * caller's newly created stakes (with hidden dice). */
export function selectStakes(
	planID: number,
	assetIDs: number[],
): Promise<{ plan_id: number; staked: number; stakes: DuelStake[] }> {
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
export function challengeDuel(
	planID: number,
	targetPlayerID: number,
	notes?: string,
): Promise<{ plan_id: number; duel_plan_id?: number; challenger_id?: number; target_id?: number; must_accept?: boolean }> {
	return apiFetch(`/plans/${planID}/challenge-duel`, {
		method: 'POST',
		body: JSON.stringify({ target_player_id: targetPlayerID, preparation_notes: notes ?? '' }),
	});
}

/** Host Festivity — challenged guest accepts or declines the duel. */
export function respondChallenge(planID: number, accept: boolean): Promise<{ plan_id: number; accepted: boolean; duel_plan_id?: number }> {
	return apiFetch(`/plans/${planID}/respond-challenge`, {
		method: 'POST',
		body: JSON.stringify({ accept }),
	});
}

/** Host Festivity — guest with an unspent IOU forces the host to take a mar option. */
export function insistHostMar(
	planID: number,
	body: { mar_option: string; asset_id?: number; rumor_text?: string },
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/insist-host-mar`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

// Make War.

export interface WarParticipantInfo {
	player_id: number;
	side: 1 | 2;
	joined_at_row: number;
	surrendered_at_row: number | null;
	entry_payment_complete: boolean;
}

export interface WarBattleCostInfo {
	id: number;
	row_number: number;
	payer_id: number;
	opponent_id: number;
	choice: string;
	asset_id_1: number | null;
	asset_id_2: number | null;
	surrendered: boolean;
	is_entry: boolean;
}

export interface WarOutstandingCost {
	payer_id: number;
	opponent_id: number;
}

export interface WarPeaceVote { player_id: number; accepted: boolean }

export interface WarPeaceProposalInfo {
	id: number;
	proposer_id: number;
	terms: string;
	status: 'open' | 'accepted' | 'rejected';
	votes: WarPeaceVote[];
	awaiting: number[];
}

export interface WarSurrenderClaimInfo {
	id: number;
	surrendered_id: number;
	claimant_id: number;
}

export interface WarStateResponse {
	war_id: number;
	origin_plan_id: number;
	status: 'active' | 'ended';
	started_at_row: number;
	ended_at_row: number | null;
	end_reason: string | null;
	current_row: number;
	participants: WarParticipantInfo[];
	battle_costs: WarBattleCostInfo[];
	/** Reverse-power-ordered (payer, opponent) pairs still owed *this* row. */
	outstanding_costs: WarOutstandingCost[];
	open_proposal: WarPeaceProposalInfo | null;
	open_claims: WarSurrenderClaimInfo[];
}

/** Make War — full war state for the panel (participants, costs, peace, claims). */
export function getWarState(planID: number): Promise<WarStateResponse> {
	return apiFetch(`/plans/${planID}/war-state`);
}

/** List all active wars in a game. Used by the turn indicator to flag rows
 * blocked on outstanding cost-of-battle payments. */
export function listWars(gameID: number | string): Promise<{ wars: WarStateResponse[] }> {
	return apiFetch(`/tables/${gameID}/wars`);
}

/** Make War — uninvited player joins a side. Free during the delay reveal;
 * after the war is active, the joiner owes a cost-of-battle entry to every
 * existing opposing participant before counting as fully joined. */
export function joinWar(planID: number, side: 1 | 2): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/join-war`, {
		method: 'POST',
		body: JSON.stringify({ side }),
	});
}

/** Make War — pay this row's cost of battle to one opponent.
 * `surrender:true` after the payment marks the payer surrendered. */
export function payBattleCost(
	planID: number,
	body: {
		opponent_id: number;
		choice: 'break_asset' | 'leverage_two';
		marginalia_id?: number;
		asset_id_1?: number;
		asset_id_2?: number;
		surrender?: boolean;
	}
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/pay-battle-cost`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Make War — late joiner pays cost of battle entry to one existing opponent. */
export function payWarEntry(
	planID: number,
	body: {
		opponent_id: number;
		choice: 'break_asset' | 'leverage_two';
		marginalia_id?: number;
		asset_id_1?: number;
		asset_id_2?: number;
	}
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/pay-war-entry`, {
		method: 'POST',
		body: JSON.stringify(body),
	});
}

/** Make War — claim one asset from a surrendered opposing participant. */
export function takeSurrenderAsset(
	planID: number,
	surrenderedID: number,
	assetID: number,
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/take-surrender-asset`, {
		method: 'POST',
		body: JSON.stringify({ surrendered_id: surrenderedID, asset_id: assetID }),
	});
}

/** Make War — propose peace terms. Only the active payer (whose turn it is to
 * pay cost) may propose; the proposer auto-votes accept. If the vote is not
 * unanimous they must still pay using break_asset/leverage_two. */
export function proposePeace(planID: number, terms: string): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/propose-peace`, {
		method: 'POST',
		body: JSON.stringify({ terms }),
	});
}

/** Make War — vote on an open peace proposal. A single 'reject' closes it;
 * unanimous accepts end the war. */
export function votePeace(
	planID: number,
	proposalID: number,
	accepted: boolean,
): Promise<PlanEcho> {
	return apiFetch(`/plans/${planID}/vote-peace`, {
		method: 'POST',
		body: JSON.stringify({ proposal_id: proposalID, accepted }),
	});
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

/**
 * Make Demands — control_leverage winner sets leverage on the *target plan*.
 * Mounted on the target plan, NOT the demand plan. Adds dice from the target
 * preparer's own assets onto the target plan's open roll.
 */
export function demandLeverage(targetPlanID: number, assetIDs: number[]): Promise<{
	plan_id: number;
	roll_id: number;
	asset_ids: number[];
}> {
	return apiFetch(`/plans/${targetPlanID}/demand-leverage`, {
		method: 'POST',
		body: JSON.stringify({ asset_ids: assetIDs }),
	});
}

/**
 * Make Demands — keep_or_change_target winner re-aims the *target plan*.
 * Mounted on the target plan, NOT the demand plan. Re-validates against the
 * target plan type's preparation rules before persisting.
 */
export function demandRetarget(
	targetPlanID: number,
	params: { target_player_id?: number | null; target_asset_id?: number | null },
): Promise<{
	plan_id: number;
	target_player_id: number | null;
	target_asset_id: number | null;
}> {
	return apiFetch(`/plans/${targetPlanID}/demand-retarget`, {
		method: 'POST',
		body: JSON.stringify(params),
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
