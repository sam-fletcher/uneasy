import { describe, expect, it, vi } from 'vitest';
import { createRawSnippet } from 'svelte';
import { render } from 'svelte/server';
import type { ChatPost, Plan, Player } from '$lib/api';
import type { ChatFeedContext } from '$lib/chatFeed';
import ChatPanel from './ChatPanel.svelte';
import FeedDivider from './chat/FeedDivider.svelte';
import SpanHeader from './chat/SpanHeader.svelte';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { declarations, styleOf } from './shared/cssTestKit';

// Server rendering also runs `onDestroy`, which reports the read marker. Keep
// the real API for everything else; that one call goes nowhere.
vi.mock('$lib/api', async (importOriginal) => ({
	...(await importOriginal<typeof import('$lib/api')>()),
	updateReadMarker: vi.fn(async () => ({ last_read_post_id: 0 })),
}));

/*
 * The feed's grammar, checked by what the components emit (server render: no
 * DOM, so no layout and no click). What a test like this can pin is the
 * STRUCTURE the redesign ruled: which divider weight is which, that a plan
 * resolution is one header row with its lines behind a rule and not a card, that
 * war's lines carry the barbute, that Send is greyed out until there is text.
 * How it looks is the screenshots' job.
 */

// Times are anchored to local noon, not to "now": the feed labels a day
// "Today" or "Yesterday" by the real calendar, and "five minutes ago" is
// yesterday for anyone running this just after midnight.
const NOON = new Date();
NOON.setHours(12, 0, 0, 0);
const iso = (minutesAgo: number) => new Date(NOON.getTime() - minutesAgo * 60_000).toISOString();
const yesterday = (minutesAgo = 0) => new Date(NOON.getTime() - 24 * 3_600_000 - minutesAgo * 60_000).toISOString();

let nextID = 1;
function post(over: Partial<ChatPost>): ChatPost {
	return {
		id: nextID++,
		game_id: 1,
		row_number: null,
		plan_id: null,
		scene_id: null,
		author_id: null,
		body: 'x',
		created_at: iso(5),
		severity: 0,
		system_code: null,
		system_data: null,
		speaking_as_asset_id: null,
		...over,
	};
}

const player = (id: number, name: string, seat: number): Player =>
	({
		id,
		game_id: 1,
		account_id: id,
		display_name: name,
		joined_at: iso(1000),
		is_facilitator: id === 1,
		token_color: null,
		seat_order: seat,
	}) as Player;

const players = [player(1, 'Ash', 1), player(2, 'Wren', 2), player(3, 'otto', 3)];

const plan = (id: number, planType: Plan['plan_type'], preparerID: number): Plan =>
	({ id, game_id: 1, plan_type: planType, preparer_id: preparerID }) as Plan;

function feedOf(posts: ChatPost[], over: Partial<ChatFeedContext> = {}): ChatFeedContext {
	return {
		gameID: 1,
		posts,
		mode: 'live',
		hasMoreBefore: false,
		hasMoreAfter: false,
		lastReadPostID: 0,
		initialReadMarker: 0,
		loadingOlder: false,
		...over,
	};
}

function renderChat(posts: ChatPost[], over: { feed?: Partial<ChatFeedContext>; plans?: Plan[] } = {}) {
	return render(ChatPanel, {
		props: {
			gameID: 1,
			feed: feedOf(posts, over.feed),
			players,
			currentPlayerID: 1,
			typingLabel: '',
			plans: over.plans ?? [],
		},
	}).body;
}

/** A plan resolution that ran without a scene: resolving, a line inside, a close. */
function planSpan(planID: number, close: ChatPost | null, extra: ChatPost[] = []): ChatPost[] {
	return [
		post({ plan_id: planID, system_code: 'plan.resolving', severity: 75, body: 'Seek Answers is resolving.' }),
		post({ plan_id: planID, author_id: 2, body: 'The harbour ledgers are in the north wing.' }),
		...extra,
		...(close ? [close] : []),
	];
}

