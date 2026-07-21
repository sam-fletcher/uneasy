import { describe, it, expect } from 'vitest';
import type { Player } from '$lib/api';
import { renderLogBody, stripLogMarkup } from './logMarkup';

// The emitter half of this markup lives in handler/system_posts.go
// (assetMark / playerMark); system_posts_test.go covers the same delimiter
// rules from the writing side. Change the two together.

function makePlayer(over: Partial<Player> = {}): Player {
	return {
		id: 1,
		game_id: 1,
		account_id: 1,
		display_name: 'alice',
		seat_order: 1,
		token_color: null,
		is_facilitator: false,
		...over,
	} as Player;
}

// Seats 1 and 2 of the default palette (playerColor.ts / app.css --player-1,2).
const PLAYERS: Player[] = [
	makePlayer({ id: 7, display_name: 'alice', seat_order: 1 }),
	makePlayer({ id: 8, display_name: 'bob', seat_order: 2 }),
];

describe('renderLogBody — asset marks', () => {
	it('renders **…** as italic emphasis, not bold', () => {
		expect(renderLogBody('destroyed **Old Keep**.', PLAYERS)).toBe('destroyed <em>Old Keep</em>.');
	});

	it('leaves a lone asterisk alone', () => {
		expect(renderLogBody('a *starred* note', PLAYERS)).toBe('a *starred* note');
	});
});

describe('renderLogBody — player marks', () => {
	it('paints a marked name in that player\'s colour', () => {
		const html = renderLogBody('@@7|alice@@ ends the scene', PLAYERS);
		expect(html).toBe(
			'<span class="player-mark" style="--mark-player-color: #C46BE8">alice</span> ends the scene'
		);
	});

	it('gives two different players two different colours', () => {
		const html = renderLogBody('@@7|alice@@ took a peer from @@8|bob@@', PLAYERS);
		expect(html).toContain('#C46BE8">alice</span>');
		expect(html).toContain('#5E8CFF">bob</span>');
	});

	it('still renders the name when the id is not in the roster', () => {
		// A player who left, or a post from another game's window: the name is
		// the content and must survive; only the colour degrades.
		const html = renderLogBody('@@99|ghost@@ acted', PLAYERS);
		expect(html).toContain('>ghost</span>');
		expect(html).toContain('#8a8a8a'); // UNKNOWN_PLAYER_COLOR
	});

	it('drops a non-hex token_color rather than injecting it into the style attribute', () => {
		// token_color is a free-text DB column; anything but a hex is refused
		// and the CSS fallback in .player-mark takes over.
		const hostile = [makePlayer({ id: 7, token_color: 'red; background: url(x)' })];
		expect(renderLogBody('@@7|alice@@ acted', hostile)).toBe(
			'<span class="player-mark">alice</span> acted'
		);
	});

	it('mixes with asset marks in one body', () => {
		const html = renderLogBody('@@7|alice@@ took **The Crown** from @@8|bob@@', PLAYERS);
		expect(html).toContain('<em>The Crown</em>');
		expect(html).toContain('>alice</span>');
		expect(html).toContain('>bob</span>');
	});
});

describe('renderLogBody — user text can never become markup', () => {
	it('escapes HTML before expanding any mark', () => {
		const html = renderLogBody('@@7|alice@@ added marginalia "<img src=x onerror=alert(1)>"', PLAYERS);
		expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;');
		expect(html).not.toContain('<img');
	});

	it('escapes HTML inside a marked name', () => {
		// Display names are free text too; the escape runs first, so the name
		// arrives at the mark parser already inert.
		const html = renderLogBody('@@7|<b>al</b>@@ acted', PLAYERS);
		expect(html).toContain('&lt;b&gt;al&lt;/b&gt;');
		expect(html).not.toContain('<b>');
	});

	it('does not treat a stray @ or a bare @@ as a mark', () => {
		// The doubled delimiter plus the digits-and-pipe shape is what keeps
		// quoted marginalia from tripping the parser.
		expect(renderLogBody('wrote "email me @home"', PLAYERS)).toBe('wrote "email me @home"');
		expect(renderLogBody('wrote "@@ meet at dawn"', PLAYERS)).toBe('wrote "@@ meet at dawn"');
		expect(renderLogBody('wrote "@@alice|7@@"', PLAYERS)).toBe('wrote "@@alice|7@@"');
	});
});

describe('stripLogMarkup', () => {
	it('reduces both marks to their plain words for the collapsed strip', () => {
		expect(stripLogMarkup('@@7|alice@@ took **The Crown** from @@8|bob@@')).toBe(
			'alice took The Crown from bob'
		);
	});

	it('leaves unmarked text untouched', () => {
		expect(stripLogMarkup('Row 4 begins')).toBe('Row 4 begins');
	});
});
