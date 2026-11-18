// seenMarkers.ts — keeping a player's "new to me" markers in step across devices.
//
// The server keeps three per-player markers that drive the pale dots: the
// highest law rev seen, the highest rumor rev seen, and the newest chat post
// read. Each device holds its own copy, set when the table loads and moved when
// the player reads something there. Reading on another device moves the server's
// copy, and nothing pushes that here (it is private to the player, so it is not
// a table-wide event), so a tab that stays open keeps a dot the player already
// cleared elsewhere. The page re-reads the three markers when the player comes
// back to it (see ConnectionOptions.onForeground in ws.ts) and folds the reply
// in with this.
//
// Pure: no Svelte, no fetches.

import type { SeenMarkersReply } from '$lib/api';

/** A page's copy of the markers. `laws` and `rumors` are null until the first
 *  load has said (so nothing lights up for a marker nobody has read yet). */
export interface SeenMarkers {
	laws: number | null;
	rumors: number | null;
	chat: number;
}

/** Folds a fresh read into what the page holds. Each marker only ever moves
 *  forward, as on the server: a reply that was in flight while the player read
 *  something here, or one that arrives after a later reply, is older than the
 *  page's copy and must not relight a dot. Returns `current` itself when nothing
 *  moved, so the caller can skip the writes (and the re-render they would cause). */
export function mergeSeenMarkers(current: SeenMarkers, fresh: SeenMarkersReply): SeenMarkers {
	const laws = Math.max(current.laws ?? 0, fresh.laws);
	const rumors = Math.max(current.rumors ?? 0, fresh.rumors);
	const chat = Math.max(current.chat, fresh.last_read_post_id);
	if (laws === current.laws && rumors === current.rumors && chat === current.chat) return current;
	return { laws, rumors, chat };
}

/** What the page knows about its own dots and markers, for deciding whether a
 *  re-read is worth a request. */
export interface DotState {
	lawsNew: boolean;
	rumorsNew: boolean;
	unreadChat: boolean;
	/** The page's Laws and Rumors markers; null until the first load has said. */
	laws: number | null;
	rumors: number | null;
}

/** Whether asking the server could change anything on screen. A re-read only
 *  ever moves markers forward, and a dot is "something above my marker", so it
 *  can only clear dots: with none lit there is nothing to clear and the request
 *  is skipped. That makes most returns to a phone free, as the page's activity
 *  ping is for the same reason. The exception is a Laws or Rumors marker that
 *  hasn't loaded yet (null), which only a read can fill. */
export function rereadCouldClearADot(s: DotState): boolean {
	return s.lawsNew || s.rumorsNew || s.unreadChat || s.laws === null || s.rumors === null;
}

/** Where chat's "N new" divider should sit after a re-read.
 *
 *  The divider is frozen where the window loaded, so that it holds still under a
 *  reader working through the unread run. It moves with the dot only when the
 *  news is that chat's own marker moved on another device, and Chat is off
 *  screen (so nobody is parked on it). It compares the server's marker with this
 *  device's own chat marker, not with the merged one: this device's reading is
 *  already in its marker, so a merged comparison would swallow a divider this
 *  device left behind whenever only Laws or Rumors moved elsewhere. Never moves
 *  back. */
export function dividerMarkerAfterReread(
	now: { divider: number; chat: number; chatOnScreen: boolean },
	fresh: SeenMarkersReply,
): number {
	if (now.chatOnScreen) return now.divider;
	if (fresh.last_read_post_id <= now.chat) return now.divider;
	return Math.max(now.divider, fresh.last_read_post_id);
}
