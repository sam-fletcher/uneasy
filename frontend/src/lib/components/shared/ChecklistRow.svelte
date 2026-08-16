<!-- ChecklistRow.svelte
  The house checklist row: one 44px line that names something a player could
  (or must) do, plus an optional body underneath it.

  Grew out of HelpDisclosure (which is now a specialisation of it — see that
  file) when the lobby and the prologue's closing stage turned out to be
  asking the same two questions with unrelated markup:
  adr/LOBBY_AND_CHECKLIST_PLAN.md D2.

  WHAT THE ROW STATES. Three things, in order: a leading GLYPH (what kind of
  item this is), the TITLE + optional subtitle, and a right-hand STATE chip
  (where this item stands). The affordance at the far right says what tapping
  does, and that is a house rule (D1, docs/STYLE_GUIDE.md "Layout &
  interaction"):

      ▾ caret   expands in place   (action='expand')
      › arrow   opens a panel elsewhere (action='navigate')
      nothing   nothing to do      (action='none')

  A caret over something that opens a sheet is a lie, so pick by what happens,
  never by how important the row is.

  THE BODY IS THE CALLER'S MARKUP, so it carries the CALLER's scope class, not
  this component's — anything styled here has to reach it through :global().
  Keep that to structure (flow, spacing); let each caller style its own prose.
-->
<script lang="ts">
	import type { Snippet } from 'svelte';

	interface Props {
		/** Phrased as the thing to do ("Turn on notifications"), not a noun. */
		title: string;
		/** One short line under the title. Optional — most rows don't need it. */
		subtitle?: string;
		/**
		 * Right-hand state chip, e.g. { text: 'off', tone: 'off' }. Tones:
		 * neutral = a plain fact ("4 set") · on = confirmed good (green) ·
		 * off = a switch worth turning on (orange, the warning family) ·
		 * warn = a deadline (gold, "the game needs you") · risk = red.
		 */
		state?: { text: string; tone?: 'neutral' | 'on' | 'off' | 'warn' | 'risk' };
		/** Row emphasis (border + fill). */
		tone?: 'default' | 'primary' | 'done' | 'blocker' | 'warn' | 'risk';
		/** Leading mark: a Snippet for a custom icon, or one of the house marks. */
		glyph?: Snippet | 'help' | 'tick' | 'circle';
		/** What the right-hand affordance does — see D1 above. */
		action?: 'expand' | 'navigate' | 'none';
		/**
		 * Required when action === 'expand': the aria-controls target. Must be
		 * unique on the page (several rows are mounted at once).
		 */
		id?: string;
		/**
		 * action='expand' → start open. action='none' with a body → the body
		 * always renders (the closing stage's blockers), so this is ignored.
		 */
		defaultOpen?: boolean;
		/** action='navigate' */
		onSelect?: () => void;
		children?: Snippet;
	}

	// `state` is destructured to another name deliberately: a variable called
	// `state` in scope turns every `$state(…)` below into a store
	// subscription on it, and the component fails at render with
	// "store_get is not a function".
	const {
		title,
		subtitle,
		state: stateChip,
		tone = 'default',
		glyph,
		action = 'none',
		id,
		defaultOpen = false,
		onSelect,
		children,
	}: Props = $props();

	// Deliberately the initial value only: `defaultOpen` says where the row
	// STARTS, and a caller re-rendering it must not slam an open row shut
	// under the reader's finger.
	// svelte-ignore state_referenced_locally
	let open = $state(defaultOpen);

	// Only an expanding row hides its body. A blocker carries a form and must
	// never be collapsible (R4: a hidden blocker is a stuck table), and a
	// navigate row has nothing to hold.
	const bodyShown = $derived(children != null && (action !== 'expand' || open));
</script>

{#snippet headContent()}
	{#if glyph}
		<span class="row-glyph" class:help={glyph === 'help'} aria-hidden="true">
			{#if typeof glyph === 'function'}
				{@render glyph()}
			{:else if glyph === 'help'}
				?
			{:else if glyph === 'tick'}
				✓
			{:else}
				○
			{/if}
		</span>
	{/if}
	<span class="row-text">
		<span class="row-title">{title}</span>
		{#if subtitle}<span class="row-subtitle">{subtitle}</span>{/if}
	</span>
	{#if stateChip}
		<span class="row-state" data-tone={stateChip.tone ?? 'neutral'}>{stateChip.text}</span>
	{/if}
{/snippet}

<!-- Two flags, not one: `open` is the expander's state and turns the caret;
     `has-body` is whether anything is rendered under the head, and closes the
     seam between the two boxes. A blocker (action='none' + a body) is never
     "open" but always needs the seam gone. -->
<section class="checklist-row" class:open class:has-body={bodyShown} data-tone={tone}>
	{#if action === 'expand'}
		<button
			type="button"
			class="row-head"
			aria-expanded={open}
			aria-controls={open ? id : undefined}
			onclick={() => (open = !open)}
		>
			{@render headContent()}
			<span class="row-caret" aria-hidden="true">▾</span>
		</button>
	{:else if action === 'navigate'}
		<button type="button" class="row-head" onclick={onSelect}>
			{@render headContent()}
			<span class="row-arrow" aria-hidden="true">›</span>
		</button>
	{:else}
		<div class="row-head">
			{@render headContent()}
		</div>
	{/if}
	{#if bodyShown}
		<div class="row-body" id={action === 'expand' ? id : undefined}>
			{@render children?.()}
		</div>
	{/if}
</section>

<style>
	.checklist-row {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}
	.row-head {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		width: 100%;
		/* Touch minimum (docs/STYLE_GUIDE.md). Rows are the primary control on
		   both screens that use them, so this is a floor, never a target. */
		min-height: 44px;
		padding: 0.5rem 0.7rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: inherit;
		font: inherit;
		text-align: left;
	}
	/* Only the two interactive variants are buttons, so the pointer and the
	   hover/focus affordances ride on the element type rather than a class. */
	button.row-head { cursor: pointer; }
	button.row-head:hover { border-color: var(--color-border-strong); }
	button.row-head:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	/* A row and its body are one box: drop the seam between them. */
	.checklist-row.has-body .row-head {
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}

	/* ── Row tones ────────────────────────────────────────────────────────
	   Borders and (for the two soft nudges) a 12% wash, carried over from the
	   closing stage's .check-card so the rebuilt rows look like the cards they
	   replace. Gold arrives as the LABEL, never as a fill (chat-bar ruling,
	   2026-07-25): a gold surface keeps its one meaning, "the game needs you". */
	.checklist-row[data-tone='primary'] .row-head {
		border-color: var(--color-border-warm-antique);
	}
	.checklist-row[data-tone='primary'] .row-title,
	.checklist-row[data-tone='blocker'] .row-title {
		color: var(--color-accent);
	}
	.checklist-row[data-tone='blocker'] .row-head {
		border-color: var(--color-border-strong);
	}
	.checklist-row[data-tone='done'] .row-head {
		border-color: var(--color-chip-green-border);
	}
	.checklist-row[data-tone='done'] .row-glyph { color: var(--color-success); }
	.checklist-row[data-tone='warn'] .row-head {
		border-color: var(--color-warning-border);
		background: color-mix(in srgb, var(--color-warning-border) 12%, var(--color-surface-sunken));
	}
	.checklist-row[data-tone='risk'] .row-head {
		border-color: var(--color-danger-muted);
		background: color-mix(in srgb, var(--color-danger-muted) 12%, var(--color-surface-sunken));
	}

	/* ── Pieces ───────────────────────────────────────────────────────────── */
	.row-glyph {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 18px;
		height: 18px;
		color: var(--color-text-muted);
		font-size: 0.95rem;
		flex: none;
	}
	/* No line-height here on purpose: the flex box already centres the mark,
	   and setting one shifts the "?" a fraction against the frame this row
	   inherited from HelpDisclosure. Measured, not guessed. */
	/* The circled "?" — HelpDisclosure's mark, and the reason this row exists. */
	.row-glyph.help {
		border-radius: 50%;
		border: 1px solid var(--color-accent);
		color: var(--color-accent);
		font-size: 0.7rem;
	}
	.row-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
	}
	.row-title {
		color: var(--color-text-secondary);
		font-size: 0.88rem;
		min-width: 0;
	}
	.row-subtitle {
		color: var(--color-text-faint);
		font-size: 0.78rem;
		line-height: 1.35;
	}
	.row-state {
		flex: none;
		padding: 0.1rem 0.4rem;
		border-radius: 999px;
		border: 1px solid var(--color-border);
		background: var(--color-surface-2);
		color: var(--color-text-muted);
		font-size: 0.72rem;
		white-space: nowrap;
	}
	.row-state[data-tone='on'] {
		background: var(--color-chip-green-bg);
		border-color: var(--color-chip-green-border);
		color: var(--color-success);
	}
	.row-state[data-tone='off'] {
		background: var(--color-warning-bg);
		border-color: var(--color-warning-border);
		color: var(--color-warning);
	}
	.row-state[data-tone='warn'] {
		background: var(--color-chip-gold-bg);
		border-color: var(--color-chip-gold-border);
		color: var(--color-chip-gold-text);
	}
	.row-state[data-tone='risk'] {
		background: var(--color-chip-red-bg);
		border-color: var(--color-chip-red-border);
		color: var(--color-chip-red-text);
	}

	/* House marks: a filled triangle expands, a chevron travels. The app has
	   no other chevron on purpose — the two must never be mistaken for each
	   other (D1). */
	.row-caret {
		flex: none;
		color: var(--color-accent);
		font-size: 0.75rem;
		transform: rotate(-90deg);
		transition: transform 0.15s ease;
	}
	.checklist-row.open .row-caret { transform: rotate(0); }
	/* Reduced motion (docs/STYLE_GUIDE.md "Motion & the deck"): the caret still
	   turns — its direction is the open/closed state — it just snaps. */
	@media (prefers-reduced-motion: reduce) {
		.row-caret { transition: none; }
	}
	/* Larger than the caret on purpose: a thin stroke and a solid triangle at
	   the same size read as the same smudge on a phone, and telling the two
	   apart is the whole point of D1. */
	.row-arrow {
		flex: none;
		color: var(--color-accent);
		font-size: 1.2rem;
		line-height: 1;
	}

	.row-body {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		border: 1px solid var(--color-border);
		border-top: none;
		border-bottom-left-radius: 8px;
		border-bottom-right-radius: 8px;
		padding: 0.6rem 0.7rem;
	}
	.checklist-row[data-tone='primary'] .row-body { border-color: var(--color-border-warm-antique); }
	.checklist-row[data-tone='blocker'] .row-body { border-color: var(--color-border-strong); }
	.checklist-row[data-tone='done'] .row-body { border-color: var(--color-chip-green-border); }
	.checklist-row[data-tone='warn'] .row-body { border-color: var(--color-warning-border); }
	.checklist-row[data-tone='risk'] .row-body { border-color: var(--color-danger-muted); }
	/* The one piece of the caller's prose this file does reach: the gap above
	   owns the vertical rhythm, so a stray browser margin on a paragraph the
	   caller dropped straight in would double it.
	   DIRECT children only. A body that is a whole component (the lobby mounts
	   HelpContent inside one) owns its own internal rhythm, and reaching into
	   it flattened HelpContent's `margin: 0 0 0.6rem` — the primer's
	   paragraphs all ran together. */
	.row-body > :global(p) { margin: 0; }
</style>
