<!--
  ShellColumn.svelte — one of the table's three panes, as a column.

  A column is laid out as a phone's whole screen would be: an optional name bar
  at the top, the pane's body, and an optional foot (the Public Record's chips, the pane tabs). The page
  gives every pane a column and says which are shown; a pane that isn't shown
  stays laid out but invisible, so its scroll position and state survive a trip
  to another tab (the chat keeps its place in the feed).

  The column is also what a sheet covers. It provides itself through context
  ($lib/sheetHost), so a sheet opened from anywhere inside it (Tones from the
  chat header, the war's panel from Play) rises over this column only.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import { provideSheetHost } from '$lib/sheetHost';
	import type { PaneId } from './panes';

	interface Props {
		pane: PaneId;
		/** The pane's accessible name. */
		label: string;
		/** False while another pane has the screen. */
		shown: boolean;
		/** The column's element, for sheets to measure. */
		el?: HTMLElement | null;
		head?: Snippet;
		foot?: Snippet;
		children: Snippet;
	}

	let { pane, label, shown, el = $bindable(null), head, foot, children }: Props = $props();

	// The record and the chat are reference beside the game, so a screen reader
	// finds them as complementary landmarks; Play is the game itself.
	const tag = $derived(pane === 'play' ? 'section' : 'aside');

	provideSheetHost(() => el);
</script>

<svelte:element
	this={tag}
	class="shell-column"
	data-pane={pane}
	data-shown={shown}
	aria-label={label}
	bind:this={el}
>
	{#if head}
		<div class="column-head">{@render head()}</div>
	{/if}
	<div class="column-body">{@render children()}</div>
	{#if foot}
		<div class="column-foot">{@render foot()}</div>
	{/if}
</svelte:element>

<style>
	.shell-column {
		position: relative;
		display: flex;
		flex-direction: column;
		min-width: 0;
		min-height: 0;
		/* The grid the page puts columns in gives every column the full row
		   height; what doesn't fit scrolls inside the body. */
		height: 100%;
	}

	/* Not on screen, but still laid out. visibility (not display) keeps the
	   scroll position, and takes the pane out of the tab order and the
	   accessibility tree. */
	.shell-column[data-shown='false'] {
		visibility: hidden;
	}

	.column-head {
		flex: none;
		display: flex;
		align-items: center;
		min-height: var(--layout-column-head);
		padding: 0 var(--space-2);
		border-bottom: 1px solid var(--color-border);
	}

	.column-body {
		flex: 1;
		min-height: 0;
		display: flex;
		flex-direction: column;
	}

	.column-foot {
		flex: none;
	}
</style>
