<!--
  plans/shared/DieTile.svelte — one die on the table.

  A die is a game piece, so it is a small square tile: the piece corner, the
  raised surface, and a neutral line of text inside. Its EDGE is the owner's
  colour, which is how you tell whose die it is (the group it stands in, "For"
  or "Against", says which side it is on). Dice carry no player shape for now,
  and the log names whoever committed each one, so a colour-blind player can
  still find out.

  Before it is rolled it shows the die mark (the house mark, in place of the
  emoji it used to be); after, its face. A die that isn't counting is not
  faded (a fade would take its number under the contrast floor): a cancelled
  die has a dashed edge and a struck-through face, and one that repeats a face
  already counted has a dashed edge and faint text. The accessible name says
  the same in words.
-->
<script lang="ts">
	import DieMark from '$lib/components/shared/DieMark.svelte';

	interface Props {
		/** The face showing, or null while the die has not been rolled. */
		face: number | null;
		/** The owner's colour: the tile's edge. */
		color: string;
		/** Whose die, and why it counts or doesn't: the tile's accessible name. */
		label: string;
		/** Cancelled by an interfering die. */
		cancelled?: boolean;
		/** A repeat of a face already counted, so it adds nothing to the score. */
		duplicate?: boolean;
	}

	const { face, color, label, cancelled = false, duplicate = false }: Props = $props();
</script>

<span
	class="die-tile"
	class:cancelled
	class:duplicate
	style:border-color={color}
	role="img"
	aria-label={label}
	title={label}
>
	{#if face == null}
		<DieMark size={20} />
	{:else}
		<span class="face">{face}</span>
	{/if}
</span>

<style>
	.die-tile {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex: none;
		box-sizing: border-box;
		width: var(--size-tap);
		height: var(--size-tap);
		border: 1px solid;
		border-radius: var(--radius-piece);
		background: var(--color-surface);
		color: var(--color-text-faint);
	}
	/* A rolled die reads as a number, the one place a value is bold. */
	.face {
		font-size: var(--text-body);
		font-weight: 600;
		color: var(--color-text);
	}
	.die-tile.cancelled { border-style: dashed; }
	.die-tile.cancelled .face {
		text-decoration: line-through;
		color: var(--color-text-faint);
	}
	.die-tile.duplicate { border-style: dashed; }
	.die-tile.duplicate .face { color: var(--color-text-faint); }
</style>
