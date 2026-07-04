import { apiFetch } from './client';
import type { ChatPost, PostWindow } from './types';

/**
 * GET /tables/{id}/posts. Four modes (Chat Overhaul Phase 1b) — pass at
 * most one of afterID/beforeID/aroundID:
 *
 * - (none): the server's initial catch-up window.
 * - afterID: everything newer — reconnect resync / live catch-up.
 * - beforeID (+limit): a page of older posts, for scroll-up pagination.
 * - aroundID (+limit): a window centred on a post, for jump-to-anchor.
 */
export function listGamePosts(
	gameID: string | number,
	opts?: { afterID?: number; beforeID?: number; aroundID?: number; limit?: number }
): Promise<PostWindow> {
	const params = new URLSearchParams();
	if (opts?.afterID != null) params.set('after', String(opts.afterID));
	else if (opts?.beforeID != null) params.set('before', String(opts.beforeID));
	else if (opts?.aroundID != null) params.set('around', String(opts.aroundID));
	if (opts?.limit != null) params.set('limit', String(opts.limit));
	const qs = params.toString();
	return apiFetch(`/tables/${gameID}/posts${qs ? `?${qs}` : ''}`);
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

/** Monotonic, private read marker (Chat Overhaul Phase 1c). Returns the
 *  stored value, which may be higher than `lastReadPostID` if another
 *  device already advanced it further. */
export function updateReadMarker(
	gameID: string | number,
	lastReadPostID: number
): Promise<{ last_read_post_id: number }> {
	return apiFetch(`/tables/${gameID}/read-marker`, {
		method: 'PUT',
		body: JSON.stringify({ last_read_post_id: lastReadPostID })
	});
}

/** One of row/planID/sceneID, alongside the system_code that anchors the
 *  jump gesture (see PublicRecord.svelte's onRowJump/onPlanJump/onSceneJump). */
export type AnchorRequest =
	| { code: string; row: number }
	| { code: string; planID: number }
	| { code: string; sceneID: number };

/** GET /tables/{id}/posts/anchor (Chat Overhaul Phase 1d). Resolves a
 *  Public Record jump gesture to the post id that anchors it, for a
 *  window that doesn't have it loaded. Throws (404) if no matching post
 *  exists — callers should catch and treat that as "nothing to jump to". */
export function getPostAnchor(
	gameID: string | number,
	req: AnchorRequest
): Promise<{ post_id: number }> {
	const params = new URLSearchParams({ code: req.code });
	if ('row' in req) params.set('row', String(req.row));
	else if ('planID' in req) params.set('plan_id', String(req.planID));
	else params.set('scene_id', String(req.sceneID));
	return apiFetch(`/tables/${gameID}/posts/anchor?${params}`);
}
