<!-- PublicRecord.svelte
  Two-state public-record sidebar.

  Collapsed (default): a thin vertical rail showing all 13 row pills, with
  ★ glyphs between rows 4|5, 8|9, 12|13 marking the algorithmic ranking
  updates. Past rows are dimmed; the current row is filled in the accent
  colour; future rows are outlined. Rows that have ≥1 plan get a numeric
  bubble at the top-right.

  After row 13, both rail and expanded list carry one more pseudo-row for
  The Shake-Up — a heavier ✶ glyph (vs. the engrailed ★ dividers), visible
  from row 1 of main_event onward (future), lit during shake_up (current,
  with its three Esteem/Knowledge/Power pips filling in as categories
  complete), and sealed once the game ends (past). This is the point: the
  game doesn't just stop at row 13, it always has a finale ahead of it.

  Expanded: shows plan chips and scene-entry summaries per row. Tapping
  the rail toggles between the two states. (Animation, mobile overlay,
  and jump-to-anchor wiring all land in later steps.)

  See PUBLIC_RECORD_SIDEBAR_SPEC.md.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { fly } from 'svelte/transition';
	import { recordDockQuery, RECORD_WIDTH_PX } from '$lib/breakpoints';
	import type { RecordRow, Plan, Player, GamePhase } from '$lib/api';
	import { highlightedRow } from '$lib/highlight';
	import { playerColorByID } from '$lib/playerColor';
	import { PLAN_SHORT } from '$lib/components/plans/shared';

	interface Props {
		rows: RecordRow[];
		currentRow: number;
		/** Drives the Shake-Up pseudo-row's state (future/current/past). */
		phase: GamePhase;
		/** Current shake-up category, if phase is 'shake_up'; fills the pips. */
		shakeUpCategory: string | null;
		/** Map of player_id → display_name for entry attribution. */
		playerNames: Map<number, string>;
		/** Used to tint plan chips with each plan preparer's color. */
		players: Player[];
		/** Tapping a row pill in the expanded view → jump chat to that row's anchor. */
		onRowJump?: (rowNumber: number) => void;
		/** Tapping a plan chip → jump chat to that plan's anchor. The status
		 *  decides which post that is (a chip sits at the plan's *resolution*
		 *  row, so anything past 'pending' anchors at plan.resolving) — the
		 *  caller maps it; see jumpToPlan in routes/table/[id]/+page.svelte. */
		onPlanJump?: (planID: number, status: Plan['status']) => void;
		/** Tapping a scene entry → jump chat to the row's first scene.started anchor.
		 *  (SceneEntry doesn't carry scene_id, so we anchor by row.) */
		onSceneJump?: (rowNumber: number) => void;
	}

	const {
		rows, currentRow, phase, shakeUpCategory, playerNames, players,
		onRowJump, onPlanJump, onSceneJump,
	}: Props = $props();

	const TOTAL_ROWS = 13;
	const ENGRAILED_AFTER = new Set([4, 8, 12]);

	// ── The Shake-Up pseudo-row ────────────────────────────────────────────────
	const SHAKEUP_CATEGORIES = ['esteem', 'knowledge', 'power'] as const;
	const SHAKEUP_INITIALS: Record<(typeof SHAKEUP_CATEGORIES)[number], string> = {
		esteem: 'E', knowledge: 'K', power: 'P',
	};
	// Reuses the same 'past' | 'current' | 'future' vocabulary as rowState()
	// so the existing .record-row[data-state=...] styling applies for free,
	// and the panel's scroll-into-view effect (which looks for
	// [data-state="current"]) lands here automatically once the Shake-Up
	// starts (every real row is 'past' by then — see currentRow above).
	const shakeUpRowState = $derived<'past' | 'current' | 'future'>(
		phase === 'ended' ? 'past' : phase === 'shake_up' ? 'current' : 'future'
	);
	function shakeUpPipState(cat: (typeof SHAKEUP_CATEGORIES)[number]): 'done' | 'current' | 'pending' {
		if (phase === 'ended') return 'done';
		if (phase !== 'shake_up' || !shakeUpCategory) return 'pending';
		const curIdx = SHAKEUP_CATEGORIES.indexOf(shakeUpCategory as (typeof SHAKEUP_CATEGORIES)[number]);
		if (curIdx < 0) return 'pending';
		const idx = SHAKEUP_CATEGORIES.indexOf(cat);
		return idx < curIdx ? 'done' : idx === curIdx ? 'current' : 'pending';
	}

	const planLabel = (p: Plan) => PLAN_SHORT[p.plan_type] ?? p.plan_type;
	const planStatusClass = (s: Plan['status']) =>
		s === 'pending' ? 'plan-pending'
			: s === 'resolving' ? 'plan-resolving'
			: s === 'resolved' ? 'plan-resolved'
			: s === 'cancelled' ? 'plan-cancelled' : '';
	const authorName = (id: number) => playerNames.get(id) ?? '?';

	// Index incoming rows by row_number so we can render a complete 1–13
	// rail even before the backend has populated every row.
	const rowMap = $derived(new Map(rows.map(r => [r.row_number, r])));
	const rowAt = (n: number): RecordRow | undefined => rowMap.get(n);
	const planCount = (n: number): number => rowAt(n)?.plans.length ?? 0;
	const rowState = (n: number): 'past' | 'current' | 'future' =>
		n < currentRow ? 'past' : n === currentRow ? 'current' : 'future';

	// ── Expand / collapse ─────────────────────────────────────────────────────
	// At the record dock (1040px, lib/breakpoints.ts) the panel is a
	// permanent column (no rail, no toggle). Below that, the rail collapses
	// and the panel overlays on tap.
	let userExpanded = $state(false);
	// Initialize synchronously (not in onMount) so the first paint already
	// reflects the right mode — otherwise wide-desktop loads briefly with
	// neither rail (hidden by CSS at the dock) nor panel (would-be-rendered
	// only after onMount flips isWide).
	let isWide = $state(
		typeof window !== 'undefined' && window.matchMedia(recordDockQuery).matches
	);
	onMount(() => {
		const mq = window.matchMedia(recordDockQuery);
		const sync = () => { isWide = mq.matches; };
		mq.addEventListener('change', sync);
		return () => mq.removeEventListener('change', sync);
	});
	const expanded = $derived(isWide || userExpanded);
	const toggle = () => { userExpanded = !userExpanded; };

	// ── Scroll the current row into view when the panel opens ─────────────────
	// Without this the list opens at row 1 and the focus player has to scroll
	// every time. Re-runs whenever expanded flips true.
	let rowListEl = $state<HTMLOListElement | null>(null);
	$effect(() => {
		if (!expanded) return;
		void tick().then(() => {
			const el = rowListEl?.querySelector('[data-state="current"]') as HTMLElement | null;
			el?.scrollIntoView({ block: 'center' });
		});
	});

	// ── Escape closes the overlay ──────────────────────────────────────────
	// The scrim's own onkeydown only fires if the scrim has DOM focus, which
	// never happens in practice (tabindex="-1", never programmatically
	// focused). A window-level listener is the only reliable way to catch
	// Escape from keyboard users. Only wired in overlay mode — past the
	// record dock the panel is a permanent column with nothing to collapse.
	$effect(() => {
		if (!expanded || isWide) return;
		const onKeydown = (e: KeyboardEvent) => {
			if (e.key === 'Escape') toggle();
		};
		window.addEventListener('keydown', onKeydown);
		return () => window.removeEventListener('keydown', onKeydown);
	});
