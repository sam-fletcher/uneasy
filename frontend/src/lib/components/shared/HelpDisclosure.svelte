<!-- HelpDisclosure.svelte
  The house collapsible help panel: a "?" row you can tap open, with whatever
  the caller passes rendered underneath it.

  Originally the prologue choosing view's local help (adr/PROLOGUE_UX_ROUND2_PLAN.md
  §1d). It is the same frame as a collapsed accordion header on purpose — where
  it first shipped it sat directly above three of them, and a second
  collapsed-row idiom on one screen would just be noise.

  SINCE adr/LOBBY_AND_CHECKLIST_PLAN.md (D2) THIS IS A SPECIALISATION of
  shared/ChecklistRow.svelte — the help glyph, an expanding caret, and nothing
  else. The frame it draws (and the 44px floor, the reduced-motion guard, the
  body's flow) all live there now; this file is the name and the defaults.
  Style changes belong in ChecklistRow, so the lobby's checklist and the
  prologue's local help can't drift apart.

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
		/** Row emphasis, passed straight through. Quiet by default. */
		tone?: 'default' | 'primary';
		/** Start expanded. Off by default — local help is quiet until asked. */
		defaultOpen?: boolean;
		/** The help itself. */
		children: Snippet;
	}

	const { title, id, tone = 'default', defaultOpen = false, children }: Props = $props();
</script>

<ChecklistRow {title} {id} {tone} {defaultOpen} glyph="help" action="expand">
	{@render children()}
</ChecklistRow>
