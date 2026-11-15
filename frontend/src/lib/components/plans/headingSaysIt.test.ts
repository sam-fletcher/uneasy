import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { join, relative } from 'node:path';

// The open step's heading says who the game is waiting on and what for ("Waiting on
// otto — place the addendum"), and the roll is its own step with its own heading.
// So a plan panel prints no line that says the same thing a second time:
//
//   - "Dice roll in progress…" (the roll is the open step, and says so);
//   - "otto is resolving Seek Answers…" (the heading names otto and the step);
//   - the old "Recording the result…" and "— resolve above".
//
// A source guard, like the legacy-markup one in planPanelsB.test.ts: a panel that
// grows one of these lines back fails here, naming the file. A line that says MORE
// than the heading ("Waiting for sol to agree to give up Dame Orr…" names the
// asset) is allowed; those are not on this list.

const HERE = __dirname;

function svelteFiles(dir: string): string[] {
	return readdirSync(dir).flatMap((name) => {
		const path = join(dir, name);
		if (statSync(path).isDirectory()) return svelteFiles(path);
		return name.endsWith('.svelte') ? [path] : [];
	});
}

/** The visible text of a .svelte file: markup only, no script, style or comments. */
function markupOf(path: string): string {
	return readFileSync(path, 'utf8')
		.replace(/<script\b[^>]*>[\s\S]*?<\/script>/gi, '')
		.replace(/<style\b[^>]*>[\s\S]*?<\/style>/gi, '')
		.replace(/<!--[\s\S]*?-->/g, '');
}

const REPEATS: Array<[string, RegExp]> = [
	['a dice-roll wait', /Dice roll in progress|resolve (?:the dice )?above|Rolling…/],
	['an "is resolving" wait', /\bis resolving\b/],
	['the decree\'s "Recording the result" wait', /Recording the result/],
];

describe('no plan panel repeats what the open step\'s heading says', () => {
	const files = svelteFiles(HERE).map(f => relative(HERE, f));

	it('finds the panels (the guard is not vacuous)', () => {
		expect(files.length).toBeGreaterThan(30);
		expect(files).toContain('SeekAnswersPanel.svelte');
		expect(files).toContain('duel/RollPhase.svelte');
		expect(files).toContain('festivity/SocializingTurn.svelte');
	});

	it.each(REPEATS)('%s is printed nowhere', (_what, pattern) => {
		const offenders = files.filter(f => pattern.test(markupOf(join(HERE, f))));
		expect(offenders).toEqual([]);
	});
});
