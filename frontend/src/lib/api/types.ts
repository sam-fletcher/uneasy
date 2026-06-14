// api/types.ts — shared type definitions for the typed API client.
// Split out of the former monolithic api.ts. Domain modules import these;
// the barrel (index.ts) re-exports everything for `$lib/api` consumers.

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
	// Total secrets on the asset (existence), public to every player. The
	// content stays gated; the viewer derives how many they can read from the
	// visible-secrets list and treats the remainder as hidden.
	secret_count: number;
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

/** A token placed on a plan's shield, slimmed to what the prep grid needs.
 *  One per (plan_type, player); cleared per-category at ranking updates. */
export interface PlanToken {
	plan_type: PlanType;
	player_id: number;
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
export type { SeekAnswersResolutionData } from '$lib/plans/resolutionData/seek_answers';
import type { SeekAnswersResolutionData } from '$lib/plans/resolutionData/seek_answers';

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

	// ── Seek Answers ──
	// Tracks flawed resources (once-per-asset) and the mar self-flaw penalty;
	// see $lib/plans/resolutionData/seek_answers.ts.
	seek_answers?: SeekAnswersResolutionData;

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
