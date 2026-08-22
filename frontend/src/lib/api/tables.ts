import { apiFetch } from './client';
import type {
	Game, Player, GamePhase, ToneTopic, ToneTopicStatus, Ranking, Law, Rumor,
	PlayerActivity,
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
	| 'await_dice_roll'
	| 'await_endgame_vote'
	| 'await_surrender_claim'
	| 'await_battle_cost'
	| 'await_main_character_choice'
	| 'await_delay_reveal'
	| 'plan_resolving'
	| 'plan_pending'
	| 'await_demand_counter'
	| 'await_demand_draft_pick'
	| 'await_demand_leverage'
	| 'await_demand_retarget'
	| 'await_festivity_guest_turn'
	| 'await_festivity_challenge_response'
	| 'await_duel_staking'
	| 'await_duel_bout'
	| 'await_take_consent'
	| 'await_question_answer'
	| 'liaise_resolving'
	| 'await_courtier_response'
	| 'await_introductions_marginalia'
	| 'await_chronicle_choices'
	| 'finale_row_complete'
	| 'scene_active'
	| 'post_scene_action'
	| 'scene_setting';

export interface RowState {
	kind: RowStateKind;
	plan_id?: number | null;
	scene_id?: number | null;
	war_id?: number | null;
	claim_id?: number | null;
	roll_id?: number | null;
	/** The full, server-authoritative set of players whose action the row is
	 *  blocked on — for every actor-naming kind, including the generic
	 *  plan_resolving / plan_pending case (the resolving plan's preparer) and the
	 *  single- and multi-actor sub-phase gates. Read directly; there is no
	 *  client-side preparer/focus proxy. Absent only for kinds with no actor. */
	acting_player_ids?: number[] | null;
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
	/** Presence + reminder summary per seat. Best-effort server-side, so
	 *  absent if that one query failed. */
	player_activity?: PlayerActivity[];
}> {
	return apiFetch(`/tables/${id}/state`);
}

/** Records that this player has the table on screen. Fire-and-forget: called
 *  on mount and whenever the tab becomes visible again, throttled client-side
 *  (see the table page) and again server-side to at most one write an hour. */
export function touchActivity(gameID: string | number): Promise<void> {
	return apiFetch(`/tables/${gameID}/activity`, { method: 'POST' });
}

/** Silences (or restores) turn reminders for the wait this player is currently
 *  blocking at this table — the profile card's bell.
 *
 *  Returns the state that actually holds, which can differ from the one asked
 *  for: with no reminder pending (the table moved on since the card was drawn)
 *  there is nothing to silence, and the server says so rather than claiming a
 *  quiet it isn't keeping. */
export function setReminderMute(
	gameID: string | number,
	muted: boolean
): Promise<{ muted: boolean }> {
	return apiFetch(`/tables/${gameID}/reminder-mute`, {
		method: 'POST',
		body: JSON.stringify({ muted }),
	});
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

