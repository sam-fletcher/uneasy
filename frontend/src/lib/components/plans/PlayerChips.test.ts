import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { declarations } from '$lib/components/shared/cssTestKit';
import type { Player } from '$lib/api';
import { playerColor } from '$lib/playerColor';
import PlayerChips from './PlayerChips.svelte';

/*
 * PlayerChips is a row of the house <Chip>, one per player: the player's shape,
 * then their name in their colour. Its look is chip.css (pieces.test.ts runs
 * the cascade for every chip state), so what is pinned here is what the
 * component adds: that it IS a chip and not a lookalike, that a pick and a
 * locked chip say so the way a chip does, and that the colour sits on the
 * name and never on the chip, since a player's colour only ever identifies the
 * player. (Before PlayerChips drew <Chip>, it drew .player-chip, and a test in
 * steps.test.ts compared that class to the chip rule by rule.)
 */

const alice = { id: 1, display_name: 'alice', seat_order: 1, token_color: null } as Player;
const bob = { id: 2, display_name: 'bob', seat_order: 2, token_color: null } as Player;

function draw(over: Record<string, unknown> = {}): string {
	return render(PlayerChips, {
		props: { players: [alice, bob], isActive: () => false, onSelect: () => {}, ...over },
	}).body;
}
/** The row's buttons, one string each. */
const chips = (html: string) => html.match(/<button\b[\s\S]*?<\/button>/g) ?? [];

describe('PlayerChips', () => {
	it('draws the house chip for every player, not a lookalike class', () => {
		const html = draw();
		expect(chips(html)).toHaveLength(2);
		for (const c of chips(html)) {
			expect(c).toMatch(/<button[^>]*type="button"/);
			expect(c).toMatch(/class="[^"]*\bchip\b/);
		}
		// The old name must not come back (the container's `player-chips` is allowed).
		expect(html).not.toMatch(/\bplayer-chip\b/);
		expect(html).not.toMatch(/\bchip-btn\b/);
	});

	it('shows a pick the way a chip does: selected, and aria-pressed for a screen reader', () => {
		const [a = '', b = ''] = chips(draw({ isActive: (p: Player) => p.id === alice.id }));
		expect(a).toMatch(/class="[^"]*\bselected\b/);
		expect(a).toContain('aria-pressed="true"');
		expect(b).not.toMatch(/\bselected\b/);
		expect(b).toContain('aria-pressed="false"');
		// Not the old `.active` that the legacy chip was marked with.
		expect(a).not.toMatch(/class="[^"]*\bactive\b/);
	});

	it("puts the player's shape first and their colour on the name, never on the chip", () => {
		const [a = ''] = chips(draw());
		const shape = a.indexOf('player-shape');
		const name = a.indexOf('>alice<');
		expect(shape, 'a shape').toBeGreaterThan(-1);
		expect(name, 'the name').toBeGreaterThan(shape);
		// The colour is the name's own: it is the span around the text.
		expect(a).toContain(`class="player-name labelled fixed" style="--player-name-color: ${playerColor(alice)};"`);
		// The button carries no inline colour (a choice wears no player colour).
		expect(a.match(/<button[^>]*>/)?.[0]).not.toMatch(/style=/);
		// Two players, two colours: the chips tell players apart by colour and shape.
		expect(playerColor(alice)).not.toBe(playerColor(bob));
		expect(chips(draw())[1]).toContain(`--player-name-color: ${playerColor(bob)}`);
	});

	it('a read-only mirror is disabled but not faded; only a chip that cannot be picked is', () => {
		// The faint rule (inlineText.css) is `.chip.unavailable …`, never bare :disabled,
		// because a mirror's chips are `disabled` too and a mirror is shown as it is.
		// So what separates the two cases is the class, and this pins it.
		for (const c of chips(draw({ readOnly: true }))) {
			expect(c).toMatch(/<button[^>]*\sdisabled/); // inert for the browser…
			expect(c).not.toMatch(/<button[^>]*class="[^"]*\bunavailable\b/); // …but not greyed out
			expect(c).toContain('player-name labelled fixed'); // keeps the colour
		}
		const [a = '', b = ''] = chips(draw({ isDisabled: (p: Player) => p.id === bob.id }));
		expect(a).not.toMatch(/\bunavailable\b/);
		expect(b).toMatch(/<button[^>]*class="[^"]*\bunavailable\b/);
		// Both still carry `fixed`: the chip is the player. It is `.unavailable` that fades it.
		expect(b).toContain('player-name labelled fixed');
		// The rule that fades a name or a shape can't match a read-only chip, which is
		// `disabled` but not `.unavailable`.
		const sheet = declarations(readFileSync(join(__dirname, '..', 'shared', 'inlineText.css'), 'utf8'));
		const fading = sheet.filter((d) => /player-name|player-shape/.test(d.selector) && /chip/.test(d.selector));
		for (const d of fading.filter((x) => /unavailable/.test(x.selector))) {
			expect(d.selector).toContain('.chip.unavailable');
		}
		expect(fading.some((d) => /:disabled|aria-disabled/.test(d.selector))).toBe(false);
	});

	it('locks every chip when read-only, and only the named ones otherwise', () => {
		for (const c of chips(draw({ readOnly: true }))) expect(c).toMatch(/<button[^>]*\sdisabled/);
		const [a = '', b = ''] = chips(draw({ isDisabled: (p: Player) => p.id === bob.id }));
		expect(a).not.toMatch(/<button[^>]*\sdisabled/);
		expect(b).toMatch(/<button[^>]*\sdisabled/);
	});

	it('keeps the name in the player colour when a chip is locked or picked', () => {
		const html = draw({ readOnly: true, isActive: (p: Player) => p.id === alice.id });
		for (const [i, p] of [alice, bob].entries()) {
			expect(chips(html)[i]).toContain(`color: ${playerColor(p)}`);
		}
	});
});
