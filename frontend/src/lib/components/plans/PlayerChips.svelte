<!-- PlayerChips.svelte
  Row of selectable player buttons used across plan prep forms to replace
  player dropdowns. Each chip carries the player's token colour as its
  accent so the colour↔player mapping reads consistently with the rest of
  the table (asset cards, chat author colours, etc).

  Selection mode is decided by the caller: pass `isActive` to mark which
  chips look selected (single or multi), and handle the toggle yourself
  in `onSelect`. Chip styles live in planPanel.css so this component is
  pure markup.
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import { playerColor } from '$lib/playerColor';

	interface Props {
		players: Player[];
		isActive: (p: Player) => boolean;
		onSelect: (p: Player) => void;
	}

	let { players, isActive, onSelect }: Props = $props();
</script>

<div class="player-chips">
	{#each players as p (p.id)}
		{@const c = playerColor(p)}
		<button
			type="button"
			class="player-chip"
			class:active={isActive(p)}
			style:--owner-color={c}
			onclick={() => onSelect(p)}
		>
			<span class="dot" aria-hidden="true"></span>
			<span class="name">{p.display_name}</span>
		</button>
	{/each}
</div>
