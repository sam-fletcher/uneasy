<!--
  Self-contained "?" button + How to play panel. Drop in anywhere (site
  header, a page's own corner) — it owns its open state and needs no props.

  Its sheets rise over whatever column the surrounding page provides
  ($lib/sheetHost): on the table page that is Play (or the left column when
  there are two), so Help never darkens the chat. With no host they cover the
  viewport.
-->
<script lang="ts">
	import '$lib/components/shared/modalShell.css';
	import RetinueSheet from './RetinueSheet.svelte';
	import HelpContent from './HelpContent.svelte';
	import HelpGlyph from './HelpGlyph.svelte';
	import FeedbackForm from './FeedbackForm.svelte';

	// gameId/route/phase: in-game context to auto-attach to feedback sent from
	// here. Callers outside a table (the logged-out entry screen, the global
	// layout header) simply omit them.
	let { gameId, route, phase }: { gameId?: string; route?: string; phase?: string } = $props();

	let open = $state(false);
	let feedbackOpen = $state(false);
</script>

<button type="button" class="help-button" onclick={() => open = true} aria-label="How to play & feedback">
	<HelpGlyph size={20} />
</button>

<RetinueSheet open={open} onClose={() => open = false}>
	<div class="help-sheet">
		<h3 class="sheet-title">How to play</h3>
		<HelpContent panel onFeedback={() => { open = false; feedbackOpen = true; }} />
	</div>
</RetinueSheet>

<!-- Separate sheet, not nested inside the one above (see decision 10 in
     adr/FEEDBACK_AND_RESET_PLAN.md) — HelpContent's trigger closes `open`
     before this one opens. -->
<RetinueSheet open={feedbackOpen} onClose={() => feedbackOpen = false}>
	<div class="help-sheet">
		<h3 class="sheet-title">Send feedback</h3>
		<FeedbackForm {gameId} {route} {phase} />
	</div>
</RetinueSheet>

<style>
	.help-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: var(--size-tap);
		height: var(--size-tap);
		flex-shrink: 0;
		padding: 0;
		color: var(--color-text-secondary);
		background: none;
		border: none;
		border-radius: var(--radius-control);
		cursor: pointer;
	}
	.help-button:hover { color: var(--color-text); background: var(--color-surface); }
	.help-button:focus-visible { outline: 2px solid var(--color-you); outline-offset: 2px; }

</style>
