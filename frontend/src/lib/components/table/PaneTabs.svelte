<!--
  PaneTabs.svelte — the tab bar at the foot of a pane.

  Below two columns it has all three tabs (Public Record, Play, Chat) and sits
  at the foot of whichever pane is showing, under the chat's message box. From
  two columns up it has two tabs, Public Record and Play, at the foot of the left
  column (Chat is docked on the right and needs none). From three columns up
  there are none: every pane is on screen.

  Each tab carries its pane's marks (PaneMarks), and the chosen one wears the
  "you" gold: a line along its top and gold text. The bar stays on the bottom edge
  at every width, so it doesn't move when a tablet is turned.
-->
<script lang="ts">
	import PaneMarks from './PaneMarks.svelte';
	import { PANE_LABEL, type PaneId, type PaneMarkState } from './panes';

	interface Props {
		/** The tabs to draw, in order. */
		panes: readonly PaneId[];
		current: PaneId;
		marks: PaneMarkState;
		onSelect: (pane: PaneId) => void;
	}

	const { panes, current, marks, onSelect }: Props = $props();
</script>

<nav class="pane-tabs" aria-label="Panes">
	{#each panes as pane (pane)}
		<button
			type="button"
			class="pane-tab"
			data-pane={pane}
			class:current={pane === current}
			aria-current={pane === current ? 'page' : undefined}
			onclick={() => onSelect(pane)}
		>
			<span>{PANE_LABEL[pane]}</span>
			<PaneMarks {pane} marked={marks[pane]} />
		</button>
	{/each}
</nav>

<style>
	.pane-tabs {
		display: grid;
		grid-auto-flow: column;
		grid-auto-columns: minmax(0, 1fr);
		flex: none;
		border-top: 1px solid var(--color-border);
		background: var(--color-bg);
		/* The phone's home indicator sits under the bar, not over it. */
		padding-bottom: env(safe-area-inset-bottom);
	}

	.pane-tab {
		display: flex;
		align-items: center;
		justify-content: center;
		gap: var(--space-2);
		min-height: var(--layout-tab-bar);
		padding: 0;
		background: none;
		border: 0;
		border-radius: 0;
		font: inherit;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		cursor: pointer;
	}

	/* The line is an inset shadow, so choosing a tab never moves the bar's
	   own 1px rule or the text below it. */
	.pane-tab.current {
		color: var(--color-you);
		box-shadow: inset 0 2px 0 var(--color-you);
	}

	.pane-tab:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: -2px;
	}
</style>
