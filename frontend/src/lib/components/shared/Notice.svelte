<!--
  Notice.svelte — a notice: a mark and coloured text. No box, no tint.

  A notice lives INSIDE a step ("Lady Merrow will be leveraged", "Nothing can
  be spent yet"), so it is a group within the step, not a thing to touch, and
  gets no border or fill. It reads like a log line: the mark first, then the
  words, both in the colour of what the notice is about:

      warning  careful (orange)             a cost, a risk, a thing to know
      danger   something is lost (red)      an error, at-risk, a loss
      success  it worked (green)
      info     information, or a gain (blue)

  The words always say it too; colour is never the only signal.

  The update banner is the one place that keeps a tinted background, because
  it is app chrome rather than game content. It is not this component.

  Marks: each tone has a default drawing, and they all live in ToneMark.svelte
  (so the player row's warning mark and a selected card's tick borrow the same
  drawings). The warning triangle and the tick are drawn in the signed-off
  mockups; danger (an octagon, the stop shape) and info (a
  circled "i") are not yet signed off (see docs/work/questions/notice-marks.md).
  Pass `mark` to use a house mark of the noun instead, which is often better:
  a <LogMark> of the family the notice is about.

  Set `live` for a notice that appears after an action, so a screen reader
  announces it. Errors that do so belong in <ErrorText>, which carries
  role="alert".
-->
<script lang="ts">
	import type { Snippet } from 'svelte';
	import ToneMark from './ToneMark.svelte';

	interface Props {
		tone?: 'warning' | 'danger' | 'success' | 'info';
		/** Replaces the tone's default mark. The caller sizes nothing: it fills 16px. */
		mark?: Snippet;
		/** Announce when it appears (role="status"). */
		live?: boolean;
		children: Snippet;
	}

	const { tone = 'warning', mark, live = false, children }: Props = $props();
</script>

<p class="notice" data-tone={tone} role={live ? 'status' : undefined}>
	<span class="notice-mark" aria-hidden="true">
		{#if mark}
			{@render mark()}
		{:else}
			<ToneMark {tone} />
		{/if}
	</span>
	<span class="notice-text">{@render children()}</span>
</p>

<style>
	.notice {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
		margin: 0;
		font-size: var(--text-small);
		line-height: 1.4;
	}
	/* The mark is centred on the FIRST line, however many lines the words wrap
	   to: its box is exactly one line tall. */
	.notice-mark {
		display: flex;
		align-items: center;
		flex: none;
		height: 1lh;
	}
	.notice-mark :global(svg) {
		display: block;
		width: 16px;
		height: 16px;
	}
	.notice-text { min-width: 0; }

	.notice[data-tone='warning'] { color: var(--color-warning); }
	.notice[data-tone='danger'] { color: var(--color-danger); }
	.notice[data-tone='success'] { color: var(--color-success); }
	.notice[data-tone='info'] { color: var(--color-highlight); }
</style>
