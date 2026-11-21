/**
 * What the e2e guard (tests/e2e/fixtures.ts) says when it fails a test.
 *
 * Plain functions with no Playwright in them, so that src/lib/e2eFixtures.test.ts
 * (part of `make check-fast`) can pin what a person reads in a red test.
 */
import { BOUNDARY_ERROR_PREFIX } from '../../src/lib/boundaryError';

/** What a page did that fails the test, and which of the three it was. */
export type Kind = 'boundary' | 'uncaught' | 'console.error';

/** One thing a page logged or threw: where the page was, and what it said. */
export type Caught = { kind: Kind; url: string; text: string };

/** How many distinct lines each section of the failure message spells out (the rest are counted). */
const SHOWN = 3;

/** What each kind is, in the words the person reading a red test needs. */
const HEADLINE: Record<Kind, string[]> = {
	boundary: [
		`The table's error boundary caught a render crash during this test (${BOUNDARY_ERROR_PREFIX}).`,
		'The player saw the fallback instead of the table, whatever the assertions above say.',
	],
	uncaught: [
		'A page threw an error that nothing caught (an event handler, an $effect, a promise nobody awaited) during this test.',
		'No boundary showed a fallback, so the assertions above may pass over whatever it left half done.',
	],
	'console.error': [
		'A page logged a console.error during this test that tests/e2e/allowedConsoleErrors.ts does not name.',
		'Fix what logs it. If the app is meant to (a spec that fakes a server failure, say), add a narrow entry there:',
		'this spec, a pattern for the line, and why.',
	],
};

/** The failure message: for each kind that happened, the page and what it logged or threw, with its stack. */
export function failureMessage(caught: Caught[]): string {
	const sections: string[] = [];
	for (const kind of Object.keys(HEADLINE) as Kind[]) {
		const distinct = new Map<string, Caught>();
		for (const c of caught) if (c.kind === kind && !distinct.has(`${c.url}\n${c.text}`)) distinct.set(`${c.url}\n${c.text}`, c);
		const all = [...distinct.values()];
		if (all.length === 0) continue;
		const lines = all.slice(0, SHOWN).map(c => `  page: ${c.url}\n  ${c.text.replaceAll('\n', '\n  ')}`);
		if (all.length > SHOWN) lines.push(`  … and ${all.length - SHOWN} more`);
		sections.push([...HEADLINE[kind], '', ...lines].join('\n'));
	}
	return sections.join('\n\n');
}

/**
 * What to show for an uncaught exception. Playwright hands over an Error whose
 * `stack` is "" when the page threw something that is not an Error (`throw 'x'`,
 * a rejection with a string or an object): the value is then in `message`, as
 * a string, and an object arrives as just "Object". A bare `stack ?? message`
 * would print a blank line for all of them and, since the failure message lists
 * each distinct line once, fold every such throw on a page into one.
 */
export function uncaughtText(error: { stack?: string; message: string }): string {
	return error.stack || `Uncaught: ${error.message} (thrown value that is not an Error, so there is no stack)`;
}
