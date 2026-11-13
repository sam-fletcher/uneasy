<!--
  ToneMark.svelte — the four marks a notice can wear, one per tone:

      warning   a triangle with an exclamation point    (careful)
      danger    an octagon with an exclamation point    (something is lost)
      success   a tick                                  (it worked)
      info      a circled "i"                           (information, or a gain)

  <Notice> draws its default mark from here, so every place that needs a tone's
  mark takes this one component rather than redrawing it: the player row's
  warning mark for an at-risk player, and the tick on a selected card (a pick
  is drawn with the success tone's tick, coloured by the card and not by the
  tone). A drawing that is copied is a drawing that drifts.

  The house line idiom (24x24, 2-unit stroke, round ends). Colour is INHERITED,
  so the caller decides what the mark means: <Notice> paints it in the tone's
  colour, a picked card paints the tick bright gold. Decoration unless given a
  `label`; the notice's words, or the checkbox's own state, say what it means.

  The warning triangle and the tick are drawn in the signed-off mockups. The
  danger octagon and the info "i" are not yet signed off
  (docs/work/questions/notice-marks.md; the one file to change is this one).
-->
<script lang="ts">
	interface Props {
		tone: 'warning' | 'danger' | 'success' | 'info';
		/** Edge length in px. */
		size?: number;
		/** Accessible name, when nothing beside the mark says it. */
		label?: string;
	}

	const { tone, size = 16, label }: Props = $props();
</script>

<svg
	class="tone-mark"
	data-tone={tone}
	viewBox="0 0 24 24"
	width={size}
	height={size}
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	role={label ? 'img' : undefined}
	aria-label={label}
	aria-hidden={label ? undefined : 'true'}
>
	{#if tone === 'warning'}
		<path d="M12 3.5L21.5 20H2.5zM12 10v4.5M12 17.5v0.01" />
	{:else if tone === 'danger'}
		<path d="M8.5 2.5h7L21.5 8.5v7l-6 6h-7l-6-6v-7zM12 7.5v5.5M12 16.5v0.01" />
	{:else if tone === 'success'}
		<path d="M5 12.5l4.5 4.5L19 7.5" />
	{:else}
		<path d="M12 3a9 9 0 1 0 0 18a9 9 0 1 0 0-18zM12 11v6M12 7.5v0.01" />
	{/if}
</svg>

<style>
	.tone-mark {
		display: block;
		flex: none;
	}
</style>
