<!--
  RowMark.svelte — the mark at the end of a tappable row. Two marks, two
  meanings, and they must never be mistaken for each other:

      kind="caret"   a chevron that points DOWN: tapping EXPANDS the row in
                     place. `open` flips it to point up (a 180° turn), so the
                     direction says which way the row will go next.
      kind="arrow"   a chevron that points RIGHT (›): tapping OPENS SOMETHING
                     ELSEWHERE (a sheet, another pane, a table).

  Down and up mean "here"; right means "elsewhere". The first version drew the
  caret as a filled triangle that turned right when shut, which pointed the
  same way as the arrow and blurred the two (Round 7, 2026-11-25).

  A row with nothing to do carries neither. Pick by what happens, never by how
  important the row is: a caret on a row that opens a sheet is a lie.

  Both are stroked paths, not typed as ▾ and ›: the typed characters came from
  a different font on every platform, so their weight and size varied. Always
  20px and secondary text colour: the mark is the only thing that says a row
  can be pressed, so it has to be seen. A parent can recolour it with `color`.
-->
<script lang="ts">
	interface Props {
		kind: 'caret' | 'arrow';
		/** Caret only: true points up (expanded), false points down (shut). */
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
	<svg
		viewBox="0 0 24 24"
		fill="none"
		stroke="currentColor"
		stroke-width="2"
		stroke-linecap="round"
		stroke-linejoin="round"
	>
		{#if kind === 'caret'}
			<path d="M6 9l6 6 6-6" />
		{:else}
			<path d="M9 5.5l6.5 6.5L9 18.5" />
		{/if}
	</svg>

</span>

<style>
	.row-mark {
		display: inline-flex;
		flex: none;
		width: 20px;
		height: 20px;
		color: var(--color-text-secondary);
	}
	.row-mark svg {
		display: block;
		width: 100%;
		height: 100%;
	}
	/* A shut caret points down; opening flips it to point up. */
	.caret { transition: transform 0.15s ease; }
	.caret.open { transform: rotate(180deg); }
	/* Reduced motion removes the travel, not the feedback: the caret still
	   changes direction, it just doesn't turn on the way. */
	@media (prefers-reduced-motion: reduce) {
		.caret { transition: none; }
	}
</style>
