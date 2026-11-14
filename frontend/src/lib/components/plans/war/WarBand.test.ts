import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { render } from 'svelte/server';
import WarBand from './WarBand.svelte';
import WarBands from './WarBands.svelte';
import WarName from './WarName.svelte';
import type { Plan, Player } from '$lib/api';
import type { PlanContext } from '../types';
import { declarations, styleOf } from '$lib/components/shared/cssTestKit';

const players = [
	{ id: 1, display_name: 'otto' },
	{ id: 2, display_name: 'Wren' },
	{ id: 3, display_name: 'Mira' },
] as Player[];

const plan = (over: Partial<Plan> = {}, enemies = [2]) => ({
	id: 9, plan_type: 'make_war', preparer_id: 1, status: 'resolved', row_number: 4,
	resolution_data: JSON.stringify({ make_war: { enemy_player_ids: enemies } }),
	...over,
}) as Plan;

const noop = () => {};
const css = (file: string) => styleOf(readFileSync(join(__dirname, file), 'utf8'));
const rules = (file: string, selector: string) =>
	Object.fromEntries(
		declarations(css(file)).filter(d => d.selector === selector).map(d => [d.prop, d.value]),
	);

describe('the war band', () => {
	it('is one pressable line: the barbute, the war’s name, where it stands, and the arrow', () => {
		const { body } = render(WarBand, { props: { plan: plan(), players, onOpen: noop } });
		expect(body).toMatch(/<button type="button" class="war-band/);
		expect(body).toContain('aria-haspopup="dialog"');
		expect(body).toContain('otto’s war on Wren');
		expect(body).toContain('· under way');
		expect(body).toContain('data-tone="war"');
		// The arrow (opens elsewhere), never the caret (expands in place).
		expect(body).toContain('data-kind="arrow"');
		expect(body).not.toContain('data-kind="caret"');
	});

	it('says "and others" when the declarer named more than one, and "planned" before it begins', () => {
		const { body } = render(WarBand, {
			props: { plan: plan({ status: 'pending' }, [2, 3]), players, onOpen: noop },
		});
		expect(body).toContain('otto’s war on Wren and others');
		expect(body).toContain('· planned');
	});

	it('has war’s red border and the name in red, but no red fill', () => {
		const band = rules('WarBand.svelte', '.war-band');
		expect(band.border).toBe('1px solid var(--color-danger)');
		// The fill is the ordinary raised surface: a war band is not a red box.
		expect(band.background).toBe('var(--color-surface)');
		expect(rules('WarBand.svelte', '.war-band-name').color).toBe('var(--color-danger)');
		expect(css('WarBand.svelte')).not.toMatch(/danger-bg|chip-red|warning-bg/);
	});

	it('is a control: 44px tall, the control corner, a focus ring in bright gold', () => {
		const band = rules('WarBand.svelte', '.war-band');
		expect(band['min-height']).toBe('var(--size-tap)');
		expect(band['border-radius']).toBe('var(--radius-control)');
		expect(rules('WarBand.svelte', '.war-band:focus-visible').outline).toBe('2px solid var(--color-you)');
	});
});

describe('the bands at the top of Play', () => {
	const ctx = { players, plans: [], assets: [] } as unknown as PlanContext;
	const base = { wars: [], players, ctx };

	it('draw nothing with no live war', () => {
		const { body } = render(WarBands, { props: { ...base, plans: [] } });
		expect(body).not.toContain('war-band');
		expect(body).not.toContain('<button');
	});

	it('draw one band for each live war, so two wars at once are two bands', () => {
		const second = plan({ id: 10, preparer_id: 3 }, [1]);
		const { body } = render(WarBands, { props: { ...base, plans: [plan(), second] } });
		expect(body.match(/aria-haspopup="dialog"/g)?.length).toBe(2);
		expect(body).toContain('otto’s war on Wren');
		expect(body).toContain('Mira’s war on otto');
	});
});

describe('the war’s name where no band names it', () => {
	it('is the barbute and the name, and is not pressable', () => {
		const { body } = render(WarName, { props: { plan: plan(), players } });
		expect(body).toContain('otto’s war on Wren');
		expect(body).toContain('data-tone="war"');
		expect(body).not.toContain('<button');
	});
});
