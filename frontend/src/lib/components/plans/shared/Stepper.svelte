<!-- shared/Stepper.svelte
  A labelled −/+ pick-count row, shared by every plan whose picker lets a
  player take an option several times (Seek Answers, Spread Rumors, Spread
  Propaganda). It owns the markup and the 44px tap targets; the parent owns
  the numbers — it passes in `count` and the two disabled flags (the cap and
  quota rules differ per plan) and handles `onstep(±1)`. The row's label is
  the children snippet, so a plan can append its own notes.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		count,
		onstep,
		decDisabled = count === 0,
		incDisabled = false,
		dimmed = false,
		children,
	}: {
		count: number;
		onstep: (delta: 1 | -1) => void;
		decDisabled?: boolean;
		incDisabled?: boolean;
		/** The option can't be taken at all (no targets, declined): fade the row. */
		dimmed?: boolean;
		children: Snippet;
	} = $props();
</script>

<div class="stepper-row" class:dimmed>
	<button type="button" class="action-btn" aria-label="Fewer" onclick={() => onstep(-1)} disabled={decDisabled}>−</button>
	<strong class="stepper-count">{count}</strong>
	<button type="button" class="action-btn" aria-label="More" onclick={() => onstep(1)} disabled={incDisabled}>+</button>
	<span>{@render children()}</span>
</div>

<style>
	.stepper-row {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		font-size: 0.82rem;
		color: var(--color-text-secondary);
		line-height: 1.4;
	}
	/* The −/+ buttons are .action-btn (44px tall); a bare glyph is only ~39px
	   wide, so give them a square 44px target too. */
	.stepper-row :global(.action-btn) { min-width: 44px; }
	.stepper-row.dimmed { opacity: 0.55; }
	.stepper-count { min-width: 1.5rem; text-align: center; }
</style>
