<!-- PublicRecord.svelte
  The Public Record's Events list: the thirteen rows of the timeline, each with
  its plans and scenes. A ranking mark stands between rows 4|5, 8|9 and 12|13,
  where the rankings update (the same LogMark the chat feed's ranking card
  carries, so one symbol means the same thing on both surfaces), and one more
  row, the Shake-Up, follows row 13: future from the start of the Main Event,
  current during the Shake-Up (its three Esteem / Knowledge / Power pips fill in
  as categories complete) and past once the game ends. The game doesn't just stop
  at row 13; it always has a finale ahead of it.

  It is a pane, not an overlay: the table page gives it a column (or a tab) and
  the Events / Laws / Rumors chips at its foot choose what the pane shows. It
  fills whatever it is put in and scrolls inside.

  A LINE is a plan or a scene, drawn as a tappable row (tapRow.css, ending in the
  arrow mark because it opens somewhere else): the preparer's shape, the name,
  at most one status word, the arrow. Tapping it opens Chat at that point (on a
  phone the page switches to the Chat tab; in columns it scrolls the chat column).
  A row's number and an empty row do nothing, so a line is the only thing here
  that responds to a tap, and its arrow is the only sign of that.

  The CURRENT row is marked by a deep gold number and a 2px deep gold bar down its
  left edge, with no fill; every other number is faint. A row the plan picker is
  pointing at (the shared `highlightedRow` store) takes the info blue for the same
  two marks instead, since "prepare here" is the blue cue.

  What a line says about its plan (the status word) is decided in
  $lib/recordLines, where a test pins it.

  The Prologue (and the lobby before it) shows the same thirteen blank rows and a
  line saying when play starts.
