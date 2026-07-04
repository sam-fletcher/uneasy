<!--
  Self-contained "?" button + How to play panel. Drop in anywhere (site
  header, a page's own corner) — it owns its open state and needs no props.
-->
<script lang="ts">
	import RetinueSheet from './RetinueSheet.svelte';
	import HelpContent from './HelpContent.svelte';

	let open = $state(false);
</script>

<button type="button" class="help-button" onclick={() => open = true} aria-label="How to play & feedback">
	<svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
		<circle cx="12" cy="12" r="10" />
		<path d="M9.5 9a2.5 2.5 0 0 1 4.5 1.5c0 1.5-2 2-2 3" />
		<line x1="12" y1="17" x2="12" y2="17" />
	</svg>
</button>

<RetinueSheet open={open} onClose={() => open = false}>
	<div class="help-sheet">
		<h3>How to play</h3>
		<HelpContent panel />
	</div>
</RetinueSheet>

<style>
	.help-button {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 44px;
		height: 44px;
		flex-shrink: 0;
		color: var(--color-accent);
		background: none;
		border: none;
		border-radius: 6px;
		cursor: pointer;
	}
	.help-button:hover { color: var(--color-accent-hover); background: var(--color-surface-2); }
	.help-button:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }
	.help-button svg { width: 24px; height: 24px; flex-shrink: 0; }

	.help-sheet h3 { margin: 0 0 0.75rem; }
</style>
