<!-- PublicRecord.svelte
  Two-state public-record sidebar.

  Collapsed (default): a thin vertical rail showing all 13 row pills, with
  ★ glyphs between rows 4|5, 8|9, 12|13 marking the algorithmic ranking
  updates. Past rows are dimmed; the current row is filled in the accent
  colour; future rows are outlined. Rows that have ≥1 plan get a numeric
  bubble at the top-right.

  Expanded: shows plan chips and scene-entry summaries per row. Tapping
  the rail toggles between the two states. (Animation, mobile overlay,
  and jump-to-anchor wiring all land in later steps.)

  See PUBLIC_RECORD_SIDEBAR_SPEC.md.
-->
<script lang="ts">
	import { onMount, tick } from 'svelte';
	import { fly } from 'svelte/transition';
	import type { RecordRow, Plan, Player } from '$lib/api';
	import { highlightedRow } from '$lib/highlight';
	import { playerColorByID } from '$lib/playerColor';

	interface Props {
		rows: RecordRow[];
		currentRow: number;
		/** Map of player_id → display_name for entry attribution. */
		playerNames: Map<number, string>;
		/** Used to tint plan chips with each plan preparer's color. */
		players: Player[];
		/** Tapping a row pill in the expanded view → jump chat to that row's anchor. */
		onRowJump?: (rowNumber: number) => void;
		/** Tapping a plan chip → jump chat to that plan's plan.prepared anchor. */
		onPlanJump?: (planID: number) => void;
		/** Tapping a scene entry → jump chat to the row's first scene.started anchor.
		 *  (SceneEntry doesn't carry scene_id, so we anchor by row.) */
		onSceneJump?: (rowNumber: number) => void;
	}

	const { rows, currentRow, playerNames, players, onRowJump, onPlanJump, onSceneJump }: Props = $props();

	const TOTAL_ROWS = 13;
	const ENGRAILED_AFTER = new Set([4, 8, 12]);

	const PLAN_LABELS: Record<string, string> = {
		exchange_courtiers:  'Exchange Courtiers',
		make_introductions:  'Make Introductions',
		spread_propaganda:   'Spread Propaganda',
		make_demands:        'Make Demands',
		propose_decree:      'Propose Decree',
		make_war:            'Make War',
		seek_answers:        'Seek Answers',
		chronicle_histories: 'Chronicle Histories',
		spread_rumors:       'Spread Rumors',
		propose_duel:        'Propose Duel',
		host_festivity:      'Host Festivity',
		clandestinely_liaise:'Clandestinely Liaise',
	};

	const planLabel = (p: Plan) => PLAN_LABELS[p.plan_type] ?? p.plan_type;
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
	// At ≥1280px the panel is a permanent third column (no rail, no toggle).
	// Below that, the rail collapses and the panel overlays on tap.
	let userExpanded = $state(false);
	// Initialize synchronously (not in onMount) so the first paint already
	// reflects the right mode — otherwise wide-desktop loads briefly with
	// neither rail (hidden by CSS at ≥1280) nor panel (would-be-rendered
	// only after onMount flips isWide).
	let isWide = $state(
		typeof window !== 'undefined' && window.matchMedia('(min-width: 1280px)').matches
	);
	onMount(() => {
		const mq = window.matchMedia('(min-width: 1280px)');
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
</script>

<!--
  The rail is always rendered (it's the layout anchor for the grid column
  on desktop and the visible strip on mobile). The expanded panel is added
  on top when expanded:
    - mobile: as a fixed overlay covering 75vw, with a tappable scrim
    - desktop: as an in-flow sibling that hides the rail behind it,
      sized to 320px so the grid column grows to match.
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
</button>

{#if expanded}
	<!-- Scrim: only rendered when overlay is in play (i.e. NOT at ≥1280
	     where the panel is a permanent column). Tap to collapse. -->
	{#if !isWide}
		<div
			class="scrim"
			role="button"
			tabindex="-1"
			aria-label="Close public record"
			onclick={toggle}
			onkeydown={(e) => { if (e.key === 'Escape') toggle(); }}
		></div>
	{/if}

	<aside
		class="expanded"
		class:permanent={isWide}
		transition:fly={{ x: -320, duration: isWide ? 0 : 180 }}
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
									onclick={() => onPlanJump?.(plan.id)}
									aria-label="Jump to {planLabel(plan)} by {authorName(plan.preparer_id)}"
								>
									<span class="plan-name">{planLabel(plan)}</span>
									<span class="plan-status">{plan.status}</span>
								</button>
							{/each}
							{#each row.entries as entry (entry.id)}
								{@const authorColor = playerColorByID(entry.author_id, players)}
								<button
									class="entry-line"
									onclick={() => onSceneJump?.(entry.row_number)}
									aria-label="Jump to scene on row {entry.row_number}"
								>
									<span class="entry-author" style:color={authorColor}>{authorName(entry.author_id)}</span>
									{entry.body}
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
		height: 100%;
		padding: 6px 0;
		background: none;
		border: none;
		border-right: 1px solid #2a2a2a;
		cursor: pointer;
	}

	@media (min-width: 1024px) {
		.rail { width: 48px; }
	}

	/* At ≥1280px the rail goes away entirely — the permanent panel takes its place. */
	@media (min-width: 1280px) {
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

	.rail-row[data-state="past"]    { color: #555; background: transparent; }
	.rail-row[data-state="current"] { color: #1a1a1a; background: #c8a96e; box-shadow: 0 0 0 2px #e0c080; }
	.rail-row[data-state="future"]  { color: #aaa; background: transparent; border: 1px solid #444; }

	/* Cross-component highlight: e.g. when a plan card in PlanPanel is
	 * hovered/selected, draw the eye to that plan's target row here. */
	.rail-row.highlighted { box-shadow: 0 0 0 2px #6dbfe0; color: #e8e4d9; }
	.rail-row[data-state="future"].highlighted { border-color: #6dbfe0; }

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
		font-weight: 700;
		color: #1a1a1a;
		background: #e07070;
		border-radius: 7px;
		line-height: 1;
	}

	.rail-star {
		font-size: 0.7rem;
		color: #c8a96e;
		opacity: 0.8;
		line-height: 0.8;
		margin: 1px 0;
	}

	/* The rail-hidden class is only used in overlay mode (the rail is in the
	   DOM but we don't visually need it under the overlay). At ≥1280 the
	   rail is gone entirely via the rule above. */

	/* ── Scrim (overlay mode only) ──────────────────────────────────────────
	   Used by both mobile and the 1024–1279 "narrow desktop" range, where
	   the panel still opens as a fixed overlay. The scrim element is only
	   *rendered* when !isWide, so we don't need a media query here. */

	.scrim {
		position: fixed;
		top: 0;
		left: 0;
		right: 0;
		bottom: 0;
		background: rgba(0, 0, 0, 0.4);
		z-index: 90;
		cursor: pointer;
	}

	/* ── Expanded view ─────────────────────────────────────────────────────── */

	.expanded {
		display: flex;
		flex-direction: column;
		background: #1a1a1a;
		border-right: 1px solid #2a2a2a;
		overflow: hidden;
		/* Default (overlay mode, < 1280px): fixed slide-in from the left. */
		position: fixed;
		top: 0;
		left: 0;
		width: 75vw;
		height: 100vh;
		z-index: 100;
	}

	/* Permanent column mode at ≥1280: in-flow, sits in its own page-grid
	   column. The .permanent class is applied by the component when isWide. */
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
		border-bottom: 1px solid #2a2a2a;
		flex-shrink: 0;
	}

	.exp-header h3 {
		margin: 0;
		font-size: 0.8rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.08em;
	}

	.collapse-btn {
		background: none;
		border: none;
		color: #888;
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

	.record-row[data-state="past"]    { opacity: 0.6; }
	.record-row[data-state="current"] { background: #2a2010; border-left: 2px solid #c8a96e; padding-left: 0.3rem; }
	.record-row[data-state="future"]  { opacity: 0.5; }

	.row-num-pill {
		flex-shrink: 0;
		width: 1.5rem;
		height: 1.5rem;
		display: flex;
		align-items: center;
		justify-content: center;
		font-size: 0.7rem;
		font-weight: 700;
		border-radius: 50%;
		background: #333;
		color: #888;
		border: none;
		padding: 0;
		cursor: pointer;
		transition: background 0.12s;
	}
	.row-num-pill:hover { background: #444; color: #e8e4d9; }

	.record-row[data-state="current"] .row-num-pill {
		background: #c8a96e;
		color: #1a1a1a;
	}

	.row-num-pill.highlighted { box-shadow: 0 0 0 2px #6dbfe0; }
	.record-row.highlighted { background: #14222a; }

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
		background: #2a2a2a;
		border: 1px solid var(--player-color, #444);
		border-left: 3px solid var(--player-color, #444);
		align-self: flex-start;
		color: inherit;
		cursor: pointer;
		font-family: inherit;
		text-align: left;
	}
	.plan-chip:hover { background: #333; }

	.plan-name { font-weight: 600; color: var(--player-color, #e8e4d9); }
	.plan-status { color: #888; font-size: 0.65rem; text-transform: uppercase; }
	/* Status colors override the right/top/bottom border (keeping the
	   preparer-color left edge intact). */
	.plan-pending   { /* default chip styling — preparer color carries identity */ }
	.plan-resolving { border-top-color: #e0a040; border-right-color: #e0a040; border-bottom-color: #e0a040; }
	.plan-resolved  { border-top-color: #6dbf7a; border-right-color: #6dbf7a; border-bottom-color: #6dbf7a; opacity: 0.7; }
	.plan-cancelled { opacity: 0.4; }

	.entry-line {
		font-size: 0.82rem;
		color: #ccc;
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
	.entry-line:hover { color: #fff; }

	.entry-author {
		font-weight: 600;
		/* color set inline from the entry author's playerColor */
		margin-right: 0.35em;
	}

	.row-empty {
		font-size: 0.75rem;
		color: #444;
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
		background: linear-gradient(to right, transparent, #5a4a2a, transparent);
	}

	.engrailed-star {
		font-size: 0.85rem;
		color: #c8a96e;
		line-height: 1;
	}
</style>
