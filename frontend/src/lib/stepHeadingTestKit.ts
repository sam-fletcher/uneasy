// Helpers for tests that need a step heading: one built by hand, and what a built one reads
// as once StepHeading.svelte draws it. Real headings come from stepHeading() (lib/stepHeading.ts).

import { render } from 'svelte/server';
import StepHeading from '$lib/components/shared/StepHeading.svelte';
import { capitalise, type StepHeading as StepHeadingData } from '$lib/stepHeading';

/** "Your move" or "Waiting on wren" over the step. */
export function aHeading(label: string, over: Partial<StepHeadingData> = {}): StepHeadingData {
	const yours = over.yours ?? true;
	return { label: capitalise(label), yours, who: yours ? [] : [{ text: 'wren' }], more: false, ...over };
}

/** The heading as the page reads it, from the DOM the component draws:
 *  "Your move · also otto. The delay reveal". Markup and comments dropped, whitespace
 *  collapsed, and the screen-reader-only full stop closed up against the name before it. */
export function readHeading(heading: StepHeadingData | null): string {
	return render(StepHeading, { props: { heading } }).body
		.replace(/<!--[\s\S]*?-->/g, '')
		.replace(/<[^>]+>/g, '')
		.replace(/\s+/g, ' ')
		.replace(/ ([.,])/g, '$1')
		.trim();
}
