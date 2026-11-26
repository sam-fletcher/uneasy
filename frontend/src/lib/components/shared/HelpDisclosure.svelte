<!-- HelpDisclosure.svelte
  THE "how this works" row: a deep-gold "?" at the start, the title in main
  text, a chevron that flips when it opens, and the explanation beneath. Every
  explainer in the app is this one component — "What can happen?" (plans/shared/
  Buffet), the Prologue's help rows, the scene-setup help — so that the "?"
  says "this explains" before a word is read, and matches the Help button's "?"
  (Round 7, 2026-11-25). Other disclosures keep their own kind glyphs; a row
  that is not an explainer does not get this one.

  It is a ChecklistRow (action='expand', glyph='help'), so the row, its
  chevron, the closing line that moves under the body when it opens, and the
  absence of any box or fill are all the shared recipe, drawn once.

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
	import ChecklistRow from './ChecklistRow.svelte';

	interface Props {
		/** Header text. Phrased as what the player would ask ("How X works"). */
		title: string;
		/**
		 * DOM id for the body, used by aria-controls. Must be unique on the
		 * page — two disclosures can be mounted at once.
		 */
		id: string;
		/** One short line under the title (the lobby primer's "a skim is enough"). */
		subtitle?: string;
		/** Start expanded. Off by default — local help is quiet until asked. */
		defaultOpen?: boolean;
		/** The help itself. */
		children: Snippet;
	}

	const { title, subtitle, id, defaultOpen = false, children }: Props = $props();
</script>

<ChecklistRow {title} {subtitle} {id} action="expand" glyph="help" {defaultOpen}>
	{@render children()}
</ChecklistRow>
