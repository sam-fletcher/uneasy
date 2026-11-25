<!--
  PlayerNames.svelte — several players' names in a sentence, each in its own
  colour and joined with commas ("alice, bob, carol"). Each name is a
  <PlayerName>, so the same rules apply (no shape in a sentence; a control's
  colour wins inside a button).
-->
<script lang="ts">
	import type { Player } from '$lib/api';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';

	interface Props {
		players: Pick<Player, 'id' | 'display_name' | 'token_color' | 'seat_order'>[];
		ids: (number | null)[];
	}

	const { players, ids }: Props = $props();
</script>

{#each ids as id, i (i)}{#if i > 0}{', '}{/if}<PlayerName {players} {id} />{/each}
