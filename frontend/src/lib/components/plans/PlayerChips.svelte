<!-- PlayerChips.svelte
  Row of selectable player buttons used across plan prep forms to replace
  player dropdowns. Each chip is the house chip: the player's shape, then
  their name in their colour. (The shape is how a colour-blind player tells
  two chips apart; the name is in the same colour, so the chip identifies the
  player without making colour the only signal.)

  Selection mode is decided by the caller: pass `isActive` to mark which
  chips look selected (single or multi), and handle the toggle yourself
  in `onSelect`. A pick is the chip's bright gold border and gold text,
  never a player's colour: a player's colour only ever identifies the player.
  Chip styles live in planPanel.css so this component is pure markup.
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
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
		<button
			type="button"
			class="player-chip"
			class:active={isActive(p)}
			aria-pressed={isActive(p)}
			style:--owner-color={playerColor(p)}
			disabled={readOnly || (isDisabled?.(p) ?? false)}
			onclick={() => onSelect(p)}
		>
			<PlayerShape player={p} size={12} />
			<span class="name">{p.display_name}</span>
		</button>
	{/each}
</div>
