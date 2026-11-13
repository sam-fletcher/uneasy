<!--
  RecordTabs.svelte — Events, Laws and Rumors: what the Public Record pane
  shows, as three chips at the pane's foot.

  They sit at the foot, above the pane tabs, so picking the pane and then picking
  its list are two taps in the same part of the screen, not one at the bottom and
  one at the top. They stay at the foot of the column at every width, because a
  column is laid out the same whatever the window's width. The chosen chip wears
  the selected state (Chip kind="tab").

  Laws and Rumors carry the pale "new" dot while they hold something you haven't
  seen (an edit counts), and the Public Record tab above carries it too. The dot
  clears when you open that chip. It carries no count; the lists hold the totals.
-->
<script lang="ts">
	import Chip from '$lib/components/shared/Chip.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import type { RecordTab } from './panes';

	interface Props {
		current: RecordTab;
		/** Which lists hold something new. Events never does. */
		lawsNew: boolean;
		rumorsNew: boolean;
		onSelect: (tab: RecordTab) => void;
	}

	const { current, lawsNew, rumorsNew, onSelect }: Props = $props();

	const tabs = $derived<{ id: RecordTab; label: string; marked: boolean; dot: string }[]>([
		{ id: 'events', label: 'Events', marked: false, dot: '' },
		{ id: 'laws', label: 'Laws', marked: lawsNew, dot: 'new laws' },
		{ id: 'rumors', label: 'Rumors', marked: rumorsNew, dot: 'new rumors' },
	]);
</script>

<nav class="record-tabs" aria-label="Public Record">
	{#each tabs as tab (tab.id)}
		<Chip kind="tab" compact selected={current === tab.id} onclick={() => onSelect(tab.id)}>
			<span>{tab.label}</span>
			{#if tab.marked}<NewDot label={tab.dot} />{/if}
		</Chip>
	{/each}
</nav>

<style>
	/* Three equal chips across the column. */
	.record-tabs {
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		gap: var(--space-2);
		flex: none;
		padding: var(--space-2);
		border-top: 1px solid var(--color-border);
		background: var(--color-bg);
	}
</style>
