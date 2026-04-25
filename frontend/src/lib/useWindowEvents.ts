// Subscribe to a set of `uneasy:*` window events for the lifetime of the
// calling component. Pass event names without the `uneasy:` prefix.
import { onMount, onDestroy } from 'svelte';

export function useWindowEvents(events: readonly string[], handler: (e: Event) => void) {
	const fullNames = events.map(e => `uneasy:${e}`);
	onMount(() => {
		for (const name of fullNames) window.addEventListener(name, handler);
	});
	onDestroy(() => {
		for (const name of fullNames) window.removeEventListener(name, handler);
	});
}
