import { describe, it, expect } from 'vitest';
import { mergeSeenMarkers, rereadCouldClearADot, dividerMarkerAfterReread } from './seenMarkers';

// The three markers a table page holds for "what is new to me": Laws, Rumors and
// chat. Another device may have moved any of them, and a re-read brings the
// news. The merge is the part that has to be right whichever order replies
// arrive in, so it is pure and pinned here.

describe('mergeSeenMarkers', () => {
	const current = { laws: 4, rumors: 2, chat: 90 };

	it('takes a marker another device moved forward', () => {
		const next = mergeSeenMarkers(current, { laws: 9, rumors: 2, last_read_post_id: 120 });
		expect(next).toEqual({ laws: 9, rumors: 2, chat: 120 });
	});

	it('never moves a marker back: a slow reply from before the player read must not relight a dot', () => {
		const next = mergeSeenMarkers(current, { laws: 1, rumors: 0, last_read_post_id: 10 });
		expect(next).toEqual(current);
	});

	it('moves each marker on its own', () => {
		const next = mergeSeenMarkers(current, { laws: 4, rumors: 7, last_read_post_id: 90 });
		expect(next).toEqual({ laws: 4, rumors: 7, chat: 90 });
	});

	it('fills a marker the page has not loaded yet (null) with the server value', () => {
		const next = mergeSeenMarkers(
			{ laws: null, rumors: null, chat: 0 },
			{ laws: 3, rumors: 0, last_read_post_id: 5 },
		);
		expect(next).toEqual({ laws: 3, rumors: 0, chat: 5 });
	});

	it('returns the same object when nothing moved, so callers can skip a write', () => {
		const next = mergeSeenMarkers(current, { laws: 4, rumors: 2, last_read_post_id: 90 });
		expect(next).toBe(current);
	});
});

// A re-read can only clear dots: markers move forward, and a dot is "something
// above my marker". So when no dot is lit there is nothing it could change, and
// the request is skipped. That makes most returns to a phone free. The one thing
// worth asking even with no dot is a marker the page hasn't loaded yet (null).

describe('rereadCouldClearADot', () => {
	const quiet = { lawsNew: false, rumorsNew: false, unreadChat: false, laws: 4, rumors: 2 };

	it('is false when no dot is lit and every marker is loaded: nothing to clear', () => {
		expect(rereadCouldClearADot(quiet)).toBe(false);
	});

	it.each([
		['a Laws dot', { lawsNew: true }],
		['a Rumors dot', { rumorsNew: true }],
		['an unread-chat dot', { unreadChat: true }],
	])('is true with %s lit', (_name, lit) => {
		expect(rereadCouldClearADot({ ...quiet, ...lit })).toBe(true);
	});

	it('is true while a Laws or Rumors marker has not loaded (null)', () => {
		expect(rereadCouldClearADot({ ...quiet, laws: null })).toBe(true);
		expect(rereadCouldClearADot({ ...quiet, rumors: null })).toBe(true);
	});
});

// Chat's "N new" divider is frozen where the window loaded. It moves with the dot
// only when the news is that chat's own marker moved elsewhere, and Chat is off
// screen. This device's own reading is already in its chat marker (it reported
// it), so comparing against that would swallow a divider this device left behind
// whenever only Laws or Rumors moved elsewhere.

describe('dividerMarkerAfterReread', () => {
	const fresh = (chat: number) => ({ laws: 0, rumors: 0, last_read_post_id: chat });

	it('moves to the server marker when chat was read elsewhere and is off screen', () => {
		expect(dividerMarkerAfterReread({ divider: 40, chat: 50, chatOnScreen: false }, fresh(80))).toBe(80);
	});

	it('stays put when only Laws or Rumors moved: the server chat marker is no further than this device\'s own', () => {
		// This device read up to 50 itself and left its divider at 40.
		expect(dividerMarkerAfterReread({ divider: 40, chat: 50, chatOnScreen: false }, fresh(50))).toBe(40);
		expect(dividerMarkerAfterReread({ divider: 40, chat: 50, chatOnScreen: false }, fresh(30))).toBe(40);
	});

	it('stays put while chat is on screen: the reader may be parked on it', () => {
		expect(dividerMarkerAfterReread({ divider: 40, chat: 50, chatOnScreen: true }, fresh(80))).toBe(40);
	});

	it('never moves back', () => {
		expect(dividerMarkerAfterReread({ divider: 90, chat: 50, chatOnScreen: false }, fresh(80))).toBe(90);
	});
});
