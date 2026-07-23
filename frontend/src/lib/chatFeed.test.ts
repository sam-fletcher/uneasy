import { describe, it, expect, vi, beforeEach } from 'vitest';
import type { ChatPost } from '$lib/api';
import { SEVERITY } from '$lib/severity';
import {
	isUnreadPost,
	countUnread,
	buildFeedItems,
	systemCodeFamily,
	markForCode,
	planOutcomeOf,
	parseSceneStartedData,
	mergeAppend,
	mergePrepend,
	isNearBottom,
	loadInitialWindow,
	fetchOlderPage,
	appendLivePost,
	reconnectResync,
	enterHistoryMode,
	returnToNow,
	resolveAnchor,
	reportReadMarker,
	type ChatFeedContext,
} from './chatFeed';

vi.mock('$lib/api', () => ({
	listGamePosts: vi.fn(),
	getPostAnchor: vi.fn(),
	updateReadMarker: vi.fn(),
}));

import { listGamePosts, getPostAnchor, updateReadMarker } from '$lib/api';

function makePost(over: Partial<ChatPost> = {}): ChatPost {
	return {
		id: 1,
		game_id: 1,
		row_number: null,
		plan_id: null,
		scene_id: null,
		author_id: null,
		body: 'x',
		created_at: '2026-07-01T10:00:00.000Z',
		severity: 0,
		system_code: null,
		system_data: null,
		speaking_as_asset_id: null,
		...over,
	};
}

function makeCtx(over: Partial<ChatFeedContext> = {}): ChatFeedContext {
	return {
		gameID: 1,
		posts: [],
		mode: 'live',
		hasMoreBefore: false,
		hasMoreAfter: false,
		lastReadPostID: 0,
		initialReadMarker: 0,
		loadingOlder: false,
		...over,
	};
}

beforeEach(() => {
	vi.mocked(listGamePosts).mockReset();
	vi.mocked(getPostAnchor).mockReset();
	vi.mocked(updateReadMarker).mockReset();
});

describe('isUnreadPost / countUnread', () => {
	it('counts a player message from someone else as unread', () => {
		const post = makePost({ id: 5, author_id: 2 });
		expect(isUnreadPost(post, 4, 1)).toBe(true);
	});

	it('never counts the viewer\'s own post as unread', () => {
		const post = makePost({ id: 5, author_id: 1 });
		expect(isUnreadPost(post, 4, 1)).toBe(false);
	});

	it('excludes posts at or before the marker', () => {
		expect(isUnreadPost(makePost({ id: 4, author_id: 2 }), 4, 1)).toBe(false);
		expect(isUnreadPost(makePost({ id: 3, author_id: 2 }), 4, 1)).toBe(false);
	});

	it('hides bookkeeping-tier system posts (below Default) from the unread rule', () => {
		const minor = makePost({ id: 5, author_id: null, severity: SEVERITY.MINOR });
		expect(isUnreadPost(minor, 4, 1)).toBe(false);
	});

	it('counts Default-and-up system posts as unread', () => {
		const boundary = makePost({ id: 5, author_id: null, severity: SEVERITY.DEFAULT });
		expect(isUnreadPost(boundary, 4, 1)).toBe(true);
	});

	it('countUnread sums the rule across the window', () => {
		const posts = [
			makePost({ id: 5, author_id: 2 }),
			makePost({ id: 6, author_id: 1 }), // own post — excluded
			makePost({ id: 7, author_id: null, severity: SEVERITY.MINOR }), // bookkeeping — excluded
			makePost({ id: 8, author_id: null, severity: SEVERITY.IMPORTANT }),
		];
		expect(countUnread(posts, 4, 1)).toBe(2);
	});
});