describe('the three divider weights', () => {
	const text = (s: string) => createRawSnippet(() => ({ render: () => `<span>${s}</span>` }));

	it('draws each weight as its own kind, in one family', () => {
		for (const kind of ['day', 'unread', 'row'] as const) {
			const body = render(FeedDivider, { props: { kind, children: text('x') } }).body;
			expect(body).toContain('role="separator"');
			expect(body).toContain(`data-kind="${kind}"`);
			expect(body).toContain('feed-divider');
		}
	});

	it('passes attributes through, so the unread divider can be found to scroll to', () => {
		const body = render(FeedDivider, {
			props: { kind: 'unread', children: text('4 new'), 'data-feed-unread-divider': '', 'aria-label': '4 new messages' },
		}).body;
		expect(body).toContain('data-feed-unread-divider');
		expect(body).toContain('aria-label="4 new messages"');
	});

	it('shows all three in the feed: the day, "N new", and a row boundary', () => {
		const posts = [
			post({ author_id: 2, body: 'old news', created_at: yesterday(10) }),
			post({ author_id: 2, body: 'newer news', created_at: iso(30) }),
			post({ author_id: 3, body: 'newest news', created_at: iso(20) }),
			post({ system_code: 'row.advanced', severity: 100, body: 'Row 6 begins.', row_number: 6, created_at: iso(10) }),
		];
		// Read up to the first message: the other three are new (two messages and a
		// row line, which is a system post at boundary severity, so it counts).
		const body = renderChat(posts, { feed: { initialReadMarker: posts[0].id } });
		expect(body).toContain('data-kind="day"');
		expect(body).toContain('Yesterday');
		expect(body).toMatch(/data-kind="unread"[^>]*aria-label="3 new messages"[\s\S]*?<!---->3 new<!---->/);
		expect(body).toContain('data-feed-unread-divider');
		expect(body).toContain('data-kind="row"');
		expect(body).toContain('Row 6 begins.');
		// The old filled number pill is gone.
		expect(body).not.toContain('row-boundary-pill');
	});

	it('says "1 new" for a single new message and reads it out in full', () => {
		const posts = [
			post({ author_id: 2, body: 'seen', created_at: iso(30) }),
			post({ author_id: 2, body: 'fresh', created_at: iso(20) }),
		];
		const body = renderChat(posts, { feed: { initialReadMarker: posts[0].id } });
		expect(body).toContain('aria-label="1 new message"');
		expect(body).toMatch(/<!---->1 new<!---->/);
	});
});

