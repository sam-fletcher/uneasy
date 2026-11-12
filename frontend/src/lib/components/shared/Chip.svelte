<!--
  Chip.svelte — the house chip, as a button that knows whether it is selected.

  Use it for choosing between a few things: Aid or Interfere, a player, Events
  or Laws or Rumors. The look is `chip.css` (read its header for the rules:
  an object, never inert, selected = a 2px bright gold border and bright gold
  text with no fill); this wrapper just makes sure the state a sighted player
  sees is also the state a screen reader hears.

      kind="choice"  (default) a toggle among alternatives: aria-pressed.
      kind="tab"     a chip standing in for a tab: aria-current="page" when
                     selected, so a screen reader announces where you are.

  `compact` is the smaller type for chips in a row of several. Extra
  attributes (aria-label, aria-controls, data-*) pass straight through to the
  <button>, as does `onclick`.
-->
<script lang="ts">
	import '$lib/components/shared/chip.css';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';

	interface Props extends Omit<HTMLButtonAttributes, 'children' | 'type'> {
		selected?: boolean;
		kind?: 'choice' | 'tab';
		compact?: boolean;
		children: Snippet;
	}

	const {
		selected = false,
		kind = 'choice',
		compact = false,
		children,
		class: extra,
		...rest
	}: Props = $props();
</script>

<button
	type="button"
	class={['chip', compact && 'compact', selected && 'selected', extra]}
	aria-pressed={kind === 'choice' ? selected : undefined}
	aria-current={kind === 'tab' && selected ? 'page' : undefined}
	{...rest}
>
	{@render children()}
</button>
