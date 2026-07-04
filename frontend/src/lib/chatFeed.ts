// chatFeed.ts — the client half of the Chat Overhaul (adr/CHAT_OVERHAUL_PLAN.md
// Phase 2). Owns the contiguous post window, live/history mode, pagination
// cursors, and the read marker; exposes pure helpers for rendering (day
// dividers, the "New messages" divider, unread counting) and orchestration
// functions that fetch/merge/report against the Phase 1 server endpoints.
//
// Follows the same shape as ws-handlers.ts: `ChatFeedContext` is an interface
// the caller (routes/table/[id]/+page.svelte) implements with get/set
// accessors backed by its own $state runes, so these plain functions stay
// unit-testable (construct a fake ctx, call, assert on its fields) while
// still driving real component reactivity in the app.

import { listGamePosts, getPostAnchor, updateReadMarker } from '$lib/api';
import type { ChatPost, AnchorRequest } from '$lib/api';
import { SEVERITY } from '$lib/severity';

export type FeedMode = 'live' | 'history';

/** Mutable feed state. `initialReadMarker` is a snapshot of the server's
 *  last_read_post_id taken when the window was (re)loaded from scratch —
 *  it anchors the "New messages" divider, which must stay put even as
 *  `lastReadPostID` advances while the player keeps reading. */
export interface ChatFeedContext {
	readonly gameID: string | number;
	posts: ChatPost[];
	mode: FeedMode;
	hasMoreBefore: boolean;
	hasMoreAfter: boolean;
	lastReadPostID: number;
	initialReadMarker: number;
	loadingOlder: boolean;
}

// A page fetched for scroll-up pagination. Smaller than the initial window
// since it's topping up a feed the player is already reading, not catching
// them up from cold — mirrors handler/posts.go's defaultPageLimit.
const PAGE_LIMIT = 50;

// ── Pure: unread rule ──────────────────────────────────────────────────────
// Shared by the badge and the divider (adr/CHAT_OVERHAUL_PLAN.md Phase 2a):
// a post counts as unread if it's newer than the marker, wasn't authored by
// the viewer, and is either a player message or a system post that cleared
// the "hide bookkeeping" bar. This is also mirrored server-side once
// profile-page badges land (Phase 6).

export function isUnreadPost(
	post: ChatPost,
	lastReadPostID: number,
	currentPlayerID: number | null
): boolean {
	if (post.id <= lastReadPostID) return false;
	if (post.author_id === currentPlayerID) return false;
	return post.author_id != null || post.severity >= SEVERITY.DEFAULT;
}

export function countUnread(
	posts: ChatPost[],
	lastReadPostID: number,
	currentPlayerID: number | null
): number {
	let n = 0;
	for (const post of posts) {
		if (isUnreadPost(post, lastReadPostID, currentPlayerID)) n++;
	}
	return n;
}

// ── Pure: system_code family (drives Phase 3's per-family log glyph and the
// ranking-burst grouping below). "plan.prepared" → "plan", "demand.resolved"
// → "demand" (Make Demands' sub-events are plan chatter), etc. Null for
// player messages (system_code is null) or a code with no dot.

export function systemCodeFamily(code: string | null): string | null {
	if (!code) return null;
	const dot = code.indexOf('.');
	return dot === -1 ? code : code.slice(0, dot);
}

// ── Pure: feed items (day dividers + "New messages" divider) ─────────────
// Scene grouping (Phase 4) extends this further. Phase 3 adds:
//   - `continuesRun` on 'post'/'ranking-group' items: true when the
//     immediately preceding item is also a system post with nothing (day/
//     unread divider) between them, so the renderer can tighten the gap and
//     read bookkeeping as a compact ledger vs. player-message prose.
//   - the 'ranking-group' kind: a maximal run of consecutive `ranking.*`
//     posts (one EmitRankingUpdated burst) collapses into a single unit so
//     it renders as one bordered card instead of a centered/left zigzag.

export type FeedItem =
	| { kind: 'day-divider'; key: string; label: string }
	| { kind: 'unread-divider'; key: string }
	| { kind: 'post'; key: string; post: ChatPost; continuesRun: boolean }
	| { kind: 'ranking-group'; key: string; posts: ChatPost[]; continuesRun: boolean };

function dayKey(iso: string): string {
	const d = new Date(iso);
	return `${d.getFullYear()}-${d.getMonth()}-${d.getDate()}`;
}

