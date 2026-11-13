<!-- shared/Stepper.svelte
  A labelled −/+ pick-count row, shared by every plan whose picker lets a
  player take an option several times (Seek Answers, Spread Rumors, Spread
  Propaganda). It owns the markup and the 44px tap targets; the parent owns
  the numbers — it passes in `count` and the two disabled flags (the cap and
  quota rules differ per plan) and handles `onstep(±1)`. The row's label is
  the children snippet, so a plan can append its own notes.

  The Stepper does not clamp: it never changes `count` itself, and it never
  stops a tap at a cap — parents must pass `incDisabled` (and `decDisabled`
  when it isn't simply "at zero"). `label` names the option for screen readers,
  so the buttons read "Fewer Take Asset", not five identical "Fewer"s.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		count,
		label = '',
		onstep,
		decDisabled = count === 0,
		incDisabled = false,
		dimmed = false,
		children,
	}: {
		count: number;
		/** What is being counted, for the buttons' accessible names. */
		label?: string;
		onstep: (delta: 1 | -1) => void;
		decDisabled?: boolean;
		incDisabled?: boolean;
		/** The option can't be taken at all (no targets, declined): fade the row. */
		dimmed?: boolean;
		children: Snippet;
	} = $props();
</script>

<div class="stepper-row" class:dimmed>
	<button type="button" class="action-btn secondary" aria-label={label ? `Fewer ${label}` : 'Fewer'} onclick={() => onstep(-1)} disabled={decDisabled}>−</button>
	<strong class="stepper-count">{count}</strong>
	<button type="button" class="action-btn secondary" aria-label={label ? `More ${label}` : 'More'} onclick={() => onstep(1)} disabled={incDisabled}>+</button>
	<span>{@render children()}</span>
</div>

<style>
	.stepper-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-small);
		color: var(--color-text-secondary);
		line-height: 1.4;
	}
	/* The −/+ buttons are .action-btn (44px tall); a bare glyph is only ~39px
	   wide, so give them a square 44px target too. */
	.stepper-row :global(.action-btn) { min-width: var(--size-tap); }
	/* An option that can't be taken at all: its words go faint, but the row is not
	   faded (a fade would take them under the contrast floor), and its buttons
	   already look unavailable. */
	.stepper-row.dimmed { color: var(--color-text-faint); }
	.stepper-count { min-width: var(--space-5); text-align: center; }
</style>
