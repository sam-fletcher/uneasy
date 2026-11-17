import { describe, expect, it, vi } from 'vitest';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { loadReveal } from './revealLoad';
import type { SimultaneousReveal } from '$lib/api';

const REVEAL = { id: 7, entries: [], is_complete: false } as unknown as SimultaneousReveal;

describe('loadReveal', () => {
	// After a Liaise resolves, the plan's reveal id is cleared while the widget is
	// still finishing a submit; its refresh then had no id and requested
	// GET /api/reveals/null. With no id, nothing may be requested.
	it('makes no request when the id is null', async () => {
		const fetchReveal = vi.fn();
		await expect(loadReveal(null, fetchReveal)).resolves.toBeNull();
		expect(fetchReveal).not.toHaveBeenCalled();
	});

	it('makes no request when the id is undefined', async () => {
		const fetchReveal = vi.fn();
		await expect(loadReveal(undefined, fetchReveal)).resolves.toBeNull();
		expect(fetchReveal).not.toHaveBeenCalled();
	});

	it('fetches the reveal for a real id, 0 included only if it is one', async () => {
		const fetchReveal = vi.fn().mockResolvedValue(REVEAL);
		await expect(loadReveal(7, fetchReveal)).resolves.toBe(REVEAL);
		expect(fetchReveal).toHaveBeenCalledExactlyOnceWith(7);
	});
});

describe('SimultaneousRevealInput', () => {
	// A component test would need a DOM this project doesn't have, so pin the wiring
	// instead: the widget reaches the API only through loadReveal, whose guard the
	// tests above hold. A direct getReveal(revealID) would bring the null request back.
	const source = readFileSync(join(__dirname, 'SimultaneousRevealInput.svelte'), 'utf8');

	it('loads its reveal through loadReveal, never getReveal directly', () => {
		expect(source).toMatch(/loadReveal\(\s*revealID\s*\)/);
		expect(source).not.toMatch(/\bgetReveal\b/);
	});

	it('takes a nullable reveal id', () => {
		expect(source).toMatch(/revealID:\s*number\s*\|\s*null/);
	});

	it('does not submit when there is no reveal id', () => {
		expect(source).toMatch(/revealID\s*==\s*null/);
	});
});
