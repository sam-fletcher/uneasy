<!--
  StepHeading.svelte — the heading of the open step: the current ask, in two lines.

      Your move · also ◆ alice         the who-line (14px)
      Aid or interfere                 the step alone, deep gold (18px)

  It is the first line of whatever step is open. The heading is the step label,
  capitalised, in the game's colour (deep gold: the game is asking). The line
  above it says who the game waits on: "Your move" in bright gold (the nudge, not
  a heading), or faint "Waiting on" followed by each owed player's shape and name
  in their colour: at most two, then "and others". When you are one of several
  owed it names the others too ("Your move · also ◆ alice"). The clock on a
  player's chip points at the same fact through a mark.

  One <h2> holds both lines so a screen reader gets one heading: "Waiting on alice
  and bob. The delay reveal". The wording comes from lib/stepHeading.ts, which is
  fed by the same derivation as the clocks, so they can't disagree.

  Pass the already-computed heading (or null, and nothing renders), so a caller
  that has to decide where the heading goes can ask once. Extra content (a plan's
  own title, a roll's difficulty) goes in the step's body, not here: the heading
  is only the ask.
-->
<script lang="ts">
	import type { StepHeading } from '$lib/stepHeading';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';

	interface Props {
		heading: StepHeading | null;
	}
	const { heading }: Props = $props();
</script>

{#if heading}
	{@const last = heading.who.length - 1}
	<h2 class="step-heading" data-yours={heading.yours}>
		<!-- The spaces are written as {' '} because Svelte trims a space at the edge of an
		     element, and "alice, bob and others" has to keep every one of them. -->
		<span class="who-line">{#if heading.yours}<span class="who-you">Your move</span>{#if heading.who.length > 0}{' '}<span class="who-faint" aria-hidden="true">·</span>{' '}<span class="who-faint">also</span>{/if}{:else}<span class="who-faint">Waiting on</span>{/if}{#each heading.who as part, i (i)}{#if i > 0}{#if i === last && !heading.more}{' '}<span class="who-faint">and</span>{:else}<span class="who-faint">,</span>{/if}{/if}{' '}{#if 'player' in part}<PlayerName player={part.player} label />{:else}<span class="who-faint">{part.text}</span>{/if}{/each}{#if heading.more}{' '}<span class="who-faint">and others</span>{/if}</span>{#if heading.label}<span class="sr-only">{'. '}</span><span class="step-label">{heading.label}</span>{/if}
	</h2>
{/if}

<style>
	.step-heading {
		margin: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
		font-size: var(--text-heading);
		font-weight: inherit;
		line-height: 1.3;
		color: var(--color-accent);
	}
	/* The line above: 14px, the secondary size. "Your move" is the nudge, in the
	   bright gold that means you; the rest is faint, and each name is in its
	   player's colour (PlayerName). */
	.who-line {
		font-size: var(--text-small);
		line-height: 1.4;
	}
	.who-you { color: var(--color-you); }
	.who-faint { color: var(--color-text-faint); }
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip: rect(0 0 0 0);
		white-space: nowrap;
	}
</style>
