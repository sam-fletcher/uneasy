import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';

/*
 * A plan's result is a word, not a typed mark. A tick or a cross typed as a
 * character is drawn in whatever font each phone supplies, so it looks
 * different from one device to the next. The house way is "Make" or "Mar" in
 * the outcome's colour (planPanel.css), as MakeMarPicker and Propose Decree
 * draw it. This reads the source of every plan panel, sub-folders included, so
 * a typed mark cannot come back unnoticed.
 */

const PLANS = __dirname;

/** Every .svelte file under a folder, as paths relative to it. */
function svelteFilesUnder(dir: string, rel = ''): string[] {
	return readdirSync(join(dir, rel), { withFileTypes: true }).flatMap((entry) => {
		const path = rel ? `${rel}/${entry.name}` : entry.name;
		if (entry.isDirectory()) return svelteFilesUnder(dir, path);
		return entry.name.endsWith('.svelte') ? [path] : [];
	});
}

// The tick and cross, heavy and light, as characters and as HTML entities.
const TYPED_MARK = /[✓✔✗✘]|&(?:#10003|#10004|#10007|#10008|#x2713|#x2714|#x2717|#x2718|check|cross);/;

describe('a plan panel shows its result in words, never as a typed mark', () => {
	const files = svelteFilesUnder(PLANS);

	it('reads every plan panel, sub-folders included', () => {
		expect(files.length).toBeGreaterThanOrEqual(50);
	});

	it.each(files)('%s has no typed tick or cross', (rel) => {
		// HTML comments are left out: a note that names the mark is not the mark.
		const src = readFileSync(join(PLANS, rel), 'utf8').replace(/<!--[\s\S]*?-->/g, '');
		expect(src, `a typed tick or cross in ${rel}`).not.toMatch(TYPED_MARK);
	});
});
