import { apiFetch } from './client';
import type {
	Game, Player, GamePhase, ToneTopic, ToneTopicStatus, Ranking, Law, Rumor,
} from './types';

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
	| 'await_take_consent'
	| 'await_question_answer'
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

