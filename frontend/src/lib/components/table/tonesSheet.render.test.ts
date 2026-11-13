import { describe, expect, it, vi } from 'vitest';
import { render } from 'svelte/server';
import type { ToneTopic, ToneTopicStatus } from '$lib/api';
import TonesSheet from './TonesSheet.svelte';

/*
 * The Tones sheet has two shapes. Before the Main Event it is the grid you set
 * tones on; once they lock it is a list of what was set. Checked by what the
 * component emits. The sheet's frame (RetinueSheet) draws nothing until it is
 * open, so every case renders open.
 */

let nextID = 1;
const topic = (name: string, status: ToneTopicStatus): ToneTopic => ({
	id: nextID++,
	game_id: 1,
	topic: name,
	status,
});

const topics = [
	topic('Betrayal', 'include'),
	topic('Plague', 'avoid_detail'),
	topic('Religion', 'default'),
	topic('Harm to children', 'never'),
	topic('Spiders', 'default'),
];

function sheet(locked: boolean, over: Partial<{ topics: ToneTopic[]; error: string }> = {}) {
	return render(TonesSheet, {
		props: {
			open: true,
			onClose: vi.fn(),
			topics: over.topics ?? topics,
			locked,
			error: over.error ?? '',
			onCycle: vi.fn(),
			onAdd: vi.fn(async () => true),
		},
	}).body;
}

describe('the Tones sheet before tones lock', () => {
	const body = sheet(false);

	it('keeps the grid, one tile per topic, to set tones on', () => {
		expect(body.match(/<button[^>]*class="tone-tile\b/g)?.length).toBe(topics.length);
		expect(body).toContain('Tap to cycle');
		expect(body).toContain('aria-label="Harm to children: Never. Tap to cycle."');
	});

	it('keeps the key and the box to add a topic, with a greyed-out main button', () => {
		expect(body).toContain('aria-label="Legend"');
		expect(body).toContain('Add a custom topic');
		expect(body).toMatch(/<button[^>]*class="action-btn primary tone-add-button[^"]*"[^>]*disabled/);
	});

	it('shows no list of what was set', () => {
		expect(body).not.toContain('<section');
		expect(body).not.toContain('with no opinion');
	});
});

describe('the Tones sheet once tones lock', () => {
	const body = sheet(true);

	it('lists only the topics someone set, grouped Never, Avoid detail, Include', () => {
		const never = body.indexOf('>Never<');
		const avoid = body.indexOf('>Avoid detail<');
		const include = body.indexOf('>Include<');
		expect(never).toBeGreaterThan(-1);
		expect(avoid).toBeGreaterThan(never);
		expect(include).toBeGreaterThan(avoid);
		for (const set of ['Harm to children', 'Plague', 'Betrayal']) expect(body).toContain(set);
		// The topics with no opinion are counted, not listed.
		expect(body).not.toContain('Religion');
		expect(body).not.toContain('Spiders');
		expect(body).toContain('2 more with no opinion.');
	});

	it('is no longer a grid of things that look pressable', () => {
		expect(body).not.toContain('tone-tile');
		expect(body).not.toContain('tone-grid');
		expect(body).not.toContain('Tap to cycle');
		expect(body).not.toContain('Add a custom topic');
		expect(body).toContain('Locked');
	});

	it('names each group in words beside its swatch, which a screen reader skips', () => {
		expect(body).toMatch(/class="swatch[^"]*" aria-hidden="true"/);
	});

	it('says so when nothing was set, and still counts the topics', () => {
		const none = sheet(true, { topics: [topic('Fire', 'default'), topic('Gore', 'default')] });
		expect(none).toContain('No tones were set.');
		expect(none).toContain('All 2 topics have no opinion.');
		expect(none).not.toContain('<section');
	});

	it('shows a failed write inside the sheet, not under it', () => {
		expect(sheet(true, { error: 'Could not save.' })).toContain('Could not save.');
	});
});
