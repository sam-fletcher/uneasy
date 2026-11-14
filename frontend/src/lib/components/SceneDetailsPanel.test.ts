import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import SceneDetailsPanel from './SceneDetailsPanel.svelte';
import { declarations, styleOf } from './shared/cssTestKit';

/*
 * The scene details panel, rendered through svelte/server (no jsdom). Pinned:
 *   - a destroyed peer never shows as a live, claimable card;
 *   - the prompt is a small heading and text, with no box or wash, and absent
 *     when the scene has none;
 *   - where, when and whose: the place wears the house scene mark (not the pin
 *     emoji), the time is plain words, the focus player is a shape and a name;
 *   - "Take over" is a secondary button for a player other than the focus
 *     player, on a peer nobody holds; "End Scene" is the one main button, the
 *     focus player's;
 *   - nothing is boxed but the cards and the buttons.
 */

const SOURCE = readFileSync(join(__dirname, 'SceneDetailsPanel.svelte'), 'utf8');

// A destroyed peer stays in the scene's peer list and in the live asset list
// (flagged); it must not render as a live, claimable card.
const peerAsset = (id: number, name: string, is_destroyed: boolean, owner_id = 1) =>
	({
		id, name, owner_id, asset_type: 'peer', is_main_character: false,
		is_destroyed, is_leveraged: false, marginalia: [],
	}) as never;
const scenePeer = (peer_asset_id: number, controller_player_id: number | null = null) =>
	({ peer_asset_id, controller_player_id }) as never;

const players = [
	{ id: 1, display_name: 'ravi', seat_order: 1, token_color: null },
	{ id: 2, display_name: 'sol', seat_order: 2, token_color: null },
] as never;

function draw(over: Record<string, unknown> = {}): string {
	return render(SceneDetailsPanel, {
		props: {
			gameID: 1,
			scene: { id: 1, focus_player_id: 2, location_holding_id: null, time_elapsed: 'hours' } as never,
			peers: [],
			assets: [],
			players,
			currentPlayerID: 1,
			isFocusPlayer: false,
			onSceneEnded: () => {},
			...over,
		},
	}).body;
}

/** The tags of one kind, whole. */
const tags = (html: string, tag: string) => html.match(new RegExp(`<${tag}\\b[^>]*>`, 'g')) ?? [];

describe('SceneDetailsPanel', () => {
	it('lists a live peer but not a destroyed one', () => {
		const body = draw({
			peers: [scenePeer(10), scenePeer(11)],
			assets: [peerAsset(10, 'Livia Liveone', false), peerAsset(11, 'Dead Duncan', true)],
		});
		expect(body).toContain('Livia Liveone');
		expect(body).not.toContain('Dead Duncan');
	});
});

describe('the prompt', () => {
	it('is a small heading and the prompt\'s words, with no label span or wrapper of its own', () => {
		const html = draw({
			scene: { id: 1, focus_player_id: 2, time_elapsed: 'hours', prompt: 'Set a scene of your character learning something new.' } as never,
		});
		expect(html).toMatch(/<h3 class="label[^"]*">Prompt<\/h3>\s*<p class="prompt[^"]*">Set a scene of your character learning something new\.<\/p>/);
		expect(html).not.toContain('prompt-label');
	});

	it('is absent when the scene carries none', () => {
		const html = draw({ scene: { id: 1, focus_player_id: 2, time_elapsed: 'hours', prompt: '' } as never });
		expect(html).not.toContain('Prompt');
		expect(html).not.toContain('class="prompt');
	});
});

describe('where, when and whose', () => {
	it('names a custom place with the house scene mark, and never the pin emoji', () => {
		const html = draw({
			scene: { id: 1, focus_player_id: 2, location_custom: 'The Orangery', time_elapsed: 'days' } as never,
		});
		expect(html).toContain('The Orangery');
		expect(html).not.toContain('📍');
		// The mark is the slot's svg (LogMark's "scene" family, the quotation marks).
		const slot = html.match(/<span class="place-mark[^"]*">([\s\S]*?)<\/span>/)![1];
		expect(slot).toContain('<svg');
		expect(slot).toContain('M4.5 17a4 4 0 0 0 4-4V7h-5v6h4');
	});

	it('names a held place by the holding, and "Unknown" when nothing says', () => {
		const held = draw({
			scene: { id: 1, focus_player_id: 2, location_holding_id: 7, time_elapsed: 'days' } as never,
			assets: [{ ...(peerAsset(7, 'Sol Keep', false, 2) as object), asset_type: 'holding' } as never],
		});
		expect(held).toMatch(/<span class="place-name[^"]*">\s*(?:<!--\[\d-->)?\s*Sol Keep/);
		const nowhere = draw({ scene: { id: 1, focus_player_id: 2, time_elapsed: 'days' } as never });
		expect(nowhere).toContain('Unknown');
	});

	it('writes the time as the chip\'s words, or as the note, in plain type', () => {
		expect(draw({ scene: { id: 1, focus_player_id: 2, time_elapsed: 'weeks' } as never })).toMatch(
			/<p class="time[^"]*">Weeks later<\/p>/,
		);
		const note = draw({ scene: { id: 1, focus_player_id: 2, time_elapsed: null, time_note: 'Past midnight' } as never });
		expect(note).toMatch(/<p class="time[^"]*">Past midnight<\/p>/);
		// Never in italics (italic is for asset names).
		expect(note).not.toContain('<em');
	});

	it('prints one of the two, never both: a note beside a chip would name a duration nobody picked', () => {
		// The server stores a chip XOR a note, but a scene from before that held
		// both ("Moments later — at night"). The chip wins and the note is not shown.
		const both = draw({
			scene: { id: 1, focus_player_id: 2, time_elapsed: 'moments', time_note: 'at night' } as never,
		});
		expect(both).toMatch(/<p class="time[^"]*">Moments later<\/p>/);
		expect(both).not.toContain('at night');
	});

	it('draws a plan-scene\'s missing time as nothing, not an empty slot', () => {
		const html = draw({ scene: { id: 1, focus_player_id: 2, time_elapsed: null, time_note: null } as never });
		expect(html).not.toContain('class="time');
	});

	it('shows the focus player as their shape and their name in their colour', () => {
		const html = draw();
		const line = html.match(/<p class="focus-line[^"]*">([\s\S]*?)<\/p>/)![1];
		expect(line).toContain('Focus:');
		expect(line).toContain('data-shape="triangle"'); // seat 2, the second shape
		expect(line).toMatch(/class="focus-name"[^>]*style="color: #8AD00B;?"[^>]*>\s*sol/i);
	});
});

