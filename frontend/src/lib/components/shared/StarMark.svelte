<!--
  StarMark.svelte — the main character's star.

  Filled when a peer IS the main character; an outline when it is the control
  that would make one (RetinueView's "Make main character" toggle). `★` means
  only the main character, app-wide, and no player's shape is ever a star, so
  the two can't be confused.

  Drawn from a path and not typed as ★: the character came from a different
  font on every platform. The outline is the house line idiom (24x24, 2-unit
  stroke, round ends); the filled star is the same path painted solid.

  Colour is inherited (deep gold, the game's colour, where the card sets it).
  Decoration unless given a `label`; put the words beside it where there is room.
-->
<script lang="ts">
	interface Props {
		/** Edge length in px. */
		size?: number;
		/** An outline (the "make main character" control) instead of the solid star. */
		outline?: boolean;
		/** Accessible name, when no words beside the mark say it. */
		label?: string;
	}

	const { size = 16, outline = false, label }: Props = $props();
</script>

<svg
	class="star-mark"
	viewBox="0 0 24 24"
	width={size}
	height={size}
	fill={outline ? 'none' : 'currentColor'}
	stroke={outline ? 'currentColor' : 'none'}
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	<path d="M12 17.75l-6.172 3.245l1.179 -6.873l-5 -4.867l6.9 -1l3.086 -6.253l3.086 6.253l6.9 1l-5 4.867l1.179 6.873z" />
</svg>

<style>
	.star-mark {
		display: block;
		flex: none;
	}
</style>