describe("a plan's resolution", () => {
	const closeMake = () =>
		post({ plan_id: 7, system_code: 'plan.resolved.make', severity: 75, body: 'Seek Answers succeeded. Next: otto.' });
	const plans = [plan(7, 'seek_answers', 3)];

	it('is a header row that folds, not a card', () => {
		const body = renderChat(planSpan(7, closeMake()), { plans });
		// The header is the one control: a tappable row with a caret and aria-expanded.
		expect(body).toMatch(/<button[^>]*class="[^"]*\bplan-header\b[^"]*"/);
		expect(body).toContain('aria-expanded');
		expect(body).toContain('data-plan-id="7"');
		expect(body).toContain('tap-row');
		expect(body).toContain('data-kind="caret"');
		// The old card's pieces are gone: its count line, its outcome box and label.
		expect(body).not.toContain('scene-count');
		expect(body).not.toContain('plan-outcome-label');
		expect(body).not.toMatch(/\d+ entries/);
	});

	it("names the plan, shows the preparer's shape, and says Make in its status word", () => {
		const body = renderChat(planSpan(7, closeMake()), { plans });
		expect(body).toContain('Seek Answers');
		// otto is seat 3: the Rose square, named for a screen reader.
		expect(body).toMatch(/data-shape="square"[^>]*>|aria-label="otto"/);
		expect(body).toContain('aria-label="otto"');
		expect(body).toMatch(/class="[^"]*\bstatus-word\b[^"]*\bsuccess\b[^"]*"[^>]*>Make</);
	});

	it('says Resolving while it is open, and a Mar in red when it marred', () => {
		const open = renderChat(planSpan(7, null), { plans });
		expect(open).toMatch(/status-word[^>]*>Resolving</);
		expect(open).not.toMatch(/status-word[^>]*\b(success|danger)\b/);
		const mar = renderChat(
			planSpan(7, post({ plan_id: 7, system_code: 'plan.resolved.mar', severity: 75, body: 'Seek Answers marred.' })),
			{ plans }
		);
		expect(mar).toMatch(/status-word[^"]*\bdanger\b[^>]*>Mar</);
	});

	it('puts its lines behind a rule, and keeps the outcome line when folded', () => {
		const posts = planSpan(7, closeMake());
		// Nothing unread, so a resolved span starts folded.
		const folded = renderChat(posts, { plans, feed: { initialReadMarker: 999 } });
		expect(folded).toContain('aria-expanded="false"');
		expect(folded).not.toContain('The harbour ledgers are in the north wing.');
		// The sentence behind the status word stays: a shut span still says what comes next.
		expect(folded).toContain('Seek Answers succeeded. Next: otto.');
		expect(folded).toMatch(/class="span-body\b/);

		// Read up to the resolving post: the unread divider lands inside the span,
		// which is what opens a resolved span.
		const open = renderChat(posts, { plans, feed: { initialReadMarker: posts[0].id } });
		expect(open).toContain('aria-expanded="true"');
		expect(open).toContain('The harbour ledgers are in the north wing.');
		expect(open).toContain('Seek Answers succeeded. Next: otto.');
	});

	it('does not say the status word twice: the outcome line is the sentence behind it', () => {
		const body = renderChat(planSpan(7, closeMake()), { plans });
		expect(body).not.toMatch(/class="plan-outcome[^>]*>\s*<span[^>]*>Make</);
	});

	it('shows a pale dot on a folded span that holds something unread, and none on an open one', () => {
		const posts = planSpan(7, closeMake());
		// Everything is unread and the divider sits above the span, not inside it:
		// the span is resolved, so it starts folded, and says there is news inside.
		const folded = renderChat(posts, { plans });
		expect(folded).toContain('aria-expanded="false"');
		expect(folded).toContain('new in this span');
		// With the divider inside the span it opens, so the dot has nothing to say.
		const open = renderChat(posts, { plans, feed: { initialReadMarker: posts[0].id } });
		expect(open).toContain('aria-expanded="true"');
		expect(open).not.toContain('new in this span');
		// And a span that was read has no dot either.
		const read = renderChat(posts, { plans, feed: { initialReadMarker: 999 } });
		expect(read).not.toContain('new in this span');
	});

	it('wears the barbute in war red for Make War, and the flag for another plan', () => {
		const war = [
			post({ plan_id: 9, system_code: 'plan.resolving', severity: 75, body: 'Make War is resolving.' }),
			post({ plan_id: 9, system_code: 'plan.make_war', severity: 50, body: 'otto paid the cost of battle.' }),
		];
		const withWar = renderChat(war, { plans: [plan(9, 'make_war', 3)] });
		expect(withWar).toContain('barbute-mark');
		expect(withWar).toContain('data-tone="war"');
		const flagOnly = renderChat(planSpan(7, closeMake()), { plans });
		expect(flagOnly).not.toContain('barbute-mark');
	});

	it('shows no Make for Make War when it ends: the book gives war none', () => {
		const war = [
			post({ plan_id: 9, system_code: 'plan.resolving', severity: 75, body: 'Make War is resolving.' }),
			post({ plan_id: 9, system_code: 'plan.resolved.make', severity: 75, body: 'Make War succeeded.' }),
		];
		const body = renderChat(war, { plans: [plan(9, 'make_war', 3)] });
		expect(body).toContain('Make War succeeded.');
		expect(body).not.toMatch(/status-word[^>]*>Make</);
	});

	it('still names the plan from the post when the plan list has not loaded', () => {
		const body = renderChat(planSpan(7, null));
		expect(body).toContain('Seek Answers');
		expect(body).not.toContain('is resolving');
	});
});

describe("war's lines", () => {
	it('carry the barbute, and nothing else in the feed does', () => {
		const lines = [
			post({ system_code: 'plan.make_war', severity: 50, body: 'otto paid the cost of battle: leveraged two assets.' }),
			post({ system_code: 'asset.created', severity: 50, body: 'Ash created a thing.' }),
		];
		const body = renderChat(lines);
		// Exactly one: the war line's. The asset line keeps its crest.
		expect(body.match(/barbute-mark/g)?.length).toBe(1);
		expect(body).toContain('data-code="plan.make_war"');
	});
});

describe('the composer', () => {
	it('greys Send out until there is text, with the house button', () => {
		const body = renderChat([post({ author_id: 2, body: 'hi' })]);
		expect(body).toMatch(/<button[^>]*class="[^"]*\baction-btn\b[^"]*\bprimary\b[^"]*\bsend\b[^"]*"[^>]*disabled/);
	});

	it('opens no keyboard on its own: nothing autofocuses', () => {
		const body = renderChat([post({ author_id: 2, body: 'hi' })]);
		expect(body).not.toContain('autofocus');
	});

	it('has no "Speaking as" control when there is only yourself to speak as', () => {
		expect(renderChat([post({ author_id: 2, body: 'hi' })])).not.toContain('Speaking as');
	});
});

describe('the header', () => {
	it('holds the Tones chip and the Hide bookkeeping switch, with no heading on a phone', () => {
		const body = renderChat([post({ author_id: 2, body: 'hi' })]);
		expect(body).toMatch(/class="chip compact"[^>]*>(<!---->)?Tones · 0 set/);
		expect(body).toContain('role="switch"');
		expect(body).toContain('Hide bookkeeping');
		expect(body).not.toContain('pane-name');
	});

	it('starts with the column name in columns', () => {
		const body = render(ChatPanel, {
			props: {
				gameID: 1,
				feed: feedOf([post({ author_id: 2, body: 'hi' })]),
				players,
				currentPlayerID: 1,
				typingLabel: '',
				showName: true,
			},
		}).body;
		expect(body).toContain('pane-name');
		expect(body).toContain('Chat');
	});
});

describe('a span header with a long banner', () => {
	// A scene's banner is a whole phrase ("Scene: alice's main character at The Mill,
	// Days later"). Squeezed in a 375px row, the status once shrank to nothing and
	// "Live" was cut to "Liv". No layout in a render test, so this reads the
	// stylesheet: the title is the one thing that shrinks, the status never does.
	const css = styleOf(readFileSync(join(__dirname, 'chat/SpanHeader.svelte'), 'utf8'));
	const decl = (selector: string, prop: string) =>
		declarations(css).find((d) => d.selector === selector && d.prop === prop)?.value;

	it('lets the title shrink and wrap', () => {
		expect(decl('.span-title', 'min-width')).toBe('0');
		expect(decl('.span-title', 'overflow-wrap')).toBe('anywhere');
	});

	it('never lets the status shrink below its own words', () => {
		expect(decl('.span-status', 'flex')).toBe('1 0 auto');
		expect(decl('.span-status', 'min-width')).toBeUndefined();
	});

	it('shows Live in a scene header, and a caret only when it can fold', () => {
		const live = render(SpanHeader, {
			props: { family: 'scene', title: 'Scene at The Mill', status: { word: 'Live', tone: 'neutral' }, expanded: true, locked: true },
		}).body;
		expect(live).toContain('>Live<');
		expect(live).toContain('disabled');
		expect(live).not.toContain('row-mark');
		const open = render(SpanHeader, { props: { family: 'scene', title: 'Scene at The Mill', expanded: false } }).body;
		expect(open).toContain('row-mark');
		expect(open).not.toContain('disabled');
	});
});
