// What the styleguide says about each token. The VALUES are never written
// here: the page reads them from the browser at render time, so a retuned
// token in app.css shows up on the page by itself. Only the names and the
// one-line jobs live in this file, and styleguide.test.ts fails if a scale
// token exists in app.css that is missing from here, or the other way round.

export interface TokenRow {
	/** The custom property, e.g. `--space-3`. */
	name: string;
	/** What it is for, in a few words. Empty when the step has no job of its own. */
	job: string;
}

export const TEXT_SIZES: TokenRow[] = [
	{ name: '--text-label', job: 'timestamps and uppercase labels. The smallest anything gets' },
	{ name: '--text-small', job: 'secondary lines, log lines, chip names, tab labels, finished steps' },
	{ name: '--text-body', job: 'chat, step text, and every input (under 16px, iPhone Safari zooms the page)' },
	{ name: '--text-heading', job: "the open step's heading, sheet titles, the title bar" },
	{ name: '--text-title', job: 'page titles' },
];

/** The one documented exception to the five sizes. */
export const TEXT_DISPLAY: TokenRow = {
	name: '--text-display',
	job: "the landing page's cover title only, in the display face",
};

/** Six steps for padding, margin and gap. Spacing alone shows what belongs together. */
export const GAPS: TokenRow[] = [1, 2, 3, 4, 5, 6].map((n) => ({ name: `--space-${n}`, job: '' }));

/** The touch floor: the smallest a pressable thing may be, along either side. */
export const TOUCH: TokenRow = {
	name: '--size-tap',
	job: 'the smallest a pressable thing may be: a button, a row, a close target',
};

export const RADII: TokenRow[] = [
	{ name: '--radius-piece', job: 'game pieces: cards, tiles, dice. Close to square, like paper' },
	{ name: '--radius-control', job: 'buttons and inputs' },
	{ name: '--radius-chip', job: 'chips: fully rounded' },
	{ name: '--radius-sheet', job: 'sheets, on the top corners only' },
	{ name: '--radius-dot', job: 'status dots only. A player is a shape, never a circle' },
];

export const SURFACES: TokenRow[] = [
	{ name: '--color-bg', job: 'the page: the panes themselves' },
	{ name: '--color-surface', job: 'raised: a game piece, a chip, a sheet' },
	{ name: '--color-surface-2', job: 'raised twice: a control standing on a raised thing' },
];

export const TEXT_SHADES: TokenRow[] = [
	{ name: '--color-text', job: 'main: what a player reads first' },
	{ name: '--color-text-secondary', job: 'secondary lines, log lines, tab labels' },
	{ name: '--color-text-faint', job: 'timestamps and captions. The dimmest any text gets' },
];

export const BORDERS: TokenRow[] = [
	{ name: '--color-border-piece', job: 'a game piece (warm)' },
	{ name: '--color-border-control', job: 'a control: button, chip, input (neutral)' },
	{ name: '--color-border-control-hover', job: 'a control under the pointer: its border, lightened' },
	{ name: '--color-border', job: 'the thin divider between rows' },
	{ name: '--color-border-quiet', job: 'quieter still: the rules between the record\'s rows' },
];

export const GOLDS: TokenRow[] = [
	{ name: '--color-accent', job: 'deep gold, the game: headings, the brand, the main button, crowns' },
	{ name: '--color-you', job: 'bright gold, you: what you picked, your focus ring, the waiting clock on you' },
];

export const MEANING: (TokenRow & { word: string })[] = [
	{ name: '--color-danger', word: 'Lost', job: 'something is lost or at risk: errors, Mar, torn marks, and war' },
	{ name: '--color-warning', word: 'Careful', job: 'caution: a cost, a risk, a leveraged card' },
	{ name: '--color-success', word: 'Worked', job: 'it worked: Make, success' },
	{ name: '--color-highlight', word: 'Gain', job: 'something to gain, or information' },
];