describe('buildFeedItems', () => {
	it('inserts a day divider once per calendar day', () => {
		const posts = [
			makePost({ id: 1, created_at: '2026-07-01T09:00:00.000Z' }),
			makePost({ id: 2, created_at: '2026-07-01T10:00:00.000Z' }),
			makePost({ id: 3, created_at: '2026-07-02T09:00:00.000Z' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const dividers = items.filter((i) => i.kind === 'day-divider');
		expect(dividers).toHaveLength(2);
		expect(items.map((i) => i.kind)).toEqual([
			'day-divider', 'post', 'post', 'day-divider', 'post',
		]);
	});

	it('places the unread divider right before the first unread post', () => {
		const posts = [
			makePost({ id: 1, author_id: 2 }),
			makePost({ id: 2, author_id: 2 }),
			makePost({ id: 3, author_id: 2 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 1, currentPlayerID: 1 });
		const idx = items.findIndex((i) => i.kind === 'unread-divider');
		expect(idx).toBeGreaterThan(-1);
		const next = items[idx + 1];
		expect(next.kind).toBe('post');
		expect(next.kind === 'post' && next.post.id).toBe(2);
		// Exactly one divider even though two posts qualify as unread.
		expect(items.filter((i) => i.kind === 'unread-divider')).toHaveLength(1);
	});

	it('omits the unread divider entirely when nothing is unread', () => {
		const posts = [makePost({ id: 1, author_id: 1 }), makePost({ id: 2, author_id: 1 })];
		const items = buildFeedItems(posts, { unreadAfterID: 0, currentPlayerID: 1 });
		expect(items.some((i) => i.kind === 'unread-divider')).toBe(false);
	});

	it('labels today and yesterday relative to the supplied `now`', () => {
		const now = new Date('2026-07-03T12:00:00.000Z');
		const posts = [
			makePost({ id: 1, created_at: '2026-07-02T09:00:00.000Z' }),
			makePost({ id: 2, created_at: '2026-07-03T09:00:00.000Z' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1, now });
		const labels = items.filter((i) => i.kind === 'day-divider').map((i) => i.kind === 'day-divider' && i.label);
		expect(labels).toEqual(['Yesterday', 'Today']);
	});

	it('marks a system post as continuing a run only when the previous item was also a system post', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'asset.created' }),
			makePost({ id: 2, author_id: null, system_code: 'marginalia.added' }),
			makePost({ id: 3, author_id: 1 }), // player message breaks the run
			makePost({ id: 4, author_id: null, system_code: 'asset.renamed' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const postItems = items.filter((i) => i.kind === 'post');
		expect(postItems.map((i) => i.kind === 'post' && i.continuesRun)).toEqual([false, true, false, false]);
	});

	it('does not carry a run across a day divider', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'asset.created', created_at: '2026-07-01T09:00:00.000Z' }),
			makePost({ id: 2, author_id: null, system_code: 'asset.renamed', created_at: '2026-07-02T09:00:00.000Z' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const postItems = items.filter((i) => i.kind === 'post');
		expect(postItems.map((i) => i.kind === 'post' && i.continuesRun)).toEqual([false, false]);
	});

	it('does not carry a run across the unread divider', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'asset.created', severity: SEVERITY.DEFAULT }),
			makePost({ id: 2, author_id: null, system_code: 'marginalia.added', severity: SEVERITY.DEFAULT }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 1, currentPlayerID: 1 });
		const postItems = items.filter((i) => i.kind === 'post');
		expect(postItems.map((i) => i.kind === 'post' && i.continuesRun)).toEqual([false, false]);
	});

	it('collapses a consecutive run of ranking.* posts into one ranking-group item', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'ranking.updated' }),
			makePost({ id: 2, author_id: null, system_code: 'ranking.category' }),
			makePost({ id: 3, author_id: null, system_code: 'ranking.plan' }),
			makePost({ id: 4, author_id: null, system_code: 'ranking.standing' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'ranking-group']);
		const group = items.find((i) => i.kind === 'ranking-group');
		expect(group?.kind === 'ranking-group' && group.posts.map((p) => p.id)).toEqual([1, 2, 3, 4]);
	});

	it('splits ranking groups when a non-ranking post interrupts the burst', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'ranking.updated' }),
			makePost({ id: 2, author_id: null, system_code: 'ranking.category' }),
			makePost({ id: 3, author_id: 1 }), // an in-between player message
			makePost({ id: 4, author_id: null, system_code: 'ranking.plan' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'ranking-group', 'post', 'ranking-group']);
	});

	it('treats a lone ranking.* post as a one-post ranking-group', () => {
		const posts = [makePost({ id: 1, author_id: null, system_code: 'ranking.updated' })];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'ranking-group']);
	});

	it('a ranking-group counts as a system post for run-continuation on both sides', () => {
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'asset.created' }),
			makePost({ id: 2, author_id: null, system_code: 'ranking.updated' }),
			makePost({ id: 3, author_id: null, system_code: 'ranking.category' }),
			makePost({ id: 4, author_id: null, system_code: 'asset.renamed' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'post', 'ranking-group', 'post']);
		const [, post1, group, post4] = items;
		expect(post1.kind === 'post' && post1.continuesRun).toBe(false);
		expect(group.kind === 'ranking-group' && group.continuesRun).toBe(true);
		expect(post4.kind === 'post' && post4.continuesRun).toBe(true);
	});
});

