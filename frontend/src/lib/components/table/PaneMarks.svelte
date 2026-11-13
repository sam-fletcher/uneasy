<!--
  PaneMarks.svelte — the marks a pane's tab or column name carries.

  Two marks, each with one meaning (docs/STYLE_GUIDE.md "Marks"):
      the gold clock   on Play: the game is waiting on YOU. Only ever you: someone
                       is almost always on the clock, so lighting it for anyone
                       would say nothing.
      the pale dot     on Public Record and Chat: something is new since you looked.

  Pictures, not counts: the lists hold their own totals and the chat's divider
  holds its unread count. The tab and the column name both render this, so the
  two can't disagree.
-->
<script lang="ts">
	import ClockMark from '$lib/components/shared/ClockMark.svelte';
	import NewDot from '$lib/components/shared/NewDot.svelte';
	import type { PaneId } from './panes';

	interface Props {
		pane: PaneId;
		/** True when this pane has its mark to show. */
		marked: boolean;
	}

	const { pane, marked }: Props = $props();
</script>

{#if marked}
	{#if pane === 'play'}
		<ClockMark tone="you" label="your move" />
	{:else if pane === 'record'}
		<NewDot label="new laws or rumors" />
	{:else}
		<NewDot label="new messages" />
	{/if}
{/if}
