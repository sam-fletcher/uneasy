<!--
  plans/shared/FinishedStep.svelte — a step that is over, as one line.

      ⚑ otto prepared Seek Answers
      ✎ “the harbour ledgers”                ⌄

  The play area reads as a plan's steps in order: finished steps collapse to a
  line like this one, and only the open step is expanded (and carries the
  heading, see shared/StepHeading). The line is small secondary text with the
  step's mark in front, and a thin rule between it and the next, so a run of
  them reads as a list.

  `wrap` lets the line run to a second line instead of being cut with an ellipsis:
  only the preparation line asks for it, because the facts fixed at preparation
  ("bob prepared Make Demands against alice's Seek Answers (row 5)") are the point
  of it and can't be shortened.

  `mark` is a house mark of the step's noun (the plan's flag, the roll's die,
  the pencil for words someone wrote); the caller passes it so the mark is
  drawn once, in LogMark, and never here.

  A line with nothing more to say is plain text and presses nothing. Give it
  `detail` and it becomes a disclosure (tapRow.css): a tappable row with a
  chevron that points down and flips up, which opens the rest beneath it: a
  long note, or the dice of a roll that is over. The line under the row moves
  to sit under the opened detail. The text stays one line either way; a long
  one is cut with an ellipsis, so give it a `detail` that holds the whole.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import type { Snippet } from 'svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';

	interface Props {
		mark: Snippet;
		/** The one line. */
		children: Snippet;
		/** What opens beneath the line. Without it the line is not pressable. */
		detail?: Snippet;
		/** Start open (rare: a result worth showing in full the moment it lands). */
		open?: boolean;
		/** Let the line wrap rather than cut it short. */
		wrap?: boolean;
	}

	// `open` seeds the row's own (uncontrolled) state once.
	// svelte-ignore state_referenced_locally
	let { mark, children, detail, open: startOpen = false, wrap = false }: Props = $props();
	// svelte-ignore state_referenced_locally
	let open = $state(startOpen);
	// A page holds many finished lines; the id is what aria-controls names.
	const uid = $props.id();
</script>

<div class="done-step" class:disclosure={!!detail} class:has-body={!!detail && open}>
	{#if detail}
		<button
			type="button"
			class="tap-row done-line"
			aria-expanded={open}
			aria-controls={open ? `done-detail-${uid}` : undefined}
			onclick={() => (open = !open)}
		>
			<span class="done-mark">{@render mark()}</span>
			<span class="done-text">{@render children()}</span>
			<RowMark kind="caret" {open} />
		</button>
		{#if open}
			<div class="done-detail disclosure-body" id={`done-detail-${uid}`}>{@render detail()}</div>
		{/if}
	{:else}
		<div class="done-line">
			<span class="done-mark">{@render mark()}</span>
			<span class="done-text" class:wrap>{@render children()}</span>
		</div>
	{/if}
</div>

<style>
	.done-line {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: 2rem;
		padding: var(--space-1) 0;
		border-bottom: 1px solid var(--color-border);
		font-size: var(--text-small);
		line-height: 1.3;
		color: var(--color-text-secondary);
	}
	/* The pressable line is the shared tappable row; it keeps its 44px floor. */
	.done-line.tap-row { min-height: var(--size-tap); }

	/* The mark is faint: it names the kind of step, it isn't the content. */
	.done-mark {
		display: inline-flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}
	.done-mark :global(svg) {
		width: 100%;
		height: 100%;
		display: block;
	}

	.done-text {
		flex: 1;
		min-width: 0;
		white-space: nowrap;
		overflow: hidden;
		text-overflow: ellipsis;
	}

	.done-text.wrap {
		white-space: normal;
		overflow: visible;
		text-overflow: clip;
	}

	/* Opened in place: indented to sit under the text, not under the mark
	   (16px mark + one gap = 24px). */
	.done-detail {
		padding: var(--space-2) 0 var(--space-3) var(--space-5);
		font-size: var(--text-small);
		line-height: 1.4;
		color: var(--color-text-secondary);
	}
</style>