function formatDayLabel(iso: string, now: Date): string {
	const d = new Date(iso);
	const yesterday = new Date(now.getFullYear(), now.getMonth(), now.getDate() - 1);
	if (dayKey(iso) === dayKey(now.toISOString())) return 'Today';
	if (dayKey(iso) === dayKey(yesterday.toISOString())) return 'Yesterday';
	const opts: Intl.DateTimeFormatOptions = { weekday: 'long', month: 'long', day: 'numeric' };
	if (d.getFullYear() !== now.getFullYear()) opts.year = 'numeric';
	return d.toLocaleDateString(undefined, opts);
}

/**
 * Builds the render list: a day divider whenever the calendar day changes,
 * and one "New messages" divider right before the first unread post (per
 * `opts.unreadAfterID` — pass a frozen snapshot, not the live marker, so the
 * divider doesn't slide out from under a player who's still reading).
 * `posts` must already be chronological (oldest → newest), which every
 * listGamePosts mode guarantees.
 */
export function buildFeedItems(
	posts: ChatPost[],
	opts: { unreadAfterID: number; currentPlayerID: number | null; now?: Date }
): FeedItem[] {
	const now = opts.now ?? new Date();
	const items: FeedItem[] = [];
	let lastDayKey: string | null = null;
	let placedDivider = false;
	const hasUnread = posts.some((p) => isUnreadPost(p, opts.unreadAfterID, opts.currentPlayerID));

	// Collapse consecutive ranking.* posts (one EmitRankingUpdated burst) into
	// a single unit so the ranking-group case below renders them as one card
	// instead of a run of separate log lines.
	const units: ChatPost[][] = [];
	for (const post of posts) {
		const prevUnit = units[units.length - 1];
		if (
			systemCodeFamily(post.system_code) === 'ranking' &&
			prevUnit != null &&
			systemCodeFamily(prevUnit[0].system_code) === 'ranking'
		) {
			prevUnit.push(post);
		} else {
			units.push([post]);
		}
	}

	let prevWasSystemPost = false;
	for (const unit of units) {
		const first = unit[0];
		const key = dayKey(first.created_at);
		if (key !== lastDayKey) {
			items.push({ kind: 'day-divider', key: `day-${key}`, label: formatDayLabel(first.created_at, now) });
			lastDayKey = key;
			prevWasSystemPost = false;
		}
		if (hasUnread && !placedDivider && unit.some((p) => isUnreadPost(p, opts.unreadAfterID, opts.currentPlayerID))) {
			items.push({ kind: 'unread-divider', key: 'unread-divider' });
			placedDivider = true;
			prevWasSystemPost = false;
		}

		const isSystemUnit = first.author_id == null;
		const continuesRun = isSystemUnit && prevWasSystemPost;

		if (unit.length > 1 || systemCodeFamily(first.system_code) === 'ranking') {
			items.push({ kind: 'ranking-group', key: `ranking-${first.id}`, posts: unit, continuesRun });
		} else {
			items.push({ kind: 'post', key: `post-${first.id}`, post: first, continuesRun });
		}
		prevWasSystemPost = isSystemUnit;
	}
	return items;
}

// ── Pure: window merge (dedup by id; WS delivery + resync overlap) ────────

export function mergeAppend(existing: ChatPost[], incoming: ChatPost[]): ChatPost[] {
	if (incoming.length === 0) return existing;
	const seen = new Set(existing.map((p) => p.id));
	const fresh = incoming.filter((p) => !seen.has(p.id));
	if (fresh.length === 0) return existing;
	return [...existing, ...fresh];
}

export function mergePrepend(existing: ChatPost[], older: ChatPost[]): ChatPost[] {
	if (older.length === 0) return existing;
	const seen = new Set(existing.map((p) => p.id));
	const fresh = older.filter((p) => !seen.has(p.id));
	if (fresh.length === 0) return existing;
	return [...fresh, ...existing];
}

// ── Pure: scroll geometry ──────────────────────────────────────────────────

export function isNearBottom(scrollTop: number, scrollHeight: number, clientHeight: number, threshold = 150): boolean {
	return scrollHeight - scrollTop - clientHeight <= threshold;
}

// ── Orchestration ──────────────────────────────────────────────────────────

/** The server's initial catch-up window. Enters (or re-enters) live mode. */
export async function loadInitialWindow(ctx: ChatFeedContext): Promise<void> {
	const result = await listGamePosts(ctx.gameID);
	ctx.posts = result.posts;
	ctx.hasMoreBefore = result.has_more_before;
	ctx.hasMoreAfter = false;
	ctx.lastReadPostID = result.last_read_post_id;
	ctx.initialReadMarker = result.last_read_post_id;
	ctx.mode = 'live';
}

