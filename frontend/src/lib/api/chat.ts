import { apiFetch } from './client';
import type { ChatPost } from './types';

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
