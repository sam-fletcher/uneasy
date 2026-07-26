<script lang="ts">
	// Site-level notice that this tab is running a build the server has since
	// replaced. See $lib/appVersion for how the staleness is detected and why
	// it's event-driven rather than polled.
	//
	// Deliberately not a forced reload: this is a play-by-post game and a
	// player may be mid-sentence in a scene description. The bar states the
	// problem and hands them the trigger.
	import { updated } from '$app/state';
	import './shared/actionButton.css';

	let dismissed = $state(false);

	const show = $derived(updated.current && !dismissed);

	// No height plumbing here on purpose. The table route needs to give up
	// exactly the room this bar takes (its shell is viewport-height, with the
	// chat strip pinned to the bottom edge), and the obvious fix — measure the
	// bar, publish the number, subtract it — is a trap: the measurement is a
	// frame behind on every reflow, so the copy re-wrapping at a different
	// width silently desyncs the shell. +layout.svelte makes the two flex
	// siblings of one viewport-height column instead, which the browser keeps
	// exact for free.
</script>

{#if show}
	<div class="update-banner" role="status">
		<p>An update is available.</p>
		<div class="actions">
			<button class="action-btn primary" onclick={() => location.reload()}>Reload</button>
			<button class="action-btn secondary" onclick={() => (dismissed = true)}>Later</button>
		</div>
	</div>
{/if}

<style>
	.update-banner {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0.5rem 1rem;
		padding: 0.5rem 1rem;
		background: var(--color-surface-sunken);
		border-bottom: 1px solid var(--color-border-warm);
	}

	/* Basis auto, so the message asks for exactly the width it needs and sits
	   on the buttons' line — the bar is one button tall (~60px) rather than
	   three stacked rows. It may still grow into slack space (pushing the
	   actions right) or shrink, and min-width:0 lets it shrink below its
	   content instead of forcing an overflow. Fluid, not a breakpoint: if the
	   message and the buttons genuinely can't share a line — a very narrow
	   phone, or longer copy later — flex-wrap drops the actions below rather
	   than squeezing them. */
	.update-banner p {
		flex: 1 1 auto;
		min-width: 0;
		font-size: 0.9rem;
		color: var(--color-text);
	}

	/* Never shrink: these are 44px tap targets and squeezing them to buy the
	   message a few pixels is the wrong trade. Wrapping is the release valve. */
	.actions {
		display: flex;
		flex: none;
		gap: 0.5rem;
		margin-left: auto;
	}
</style>