/** Scroll-up pagination: fetches one older page and prepends it. */
export async function fetchOlderPage(ctx: ChatFeedContext): Promise<void> {
	if (ctx.loadingOlder || !ctx.hasMoreBefore || ctx.posts.length === 0) return;
	ctx.loadingOlder = true;
	try {
		const oldestID = ctx.posts[0].id;
		const result = await listGamePosts(ctx.gameID, { beforeID: oldestID, limit: PAGE_LIMIT });
		ctx.posts = mergePrepend(ctx.posts, result.posts);
		ctx.hasMoreBefore = result.has_more_before;
	} finally {
		ctx.loadingOlder = false;
	}
}

/**
 * Appends a live WS post. No-op in history mode: the window there is a
 * fixed historical slice, and a freshly-arrived post would be discontiguous
 * with it (adr/CHAT_OVERHAUL_PLAN.md Phase 2b). The player picks it up via
 * "Return to now".
 */
export function appendLivePost(ctx: ChatFeedContext, post: ChatPost): void {
	if (ctx.mode !== 'live') return;
	ctx.posts = mergeAppend(ctx.posts, [post]);
}

/**
 * Reconnect resync (Phase 2b). Replaces the old "refetch the whole feed on
 * every reconnect" — in live mode, fetches only what's newer than the last
 * loaded post; in history mode, does nothing (the window is intentionally
 * not "now"; Return to now handles catching up). Safe on every (re)connect,
 * including the first one — an empty window falls back to the initial load.
 */
export async function reconnectResync(ctx: ChatFeedContext): Promise<void> {
	if (ctx.mode === 'history') return;
	if (ctx.posts.length === 0) {
		await loadInitialWindow(ctx);
		return;
	}
	const newestID = ctx.posts[ctx.posts.length - 1].id;
	const result = await listGamePosts(ctx.gameID, { afterID: newestID });
	ctx.posts = mergeAppend(ctx.posts, result.posts);
	ctx.lastReadPostID = Math.max(ctx.lastReadPostID, result.last_read_post_id);
}

/** Enters history mode with a window centred on `anchorPostID`. */
export async function enterHistoryMode(ctx: ChatFeedContext, anchorPostID: number): Promise<void> {
	const result = await listGamePosts(ctx.gameID, { aroundID: anchorPostID });
	ctx.posts = result.posts;
	ctx.hasMoreBefore = result.has_more_before;
	ctx.hasMoreAfter = result.has_more_after;
	ctx.mode = 'history';
}

/** "Return to now": refetches the initial window and re-enters live mode. */
export async function returnToNow(ctx: ChatFeedContext): Promise<void> {
	await loadInitialWindow(ctx);
}

function matchesAnchor(post: ChatPost, req: AnchorRequest): boolean {
	if (post.system_code !== req.code) return false;
	if ('row' in req) return post.row_number === req.row;
	if ('planID' in req) return post.plan_id === req.planID;
	return post.scene_id === req.sceneID;
}

/**
 * Resolves a Public Record jump gesture to a post id (Phase 2e): checks the
 * loaded window first (cheap fast path), then falls back to the Phase 1d
 * anchor endpoint. Returns null if nothing anchors the request at all (the
 * caller decides what "no anchor" means — e.g. row 1 has no row.advanced
 * post, so jumpToRow(1) falls back to the very first post instead).
 */
export async function resolveAnchor(
	ctx: ChatFeedContext,
	req: AnchorRequest
): Promise<{ postID: number; inWindow: boolean } | null> {
	const match = ctx.posts.find((p) => matchesAnchor(p, req));
	if (match) return { postID: match.id, inWindow: true };
	try {
		const { post_id } = await getPostAnchor(ctx.gameID, req);
		return { postID: post_id, inWindow: false };
	} catch {
		return null;
	}
}

/**
 * Reports the read marker to the server (Phase 2d). Only meaningful in live
 * mode — a history window can be discontiguous with the true unread span,
 * so reporting its newest id there could mark posts the player never saw as
 * read (the plan's "conservative by design" principle). Callers debounce
 * and gate this on panel-visible + document-visible + scrolled-near-bottom.
 */
export async function reportReadMarker(ctx: ChatFeedContext): Promise<void> {
	if (ctx.mode !== 'live' || ctx.posts.length === 0) return;
	const newest = ctx.posts[ctx.posts.length - 1].id;
	if (newest <= ctx.lastReadPostID) return;
	const { last_read_post_id } = await updateReadMarker(ctx.gameID, newest);
	ctx.lastReadPostID = last_read_post_id;
}