describe('buildFeedItems — scene grouping (Phase 4b)', () => {
	function sceneStarted(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			id: 1,
			author_id: null,
			system_code: 'scene.started',
			scene_id: 7,
			severity: SEVERITY.IMPORTANT,
			body: 'Scene: Aldric at The Mill, Days later',
			system_data: {
				scene_id: 7,
				kind: 'turn',
				focus_player_id: 3,
				location: 'The Mill',
				time_label: 'Days later',
				prompt: 'What do you do?',
				participants: ['Aldric', 'Lady Wren'],
			},
			...over,
		});
	}
	function sceneEnded(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			author_id: null,
			system_code: 'scene.ended',
			scene_id: 7,
			severity: SEVERITY.IMPORTANT,
			body: 'Sam ends the scene',
			...over,
		});
	}

	it('wraps everything between scene.started and scene.ended in one closed group', () => {
		const posts = [
			sceneStarted({ id: 1 }),
			makePost({ id: 2, author_id: 2, scene_id: 7, body: 'hello' }),
			makePost({ id: 3, author_id: 2, scene_id: 7, speaking_as_asset_id: 9, body: 'in character' }),
			sceneEnded({ id: 4 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'scene-group']);
		const group = items[1];
		if (group.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.sceneID).toBe(7);
		expect(group.startPost?.id).toBe(1);
		expect(group.endPost?.id).toBe(4);
		expect(group.open).toBe(false);
		// The two inner posts, not the start/end boundary markers.
		expect(group.messageCount).toBe(2);
		expect(group.items.map((i) => i.kind)).toEqual(['post', 'post', 'post']);
	});

	it('renders open when the scene has not ended yet', () => {
		const posts = [sceneStarted({ id: 1 }), makePost({ id: 2, author_id: 2, scene_id: 7 })];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		expect(group?.kind === 'scene-group' && group.open).toBe(true);
		expect(group?.kind === 'scene-group' && group.endPost).toBeNull();
	});

	it('renders open when the loaded window is truncated before the scene ends', () => {
		const posts = [
			sceneStarted({ id: 1 }),
			makePost({ id: 2, author_id: 2, scene_id: 7 }),
			makePost({ id: 3, author_id: 2, scene_id: 7 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		expect(group?.kind === 'scene-group' && group.open).toBe(true);
	});

	it('infers a headerless open group when the window starts mid-scene (front truncation)', () => {
		// An `around` fetch can land inside a scene without ever loading its
		// scene.started post — only the scene_id stamp on later posts marks it.
		const posts = [
			makePost({ id: 50, author_id: 2, scene_id: 7, body: 'already mid-scene' }),
			makePost({ id: 51, author_id: 3, scene_id: 7 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['scene-group']);
		const group = items[0];
		if (group.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.sceneID).toBe(7);
		expect(group.startPost).toBeNull();
		expect(group.open).toBe(true);
		expect(group.items.map((i) => i.kind)).toEqual(['day-divider', 'post', 'post']);
	});

	it('closes a front-truncated group once its scene.ended is reached', () => {
		const posts = [makePost({ id: 50, author_id: 2, scene_id: 7 }), sceneEnded({ id: 51 })];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		expect(group?.kind === 'scene-group' && group.open).toBe(false);
		expect(group?.kind === 'scene-group' && group.endPost?.id).toBe(51);
	});

	it('keeps table-talk and in-character posts in one chronology inside the group', () => {
		const posts = [
			sceneStarted({ id: 1 }),
			makePost({ id: 2, author_id: 2, scene_id: 7, speaking_as_asset_id: null }), // table-talk
			makePost({ id: 3, author_id: 2, scene_id: 7, speaking_as_asset_id: 9 }), // in-character
			sceneEnded({ id: 4 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		if (group?.kind !== 'scene-group') throw new Error('expected a scene-group');
		const innerPostIDs = group.items
			.filter((i) => i.kind === 'post')
			.map((i) => i.kind === 'post' && i.post.id);
		expect(innerPostIDs).toEqual([2, 3, 4]);
	});

	it('counts unread posts inside the scene and flags the unread-divider position', () => {
		const posts = [
			sceneStarted({ id: 1 }),
			makePost({ id: 2, author_id: 2, scene_id: 7 }), // read
			makePost({ id: 3, author_id: 2, scene_id: 7 }), // unread
			makePost({ id: 4, author_id: 2, scene_id: 7 }), // unread
		];
		const items = buildFeedItems(posts, { unreadAfterID: 2, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		if (group?.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.unreadCount).toBe(2);
		expect(group.unreadDividerInside).toBe(true);
		expect(group.items.some((i) => i.kind === 'unread-divider')).toBe(true);
	});

	it('does not flag unreadDividerInside when the divider falls before the scene starts', () => {
		const posts = [
			makePost({ id: 1, author_id: 2 }), // unread, before the scene, outside any group
			sceneStarted({ id: 2 }),
			makePost({ id: 3, author_id: 2, scene_id: 7 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 0, currentPlayerID: 1 });
		expect(items.some((i) => i.kind === 'unread-divider')).toBe(true);
		const group = items.find((i) => i.kind === 'scene-group');
		expect(group?.kind === 'scene-group' && group.unreadDividerInside).toBe(false);
	});

	it('excludes the boundary markers themselves from messageCount', () => {
		const posts = [sceneStarted({ id: 1 }), makePost({ id: 2, author_id: 2, scene_id: 7 }), sceneEnded({ id: 3 })];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		expect(group?.kind === 'scene-group' && group.messageCount).toBe(1);
	});

	it('leaves posts sent between scenes at the top level', () => {
		const posts = [
			sceneStarted({ id: 1 }),
			makePost({ id: 2, author_id: 2, scene_id: 7 }),
			sceneEnded({ id: 3 }),
			makePost({ id: 4, author_id: 2, scene_id: null, body: 'between scenes' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'scene-group', 'post']);
		const lastItem = items[2];
		expect(lastItem.kind === 'post' && lastItem.post.id).toBe(4);
	});
});

describe('buildFeedItems — plan-resolution spans (hierarchy-plan S3)', () => {
	function planResolving(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			id: 10,
			author_id: null,
			system_code: 'plan.resolving',
			plan_id: 4,
			row_number: 3,
			severity: SEVERITY.IMPORTANT,
			body: 'Seek Answers is resolving.',
			...over,
		});
	}
	function planResolved(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			author_id: null,
			system_code: 'plan.resolved.make',
			plan_id: 4,
			row_number: 3,
			severity: SEVERITY.IMPORTANT,
			body: 'Seek Answers succeeded.',
			...over,
		});
	}
	function planSceneStarted(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			author_id: null,
			system_code: 'scene.started',
			plan_id: 4,
			scene_id: 12,
			severity: SEVERITY.IMPORTANT,
			body: 'Host Festivity — the scene opens.',
			system_data: {
				scene_id: 12,
				kind: 'plan',
				focus_player_id: 3,
				plan_id: 4,
				participants: ['Aldric', 'Lady Wren'],
			},
			...over,
		});
	}
	function planSceneEnded(over: Partial<ChatPost> = {}): ChatPost {
		return makePost({
			author_id: null,
			system_code: 'scene.ended',
			plan_id: 4,
			scene_id: 12,
			severity: SEVERITY.IMPORTANT,
			body: 'Host Festivity — the scene ends.',
			...over,
		});
	}

	// ── Plain span (the eight plan types with no PlanSceneStager) ────────────

	it('opens at plan.resolving, absorbs the resolution, and closes at the terminal post', () => {
		const posts = [
			planResolving({ id: 10 }),
			makePost({ id: 11, author_id: null, system_code: 'roll.resolved', plan_id: 4, severity: SEVERITY.IMPORTANT }),
			makePost({ id: 12, author_id: null, system_code: 'plan.seek_answers', plan_id: 4, severity: SEVERITY.DEFAULT }),
			planResolved({ id: 13 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'plan-group']);
		const group = items[1];
		if (group.kind !== 'plan-group') throw new Error('expected a plan-group');
		expect(group.planID).toBe(4);
		expect(group.resolvingPost.id).toBe(10);
		expect(group.outcomePost?.id).toBe(13);
		expect(group.outcome).toBe('make');
		expect(group.open).toBe(false);
		// The boundary posts are the card's header/footer, not body lines.
		expect(group.items.map((i) => (i.kind === 'post' ? i.post.id : i.kind))).toEqual([11, 12]);
		expect(group.messageCount).toBe(2);
	});

	it('keeps plan.prepared out of the span entirely, at its own position', () => {
		// Preparation can precede resolution by many rows — the whole reason the
		// span opens at plan.resolving instead (chronology invariant).
		const posts = [
			makePost({ id: 1, author_id: null, system_code: 'plan.prepared', plan_id: 4, severity: SEVERITY.IMPORTANT }),
			makePost({ id: 2, author_id: null, system_code: 'row.advanced', severity: SEVERITY.BOUNDARY }),
			planResolving({ id: 10 }),
			planResolved({ id: 11 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'post', 'post', 'plan-group']);
		const prepared = items[1];
		expect(prepared.kind === 'post' && prepared.post.id).toBe(1);
	});

	it('absorbs bystander posts from other players in order', () => {
		// Asset/marginalia edits are never row-state gated, so they can land
		// mid-resolution — same category of content a scene already absorbs.
		const posts = [
			planResolving({ id: 10 }),
			makePost({ id: 11, author_id: null, system_code: 'asset.updated', severity: SEVERITY.MINOR }),
			makePost({ id: 12, author_id: 5, body: 'nice roll' }),
			planResolved({ id: 13, system_code: 'plan.resolved.mar', body: 'Seek Answers marred.' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'plan-group');
		if (group?.kind !== 'plan-group') throw new Error('expected a plan-group');
		expect(group.items.map((i) => (i.kind === 'post' ? i.post.id : i.kind))).toEqual([11, 12]);
		expect(group.outcome).toBe('mar');
	});

	it('stays open while the plan is still resolving', () => {
		const posts = [planResolving({ id: 10 }), makePost({ id: 11, author_id: 5, body: 'watching' })];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'plan-group');
		if (group?.kind !== 'plan-group') throw new Error('expected a plan-group');
		expect(group.open).toBe(true);
		expect(group.outcomePost).toBeNull();
	});

	it('returns to the top level after the span closes', () => {
		const posts = [
			planResolving({ id: 10 }),
			planResolved({ id: 11 }),
			makePost({ id: 12, author_id: 5, body: 'after the plan' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'plan-group', 'post']);
		const after = items[2];
		expect(after.kind === 'post' && after.post.id).toBe(12);
	});

	it('resolves one plan per span, back to back', () => {
		// Plan resolution is exclusive table-wide (model/row_state.go serializes
		// the table on one step at a time), so spans never overlap — they queue.
		const posts = [
			planResolving({ id: 10, plan_id: 4 }),
			planResolved({ id: 11, plan_id: 4 }),
			planResolving({ id: 12, plan_id: 5, body: 'Spread Rumors is resolving.' }),
			planResolved({ id: 13, plan_id: 5, system_code: 'plan.cancelled', body: 'Spread Rumors cancelled.' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'plan-group', 'plan-group']);
		const [, first, second] = items;
		if (first.kind !== 'plan-group' || second.kind !== 'plan-group') throw new Error('expected two plan-groups');
		expect([first.planID, second.planID]).toEqual([4, 5]);
		expect(second.outcome).toBe('cancelled');
	});

	it('renders a terminal post with no open span as an ordinary log line', () => {
		// A pending plan cancelled before it ever resolved (e.g. by Make
		// Demands), or a window truncated past its plan.resolving.
		const posts = [
			makePost({
				id: 10,
				author_id: null,
				system_code: 'plan.cancelled',
				plan_id: 4,
				severity: SEVERITY.DEFAULT,
				body: 'Seek Answers cancelled.',
			}),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'post']);
	});

	it('counts unread posts inside the span and flags the unread-divider position', () => {
		const posts = [
			planResolving({ id: 10 }),
			makePost({ id: 11, author_id: 5 }), // read
			makePost({ id: 12, author_id: 5 }), // unread
			planResolved({ id: 13 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 11, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'plan-group');
		if (group?.kind !== 'plan-group') throw new Error('expected a plan-group');
		expect(group.unreadCount).toBe(1);
		expect(group.unreadDividerInside).toBe(true);
		expect(group.items.some((i) => i.kind === 'unread-divider')).toBe(true);
	});

	// ── Folded span (the four PlanSceneStager types) ────────────────────────

	it('folds into the scene container when the plan stages a scene', () => {
		const posts = [
			planResolving({ id: 10, body: 'Host Festivity is resolving.' }),
			planSceneStarted({ id: 11 }),
			makePost({ id: 12, author_id: 5, scene_id: 12, speaking_as_asset_id: 9, body: 'in character' }),
			planResolved({ id: 13, body: 'The festivity drew to a close.' }),
			planSceneEnded({ id: 14 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		// One container, not a plan card wrapping a scene card.
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'scene-group']);
		const group = items[1];
		if (group.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.sceneID).toBe(12);
		expect(group.planID).toBe(4);
		expect(group.resolvingPost?.id).toBe(10);
		expect(group.startPost?.id).toBe(11);
		expect(group.outcomePost?.id).toBe(13);
		expect(group.outcome).toBe('make');
		expect(group.endPost?.id).toBe(14);
		expect(group.open).toBe(false);
		// The terminal post is the container's footer, not a body line; the
		// scene.ended marker still renders inline as it always has.
		expect(group.items.map((i) => (i.kind === 'post' ? i.post.id : i.kind))).toEqual([12, 14]);
		expect(group.messageCount).toBe(1);
	});

	it('keeps the folded container open until scene.ended, not the terminal post', () => {
		const posts = [
			planResolving({ id: 10 }),
			planSceneStarted({ id: 11 }),
			planResolved({ id: 12 }),
			makePost({ id: 13, author_id: 5, scene_id: 12, body: 'a last word' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		if (group?.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.open).toBe(true);
		expect(group.outcomePost?.id).toBe(12);
		// Still absorbing positionally after the outcome landed.
		expect(group.items.map((i) => (i.kind === 'post' ? i.post.id : i.kind))).toEqual([13]);
	});

	it('returns to the top level after a folded span closes', () => {
		const posts = [
			planResolving({ id: 10 }),
			planSceneStarted({ id: 11 }),
			planResolved({ id: 12 }),
			planSceneEnded({ id: 13 }),
			makePost({ id: 14, author_id: 5, body: 'after the festivity' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'scene-group', 'post']);
	});

	it('hoists anything absorbed before the scene opened back out of the fold', () => {
		// kickoffPlanResolution emits plan.resolving and opens the scene back to
		// back, but a bystander edit can still interleave — it must keep its real
		// position ahead of the container, not slide inside it.
		const posts = [
			planResolving({ id: 10 }),
			makePost({ id: 11, author_id: null, system_code: 'asset.updated', severity: SEVERITY.DEFAULT }),
			planSceneStarted({ id: 12 }),
			planResolved({ id: 13 }),
			planSceneEnded({ id: 14 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'post', 'scene-group']);
		const hoisted = items[1];
		expect(hoisted.kind === 'post' && hoisted.post.id).toBe(11);
		const group = items[2];
		expect(group.kind === 'scene-group' && group.resolvingPost?.id).toBe(10);
	});

	it('leaves an ordinary turn-scene unfolded', () => {
		const posts = [
			makePost({
				id: 10,
				author_id: null,
				system_code: 'scene.started',
				scene_id: 12,
				severity: SEVERITY.IMPORTANT,
				body: 'Scene: Aldric at The Mill',
			}),
			makePost({ id: 11, author_id: 5, scene_id: 12, body: 'hello' }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		const group = items.find((i) => i.kind === 'scene-group');
		if (group?.kind !== 'scene-group') throw new Error('expected a scene-group');
		expect(group.planID).toBeNull();
		expect(group.resolvingPost).toBeNull();
		expect(group.outcomePost).toBeNull();
	});

	it('does not fold a scene belonging to a different plan', () => {
		// Defensive: scenes are exclusive, so this shouldn't arise — but a
		// mismatched plan_id must never quietly reparent the span.
		const posts = [
			planResolving({ id: 10, plan_id: 4 }),
			planSceneStarted({ id: 11, plan_id: 9, scene_id: 12 }),
		];
		const items = buildFeedItems(posts, { unreadAfterID: 999, currentPlayerID: 1 });
		expect(items.map((i) => i.kind)).toEqual(['day-divider', 'plan-group']);
		const group = items[1];
		if (group.kind !== 'plan-group') throw new Error('expected a plan-group');
		const inner = group.items.find((i) => i.kind === 'scene-group');
		expect(inner?.kind === 'scene-group' && inner.resolvingPost).toBeNull();
	});
});

describe('planOutcomeOf', () => {
	it('maps every terminal code EmitPlanResolved can write', () => {
		expect(planOutcomeOf('plan.resolved.make')).toBe('make');
		expect(planOutcomeOf('plan.resolved.mar')).toBe('mar');
		expect(planOutcomeOf('plan.cancelled')).toBe('cancelled');
		expect(planOutcomeOf('plan.resolved')).toBe('other');
	});

	it('returns null for anything that does not close a span', () => {
		expect(planOutcomeOf('plan.resolving')).toBeNull();
		expect(planOutcomeOf('plan.prepared')).toBeNull();
		expect(planOutcomeOf(null)).toBeNull();
	});
});

describe('parseSceneStartedData', () => {
	it('parses a well-formed payload', () => {
		const data = parseSceneStartedData({
			scene_id: 7,
			kind: 'turn',
			focus_player_id: 3,
			location: 'The Mill',
			time_label: 'Days later',
			prompt: 'What do you do?',
			participants: ['Aldric', 'Lady Wren'],
		});
		expect(data).toEqual({
			scene_id: 7,
			kind: 'turn',
			focus_player_id: 3,
			location: 'The Mill',
			time_label: 'Days later',
			prompt: 'What do you do?',
			participants: ['Aldric', 'Lady Wren'],
		});
	});

	it('returns null for missing or malformed data', () => {
		expect(parseSceneStartedData(null)).toBeNull();
		expect(parseSceneStartedData({})).toBeNull();
		expect(parseSceneStartedData('nonsense')).toBeNull();
	});

	it('defaults kind, and empty strings/array, when fields are absent', () => {
		const data = parseSceneStartedData({ scene_id: 5 });
		expect(data).toEqual({
			scene_id: 5,
			kind: 'turn',
			focus_player_id: 0,
			location: '',
			time_label: '',
			prompt: '',
			participants: [],
		});
	});
});

describe('systemCodeFamily', () => {
	it('extracts the prefix before the first dot', () => {
		expect(systemCodeFamily('plan.prepared')).toBe('plan');
		expect(systemCodeFamily('ranking.updated')).toBe('ranking');
	});

	it('returns null for null/no-dot codes', () => {
		expect(systemCodeFamily(null)).toBeNull();
	});
});

describe('markForCode', () => {
	// The routing table nothing else guards (adr/LOG_MARKS_PLAN.md S3 item 1):
	// unlike colour literals, which the app.css guard test catches, the mark map
	// had no unit coverage at all. `markForCode` is `systemCodeFamily` plus the
	// hostile-verb override, so these assertions are what keep a family from
	// silently losing its mark (rule 1: every family has one) or a bookkeeping
	// verb from wrongly inheriting a hostile one.

	// One representative real code per non-hostile family in "The set" table.
	// The two remaining keys — tear and seize — are only reachable through the
	// hostile override, so they're covered by the routing test below, not here.
	const familyForCode: Record<string, string> = {
		'plan.prepared': 'plan',
		'demand.resolved': 'demand',
		'asset.created': 'asset',
		'marginalia.added': 'marginalia',
		'roll.resolved': 'roll',
		'law.enacted': 'law',
		'scene.started': 'scene',
		'ranking.updated': 'ranking',
		'shake_up.category': 'shake_up',
		'prologue.track_ranked': 'prologue',
		'rumor.created': 'rumor',
		'secret.written': 'secret',
	};

	it('resolves a mark for every non-hostile family in the set', () => {
		for (const [code, family] of Object.entries(familyForCode)) {
			expect(markForCode(code)).toBe(family);
		}
	});

	it('routes the three hostile verbs to their second mark', () => {
		expect(markForCode('marginalia.torn')).toBe('tear');
		expect(markForCode('asset.taken')).toBe('seize');
		expect(markForCode('asset.destroyed')).toBe('seize');
		expect(markForCode('asset.leveraged')).toBe('seize');
	});

	it('leaves bookkeeping verbs on the family mark, not the hostile one', () => {
		// The amendment is verb-scoped, not family-scoped: only marginalia.torn
		// becomes the tear — marginalia.added stays the pencil — and only
		// asset.taken/destroyed/leveraged become the crossed swords.
		expect(markForCode('marginalia.added')).toBe('marginalia');
		expect(markForCode('marginalia.edited')).toBe('marginalia');
		expect(markForCode('asset.created')).toBe('asset');
		expect(markForCode('asset.renamed')).toBe('asset');
	});

	it('returns null for a player message (no system_code)', () => {
		expect(markForCode(null)).toBeNull();
	});
});

describe('mergeAppend / mergePrepend', () => {
	it('appends only posts not already present, preserving order', () => {
		const existing = [makePost({ id: 1 }), makePost({ id: 2 })];
		const incoming = [makePost({ id: 2 }), makePost({ id: 3 })];
		const merged = mergeAppend(existing, incoming);
		expect(merged.map((p) => p.id)).toEqual([1, 2, 3]);
	});

	it('prepends only posts not already present, preserving order', () => {
		const existing = [makePost({ id: 3 }), makePost({ id: 4 })];
		const older = [makePost({ id: 1 }), makePost({ id: 2 }), makePost({ id: 3 })];
		const merged = mergePrepend(existing, older);
		expect(merged.map((p) => p.id)).toEqual([1, 2, 3, 4]);
	});

	it('returns the same reference when there is nothing new (no needless rerender)', () => {
		const existing = [makePost({ id: 1 })];
		expect(mergeAppend(existing, [makePost({ id: 1 })])).toBe(existing);
		expect(mergePrepend(existing, [makePost({ id: 1 })])).toBe(existing);
	});
});

describe('isNearBottom', () => {
	it('is true within the threshold', () => {
		expect(isNearBottom(850, 1000, 100, 150)).toBe(true); // gap = 50
	});
	it('is false beyond the threshold', () => {
		expect(isNearBottom(700, 1000, 100, 150)).toBe(false); // gap = 200
	});
});

describe('loadInitialWindow', () => {
	it('populates the window and snapshots the read marker', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 10 })],
			has_more_before: true,
			has_more_after: false,
			last_read_post_id: 8,
		});
		const ctx = makeCtx({ mode: 'history' });
		await loadInitialWindow(ctx);
		expect(ctx.posts.map((p) => p.id)).toEqual([10]);
		expect(ctx.hasMoreBefore).toBe(true);
		expect(ctx.lastReadPostID).toBe(8);
		expect(ctx.initialReadMarker).toBe(8);
		expect(ctx.mode).toBe('live');
		expect(listGamePosts).toHaveBeenCalledWith(1);
	});
});

describe('fetchOlderPage', () => {
	it('does nothing when there is no more before', async () => {
		const ctx = makeCtx({ posts: [makePost({ id: 5 })], hasMoreBefore: false });
		await fetchOlderPage(ctx);
		expect(listGamePosts).not.toHaveBeenCalled();
	});

	it('does nothing while already loading', async () => {
		const ctx = makeCtx({ posts: [makePost({ id: 5 })], hasMoreBefore: true, loadingOlder: true });
		await fetchOlderPage(ctx);
		expect(listGamePosts).not.toHaveBeenCalled();
	});

	it('prepends the older page and updates the cursor', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 3 }), makePost({ id: 4 })],
			has_more_before: false,
			has_more_after: false,
			last_read_post_id: 0,
		});
		const ctx = makeCtx({ posts: [makePost({ id: 5 })], hasMoreBefore: true });
		await fetchOlderPage(ctx);
		expect(ctx.posts.map((p) => p.id)).toEqual([3, 4, 5]);
		expect(ctx.hasMoreBefore).toBe(false);
		expect(ctx.loadingOlder).toBe(false);
		expect(listGamePosts).toHaveBeenCalledWith(1, { beforeID: 5, limit: 50 });
	});
});

describe('appendLivePost', () => {
	it('appends in live mode', () => {
		const ctx = makeCtx({ posts: [makePost({ id: 1 })], mode: 'live' });
		appendLivePost(ctx, makePost({ id: 2 }));
		expect(ctx.posts.map((p) => p.id)).toEqual([1, 2]);
	});

	it('dedups a post already in the window', () => {
		const ctx = makeCtx({ posts: [makePost({ id: 1 })], mode: 'live' });
		appendLivePost(ctx, makePost({ id: 1 }));
		expect(ctx.posts.map((p) => p.id)).toEqual([1]);
	});

	it('is a no-op in history mode — the window stays a fixed historical slice', () => {
		const ctx = makeCtx({ posts: [makePost({ id: 1 })], mode: 'history' });
		appendLivePost(ctx, makePost({ id: 99 }));
		expect(ctx.posts.map((p) => p.id)).toEqual([1]);
	});
});

describe('reconnectResync', () => {
	it('falls back to the initial window when nothing is loaded yet', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 1 })],
			has_more_before: false,
			has_more_after: false,
			last_read_post_id: 0,
		});
		const ctx = makeCtx();
		await reconnectResync(ctx);
		expect(listGamePosts).toHaveBeenCalledWith(1);
		expect(ctx.posts.map((p) => p.id)).toEqual([1]);
	});

	it('fetches only what is newer than the last loaded post in live mode', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 6 })],
			has_more_before: false,
			has_more_after: false,
			last_read_post_id: 5,
		});
		const ctx = makeCtx({ posts: [makePost({ id: 5 })], lastReadPostID: 2 });
		await reconnectResync(ctx);
		expect(listGamePosts).toHaveBeenCalledWith(1, { afterID: 5 });
		expect(ctx.posts.map((p) => p.id)).toEqual([5, 6]);
		expect(ctx.lastReadPostID).toBe(5); // took the max
	});

	it('does nothing in history mode', async () => {
		const ctx = makeCtx({ mode: 'history', posts: [makePost({ id: 5 })] });
		await reconnectResync(ctx);
		expect(listGamePosts).not.toHaveBeenCalled();
	});

	it('re-windows instead of merging when the catch-up was truncated', async () => {
		// First call: the after-fetch, truncated at the server's cap.
		// Second call: the initial-window fallback.
		vi.mocked(listGamePosts)
			.mockResolvedValueOnce({
				posts: [makePost({ id: 6 }), makePost({ id: 7 })],
				has_more_before: false,
				has_more_after: true,
				last_read_post_id: 5,
			})
			.mockResolvedValueOnce({
				posts: [makePost({ id: 900 }), makePost({ id: 901 })],
				has_more_before: true,
				has_more_after: false,
				last_read_post_id: 5,
			});
		const ctx = makeCtx({ posts: [makePost({ id: 5 })], lastReadPostID: 2 });
		await reconnectResync(ctx);
		expect(listGamePosts).toHaveBeenNthCalledWith(1, 1, { afterID: 5 });
		expect(listGamePosts).toHaveBeenNthCalledWith(2, 1);
		// Window replaced wholesale — no hole between 7 and 900.
		expect(ctx.posts.map((p) => p.id)).toEqual([900, 901]);
		expect(ctx.hasMoreBefore).toBe(true);
		expect(ctx.mode).toBe('live');
	});
});

