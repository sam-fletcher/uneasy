<!--
  StepHeading.svelte — the heading of the open step: the current ask.

      Your move — aid or interfere
      Waiting on otto — the roll

  It is the first line of whatever step is open, in the game's colour (deep
  gold: the game is asking), at the heading size. This is the one place the
  ask is stated in words; the clock on a player's chip points at the same
  fact through a mark. The wording comes from lib/stepHeading.ts, which is
  fed by the same derivation as the clocks, so they can't disagree.

  Pass the already-computed heading (or null, and nothing renders), so a
  caller that has to decide where the heading goes can ask once. Extra
  content (a plan's own title, a roll's difficulty) goes in the step's
  body, not here: the heading is only the ask.
-->
<script lang="ts">
	import type { StepHeading } from '$lib/stepHeading';

	interface Props {
		heading: StepHeading | null;
	}
	const { heading }: Props = $props();
</script>

{#if heading}
	<h2 class="step-heading" data-yours={heading.yours}>{heading.text}</h2>
{/if}

<style>
	.step-heading {
		margin: 0;
		font-size: var(--text-heading);
		color: var(--color-accent);
	}
</style>
