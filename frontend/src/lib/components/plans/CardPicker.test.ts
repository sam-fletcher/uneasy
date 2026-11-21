import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import CardPicker from './CardPicker.svelte';
import BreakPicker from './BreakPicker.svelte';

// A watcher is shown the actor's picker greyed out and inert (the read-only mirror).
// `pointer-events: none` on the mirror stops a tap, but the rows are role=checkbox
// elements, not form controls, so a disabled fieldset doesn't stop the keyboard:
// they have to say they are disabled themselves. The single and multi-select cards
// always did; the marginalia rows ignored `readOnly`.

const players = [{ id: 1, display_name: 'ravi' }];
const note = (id: number, assetID: number) => ({ id, asset_id: assetID, position: id, text: `note ${id}`, is_torn: false });
const asset = {
	id: 5, name: 'Courtier', owner_id: 1, asset_type: 'peer', is_destroyed: false,
	is_main_character: false, is_leveraged: false, secret_count: 0, marginalia: [note(50, 5), note(51, 5)],
};

const rows = (html: string) => [...html.matchAll(/<[^>]*role="checkbox"[^>]*>/g)].map(m => m[0]);

describe('CardPicker in marginalia mode', () => {
	const draw = (readOnly: boolean) => render(CardPicker, {
		props: {
			label: 'Marginalium to tear', items: [asset] as never, players: players as never,
			marginaliaMode: true, selectedMarginaliaID: null, onSelectMarginalia: () => {}, readOnly,
		},
	}).body;

	it('leaves the rows operable for the player who picks', () => {
		const r = rows(draw(false));
		expect(r).toHaveLength(2);
		for (const row of r) {
			expect(row).toContain('aria-disabled="false"');
			expect(row).toContain('tabindex="0"');
		}
	});

	it('takes every row out of the tab order and marks it disabled when read-only', () => {
		const r = rows(draw(true));
		expect(r).toHaveLength(2);
		for (const row of r) {
			expect(row).toContain('aria-disabled="true"');
			expect(row).toContain('tabindex="-1"');
		}
	});
});

describe('BreakPicker, read-only', () => {
	const draw = (readOnly?: boolean) => render(BreakPicker, {
		props: {
			label: 'Marginalium to break', items: [asset] as never, players: players as never,
			emptyMessage: 'Nothing.', selectedMarginaliaID: null, onSelect: () => {}, readOnly,
		},
	}).body;

	it('is operable by default', () => {
		for (const row of rows(draw())) expect(row).toContain('aria-disabled="false"');
	});

	it('passes read-only down to its rows', () => {
		const r = rows(draw(true));
		expect(r).toHaveLength(2);
		for (const row of r) expect(row).toContain('aria-disabled="true"');
	});
});
