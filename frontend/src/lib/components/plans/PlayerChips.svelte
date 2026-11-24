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
  neutral.

  Two different "can't press" cases, told apart on purpose:
    - `isDisabled`: this player genuinely can't be picked. The chip is greyed
      out and faint throughout, its shape and name too (it gets `unavailable`,
      which inlineText.css reads).
    - `readOnly`: a mirror of someone else's choice. The chips are inert but
      shown as they are, not faded, so the name and shape keep the player's
      colour. (The chips are `disabled` for the browser, but the faint rule
      isn't keyed on that.)
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import Chip from '$lib/components/shared/Chip.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';

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
	const unavailable = (p: Player) => isDisabled?.(p) ?? false;
</script>

<div class="player-chips">
	{#each players as p (p.id)}
		<Chip
			selected={isActive(p)}
			disabled={readOnly || unavailable(p)}
			class={unavailable(p) ? 'unavailable' : undefined}
			onclick={() => onSelect(p)}
		>
			<PlayerName player={p} label fixed />
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