describe('enterHistoryMode / returnToNow', () => {
	it('replaces the window with an around-fetch and switches to history mode', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 40 }), makePost({ id: 41 })],
			has_more_before: true,
			has_more_after: true,
			last_read_post_id: 100,
		});
		const ctx = makeCtx({ mode: 'live', posts: [makePost({ id: 500 })] });
		await enterHistoryMode(ctx, 41);
		expect(listGamePosts).toHaveBeenCalledWith(1, { aroundID: 41 });
		expect(ctx.mode).toBe('history');
		expect(ctx.posts.map((p) => p.id)).toEqual([40, 41]);
		expect(ctx.hasMoreAfter).toBe(true);
	});

	it('returnToNow refetches the initial window and re-enters live mode', async () => {
		vi.mocked(listGamePosts).mockResolvedValue({
			posts: [makePost({ id: 500 })],
			has_more_before: true,
			has_more_after: false,
			last_read_post_id: 100,
		});
		const ctx = makeCtx({ mode: 'history', posts: [makePost({ id: 40 })] });
		await returnToNow(ctx);
		expect(ctx.mode).toBe('live');
		expect(ctx.posts.map((p) => p.id)).toEqual([500]);
	});
});

describe('resolveAnchor', () => {
	it('takes the fast path when the anchor is already in the loaded window', async () => {
		const ctx = makeCtx({
			posts: [makePost({ id: 12, system_code: 'plan.prepared', plan_id: 7 })],
		});
		const result = await resolveAnchor(ctx, { code: 'plan.prepared', planID: 7 });
		expect(result).toEqual({ postID: 12, inWindow: true });
		expect(getPostAnchor).not.toHaveBeenCalled();
	});

	it('falls back to the anchor endpoint when the window misses', async () => {
		vi.mocked(getPostAnchor).mockResolvedValue({ post_id: 99 });
		const ctx = makeCtx({ posts: [] });
		const result = await resolveAnchor(ctx, { code: 'row.advanced', row: 5 });
		expect(result).toEqual({ postID: 99, inWindow: false });
		expect(getPostAnchor).toHaveBeenCalledWith(1, { code: 'row.advanced', row: 5 });
	});

	it('returns null when nothing anchors the request', async () => {
		vi.mocked(getPostAnchor).mockRejectedValue(new Error('HTTP 404'));
		const ctx = makeCtx({ posts: [] });
		const result = await resolveAnchor(ctx, { code: 'row.advanced', row: 1 });
		expect(result).toBeNull();
	});
});

describe('reportReadMarker', () => {
	it('reports the newest loaded post id and applies the server-clamped result', async () => {
		vi.mocked(updateReadMarker).mockResolvedValue({ last_read_post_id: 42 });
		const ctx = makeCtx({ posts: [makePost({ id: 42 })], lastReadPostID: 10 });
		await reportReadMarker(ctx);
		expect(updateReadMarker).toHaveBeenCalledWith(1, 42);
		expect(ctx.lastReadPostID).toBe(42);
	});

	it('does nothing when the newest post is not past the marker', async () => {
		const ctx = makeCtx({ posts: [makePost({ id: 10 })], lastReadPostID: 10 });
		await reportReadMarker(ctx);
		expect(updateReadMarker).not.toHaveBeenCalled();
	});

	it('does nothing in history mode, even scrolled to the bottom of it', async () => {
		const ctx = makeCtx({ mode: 'history', posts: [makePost({ id: 200 })], lastReadPostID: 10 });
		await reportReadMarker(ctx);
		expect(updateReadMarker).not.toHaveBeenCalled();
	});
});
