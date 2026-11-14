<!--
  WarName.svelte — a war's name where no band names it.

      [barbute] otto’s war on Wren

  The band at the top of Play names every war that is planned or under way,
  and one band above one panel is enough. This line is for the panels the
  band does not cover: the delay vote (the war has no row yet, so no band), a
  surrender claim left over from a war that has just ended, and the play area
  when two wars owe a cost at once and each panel must say which war it is.
  Same mark and colour as the band, but it is only a label, so it is not drawn
  as a control.
-->
<script lang="ts">
	import type { Plan, Player } from '$lib/api';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import { warName } from '$lib/warText';

	let { plan, players }: { plan: Plan; players: Player[] } = $props();
</script>

<p class="war-name">
	<BarbuteMark size={16} />
	<span>{warName(plan, players)}</span>
</p>

<style>
	.war-name {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-danger);
	}
</style>
