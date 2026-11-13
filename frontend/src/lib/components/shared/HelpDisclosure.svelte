<!-- HelpDisclosure.svelte
  The house collapsible help panel: a "?" row you can tap open, with whatever
  the caller passes rendered underneath it.

  Originally the prologue choosing view's local help (adr/PROLOGUE_UX_ROUND2_PLAN.md
  §1d). It sits directly above other collapsed rows on some screens, so it
  draws the same tappable row they do: a 44px line of text with a caret at its
  end and a rule under it. No box, no fill.

  It draws that row itself, from the two shared recipes (tapRow.css for the
  line, RowMark for the caret), rather than wrapping ChecklistRow. The row is
  the same on both, so nothing here can drift from it; what this file owns is
  the help mark and the body's flow.

  WHAT IT IS FOR. Rules a player needs *at this control*, on a screen where
  those rules would otherwise push the control itself off the first screen. It
  is not a replacement for the global ? panel (HelpContent), which teaches the
  game; this teaches one step, next to the step.

  The body is the caller's markup, so it carries the CALLER's scope class, not
  this component's — anything styled here has to reach it through :global().
  Keep that to structure (flow, spacing); let each caller style its own prose.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import type { Snippet } from 'svelte';
	import HelpGlyph from '$lib/components/HelpGlyph.svelte';
	import RowMark from './RowMark.svelte';

	interface Props {
		/** Header text. Phrased as what the player would ask ("How X works"). */
		title: string;
		/**
		 * DOM id for the body, used by aria-controls. Must be unique on the
		 * page — two disclosures can be mounted at once.
		 */
		id: string;
		/** Row emphasis: a primary row's title takes the game's gold. Quiet by default. */
		tone?: 'default' | 'primary';
		/** Start expanded. Off by default — local help is quiet until asked. */
		defaultOpen?: boolean;
		/** The help itself. */
		children: Snippet;
	}

	const { title, id, tone = 'default', defaultOpen = false, children }: Props = $props();

	// Deliberately the initial value only: `defaultOpen` says where the panel
	// STARTS, and a caller re-rendering it must not slam an open panel shut
	// under the reader's finger.
	// svelte-ignore state_referenced_locally
	let open = $state(defaultOpen);
</script>

<section class="help-disclosure" class:open data-tone={tone}>
	<button
		type="button"
		class="tap-row help-head"
		aria-expanded={open}
		aria-controls={open ? id : undefined}
		onclick={() => (open = !open)}
	>
		<span class="help-glyph" aria-hidden="true"><HelpGlyph size={16} /></span>
		<span class="help-title">{title}</span>
		<RowMark kind="caret" {open} />
	</button>
	{#if open}
		<div class="help-body" {id}>
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
	/* While open, the rule moves from under the head to under the body, so the
	   head and its help read as one entry. */
	.help-disclosure.open .help-head { border-bottom-color: transparent; }

	/* The help mark takes the game's deep gold, and sits in a slot exactly as
	   wide as the caret, so the row has one mark size. */
	.help-glyph {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-accent);
	}
	.help-title {
		flex: 1;
		min-width: 0;
		color: var(--color-text);
		font-size: var(--text-body);
	}
	.help-disclosure[data-tone='primary'] .help-title { color: var(--color-accent); }

	.help-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		padding: var(--space-2) 0 var(--space-3);
		border-bottom: 1px solid var(--color-border);
	}
	/* The one piece of the caller's prose this file reaches: the gap above owns
	   the rhythm, so a browser margin on a paragraph the caller drops in would
	   double it. Direct children only, so a whole component in the body keeps
	   its own spacing. */
	.help-body > :global(p) { margin: 0; }
</style>
