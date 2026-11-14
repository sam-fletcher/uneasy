<!--
  WarBand.svelte — one live war, as one line at the top of Play.

      [barbute] otto’s war on Wren · under way                 ›

  Each war that is planned or under way has one of these at the top of Play,
  because the war goes on across rows after its plan has resolved. Pressing it
  opens that war's panel as a sheet over Play (the sides, joining or staying
  out, and the entry payment).

  Red marks the war: the border, the name and the barbute. The name spells out
  "war", so the red is never the only cue. The fill is the ordinary raised
  surface. The band is a button, so it is drawn like one (the control corner, a
  44px height) and ends in the arrow, which means "opens something", not the
  caret, which means "expands here".
-->
<script lang="ts">
	import type { Plan, Player } from '$lib/api';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';
	import { warName, warStatusWord } from '$lib/warText';

	let { plan, players, onOpen }: {
		plan: Plan;
		players: Player[];
		onOpen: () => void;
	} = $props();
</script>

<button type="button" class="war-band" aria-haspopup="dialog" onclick={onOpen}>
	<BarbuteMark size={16} />
	<span class="war-band-name">{warName(plan, players)}</span>
	<span class="war-band-status">· {warStatusWord(plan)}</span>
	<RowMark kind="arrow" />
</button>

<style>
	.war-band {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		width: 100%;
		min-height: var(--size-tap);
		padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
		background: var(--color-surface);
		border: 1px solid var(--color-danger);
		border-radius: var(--radius-control);
		color: var(--color-text-secondary);
		font: inherit;
		font-size: var(--text-small);
		text-align: left;
		cursor: pointer;
	}
	.war-band:hover { background: var(--color-surface-2); }
	.war-band:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}

	.war-band-name {
		min-width: 0;
		color: var(--color-danger);
		overflow-wrap: anywhere;
	}
	/* The status takes the spare width, so the arrow stays at the far end. */
	.war-band-status {
		flex: 1;
		min-width: 0;
	}
</style>
