import { apiFetch } from './client';
import type { GamePhase } from './types';

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
