import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import SceneSetupForm from './SceneSetupForm.svelte';
import { declarations, styleOf } from './shared/cssTestKit';
import type { Asset, Player, SceneSetupDraft } from '$lib/api';

/*
 * The scene setup form, rendered through svelte/server (no jsdom). Pinned:
 *   - it is a prompt, a help row and three groups under small headings, with no
 *     box or wash around any of them (only its controls and cards are boxed);
 *   - the focus player has one main button, greyed until a place and a time are
 *     chosen; a watcher has none, and every control of theirs is disabled;
 *   - a watcher's form mirrors the focus player's draft;
 *   - the opening scene offers no time chips;
 *   - the controls come from the shared recipes (the chip, the card).
 * What a render can't show (typing, the draft reaching a watcher's open page,
 * the scroll) belongs to the e2e spec (scenes.spec.ts).
 */

const SOURCE = readFileSync(join(__dirname, 'SceneSetupForm.svelte'), 'utf8');

const player = (id: number, display_name: string, seat_order: number) =>
	({ id, display_name, seat_order, token_color: null }) as unknown as Player;
const players = [player(1, 'ravi', 1), player(2, 'sol', 2)];

const asset = (id: number, name: string, asset_type: string, owner_id: number, is_main_character = false) =>
	({
		id, name, asset_type, owner_id, is_main_character,
		is_destroyed: false, is_leveraged: false, marginalia: [], secret_count: 0,
	}) as unknown as Asset;

const assets = [
	asset(1, 'Ravi Holdfast', 'holding', 1),
	asset(2, 'Sol Keep', 'holding', 2),
	asset(3, 'Ravi Main', 'peer', 1, true),
	asset(4, 'Captain Vell', 'peer', 1),
	asset(5, 'Brother Ansel', 'peer', 2),
];

function draw(over: Record<string, unknown> = {}): string {
	return render(SceneSetupForm, {
		props: {
			gameID: 1,
			assets,
			players,
			focusPlayerID: 1,
			prompt: 'Set a scene of your character learning something new.',
			onSceneStarted: () => {},
			...over,
		},
	}).body;
}

const draft = (over: Partial<SceneSetupDraft> = {}): SceneSetupDraft => ({
	player_id: 1,
	holding_id: null,
	custom_location: '',
	time_elapsed: '',
	time_note: '',
	present_peer_ids: [],
	...over,
});

/** The tags of one kind, whole. */
const tags = (html: string, tag: string) => html.match(new RegExp(`<${tag}\\b[^>]*>`, 'g')) ?? [];
const chipsIn = (html: string) => html.match(/<button[^>]*class="chip[^"]*"[^>]*>[^<]*(?:<!---->)?[^<]*/g) ?? [];

