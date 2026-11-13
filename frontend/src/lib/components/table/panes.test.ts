import { describe, expect, it } from 'vitest';
import { leftPaneOf, panesShown, paneMarksFor, tabBarFor, PANES, type PaneId } from './panes';

/*
 * The table's panes sit side by side when the window is wide and swap with tabs
 * when it is narrow, wearing the same marks either way. These are the pure
 * functions the page asks (which panes show, which tabs, which marks), so the
 * layout's logic is pinned down without a browser.
 */

describe('which panes are on screen', () => {
	it('a phone shows only the chosen pane', () => {
		for (const pane of PANES) {
			const shown = panesShown('phone', pane);
			expect(Object.entries(shown).filter(([, on]) => on).map(([p]) => p)).toEqual([pane]);
		}
	});

	it('two columns always show Chat, and the left column is Public Record or Play', () => {
		expect(panesShown('two', 'play')).toEqual({ record: false, play: true, chat: true });
		expect(panesShown('two', 'record')).toEqual({ record: true, play: false, chat: true });
	});

	it('a phone-only "Chat" choice falls back to Play when the left column is all there is', () => {
		// Turning a phone on its side while on the Chat tab: Chat is docked now,
		// and the left column has to show something.
		expect(leftPaneOf('chat')).toBe('play');
		expect(panesShown('two', 'chat')).toEqual({ record: false, play: true, chat: true });
	});

	it('three columns show every pane whatever was chosen', () => {
		for (const pane of PANES) {
			expect(panesShown('three', pane)).toEqual({ record: true, play: true, chat: true });
		}
	});
});

describe('the tab bar', () => {
	it('has all three tabs, in order, on a phone', () => {
		const bar = tabBarFor('phone', 'chat');
		expect(bar.panes).toEqual(['record', 'play', 'chat']);
		expect(bar.current).toBe('chat');
	});

	it('has the left column’s two tabs when Chat is docked, and Chat needs none', () => {
		const bar = tabBarFor('two', 'record');
		expect(bar.panes).toEqual(['record', 'play']);
		expect(bar.current).toBe('record');
		expect(tabBarFor('two', 'chat').current).toBe('play');
	});

	it('has no tabs once every pane is a column', () => {
		expect(tabBarFor('three', 'play').panes).toEqual([]);
	});
});

describe('the marks on a tab or a column name', () => {
	const none = { lawsNew: false, rumorsNew: false, yourMove: false, unreadChat: false };

	it('shows nothing when there is nothing to say', () => {
		expect(paneMarksFor(none)).toEqual({ record: false, play: false, chat: false });
	});

	it('lights Public Record for either list, so a sub-tab’s dot shows on its parent', () => {
		expect(paneMarksFor({ ...none, lawsNew: true }).record).toBe(true);
		expect(paneMarksFor({ ...none, rumorsNew: true }).record).toBe(true);
	});

	it('lights Play only for the viewer’s own move', () => {
		expect(paneMarksFor({ ...none, yourMove: true })).toEqual({ record: false, play: true, chat: false });
		// Nothing else lights it: new laws, rumors and messages are the dot's, and
		// someone is almost always on the clock, so it would say nothing if it
		// lit for anyone but you.
		const everythingElse = paneMarksFor({ lawsNew: true, rumorsNew: true, yourMove: false, unreadChat: true });
		expect(everythingElse.play).toBe(false);
	});

	it('lights Chat for unread messages and nothing else', () => {
		const marks = paneMarksFor({ ...none, unreadChat: true });
		expect(marks).toEqual({ record: false, play: false, chat: true });
		// Every mark is a plain yes or no: no count can reach the chrome.
		for (const pane of PANES as readonly PaneId[]) expect(typeof marks[pane]).toBe('boolean');
	});
});