describe('the buttons', () => {
	const unclaimed = {
		peers: [scenePeer(10)],
		assets: [peerAsset(10, 'Livia Liveone', false)],
	};

	it('offers "Take over" on an unclaimed peer to a player who is not the focus player, as a secondary button', () => {
		const html = draw(unclaimed);
		const take = tags(html, 'button').filter((b) => b.includes('claim-btn'));
		expect(take).toHaveLength(1);
		expect(take[0]).toContain('action-btn secondary');
		expect(html).toContain('Take over');
		// No main button for a watcher: the step has nothing to commit.
		expect(html).not.toContain('action-btn primary');
		expect(html).not.toContain('End Scene');
	});

	it('does not offer it to the focus player, on a claimed peer, or with no one logged in', () => {
		expect(draw({ ...unclaimed, isFocusPlayer: true, currentPlayerID: 2 })).not.toContain('Take over');
		expect(draw({ ...unclaimed, peers: [scenePeer(10, 2)] })).not.toContain('Take over');
		expect(draw({ ...unclaimed, currentPlayerID: null })).not.toContain('Take over');
	});

	it('says who plays a peer: you, someone else, or nobody yet', () => {
		const mixed = draw({
			peers: [scenePeer(10), scenePeer(11, 1), scenePeer(12, 2)],
			assets: [peerAsset(10, 'Ten', false), peerAsset(11, 'Eleven', false), peerAsset(12, 'Twelve', false)],
		});
		expect(mixed).toContain('Unclaimed — open to be played');
		expect(mixed).toContain('Played by you');
		expect(mixed).toContain('Played by sol');
	});

	it('gives the focus player "End Scene" as the one main button', () => {
		const html = draw({ isFocusPlayer: true, currentPlayerID: 2 });
		const main = tags(html, 'button').filter((b) => b.includes('action-btn'));
		expect(main).toHaveLength(1);
		expect(main[0]).toContain('action-btn primary');
		expect(html).toContain('End Scene');
	});
});

describe('the stylesheet', () => {
	const css = styleOf(SOURCE);
	const decls = declarations(css);
	const props = (selector: string) => decls.filter((d) => d.selector === selector).map((d) => d.prop);

	it('boxes nothing of its own: no fill, border, shadow or corner on a group, a heading or a line', () => {
		for (const sel of [
			'.scene-details', '.group', '.label', '.prompt', '.scene-header', '.place', '.time',
			'.focus-line', '.peer-list', '.peer-row', '.end-bar',
		]) {
			const p = props(sel);
			for (const banned of [
				'background', 'background-color', 'border', 'border-top', 'border-left', 'border-radius', 'box-shadow', 'opacity',
			]) {
				expect(p, `${sel} must not set ${banned}`).not.toContain(banned);
			}
		}
		expect(css).not.toContain('--color-surface-active');
		expect(css).not.toContain('--color-border-warm');
	});

	it('draws "Take over" from the shared button, adding only its place', () => {
		// The look (44px, the corner, the border, the fill) is actionButton.css's;
		// this file may only say where the button sits.
		expect(props('.claim-btn')).toEqual(['align-self']);
		expect(css).not.toContain('36px');
	});

	// A declaration for `selector`, whether the rule names it alone or in a list.
	const value = (selector: string, prop: string) =>
		decls.find((d) => d.prop === prop && d.selector.split(',').map((s) => s.trim()).includes(selector))?.value;

	it('draws a small heading as a label and the prompt as step text', () => {
		expect(value('.label', 'font-size')).toBe('var(--text-label)');
		expect(value('.label', 'text-transform')).toBe('uppercase');
		expect(value('.label', 'color')).toBe('var(--color-text-secondary)');
		expect(value('.prompt', 'font-size')).toBe('var(--text-body)');
		expect(value('.prompt', 'color')).toBe('var(--color-text)');
	});

	it('draws the place as step text, and the time and focus a step down, in secondary', () => {
		expect(value('.place', 'font-size')).toBe('var(--text-body)');
		for (const sel of ['.time', '.focus-line']) {
			expect(value(sel, 'font-size')).toBe('var(--text-small)');
			expect(value(sel, 'color')).toBe('var(--color-text-secondary)');
		}
	});

	it('lets the page scroll the panel, not the panel itself', () => {
		expect(props('.scene-details')).not.toContain('overflow-y');
		expect(props('.scene-details')).not.toContain('overflow');
		expect(decls).toContainEqual({ selector: '.scene-details', prop: 'flex-shrink', value: '0' });
	});

	it('writes no colour literal', () => {
		expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
		expect(css).not.toMatch(/\brgba?\(/);
	});
});