</script>

<!--
  The rail is always rendered (it's the layout anchor for the grid column
  on desktop and the visible strip on mobile). The expanded panel is added
  on top when expanded:
    - below the record dock: as a fixed 280px overlay with a tappable scrim
    - at the dock: as an in-flow sibling filling its own 280px grid column.
-->
<button
	class="rail"
	onclick={toggle}
	aria-label="Expand public record"
	aria-expanded={expanded}
>
	{#each Array(TOTAL_ROWS) as _, i}
		{@const n = i + 1}
		{@const count = planCount(n)}
		<span
			class="rail-row"
			data-state={rowState(n)}
			class:highlighted={$highlightedRow === n}
			aria-label="Row {n}"
		>
			<span class="rail-num">{n}</span>
			{#if count > 0}
				<span class="rail-bubble" aria-label="{count} plan{count === 1 ? '' : 's'}">{count}</span>
			{/if}
		</span>
		{#if ENGRAILED_AFTER.has(n)}
			<span class="rail-star" aria-hidden="true">★</span>
		{/if}
	{/each}
	<span class="rail-shakeup" data-state={shakeUpRowState} aria-label="The Shake-Up">
		<span class="rail-shakeup-glyph" aria-hidden="true">✶</span>
	</span>
</button>

{#if expanded}
	<!-- Scrim: only rendered when overlay is in play (i.e. NOT past the
	     record dock where the panel is a permanent column). Tap to collapse. -->
	{#if !isWide}
		<button
			class="scrim"
			tabindex="-1"
			aria-label="Close public record"
			onclick={toggle}
		></button>
	{/if}

	<aside
		class="expanded"
		class:permanent={isWide}
		transition:fly={{ x: -RECORD_WIDTH_PX, duration: isWide ? 0 : 180 }}
	>
		<header class="exp-header">
			<h3>Public Record</h3>
			{#if !isWide}
				<button class="collapse-btn" onclick={toggle} aria-label="Collapse public record">‹</button>
			{/if}
		</header>

		<ol class="row-list" bind:this={rowListEl}>
			{#each Array(TOTAL_ROWS) as _, i}
				{@const n = i + 1}
				{@const row = rowAt(n)}
				<li
					class="record-row"
					data-state={rowState(n)}
					class:highlighted={$highlightedRow === n}
				>
					<button
						class="row-num-pill"
						class:highlighted={$highlightedRow === n}
						onclick={() => onRowJump?.(n)}
						aria-label="Jump to row {n}"
					>
						{n}
					</button>
					<div class="row-content">
						{#if row}
							{#each row.plans as plan (plan.id)}
								{@const tint = playerColorByID(plan.preparer_id, players)}
								<button
									class="plan-chip {planStatusClass(plan.status)}"
									style:--player-color={tint}
									onclick={() => onPlanJump?.(plan.id, plan.status)}
									aria-label="Jump to {planLabel(plan)} by {authorName(plan.preparer_id)}"
								>
									<span class="plan-name">{planLabel(plan)}</span>
									{#if plan.status !== 'resolved'}
										<span class="plan-status">{plan.status}</span>
									{/if}
								</button>
							{/each}
							{#each row.entries as entry (entry.id)}
								{@const authorColor = playerColorByID(entry.author_id, players)}
								<button
									class="entry-line"
									onclick={() => onSceneJump?.(entry.row_number)}
									aria-label="Jump to scene on row {entry.row_number}"
								>
									{#if entry.body.startsWith('Scene:')}
										<span class="entry-scene-label" style:color={authorColor}>Scene:</span>{entry.body.slice('Scene:'.length)}
									{:else}
										{entry.body}
									{/if}
								</button>
							{/each}
							{#if row.plans.length === 0 && row.entries.length === 0}
								<span class="row-empty">—</span>
							{/if}
						{:else}
							<span class="row-empty">—</span>
						{/if}
					</div>
				</li>

				{#if ENGRAILED_AFTER.has(n)}
					<li class="engrailed" aria-label="Ranking update">
						<span class="engrailed-line"></span>
						<span class="engrailed-star">★</span>
						<span class="engrailed-line"></span>
					</li>
				{/if}
			{/each}

			<li class="record-row shakeup-row" data-state={shakeUpRowState}>
				<span class="row-star-pill" aria-hidden="true">✶</span>
				<div class="row-content">
					<span class="shakeup-label">The Shake-Up</span>
					<div class="shakeup-pips" aria-label="Esteem, Knowledge, Power">
						{#each SHAKEUP_CATEGORIES as cat (cat)}
							<span class="pip" data-state={shakeUpPipState(cat)} aria-label={cat}>{SHAKEUP_INITIALS[cat]}</span>
						{/each}
					</div>
				</div>
			</li>
		</ol>
	</aside>
{/if}

<style>
	/* ── Rail (collapsed) ──────────────────────────────────────────────────── */

	.rail {
		display: flex;
		flex-direction: column;
		align-items: center;
		gap: 2px;
		width: 44px;
		/* Never below the touch minimum — the phase column takes the squeeze
		   (it is designed down to 300; docs/STYLE_GUIDE.md "Layout widths"). */
		flex-shrink: 0;
		height: 100%;
		padding: 6px 0;
		background: none;
		border: none;
		border-right: 1px solid var(--color-surface-2);
		cursor: pointer;
	}

	/* At the record dock the rail goes away entirely — the permanent panel
	   takes its place. (The rail is 44px — the touch minimum — at every
	   viewport; docs/STYLE_GUIDE.md "Layout widths".) */
	@media (min-width: 1070px) {
		.rail { display: none; }
	}

	.rail-row {
		position: relative;
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		border-radius: 50%;
		font-size: 0.78rem;
		font-weight: 600;
		flex-shrink: 0;
	}

	.rail-row[data-state="past"]    { color: var(--color-text-faint); background: transparent; }
	.rail-row[data-state="current"] { color: var(--color-bg); background: var(--color-accent); box-shadow: 0 0 0 2px var(--color-accent-hover); }
	.rail-row[data-state="future"]  { color: var(--color-text-muted); background: transparent; border: 1px solid var(--color-border-strong); }

	/* Cross-component highlight: e.g. when a plan card in PlanPanel is
	 * hovered/selected, draw the eye to that plan's target row here. */
	.rail-row.highlighted { box-shadow: 0 0 0 2px var(--color-highlight); color: var(--color-text); }
	.rail-row[data-state="future"].highlighted { border-color: var(--color-highlight); }

	.rail-num { line-height: 1; }

	.rail-bubble {
		position: absolute;
		top: -3px;
		right: -4px;
		min-width: 14px;
		height: 14px;
		padding: 0 3px;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.6rem;
		font-weight: 600;
		color: var(--color-bg);
		background: var(--color-highlight);
		border-radius: 7px;
		line-height: 1;
	}

	.rail-star {
		font-size: 0.7rem;
		color: var(--color-accent);
		opacity: 0.8;
		line-height: 0.8;
		margin: 1px 0;
	}

	/* The Shake-Up's rail glyph — heavier than the engrailed ★ dividers above
	   (bigger, full opacity, no dimming) since it marks the finale, not just
	   a ranking checkpoint. */
	.rail-shakeup {
		display: flex;
		align-items: center;
		justify-content: center;
		width: 32px;
		height: 32px;
		flex-shrink: 0;
		margin-top: 2px;
	}
	.rail-shakeup-glyph { font-size: 1.2rem; line-height: 1; }
	.rail-shakeup[data-state="future"]  .rail-shakeup-glyph { color: var(--color-text-muted); opacity: 0.7; }
	.rail-shakeup[data-state="current"] .rail-shakeup-glyph { color: var(--color-accent); text-shadow: 0 0 6px var(--color-accent-hover); }
	.rail-shakeup[data-state="past"]    .rail-shakeup-glyph { color: var(--color-text-faint); opacity: 0.8; }

	/* The rail-hidden class is only used in overlay mode (the rail is in the
	   DOM but we don't visually need it under the overlay). At the record
	   dock the rail is gone entirely via the rule above. */

	/* ── Scrim (overlay mode only) ──────────────────────────────────────────
	   Used everywhere below the record dock (including the 790–1039 docked-
	   chat range), where the panel still opens as a fixed overlay. The scrim
	   element is only *rendered* when !isWide, so no media query here. */

	.scrim {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.4);
		border: none;
		padding: 0;
		z-index: 90;
		cursor: pointer;
	}

	/* ── Expanded view ─────────────────────────────────────────────────────── */

	.expanded {
		display: flex;
		flex-direction: column;
		background: var(--color-bg);
		border-right: 1px solid var(--color-surface-2);
		overflow: hidden;
		/* Default (overlay mode, below the record dock): fixed slide-in from
		   the left at the record width token — the same 316 the docked panel
		   gets, so the record renders identically everywhere. The uncovered
		   remainder ("peek") doubles as the scrim's tap-to-close target: at
		   the 360 viewport floor it is exactly the 44px touch minimum, and
		   below that (344 fold covers) the max-width clamp shrinks the panel
		   instead so the peek never drops under 44. */
		position: fixed;
		top: 0;
		left: 0;
		width: 316px;
		max-width: calc(100vw - 44px);
		height: 100vh;
		z-index: 100;
	}

	/* Permanent column mode at the record dock: in-flow, sits in its own
	   page-grid column. The .permanent class is applied when isWide. */
	.expanded.permanent {
		position: relative;
		top: auto;
		left: auto;
		width: 100%;
		height: 100%;
		z-index: auto;
	}

	.exp-header {
		display: flex;
		align-items: center;
		justify-content: space-between;
		padding: 0.5rem 0.6rem;
		border-bottom: 1px solid var(--color-surface-2);
		flex-shrink: 0;
	}

	.exp-header h3 {
		margin: 0;
		font-size: 0.8rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.collapse-btn {
		background: none;
		border: none;
		color: var(--color-text-muted);
		font-size: 1.2rem;
		cursor: pointer;
		padding: 0.2rem 0.5rem;
		min-width: 44px;
		min-height: 44px;
		line-height: 1;
	}

	.row-list {
		list-style: none;
		margin: 0;
		padding: 0.4rem 0.5rem;
		overflow-y: auto;
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
	}

	.record-row {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
		padding: 0.35rem 0.4rem;
		border-radius: 4px;
	}

	/* Past rows hold real content (earlier plans + scenes), so they stay at
	   full contrast. Timeline position is already conveyed by the current
	   row's warm fill + accent border and by the rail, so we don't need to
	   dim past content. Future rows are nearly always empty (—), so a light
	   dim is enough to read as "not yet" without greying out anything real. */
	.record-row[data-state="past"]    { opacity: 1; }
	.record-row[data-state="current"] { background: var(--color-surface-gold-dim); border-left: 2px solid var(--color-accent); padding-left: 0.3rem; }
	.record-row[data-state="future"]  { opacity: 0.8; }

	.row-num-pill {
		flex-shrink: 0;
		width: 1.5rem;
		height: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.7rem;
		border-radius: 50%;
		background: var(--color-border);
		color: var(--color-text-muted);
		border: none;
		padding: 0;
		cursor: pointer;
		transition: background 0.12s;
	}
	.row-num-pill:hover { background: var(--color-border-strong); color: var(--color-text); }

	.record-row[data-state="current"] .row-num-pill {
		background: var(--color-accent);
		color: var(--color-bg);
	}

	.row-num-pill.highlighted { box-shadow: 0 0 0 2px var(--color-highlight); }
	.record-row.highlighted { background: color-mix(in srgb, var(--color-highlight) 12%, var(--color-surface)); }

	.row-content {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}

	.plan-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.72rem;
		padding: 0.15rem 0.45rem;
		border-radius: 10px;
		/* --player-color is set inline to the preparer's color. The chip's
		   border uses it directly (matching ChatPanel's name-color treatment);
		   background stays the neutral dark so other status borders (resolving,
		   resolved) can override the right/top/bottom edges. */
		background: var(--color-surface-2);
		border: 1px solid var(--player-color, var(--color-border-strong));
		border-left: 3px solid var(--player-color, var(--color-border-strong));
		align-self: flex-start;
		color: inherit;
		cursor: pointer;
		font-family: inherit;
		text-align: left;
	}
	.plan-chip:hover { background: var(--color-border); }

	.plan-name { color: var(--player-color, var(--color-text)); }
	.plan-status { color: var(--color-text-muted); font-size: 0.65rem; text-transform: uppercase; }
	/* Status colors override the right/top/bottom border (keeping the
	   preparer-color left edge intact). Pending chips get no override — the
	   preparer color on the left edge carries identity. */
	.plan-resolving .plan-status { color: var(--color-text); }
	.plan-resolved  { opacity: 0.7; }
	.plan-cancelled { opacity: 0.4; }

	.entry-line {
		font-size: 0.82rem;
		color: var(--color-text);
		line-height: 1.4;
		margin: 0;
		word-break: break-word;
		background: none;
		border: none;
		padding: 0.1rem 0;
		text-align: left;
		font-family: inherit;
		cursor: pointer;
		display: block;
		width: 100%;
	}
	.entry-line:hover { color: var(--color-accent-hover); }

	/* .entry-scene-label color is set inline from the entry author's
	   playerColor; the trailing space in the "Scene: …" body provides the
	   gap to the summary text. */

	.row-empty {
		font-size: 0.75rem;
		color: var(--color-border-strong);
	}

	/* ── Engrailed divider in expanded view ────────────────────────────────── */

	.engrailed {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.25rem 0;
	}

	.engrailed-line {
		flex: 1;
		height: 1px;
		background: linear-gradient(to right, transparent, var(--color-border-warm-antique), transparent);
	}

	.engrailed-star {
		font-size: 0.85rem;
		color: var(--color-accent);
		line-height: 1;
	}

	/* ── The Shake-Up pseudo-row ────────────────────────────────────────────── */

	.shakeup-row {
		margin-top: 0.3rem;
		padding-top: 0.6rem;
		border-top: 1px dashed var(--color-border-strong);
	}

	.row-star-pill {
		flex-shrink: 0;
		width: 1.5rem;
		height: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 1.05rem;
		line-height: 1;
	}
	.shakeup-row[data-state="current"] .row-star-pill { color: var(--color-accent); }
	.shakeup-row[data-state="past"]    .row-star-pill { color: var(--color-text-faint); }
	.shakeup-row[data-state="future"]  .row-star-pill { color: var(--color-text-muted); }

	.shakeup-label {
		font-size: 0.82rem;
		font-weight: 600;
		color: var(--color-text);
	}
	.shakeup-row[data-state="future"] .shakeup-label { color: var(--color-text-muted); }

	.shakeup-pips { display: flex; gap: 0.3rem; margin-top: 0.25rem; }

	.pip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.15rem;
		height: 1.15rem;
		border-radius: 50%;
		font-size: 0.6rem;
		font-weight: 700;
		border: 1px solid var(--color-border-strong);
		color: var(--color-text-muted);
		flex-shrink: 0;
	}
	.pip[data-state="done"] {
		background: var(--color-accent);
		color: var(--color-bg);
		border-color: var(--color-accent);
	}
	.pip[data-state="current"] { border-color: var(--color-accent); color: var(--color-accent); }
	.pip[data-state="pending"] { opacity: 0.6; }
</style>