describe('the form', () => {
	it('asks its questions under small headings, after the prompt', () => {
		const html = draw();
		const labels = [...html.matchAll(/<h3 class="label[^"]*">([^<]*)<\/h3>/g)].map((m) => m[1]);
		expect(labels).toEqual(['Prompt', 'Where', 'When', 'Who else is here']);
		expect(html).toMatch(/<p class="prompt[^"]*">Set a scene of your character learning something new\.<\/p>/);
	});

	it('draws the prompt and the groups with no wrapper that could be a box', () => {
		// The prompt is a heading and a paragraph in a plain group, not a block of
		// its own: nothing here carries the old .prompt panel's label span.
		const html = draw();
		expect(html).not.toContain('prompt-label');
		expect(html).not.toContain('custom-panel');
	});

	it('shows the help row to the focus player and to a watcher alike', () => {
		for (const readOnly of [false, true]) {
			const html = draw({ readOnly });
			expect(html).toContain('How to set a good scene');
			// The row is the one control a watcher can operate: it is not disabled.
			const head = html.match(/<button[^>]*class="tap-row help-head[^"]*"[^>]*>/)![0];
			expect(head).not.toContain('disabled');
		}
	});

	it('offers the six durations as chips, and none on the opening scene', () => {
		const labels = [...draw().matchAll(/class="chip compact[^"]*"[^>]*>(?:<!---->)?([^<]*)</g)].map((m) => m[1]);
		expect(labels).toEqual([
			'Moments later', 'Hours later', 'Days later', 'Weeks later', 'Flashback', 'Simultaneous',
		]);

		const opening = draw({ isFirstScene: true });
		expect(opening).not.toContain('class="chip');
		// The note is then the only way to say when, and its label says so.
		expect(opening).toContain('aria-label="When the scene opens"');
		expect(opening).toContain('placeholder="At night? At the coronation?"');
	});

	it('lists the focus player\'s main character as always present, and offers every other live peer', () => {
		const html = draw();
		const who = html.slice(html.indexOf('Who else is here'));
		expect(who).toContain('Ravi Main');
		expect(who).toContain('Always present');
		expect(who).toContain('Captain Vell');
		expect(who).toContain('Brother Ansel');
	});

	it('says so when there is nothing to pick, in plain words', () => {
		const html = draw({ assets: [] });
		expect(html).toContain('No holdings yet — use a custom location below.');
		expect(html).toContain('No other peers in play yet.');
		expect(html).not.toContain('<em');
		expect(html).not.toContain('<i>');
	});
});

describe('as the focus player', () => {
	it('has one main button, greyed out until a place and a time are chosen', () => {
		const html = draw();
		const buttons = tags(html, 'button').filter((b) => b.includes('action-btn'));
		expect(buttons).toHaveLength(1);
		expect(buttons[0]).toContain('action-btn primary');
		expect(buttons[0]).toContain('disabled');
		expect(html).toContain('Begin Scene');
	});

	it('has live controls: nothing is disabled but the always-present card and the button', () => {
		const html = draw();
		for (const input of tags(html, 'input')) expect(input).not.toContain('disabled');
		for (const chip of chipsIn(html)) expect(chip).not.toContain('disabled');
	});
});

describe('as a watcher', () => {
	it('has no button to begin the scene', () => {
		const html = draw({ readOnly: true });
		expect(html).not.toContain('Begin Scene');
		expect(html).not.toContain('action-btn');
	});

	it('has every control disabled, the fields and the chips and the cards alike', () => {
		const html = draw({ readOnly: true });
		const inputs = tags(html, 'input');
		expect(inputs).toHaveLength(2);
		for (const input of inputs) expect(input).toContain('disabled');
		const chips = chipsIn(html);
		expect(chips).toHaveLength(6);
		for (const chip of chips) expect(chip).toContain('disabled');
		// Every card's checkbox says it is not pressable.
		const boxes = tags(html, 'span').filter((s) => s.includes('role="checkbox"'));
		expect(boxes.length).toBeGreaterThan(0);
		for (const box of boxes) expect(box).toContain('aria-disabled="true"');
	});

	it('sees the focus player\'s draft: the chip, the holding and the peer they picked', () => {
		const html = draw({
			readOnly: true,
			draft: draft({ holding_id: 1, time_elapsed: 'days', present_peer_ids: [4] }),
		});
		const pressed = (label: string) =>
			html.match(new RegExp(`aria-pressed="(true|false)"[^>]*>(?:<!---->)?${label}<`))?.[1];
		expect(pressed('Days later')).toBe('true');
		expect(pressed('Hours later')).toBe('false');

		// The cards, in the order they are drawn: two holdings, then the main
		// character (always checked), then the peers.
		const checked = (name: string) => {
			const card = html.slice(html.indexOf(name) - 900, html.indexOf(name));
			return [...card.matchAll(/aria-checked="(true|false)"/g)].pop()![1];
		};
		expect(checked('Ravi Holdfast')).toBe('true');
		expect(checked('Sol Keep')).toBe('false');
		expect(checked('Captain Vell')).toBe('true');
		expect(checked('Brother Ansel')).toBe('false');
	});

	it('sees a place or a time the focus player typed', () => {
		const html = draw({
			readOnly: true,
			draft: draft({ custom_location: 'The Orangery', time_note: 'Past midnight' }),
		});
		expect(html).toMatch(/placeholder="Another location" value="The Orangery"/);
		expect(html).toMatch(/placeholder="Another time" value="Past midnight"/);
	});

	it('starts blank for a late joiner who has seen no draft', () => {
		const html = draw({ readOnly: true, draft: null });
		expect(html).toMatch(/placeholder="Another location" value=""/);
		expect(html).not.toContain('chip compact selected');
	});
});

describe('the stylesheet', () => {
	const css = styleOf(SOURCE);
	const decls = declarations(css);
	const props = (selector: string) => decls.filter((d) => d.selector === selector).map((d) => d.prop);

	it('gives the prompt and the groups no box, no fill and no wash', () => {
		for (const sel of ['.prompt', '.group', '.label', '.scene-setup', '.cards', '.chips', '.hint']) {
			const p = props(sel);
			for (const banned of ['background', 'background-color', 'border', 'border-left', 'border-radius', 'box-shadow', 'opacity']) {
				expect(p, `${sel} must not set ${banned}`).not.toContain(banned);
			}
		}
		// The retired wash and the warm panel border, by name.
		expect(css).not.toContain('--color-surface-active');
		expect(css).not.toContain('--color-border-warm');
		expect(css).not.toContain('--color-surface-sunken');
	});

	it('does not fade or restyle the form for a watcher', () => {
		// A watcher's controls are disabled, and each draws its own disabled look.
		expect(css).not.toContain('.readonly');
		expect(css).not.toContain('opacity');
		expect(SOURCE).not.toContain('class:readonly');
	});

	it('draws a field as a control: 16px type, 44px tall, the control border, the raised fill', () => {
		const field = decls.filter((d) => d.selector === "input[type='text']");
		const value = (prop: string) => field.find((d) => d.prop === prop)?.value;
		expect(value('font-size')).toBe('var(--text-body)');
		expect(value('min-height')).toBe('var(--size-tap)');
		expect(value('border')).toBe('1px solid var(--color-border-control)');
		expect(value('border-radius')).toBe('var(--radius-control)');
		expect(value('background')).toBe('var(--color-surface)');
	});

	it('draws a small heading as a label: 12px, capitals, secondary, not the step\'s gold', () => {
		const label = decls.filter((d) => d.selector === '.label');
		const value = (prop: string) => label.find((d) => d.prop === prop)?.value;
		expect(value('font-size')).toBe('var(--text-label)');
		expect(value('text-transform')).toBe('uppercase');
		expect(value('color')).toBe('var(--color-text-secondary)');
	});

	it('draws the prompt as step text', () => {
		const prompt = decls.filter((d) => d.selector === '.prompt');
		const value = (prop: string) => prompt.find((d) => d.prop === prop)?.value;
		expect(value('font-size')).toBe('var(--text-body)');
		expect(value('color')).toBe('var(--color-text)');
	});

	it('keeps the groups apart by the step gap, and a heading closer to what it heads', () => {
		expect(decls).toContainEqual({ selector: '.scene-setup', prop: 'gap', value: 'var(--space-4)' });
		expect(decls).toContainEqual({ selector: '.group', prop: 'gap', value: 'var(--space-2)' });
	});

	it('lets the page scroll the form, not the form itself', () => {
		expect(props('.scene-setup')).not.toContain('overflow-y');
		expect(props('.scene-setup')).not.toContain('overflow');
		expect(decls).toContainEqual({ selector: '.scene-setup', prop: 'flex-shrink', value: '0' });
	});

	it('has no width query of its own: the chips wrap by themselves', () => {
		expect(css).not.toContain('@container');
		expect(css).not.toContain('@media');
	});

	it('writes no colour literal', () => {
		expect(css).not.toMatch(/#[0-9a-fA-F]{3,8}\b/);
		expect(css).not.toMatch(/\brgba?\(/);
	});
});
