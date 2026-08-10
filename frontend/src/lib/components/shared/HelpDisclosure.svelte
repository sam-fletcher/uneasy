<!-- HelpDisclosure.svelte
  The house collapsible help panel: a "?" row you can tap open, with whatever
  the caller passes rendered underneath it.

  Originally the prologue choosing view's local help (adr/PROLOGUE_UX_ROUND2_PLAN.md
  §1d). It is the same frame as a collapsed accordion header on purpose — where
  it first shipped it sat directly above three of them, and a second
  collapsed-row idiom on one screen would just be noise.

  WHAT IT IS FOR. Rules a player needs *at this control*, on a screen where
  those rules would otherwise push the control itself off the first screen. It
  is not a replacement for the global ? panel (HelpContent), which teaches the
  game; this teaches one step, next to the step.

  The body is the caller's markup, so it carries the CALLER's scope class, not
  this component's — anything styled here has to reach it through :global().
  Keep that to structure (flow, spacing); let each caller style its own prose.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		/** Header text. Phrased as what the player would ask ("How X works"). */
		title: string;
		/**
		 * DOM id for the body, used by aria-controls. Must be unique on the
		 * page — two disclosures can be mounted at once.
		 */
		id: string;
		/** The help itself. */
		children: Snippet;
	}

	const { title, id, children }: Props = $props();

	let open = $state(false);
</script>

<section class="help-disclosure" class:open>
	<button
		type="button"
		class="disc-head"
		aria-expanded={open}
		aria-controls={open ? id : undefined}
		onclick={() => (open = !open)}
	>
		<span class="disc-glyph" aria-hidden="true">?</span>
		<span class="disc-title">{title}</span>
		<span class="disc-caret" aria-hidden="true">▾</span>
	</button>
	{#if open}
		<div class="disc-body" {id}>
			{@render children()}
		</div>
	{/if}
</section>

<style>
	.help-disclosure {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.disc-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		min-height: 44px;
		padding: 0.5rem 0.7rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: inherit;
		font: inherit;
		text-align: left;
		cursor: pointer;
	}
	.help-disclosure.open .disc-head {
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}
	.disc-glyph {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		border-radius: 50%;
		border: 1px solid var(--color-accent);
		color: var(--color-accent);
		font-size: 0.7rem;
		flex: none;
	}
	.disc-title { flex: 1; color: var(--color-text-secondary); font-size: 0.88rem; min-width: 0; }
	.disc-caret {
		flex: none;
		color: var(--color-accent);
		font-size: 0.75rem;
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.help-disclosure.open .disc-caret { transform: rotate(0); }
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): the caret still
	   turns — its direction is the open/closed state — it just snaps. */
	@media (prefers-reduced-motion: reduce) {
		.disc-caret { transition: none; }
	}
	.disc-body {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		border: 1px solid var(--color-border);
		border-top: none;
		border-bottom-left-radius: 8px;
		border-bottom-right-radius: 8px;
		padding: 0.6rem 0.7rem;
	}
	/* The one piece of the caller's prose this file does reach: the gap above
	   owns the vertical rhythm, so a stray browser margin would double it. */
	.disc-body :global(p) { margin: 0; }
</style>
