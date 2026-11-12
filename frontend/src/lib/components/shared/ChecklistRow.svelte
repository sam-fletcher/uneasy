<!-- ChecklistRow.svelte
  The house checklist row: one 44px line that names something a player could
  (or must) do, plus an optional body underneath it.

  Grew out of HelpDisclosure (which is now a specialisation of it — see that
  file) when the lobby and the prologue's closing stage turned out to be
  asking the same two questions with unrelated markup:
  adr/LOBBY_AND_CHECKLIST_PLAN.md D2.

  WHAT THE ROW STATES. Three things, in order: a leading GLYPH (what kind of
  item this is), the TITLE + optional subtitle, and a right-hand STATE word
  (where this item stands). The affordance at the far right says what tapping
  does, and that is a house rule (D1, docs/STYLE_GUIDE.md "Layout &
  interaction"):

      ▾ caret   expands in place   (action='expand')
      › arrow   opens a panel elsewhere (action='navigate')
      nothing   nothing to do      (action='none')

  A caret over something that opens a sheet is a lie, so pick by what happens,
  never by how important the row is.

  NO BOX. The row is the house tappable row (tapRow.css): a 44px line with a
  thin rule under it. Its caret or arrow is the only thing that says it can be
  pressed; there is no border or fill. The state is a small uppercase word in
  its colour (statusText.css .status-word), not a chip, and a tone colours the
  title and the state word, never a border or a wash. A row that is merely
  one of several things you could do stays quiet; only a deadline or a risk
  takes a colour.

  THE BODY IS THE CALLER'S MARKUP, so it carries the CALLER's scope class, not
  this component's — anything styled here has to reach it through :global().
  Keep that to structure (flow, spacing); let each caller style its own prose.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import '$lib/components/shared/statusText.css';
	import type { Snippet } from 'svelte';
	import HelpGlyph from '$lib/components/HelpGlyph.svelte';
	import RowMark from './RowMark.svelte';

	interface Props {
		/** Phrased as the thing to do ("Turn on notifications"), not a noun. */
		title: string;
		/** One short line under the title. Optional — most rows don't need it. */
		subtitle?: string;
		/**
		 * Right-hand state word, e.g. { text: 'off', tone: 'off' }: small
		 * uppercase text in a colour, not a chip. Tones:
		 * neutral = a plain fact ("4 set", secondary) · on = confirmed good
		 * (green) · off = a switch worth turning on, and warn = a deadline
		 * (both orange, the warning family, so a warn row's title and its state
		 * word are one colour) · risk = red.
		 */
		state?: { text: string; tone?: 'neutral' | 'on' | 'off' | 'warn' | 'risk' };
		/** Row emphasis: colours the title (and the glyph, for done). Never a border or a fill. */
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

	// The state word's colour comes from the shared status-word recipe, so a
	// checklist state and a record line's "Mar" are the same kind of text.
	const STATE_CLASS = { neutral: '', on: 'success', off: 'warning', warn: 'warning', risk: 'danger' };
</script>

{#snippet headContent()}
	{#if glyph}
		<span class="row-glyph" class:help={glyph === 'help'} aria-hidden="true">
			{#if typeof glyph === 'function'}
				{@render glyph()}
			{:else if glyph === 'help'}
				<HelpGlyph size={16} />
			{:else if glyph === 'tick'}
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
					<path d="M5 12.5l4.5 4.5L19 7.5" />
				</svg>
			{:else}
				<svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
					<circle cx="12" cy="12" r="8" />
				</svg>
			{/if}
		</span>
	{/if}
	<span class="row-text">
		<span class="row-title">{title}</span>
		{#if subtitle}<span class="row-subtitle">{subtitle}</span>{/if}
	</span>
	{#if stateChip}
		<span
			class="row-state status-word {STATE_CLASS[stateChip.tone ?? 'neutral']}"
			data-tone={stateChip.tone ?? 'neutral'}>{stateChip.text}</span
		>
	{/if}
{/snippet}

<!-- Two flags, not one: `open` is the expander's state and turns the caret;
     `has-body` is whether anything is rendered under the head, and moves the
     closing rule from under the head to under the body. A blocker
     (action='none' + a body) is never "open" but always needs that. -->
<section class="checklist-row" class:open class:has-body={bodyShown} data-tone={tone}>
	{#if action === 'expand'}
		<button
			type="button"
			class="tap-row row-head"
			aria-expanded={open}
			aria-controls={open ? id : undefined}
			onclick={() => (open = !open)}
		>
			{@render headContent()}
			<RowMark kind="caret" {open} />
		</button>
	{:else if action === 'navigate'}
		<button type="button" class="tap-row row-head" onclick={onSelect}>
			{@render headContent()}
			<RowMark kind="arrow" />
		</button>
	{:else}
		<div class="tap-row row-head">
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
	/* .tap-row (tapRow.css) owns the 44px line, the rule under it and the
	   hover and focus states. */
	/* A row and its body share one closing rule: it moves from under the head
	   to under the body, so the two read as one entry. */
	.checklist-row.has-body .row-head { border-bottom-color: transparent; }

	/* ── Row tones ────────────────────────────────────────────────────────
	   A tone colours TEXT, never a border or a wash (a notice is a mark and
	   coloured text, and gold is a label, not a fill). The words in the state
	   say the same thing, so colour is never the only signal. */
	.checklist-row[data-tone='primary'] .row-title,
	.checklist-row[data-tone='blocker'] .row-title {
		color: var(--color-accent);
	}
	.checklist-row[data-tone='done'] .row-glyph { color: var(--color-success); }
	.checklist-row[data-tone='warn'] .row-title { color: var(--color-warning); }
	.checklist-row[data-tone='risk'] .row-title { color: var(--color-danger); }

	/* ── Pieces ───────────────────────────────────────────────────────────── */
	.row-glyph {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 16px;
		height: 16px;
		color: var(--color-text-secondary);
		flex: none;
	}
	/* The circled "?" — HelpDisclosure's mark, and the reason this row exists. */
	.row-glyph.help { color: var(--color-accent); }
	.row-text {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.row-title {
		color: var(--color-text);
		font-size: var(--text-body);
		min-width: 0;
	}
	.row-subtitle {
		color: var(--color-text-secondary);
		font-size: var(--text-small);
		line-height: 1.35;
	}
	.row-state { flex: none; }

	.row-body {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		border-bottom: 1px solid var(--color-border);
		padding: var(--space-2) 0 var(--space-3);
	}
	/* The one piece of the caller's prose this file does reach: the gap above
	   owns the vertical rhythm, so a stray browser margin on a paragraph the
	   caller dropped straight in would double it.
	   DIRECT children only. A body that is a whole component (the lobby mounts
	   HelpContent inside one) owns its own internal rhythm, and reaching into
	   it flattened HelpContent's `margin: 0 0 0.6rem` — the primer's
	   paragraphs all ran together. */
	.row-body > :global(p) { margin: 0; }
</style>
