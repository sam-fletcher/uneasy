<!--
  PlayerShape.svelte — a player's shape: the half of their identity a
  colour-blind player can read.

  Draw it wherever a colour stands for a player and no name is nearby (a plan
  card's preparer, a record line, an owner mark on a card), and beside every
  name in the player row so the key is always on screen. It replaces the old
  colour dot rather than joining it. A byline, where the name is the label,
  doesn't need one.

  The five outlines, in seat order, live in `$lib/playerShape.ts` next to the
  palette: seat 1 is always the Lavender diamond, seat 2 the Peridot triangle,
  and so on. Colour and shape both come from the seat, so pass the Player (or
  just a `seat`) and never choose either by hand.

  Accessibility: with a name nearby the shape is decoration and is hidden
  from a screen reader (the default). Standing alone, pass `label` (the
  player's name) and it becomes an image with that text.

  Sizing: 12px is the smallest it should be drawn, and the default; the
  outlines were tuned to look equally heavy at that size. The shape is a
  filled outline on a 16x16 grid.
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { playerShapeFor } from '$lib/playerShape';

	interface Props {
		/** Whose shape. Takes their seat for the outline and their colour for the paint. */
		player?: Pick<Player, 'token_color' | 'seat_order'> | null;
		/** A bare seat order (1-based), when you have no Player to hand. */
		seat?: number | null;
		/** Edge length in px. Never below 12. */
		size?: number;
		/**
		 * Paint override. Defaults to the player's colour; pass "currentColor"
		 * to take the surrounding text colour instead.
		 */
		color?: string;
		/** The player's name, when nothing else beside the shape says it. */
		label?: string;
	}

	const { player = null, seat = null, size = 12, color, label }: Props = $props();

	const spec = $derived(playerShapeFor(player ? player.seat_order : seat));
	const fill = $derived(
		color ?? playerColor(player ?? { token_color: null, seat_order: seat })
	);
</script>

<svg
	class="player-shape"
	viewBox="0 0 16 16"
	width={size}
	height={size}
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
	data-shape={spec.name}
>
	<path d={spec.path} {fill} />
</svg>

<style>
	/* Block, so an inline svg's line-box leading never adds height to the row
	   it sits in; and never shrunk by a flex parent. */
	.player-shape {
		display: block;
		flex: none;
	}
</style>
