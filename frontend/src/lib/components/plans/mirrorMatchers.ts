// What a watcher's mirror looks like in a server-rendered page, for the plan panels' tests.
//
// The recipe is in planPanel.css: a mirror is a <fieldset disabled> carrying both
// `resolve-mirror-wrap` (the layout) and `resolve-mirror` (inert, no pointer events); the live
// version of the same wrapper carries only the first. A class has to be matched as a whole token:
// `\bresolve-mirror\b` also matches the front of `resolve-mirror-wrap` (a hyphen is a word
// boundary), so a mirror that lost its class would still have passed.

const token = (name: string) => `(?:[^"]*\\s)?${name}(?:\\s[^"]*)?`;
const OPEN_MIRROR = `<fieldset[^>]*class="${token('resolve-mirror')}"[^>]*`;

/** A disabled fieldset with the mirror class. */
export const MIRROR = new RegExp(`${OPEN_MIRROR}\\sdisabled(?=[\\s>=/])`);

/** The wrapper without the mirror class: the actor's live fieldset. */
export const LIVE_WRAP = new RegExp(`<fieldset[^>]*class="(?!${token('resolve-mirror')}")${token('resolve-mirror-wrap')}"`);

/** The markup of every mirror on the page, each from its opening fieldset to the first closing one
 *  (a mirror holds no fieldset of its own). */
export function mirrorsOf(html: string): string[] {
	return html.match(new RegExp(`${OPEN_MIRROR}\\sdisabled(?=[\\s>=/])[^>]*>[\\s\\S]*?<\\/fieldset>`, 'g')) ?? [];
}

/** The first mirror's markup, or '' when there is none. */
export function mirrorOf(html: string): string {
	return mirrorsOf(html)[0] ?? '';
}
