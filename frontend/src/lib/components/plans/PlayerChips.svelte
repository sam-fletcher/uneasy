<!-- PlayerChips.svelte
  Row of selectable player buttons used across plan prep forms to replace
  player dropdowns. Each chip is the house <Chip>: the player's shape, then
  their name in their colour. (The shape is how a colour-blind player tells
  two chips apart; the name is in the same colour, so the chip identifies the
  player without making colour the only signal.)

  Selection mode is decided by the caller: pass `isActive` to mark which
  chips look selected (single or multi), and handle the toggle yourself
  in `onSelect`. A pick is the chip's bright gold border and gold text,
  never a player's colour: a player's colour only ever identifies the player,
  so the colour is on the name (a child of the chip) and the chip itself stays
  neutral. A chip that can't be pressed takes the chip's locked look and its
  name keeps the player's colour.
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import Chip from '$lib/components/shared/Chip.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

	interface Props {
		players: Player[];
		isActive: (p: Player) => boolean;
		onSelect: (p: Player) => void;
		/** When true, render selected chips but block clicks (used by
		 *  non-focus viewers mirroring the focus player's choices). */
		readOnly?: boolean;
		/** Marks chips that cannot be picked (the caller says why, nearby). */
		isDisabled?: (p: Player) => boolean;
	}

	let { players, isActive, onSelect, readOnly = false, isDisabled }: Props = $props();
</script>

<div class="player-chips">
	{#each players as p (p.id)}
		<Chip
			selected={isActive(p)}
			disabled={readOnly || (isDisabled?.(p) ?? false)}
			onclick={() => onSelect(p)}
		>
			<PlayerShape player={p} size={12} />
			<span style:color={playerColor(p)}>{p.display_name}</span>
		</Chip>
	{/each}
</div>

<style>
	.player-chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
