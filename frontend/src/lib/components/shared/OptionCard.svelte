<!--
  OptionCard.svelte — the house option card, as a button that knows whether it
  is selected.

  Use it for an option that needs more than a word: a make or mar option with
  its level and what it does, a demand to draft, a plan to aim at, a stake to
  play. A few short words are a <Chip>. The look is `optionCard.css` (read its
  header for the rules: a control, selected = the 2px bright gold border and a
  tick with no fill, locked = a hairline and faint words); this wrapper makes
  the state a sighted player sees the state a screen reader hears
  (aria-pressed), and lays the parts out once.

      title        the option's name
      badge        a signed level ("+1", "−2") before the text
      description  what it does, a line under the title
      reason       why it is locked, a line under the description
      aside        something at the end of the row: the duel's die, beside
                   the stake it belongs to

  Lock it with `disabled`, or with `aria-disabled` where a tap should explain
  why (the card's look follows the attribute); the caller decides which, and
  what a tap on a locked card does. Extra attributes (aria-label, data-*) and
  `onclick` pass straight through to the <button>.
-->
<script lang="ts">
	import '$lib/components/shared/optionCard.css';
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import ToneMark from '$lib/components/shared/ToneMark.svelte';

	interface Props extends Omit<HTMLButtonAttributes, 'children' | 'type' | 'title'> {
		title: string;
		selected?: boolean;
		badge?: string;
		description?: string;
		reason?: string;
		aside?: Snippet;
	}

	const {
		title,
		selected = false,
		badge,
		description,
		reason,
		aside,
		class: extra,
		...rest
	}: Props = $props();
</script>

<button
	type="button"
	class={['option-card', selected && 'selected', extra]}
	aria-pressed={selected}
	{...rest}
>
	{#if badge}<span class="option-badge">{badge}</span>{/if}
	<span class="option-text">
		<span class="option-title">{title}</span>
		{#if description}<span class="option-desc">{description}</span>{/if}
		{#if reason}<span class="option-desc">{reason}</span>{/if}
	</span>
	{#if aside}<span class="option-aside">{@render aside()}</span>{/if}
	{#if selected}
		<span class="option-picked"><ToneMark tone="success" label="Picked" /></span>
	{/if}
</button>
