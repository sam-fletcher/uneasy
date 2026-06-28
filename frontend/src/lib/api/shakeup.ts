import { apiFetch } from './client';

export type ShakeUpCategory = 'esteem' | 'knowledge' | 'power';

export interface ShakeUpOptionInfo {
	Key: string;
	Category: ShakeUpCategory;
	Description: string;
	NeedsAsset: boolean;
	// Break options also require a marginalia choice: breaking = tearing one
	// marginalia off the target asset.
	NeedsMarginalia: boolean;
	BumpsTrack: string;
}

export interface ShakeUpSpend {
	id: number;
	game_id: number;
	player_id: number;
	category: ShakeUpCategory;
	option_key: string;
	target_asset_id: number | null;
	target_marginalia_id: number | null;
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
	// During the spending step (no open spend), the player whose turn it is
	// to announce, per reverse-rank order. Absent otherwise.
	current_actor?: number;
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
	body: {
		option_key: string;
		target_asset_id?: number;
		target_marginalia_id?: number;
		target_player_id?: number;
	}
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
