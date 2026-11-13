<!--
  WarEntry.svelte — the way into the wars, at the top of Play.

  A stopgap. The War button used to sit in the page header; the header is now
  just Home, the title and Help, so the same button moves to the top of Play
  unchanged. The Play and war pieces replace it with a one-line band for each war
  and the war's own panel, and this file goes with them.

  It opens the war drawer (a sheet over Play, owned by MainEventView). The
  button's tint comes from two counts MainEventView publishes: red once any war
  is under way, the warning colour while every war is only planned, and the
  number when there is more than one.
-->
<script lang="ts">
	import { warDrawerOpen, activeWarCount, pendingWarCount } from '$lib/warDrawer';

	const total = $derived($activeWarCount + $pendingWarCount);
</script>

{#if total > 0}
	<div class="war-entry">
		<button
			type="button"
			class="war-button"
			class:pending={$activeWarCount === 0}
			onclick={() => warDrawerOpen.set(true)}
			aria-label="Open wars"
		>
			<span class="lbl">War</span>{#if total > 1}<span class="count">{total}</span>{/if}
		</button>
	</div>
{/if}

<style>
	.war-entry {
		flex: none;
		padding: var(--space-2) var(--space-2) 0;
	}

	.war-button {
		display: inline-flex;
		align-items: center;
		min-height: var(--size-tap);
		padding: 0;
		font-family: var(--font-serif);
		font-size: var(--text-small);
		font-weight: 400;
		background: var(--color-chip-red-bg);
		color: var(--color-chip-red-text);
		border: 1px solid var(--color-chip-red-border);
		border-radius: var(--radius-piece);
		cursor: pointer;
	}
	.war-button .lbl { padding: var(--space-1) var(--space-3); }
	.war-button:hover { background: color-mix(in srgb, var(--color-chip-red-bg) 92%, white); }
	.war-button:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }

	.war-button .count {
		align-self: stretch;
		display: flex;
		align-items: center;
		padding: 0 var(--space-2);
		border-left: 1px solid var(--color-chip-red-border);
		font-size: var(--text-label);
		font-variant-numeric: tabular-nums;
	}

	/* Only planned wars: the warning colour instead of red. Two states in all —
	   red if any war is active, warning otherwise; the drawer carries the
	   active/pending breakdown. */
	.war-button.pending {
		background: var(--color-warning-bg);
		border-color: var(--color-warning-border);
		color: var(--color-warning);
	}
	.war-button.pending .count { border-left-color: var(--color-warning-border); }
	.war-button.pending:hover { background: color-mix(in srgb, var(--color-warning-bg) 92%, white); }
</style>
