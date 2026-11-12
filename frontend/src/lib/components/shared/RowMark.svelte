<!--
  RowMark.svelte — the mark at the end of a tappable row. Two marks, two
  meanings, and they must never be mistaken for each other:

      kind="caret"   a filled triangle: tapping EXPANDS the row in place.
                     `open` says which way it faces (down when open, right
                     when shut), and it turns between the two.
      kind="arrow"   a stroked chevron: tapping OPENS SOMETHING ELSEWHERE (a
                     sheet, another pane, a table). It is deliberately drawn
                     larger and thinner than the caret.

  A row with nothing to do carries neither. Pick by what happens, never by how
  important the row is: a caret on a row that opens a sheet is a lie.

  Both are drawn from paths, not typed as ▾ and ›: the typed characters came
  from a different font on every platform, so their weight and size varied.
  Always 16px: the same size as the glyph slot at the start of a row, so a row
  has one mark size. Faint by default, because the mark is a hint and the row's
  text is the content; a parent can recolour it with `color`.
-->
<script lang="ts">
	interface Props {
		kind: 'caret' | 'arrow';
		/** Caret only: true faces down (expanded), false faces right (shut). */
		open?: boolean;
	}

	const { kind, open = false }: Props = $props();
</script>

<span
	class="row-mark"
	class:caret={kind === 'caret'}
	class:open
	data-kind={kind}
	aria-hidden="true"
>
	{#if kind === 'caret'}
		<svg viewBox="0 0 24 24" fill="currentColor">
			<path d="M5 8h14l-7 9z" />
		</svg>
	{:else}
		<svg
			viewBox="0 0 24 24"
			fill="none"
			stroke="currentColor"
			stroke-width="2"
			stroke-linecap="round"
			stroke-linejoin="round"
		>
			<path d="M9 5.5l6.5 6.5L9 18.5" />
		</svg>
	{/if}
</span>

<style>
	.row-mark {
		display: inline-flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}
	.row-mark svg {
		display: block;
		width: 100%;
		height: 100%;
	}
	/* A shut caret faces right; opening swings it down. */
	.caret {
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.caret.open { transform: rotate(0); }
	/* Reduced motion removes the travel, not the feedback: the caret still
	   changes direction, it just doesn't turn on the way. */
	@media (prefers-reduced-motion: reduce) {
		.caret { transition: none; }
	}
</style>
