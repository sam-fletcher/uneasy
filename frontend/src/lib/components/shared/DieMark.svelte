<!--
  DieMark.svelte — the die. A rounded square with two pips.

  Stands for a die that has not been rolled yet, in the roll's step, and
  anywhere else the game says "a die" without a face to show. It replaces the
  🎲 emoji, which drew in a different font on every platform; a mark has to
  render from something the app ships.

  It is the `roll` family in LogMark.svelte (the chat log's die), borrowed by
  noun, so the roll step and the log draw the same die. The pip count is a
  legibility choice, never a value.

  Colour is inherited: a die in its tile wears the faint text colour, and the
  tile's edge (not this mark) carries the owner's colour. Dice carry no player
  shape for now.
-->
<script lang="ts">
	import LogMark from '../LogMark.svelte';

	interface Props {
		/** Edge length in px. */
		size?: number;
		/** Accessible name, when no words beside the mark say it. */
		label?: string;
	}

	const { size = 20, label }: Props = $props();
</script>

<span
	class="die-mark"
	style:width="{size}px"
	style:height="{size}px"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<LogMark family="roll" />
</span>

<style>
	.die-mark {
		display: inline-flex;
		flex: none;
	}
</style>
