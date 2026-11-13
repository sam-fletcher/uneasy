// panes.ts — the table's three panes, and what each one's tab or column name says.
//
// The table has three panes: Public Record, Play and Chat, left to right. Wide
// windows show them side by side as columns; narrow ones swap them with tabs. A
// pane's tab and its column name wear the same marks, so a mark means the same
// thing at any width:
//
//     Public Record   the pale "new" dot, whenever Laws or Rumors has something new
//     Play            the gold clock, only while it is YOUR move
//     Chat            the pale "new" dot, while there are messages you haven't seen

export type PaneId = 'record' | 'play' | 'chat';

/** The panes in the order they sit, left to right and along the tab bar. */
export const PANES: readonly PaneId[] = ['record', 'play', 'chat'];

/** What each pane is called: the game's own words for them. */
export const PANE_LABEL: Record<PaneId, string> = {
	record: 'Public Record',
	play: 'Play',
	chat: 'Chat',
};

/** Which panes want a mark right now. For `play` it means "your move". */
export type PaneMarkState = Record<PaneId, boolean>;

/** What the Public Record pane shows. Chosen with the chips at its foot. */
export type RecordTab = 'events' | 'laws' | 'rumors';

/** How many columns the layout draws (lib/tableLayout.svelte.ts). */
export type LayoutMode = 'phone' | 'two' | 'three';

/**
 * The left column's pane when Chat is docked. The tab the player chose can be
 * Chat only on a phone, where it means nothing in two columns, so it falls back
 * to Play.
 */
export function leftPaneOf(active: PaneId): 'record' | 'play' {
	return active === 'record' ? 'record' : 'play';
}

/**
 * Which panes are on screen. A phone shows the chosen one. Two columns show
 * Chat and whichever of Public Record and Play the left column is on. Three
 * show everything. A pane that isn't shown stays laid out but hidden, so it
 * keeps its scroll position and state.
 */
export function panesShown(mode: LayoutMode, active: PaneId): PaneMarkState {
	if (mode === 'three') return { record: true, play: true, chat: true };
	if (mode === 'two') {
		const left = leftPaneOf(active);
		return { record: left === 'record', play: left === 'play', chat: true };
	}
	return { record: active === 'record', play: active === 'play', chat: active === 'chat' };
}

/**
 * The tab bar the layout calls for, and which tab is current: all three tabs on
 * a phone; the left column's two when Chat is docked (Chat needs no tab, it is
 * always there); none from three columns up, where every pane is a column.
 */
export function tabBarFor(mode: LayoutMode, active: PaneId): { panes: readonly PaneId[]; current: PaneId } {
	if (mode === 'phone') return { panes: PANES, current: active };
	if (mode === 'two') return { panes: ['record', 'play'], current: leftPaneOf(active) };
	return { panes: [], current: active };
}

/**
 * The marks to draw, one per pane. Public Record carries the "new" dot while
 * Laws or Rumors holds something new; Play carries the clock only when it is the
 * viewer's move; Chat carries the dot while there are messages the viewer hasn't
 * seen. Each is true or false: no counts reach the chrome.
 */
export function paneMarksFor(state: {
	lawsNew: boolean;
	rumorsNew: boolean;
	yourMove: boolean;
	unreadChat: boolean;
}): PaneMarkState {
	return {
		record: state.lawsNew || state.rumorsNew,
		play: state.yourMove,
		chat: state.unreadChat,
	};
}
