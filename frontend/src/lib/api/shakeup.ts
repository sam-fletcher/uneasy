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
	// claim_title: the chosen title id + freeform marginalia flavor text.
	target_title_id: string | null;
	title_flavor: string | null;
	base_cost: number;
	final_cost: number | null;
	committed_at: string | null;
	applied: boolean;
	created_at: string;
}

// A title the "Claim a new title" picker may offer (not yet claimed game-wide).
export interface ClaimableTitle {
	id: string;
	name: string;
	description: string;
	in_succession: boolean;
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
	// Titles still unclaimed game-wide, for the "Claim a new title" picker.
	claimable_titles?: ClaimableTitle[];
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
		target_title_id?: string;
		title_flavor?: string;
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
