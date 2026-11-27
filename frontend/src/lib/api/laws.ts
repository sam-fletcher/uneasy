import { apiFetch } from './client';
import type { Law, LawsRumorsSeen, Rumor, Treaty } from './types';

export function listLaws(gameID: number): Promise<{ laws: Law[]; treaties: Treaty[]; seen_rev: number }> {
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

export function listRumors(gameID: number): Promise<{ rumors: Rumor[]; seen_rev: number }> {
	return apiFetch(`/tables/${gameID}/rumors`);
}

export function updateRumor(rumorID: number, text: string): Promise<{ rumor: Rumor }> {
	return apiFetch(`/rumors/${rumorID}`, {
		method: 'PATCH',
		body: JSON.stringify({ text }),
	});
}

/** Records that the player has seen laws up to `seenRev` (the highest `rev` the
 *  client has shown them; see $lib/lawsRumorsSeen). Monotonic and private, like
 *  updateReadMarker for chat. Returns the stored value, which may be higher than
 *  asked if another device already went further, or lower if `seenRev` was past
 *  the newest law. */
export function markLawsSeen(gameID: string | number, seenRev: number): Promise<{ seen_rev: number }> {
	return apiFetch(`/tables/${gameID}/seen/laws`, {
		method: 'PUT',
		body: JSON.stringify({ seen_rev: seenRev }),
	});
}

/** markLawsSeen for rumors. */
export function markRumorsSeen(gameID: string | number, seenRev: number): Promise<{ seen_rev: number }> {
	return apiFetch(`/tables/${gameID}/seen/rumors`, {
		method: 'PUT',
		body: JSON.stringify({ seen_rev: seenRev }),
	});
}

/** The reply of getSeenMarkers: the Laws and Rumors markers as `GET /state`
 *  carries them, and chat's as `GET /posts` does. */
export type SeenMarkersReply = LawsRumorsSeen & { last_read_post_id: number };

/** The viewer's three "new to me" markers in one small read: the highest law rev
 *  and rumor rev they have seen, and the newest chat post they have read. This is
 *  what a tab asks when the player comes back to it, to learn what they read on
 *  another device (see $lib/seenMarkers). It is the same data `GET /state` and
 *  `GET /posts` carry, without the board or the feed around it. Always the
 *  caller's own markers; nothing here names another player. */
export function getSeenMarkers(gameID: string | number): Promise<SeenMarkersReply> {
	return apiFetch(`/tables/${gameID}/seen`);
}
