<!--
  PaneName.svelte — a column's name, in small capitals, with the marks its tab
  would carry.

  From three columns, Public Record, Play and Chat each start with their name,
  since no tab names them there; from two columns only Chat does, because the
  left column's two tabs name the other two. On a phone the tab bar does it all
  and Chat's header carries no name. The marks are the same ones as the tabs
  (PaneMarks), so a mark means one thing at every width.
-->
<script lang="ts">
	import PaneMarks from './PaneMarks.svelte';
	import { PANE_LABEL, type PaneId } from './panes';

	interface Props {
		pane: PaneId;
		marked: boolean;
	}

	const { pane, marked }: Props = $props();
</script>

<h2 class="pane-name">
	<span>{PANE_LABEL[pane]}</span>
	<PaneMarks {pane} {marked} />
</h2>

<style>
	.pane-name {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		margin: 0;
		font-size: var(--text-label);
		font-weight: 400;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}
</style>