-->
<script lang="ts">
	import '$lib/components/shared/tapRow.css';
	import '$lib/components/shared/statusText.css';
	import { tick } from 'svelte';
	import type { RecordRow, Plan, Player, GamePhase } from '$lib/api';
	import { highlightedRow } from '$lib/highlight';
	import { PLAN_SHORT } from '$lib/components/plans/shared';
	import { parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';
	import { planStatusWord, sceneSummary } from '$lib/recordLines';
	import LogMark from '$lib/components/LogMark.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';
	import RowMark from '$lib/components/shared/RowMark.svelte';

	interface Props {
		rows: RecordRow[];
		currentRow: number;
		/** Drives the Shake-Up row's state (future/current/past) and the "play starts" note. */
		phase: GamePhase;
		/** Current shake-up category, if phase is 'shake_up'; fills the pips. */
		shakeUpCategory: string | null;
		/** Map of player_id → display_name, for a line's accessible name. */
		playerNames: Map<number, string>;
		/** Whose shape a line carries (the plan's preparer, the scene's author). */
		players: Player[];
		/** Tapping a plan line → open chat at that plan. The status decides which
		 *  post that is (a line sits at the plan's *resolution* row, so anything past
		 *  'pending' anchors at plan.resolving) — the caller maps it; see jumpToPlan
		 *  in routes/table/[id]/+page.svelte. */
		onPlanJump?: (planID: number, status: Plan['status']) => void;
		/** Tapping a scene line → open chat at the row's first scene.started post.
		 *  (SceneEntry doesn't carry scene_id, so we anchor by row.) */
		onSceneJump?: (rowNumber: number) => void;
	}

	const {
		rows, currentRow, phase, shakeUpCategory, playerNames, players,
		onPlanJump, onSceneJump,
	}: Props = $props();

	const TOTAL_ROWS = 13;
	const ENGRAILED_AFTER = new Set([4, 8, 12]);

	// Before the Main Event there is nothing to read here yet, so the list says
	// when play starts instead of looking broken.
	const beforePlay = $derived(
		phase === 'lobby' || phase === 'tone_setting' || phase === 'prologue'
	);

	// ── The Shake-Up row ──────────────────────────────────────────────────────
	const SHAKEUP_CATEGORIES = ['esteem', 'knowledge', 'power'] as const;
	const SHAKEUP_INITIALS: Record<(typeof SHAKEUP_CATEGORIES)[number], string> = {
		esteem: 'E', knowledge: 'K', power: 'P',
	};
	// Same 'past' | 'current' | 'future' vocabulary as rowState(), so the same
	// styling applies, and the scroll-into-view effect below (which looks for
	// [data-state="current"]) lands here once the Shake-Up starts: every real row
	// is 'past' by then.
	const shakeUpRowState = $derived<'past' | 'current' | 'future'>(
		phase === 'ended' ? 'past' : phase === 'shake_up' ? 'current' : 'future'
	);
	// The pips only mean something once the Shake-Up is under way: before it
	// there is nothing to fill in.
	const showPips = $derived(phase === 'shake_up' || phase === 'ended');
	function shakeUpPipState(cat: (typeof SHAKEUP_CATEGORIES)[number]): 'done' | 'current' | 'pending' {
		if (phase === 'ended') return 'done';
		if (phase !== 'shake_up' || !shakeUpCategory) return 'pending';
		const curIdx = SHAKEUP_CATEGORIES.indexOf(shakeUpCategory as (typeof SHAKEUP_CATEGORIES)[number]);
		if (curIdx < 0) return 'pending';
		const idx = SHAKEUP_CATEGORIES.indexOf(cat);
		return idx < curIdx ? 'done' : idx === curIdx ? 'current' : 'pending';
	}

	// A Make Introductions plan on a future row is usually a plan being made;
	// a *synthetic* one is a peer on the road, and reading "Make Introductions"
	// there misdescribes it. Name the traveller instead. Peers lost past row 13
	// leave no line at all: they never got a row.
	const planLabel = (p: Plan) => {
		if (p.plan_type === 'make_introductions') {
			const mi = parseMakeIntroductionsData(p);
			if (mi.delayed_arrival) return `Arriving: ${mi.delayed_draft?.name ?? 'a newcomer'}`;
		}
		return PLAN_SHORT[p.plan_type] ?? p.plan_type;
	};
	// A cancelled plan is never listed: it is only cancelled because no valid row
	// exists for it (a delay reveal past row 13 with no Explosive Finale slot), so
	// its row_number is NULL and it never appears in row.plans at all.
	const playerOf = (id: number) => players.find((p) => p.id === id) ?? null;
	const nameOf = (id: number) => playerNames.get(id) ?? '?';

	// Index incoming rows by row_number so we can render a complete 1–13
	// list even before the backend has populated every row.
	const rowMap = $derived(new Map(rows.map(r => [r.row_number, r])));
	const rowAt = (n: number): RecordRow | undefined => rowMap.get(n);
	const rowState = (n: number): 'past' | 'current' | 'future' =>
		n < currentRow ? 'past' : n === currentRow ? 'current' : 'future';
	const hasLines = (row: RecordRow | undefined) =>
		!!row && (row.plans.length > 0 || row.entries.length > 0);

	// ── Scroll the current row into view ──────────────────────────────────────
	// Without this the list opens at row 1 and the focus player has to scroll
	// every time. It runs when the list first renders, and the page mounts the
	// list again each time Events is chosen, so it lands on the current row
	// each time.
	let rowListEl = $state<HTMLOListElement | null>(null);
	$effect(() => {
		if (!rowListEl) return;
		void tick().then(() => {
			const el = rowListEl?.querySelector('[data-state="current"]') as HTMLElement | null;
			el?.scrollIntoView({ block: 'center' });
		});
	});
</script>

<div class="record">
	<!-- The engrailed pattern (a row of small scallops), drawn once and used by the
	     three ranking dividers. Not display:none: some browsers won't paint a
	     pattern whose svg isn't rendered. -->
	<svg class="defs" width="0" height="0" aria-hidden="true" focusable="false">
		<defs>
			<pattern id="record-engrailed" width="10" height="6" patternUnits="userSpaceOnUse">
				<path d="M0 0.5A5 5 0 0 0 10 0.5" fill="none" stroke-width="1"
					style="stroke: var(--color-border-warm-antique)" />
			</pattern>
		</defs>
	</svg>

	<ol class="row-list" bind:this={rowListEl}>
		{#if beforePlay}
			<li class="starts-note">
				<p class="muted-text">Play starts when the Prologue ends. Each row fills in as the game reaches it.</p>
			</li>
		{/if}

		{#each Array(TOTAL_ROWS) as _, i}
			{@const n = i + 1}
			{@const row = rowAt(n)}
			<li
				class="record-row"
				data-state={rowState(n)}
				class:highlighted={$highlightedRow === n}
			>
				<span class="row-num" class:with-lines={hasLines(row)}>
					<span class="sr-only">Row&nbsp;</span>{n}
				</span>
				{#if row && hasLines(row)}
					<div class="lines">
						{#each row.plans as plan (plan.id)}
							{@const word = planStatusWord(plan)}
							<button
								class="tap-row line"
								onclick={() => onPlanJump?.(plan.id, plan.status)}
								aria-label="{planLabel(plan)} by {nameOf(plan.preparer_id)}{word ? `, ${word.text}` : ''}. Open in Chat"
							>
								<PlayerShape player={playerOf(plan.preparer_id)} />
								<span class="line-text">{planLabel(plan)}</span>
								{#if word}
									<span
										class="status-word"
										class:success={word.tone === 'success'}
										class:danger={word.tone === 'danger'}
									>{word.text}</span>
								{/if}
								<RowMark kind="arrow" />
							</button>
						{/each}
						{#each row.entries as entry (entry.id)}
							{@const scene = sceneSummary(entry.body)}
							<button
								class="tap-row line"
								onclick={() => onSceneJump?.(entry.row_number)}
								aria-label="{scene.isScene ? 'Scene' : 'Note'} by {nameOf(entry.author_id)}: {scene.text}. Open in Chat"
							>
								<PlayerShape player={playerOf(entry.author_id)} />
								{#if scene.isScene}
									<span class="scene-mark"><LogMark family="scene" /></span>
								{/if}
								<span class="line-text">{scene.text}</span>
								<RowMark kind="arrow" />
							</button>
						{/each}
					</div>
				{/if}
			</li>

			{#if ENGRAILED_AFTER.has(n)}
				<li class="engrailed">
					<span class="sr-only">Ranking update</span>
					<svg class="engrailed-line" height="6" aria-hidden="true"><rect width="100%" height="6" fill="url(#record-engrailed)" /></svg>
					<span class="engrailed-rank"><LogMark family="ranking" /></span>
					<svg class="engrailed-line" height="6" aria-hidden="true"><rect width="100%" height="6" fill="url(#record-engrailed)" /></svg>
				</li>
			{/if}
		{/each}

		<li class="shakeup-row" data-state={shakeUpRowState}>
			<span class="row-num shakeup-mark"><span class="burst"><LogMark family="shake_up" /></span></span>
			<span class="shakeup-label">The Shake-Up</span>
			{#if showPips}
				<span class="shakeup-pips" role="group" aria-label="Esteem, Knowledge, Power">
					{#each SHAKEUP_CATEGORIES as cat (cat)}
						<span class="pip" data-state={shakeUpPipState(cat)} role="img" aria-label="{cat}: {shakeUpPipState(cat)}">{SHAKEUP_INITIALS[cat]}</span>
					{/each}
				</span>
			{/if}
		</li>
	</ol>
</div>

<style>
	/* ── The pane ──────────────────────────────────────────────────────────── */

	/* Fills its column and scrolls inside it. */
	.record {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-height: 0;
		background: var(--color-bg);
	}

	.defs {
		position: absolute;
		width: 0;
		height: 0;
	}

	.row-list {
		list-style: none;
		margin: 0;
		padding: 0 var(--space-4) var(--space-4);
		overflow-y: auto;
		flex: 1;
	}

	.starts-note { padding: var(--space-3) 0 var(--space-2); }

	/* ── A row ─────────────────────────────────────────────────────────────── */

	/* The row is a pane of the list, not a thing to touch: a hairline under it,
	   no box and no fill. The current row's mark is the bar down its left edge,
	   drawn inset so it never changes the row's size. */
	.record-row {
		display: flex;
		border-bottom: 1px solid var(--color-border-quiet);
	}
	.record-row[data-state='current'],
	.shakeup-row[data-state='current'] { box-shadow: inset 2px 0 0 var(--color-accent); }
	/* The plan picker is pointing at this row. It wins over "current", so the two
	   never show as one gold and one blue bar. */
	.record-row.highlighted { box-shadow: inset 2px 0 0 var(--color-highlight); }

	/* The row's number: faint, except the current row's, which is deep gold (the
	   game's colour). It lines up with the first line, and an empty row is only as
	   tall as its number needs. */
	.row-num {
		flex: none;
		display: flex;
		align-items: center;
		box-sizing: border-box;
		width: 36px;
		height: 28px;
		padding-left: var(--space-2);
		font-size: var(--text-small);
		color: var(--color-text-faint);
	}
	.row-num.with-lines { height: var(--size-tap); }
	.record-row[data-state='current'] .row-num { color: var(--color-accent); }
	.record-row.highlighted .row-num { color: var(--color-highlight); }

	/* Whatever a row holds, plan lines first and then scenes. */
	.lines {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	/* A line is a tapRow, but its hairline belongs to the row, not to each line,
	   so two plans in one row aren't ruled apart. The 44px floor stays. */
	.line {
		border-bottom: 0;
		padding-top: 0;
		padding-bottom: 0;
		font-size: var(--text-small);
		color: var(--color-text);
	}

	/* A plan's name carries no fixed length: a delayed arrival is named by
	   whoever wrote it, and a scene's summary is free text. Truncate rather than
	   let one push the row wider than its column; the tap opens the whole thing. */
	.line-text {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.scene-mark {
		display: flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}

	/* ── Ranking dividers ───────────────────────────────────────────────────── */

	.engrailed {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		height: 20px;
		border-bottom: 1px solid var(--color-border-quiet);
	}
	.engrailed-line { flex: 1; min-width: 0; }

	/* The podium: the app's one ranking symbol, in deep gold, the game's colour. */
	.engrailed-rank {
		display: flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-accent);
	}

	/* ── The Shake-Up row ───────────────────────────────────────────────────── */

	/* The house burst, not a typed star: a typed symbol falls back to a different
	   font on every platform. Faint until the Shake-Up is under way. */
	.shakeup-row {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		min-height: var(--size-tap);
		color: var(--color-text-faint);
	}
	.shakeup-mark { height: auto; }
	.burst { display: flex; width: 16px; height: 16px; }
	.shakeup-label { font-size: var(--text-small); }
	.shakeup-row[data-state='current'] { color: var(--color-accent); }

	.shakeup-pips { display: flex; gap: var(--space-1); margin-left: auto; }

	/* A pip is a status dot with an initial in it: the one circle this list is
	   allowed. Done is filled, the one in play is outlined in gold, the rest are
	   quiet. Nothing fades with opacity, which would take words under their floor. */
	.pip {
		display: inline-flex;
		align-items: center;
		justify-content: center;
		box-sizing: border-box;
		width: 20px;
		height: 20px;
		flex: none;
		border-radius: var(--radius-dot);
		border: 1px solid var(--color-border-control);
		font-size: var(--text-label);
		font-weight: 600;
		color: var(--color-text-faint);
	}
	.pip[data-state='done'] {
		background: var(--color-accent);
		border-color: var(--color-accent);
		color: var(--color-bg);
	}
	.pip[data-state='current'] { border-color: var(--color-accent); color: var(--color-accent); }

	/* Words for a screen reader only: off-screen for the eye. */
	.sr-only {
		position: absolute;
		width: 1px;
		height: 1px;
		padding: 0;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}
</style>
