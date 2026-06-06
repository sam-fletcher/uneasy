import { apiFetch } from './client';
import type { DiceRoll, DiceRollDie, RollIntent, RollParticipant, VoteView } from './types';

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
