// logMarkup.ts — the tiny markup subset system-log bodies are written in, and
// its renderer (adr/CHAT_VISUAL_HIERARCHY_PLAN.md S1/S4).
//
// Only the BACKEND writes this markup, in handler/system_posts.go; player chat
// messages are never passed through here and show their asterisks verbatim.
// Two marks exist, one per element identity the log names:
//
//   **name**      an asset name          → <em> (italic, not bold — see below)
//   @@<id>|name@@ a player name          → that player's colour
//
// which is the "one job per channel" ruling applied to typography: shape and
// position carry structure, weight and colour carry importance, and typography
// identifies *what kind of thing* a word is. Verbs and everything else in a
// body deliberately stay plain.
//
// Both marks are deliberately doubled-delimiter. A lone '*' inside quoted
// marginalia can't open an emphasis span, and a lone '@' (or even '@@') can't
// open a player mark — it takes '@@', digits, and a '|' to match. Bodies quote
// player-authored marginalia, rumors, laws and free-text answers verbatim, so
// that margin matters. Neither delimiter contains &, < or >, so both survive
// escapeHtml unchanged — which is what lets us escape FIRST and expand marks
// second (see renderLogBody).
//
// Old posts stored before a mark existed simply render plain; nothing
// rewrites history.

import type { Player } from './api';
import { playerColorByID } from './playerColor';

/** Asset names: **Old Keep**. Non-greedy so adjacent marks don't merge. */
const ASSET_MARK = /\*\*(.+?)\*\*/g;

/**
 * Player names: @@12|alice@@. The id is digits-only and the name may not
 * contain either delimiter char — playerMark() drops the mark entirely rather
 * than emit a name that would break those rules, so a body either has a
 * well-formed token or a plain name, never a half-parsed one.
 */
const PLAYER_MARK = /@@(\d+)\|([^@|]*)@@/g;

/**
 * Hex colours only reach the style attribute. playerColorByID can return a
 * player's `token_color`, a free-text DB column, and this value is
 * interpolated into markup — so anything that isn't a plain hex is dropped and
 * the CSS fallback in `.player-mark` takes over.
 */
const HEX_COLOR = /^#[0-9a-fA-F]{3,8}$/;

type ColorPlayer = Pick<Player, 'id' | 'token_color' | 'seat_order'>;

function escapeHtml(s: string): string {
	return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

/**
 * Renders a system-log body to HTML for `{@html}`.
 *
 * Order matters and is load-bearing: escape first (names, marginalia and rumor
 * text in these bodies are all user input), then expand the marks the server
 * wrote into the tags we chose. Nothing after the escape can introduce markup
 * the caller didn't intend, because the only tags in the output are the ones
 * this function emits and the only attribute value it interpolates is a
 * validated hex colour.
 *
 * Asset emphasis renders ITALIC, not bold — a deliberate style choice so names
 * read distinctly from prose without bold's weight (bold is reserved for
 * standalone numeric counters app-wide).
 *
 * `players` is the game's roster, used to resolve a marked id to its colour;
 * an id that isn't in the roster falls back to the unknown-player grey, same
 * as a chat byline's would.
 */
export function renderLogBody(body: string, players: ColorPlayer[]): string {
	return escapeHtml(body)
		.replace(ASSET_MARK, '<em>$1</em>')
		.replace(PLAYER_MARK, (_match, id: string, name: string) => {
			const color = playerColorByID(Number(id), players);
			const style = HEX_COLOR.test(color) ? ` style="--mark-player-color: ${color}"` : '';
			return `<span class="player-mark"${style}>${name}</span>`;
		});
}

/**
 * Drops the delimiters for plain-text contexts (the mobile collapsed strip)
 * where no markup can render. Leaves the words themselves exactly as written.
 */
export function stripLogMarkup(body: string): string {
	return body.replace(ASSET_MARK, '$1').replace(PLAYER_MARK, '$2');
}
