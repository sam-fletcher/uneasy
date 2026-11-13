<!-- tokens: not migrated -->
<!-- PublicRecord.svelte
  The Public Record's Events list: the thirteen rows of the timeline, each with
  its plans and scenes, a podium mark between rows 4|5, 8|9, 12|13 marking the
  algorithmic ranking updates (the same LogMark the chat feed's ranking card
  carries — one symbol across surfaces; see adr/LOG_MARKS_PLAN.md), and one
  more pseudo-row for The Shake-Up after row 13: future from row 1 of the main
  event, lit during the shake-up (with its three Esteem/Knowledge/Power pips
  filling in as categories complete), and sealed once the game ends. The game
  doesn't just stop at row 13, it always has a finale ahead of it.

  It is a pane, not an overlay: the table page gives it a column (or a tab) and
  the Events / Laws / Rumors chips at its foot choose what the pane shows. It
  fills whatever it is put in and scrolls inside. Past rows hold the plans and
  scenes that happened, and tapping one jumps the chat to it.

  See PUBLIC_RECORD_SIDEBAR_SPEC.md.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import type { RecordRow, Plan, Player, GamePhase } from '$lib/api';
	import { highlightedRow } from '$lib/highlight';
	import { playerColorByID } from '$lib/playerColor';
	import { PLAN_SHORT } from '$lib/components/plans/shared';
	import { parseMakeIntroductionsData } from '$lib/plans/resolutionData/make_introductions';
	import LogMark from '$lib/components/LogMark.svelte';

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
		/** Tapping a row pill → jump chat to that row's anchor. */
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

	// A Make Introductions plan on a future row is usually a plan being made;
	// a *synthetic* one is a peer on the road, and reading "Make Intro." there
	// misdescribes it. Name the traveller instead (D9,
	// adr/DRAFT_PEERS_AND_BLANK_ASSETS_PLAN.md). Peers lost past row 13 leave no
	// chip at all — they never got a row.
	const planLabel = (p: Plan) => {
		if (p.plan_type === 'make_introductions') {
			const mi = parseMakeIntroductionsData(p);
			if (mi.delayed_arrival) return `Arriving: ${mi.delayed_draft?.name ?? 'a newcomer'}`;
		}
		return PLAN_SHORT[p.plan_type] ?? p.plan_type;
	};
	// 'cancelled' gets no class: a plan is only ever cancelled because no valid
	// row exists for it (a delay reveal past row 13 with no Explosive Finale
	// slot), so its row_number is NULL and it never appears in row.plans at all.
	// The styling that used to exist for it was unreachable — see
	// adr/ENDGAME_VOTE_AND_FINALE_PLAN.md §6.
	const planStatusClass = (s: Plan['status']) =>
		s === 'pending' ? 'plan-pending'
			: s === 'resolving' ? 'plan-resolving'
			: s === 'resolved' ? 'plan-resolved' : '';
	const authorName = (id: number) => playerNames.get(id) ?? '?';

	// Index incoming rows by row_number so we can render a complete 1–13
	// list even before the backend has populated every row.
	const rowMap = $derived(new Map(rows.map(r => [r.row_number, r])));
	const rowAt = (n: number): RecordRow | undefined => rowMap.get(n);
	const rowState = (n: number): 'past' | 'current' | 'future' =>
		n < currentRow ? 'past' : n === currentRow ? 'current' : 'future';

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
					<span class="engrailed-rank"><LogMark family="ranking" /></span>
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
	   row's warm fill + accent border, so we don't need to dim past content. Future rows are nearly always empty (—), so a light
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
		max-width: 100%;
		color: inherit;
		cursor: pointer;
		font-family: inherit;
		text-align: left;
	}
	.plan-chip:hover { background: var(--color-border); }

	/* A delayed-arrival chip carries a player-written name, so the label has no
	   fixed length the way the plan short-names do; truncate rather than let a
	   long one push the row wider than its column. */
	.plan-name {
		color: var(--player-color, var(--color-text));
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}
	.plan-status { color: var(--color-text-muted); font-size: 0.65rem; text-transform: uppercase; }
	/* Status colors override the right/top/bottom border (keeping the
	   preparer-color left edge intact). Pending chips get no override — the
	   preparer color on the left edge carries identity. */
	.plan-resolving .plan-status { color: var(--color-text); }
	.plan-resolved  { opacity: 0.7; }

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

	/* ── Engrailed divider ──────────────────────────────────────────────────── */

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

	/* The podium: the app's one ranking symbol. Its own base bar sits on the
	   flanking hairlines, so it reads as standing on the engrailed line rather
	   than floating over it. */
	.engrailed-rank {
		display: flex;
		width: 16px;
		height: 16px;
		color: var(--color-accent);
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
