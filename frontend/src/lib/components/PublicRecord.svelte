<!-- PublicRecord.svelte
  Shows the 13-row public record timeline with scene entries (summaries)
  and plan markers. Engrailed lines appear after rows 4, 8, and 12.
  The current row is highlighted; past rows are dimmed.
-->
<script lang="ts">
	import type { RecordRow, SceneEntry, Plan } from '$lib/api';

	interface Props {
		rows: RecordRow[];
		currentRow: number;
		/** Map of player_id → display_name for attribution. */
		playerNames: Map<number, string>;
		/** Called when the user clicks a row to jump the scene view there. */
		onRowClick?: (rowNumber: number) => void;
	}

	const { rows, currentRow, playerNames, onRowClick }: Props = $props();

	// Rows after which engrailed lines appear (ranking update markers).
	const ENGRAILED_AFTER = new Set([4, 8, 12]);

	// Plan type labels — human-readable, used until the full plan UI arrives.
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
	};

	function planLabel(plan: Plan): string {
		return PLAN_LABELS[plan.plan_type] ?? plan.plan_type;
	}

	function planStatusClass(status: Plan['status']): string {
		switch (status) {
			case 'pending':   return 'plan-pending';
			case 'resolving': return 'plan-resolving';
			case 'resolved':  return 'plan-resolved';
			case 'cancelled': return 'plan-cancelled';
			default:          return '';
		}
	}

	function authorName(authorID: number): string {
		return playerNames.get(authorID) ?? '?';
	}
</script>

<div class="public-record">
	<h3 class="record-heading">Public Record</h3>

	{#if rows.length === 0}
		<p class="empty-record">The public record has not started yet.</p>
	{:else}
		<ol class="row-list">
			{#each rows as row (row.row_number)}
				<!-- Engrailed line: ranking update divider BEFORE each of rows 5, 9, 13 -->
				{#if ENGRAILED_AFTER.has(row.row_number - 1)}
					<li class="engrailed" aria-label="Ranking update">
						<span class="engrailed-label">⁂ ranking update</span>
					</li>
				{/if}

				<li
					class="record-row"
					class:current={row.row_number === currentRow}
					class:past={row.row_number < currentRow}
					class:future={row.row_number > currentRow}
				>
					<!-- Row number pill -->
					<button
						class="row-num"
						onclick={() => onRowClick?.(row.row_number)}
						title="Jump to row {row.row_number}"
						aria-label="Row {row.row_number}"
					>
						{row.row_number}
					</button>

					<div class="row-content">
						<!-- Plans scheduled on this row -->
						{#each row.plans as plan (plan.id)}
							<div class="plan-chip {planStatusClass(plan.status)}">
								<span class="plan-name">{planLabel(plan)}</span>
								<span class="plan-status">{plan.status}</span>
							</div>
						{/each}

						<!-- Scene summaries (entries) -->
						{#each row.entries as entry (entry.id)}
							<p class="entry-line">
								<span class="entry-author">{authorName(entry.author_id)}</span>
								{entry.body}
							</p>
						{/each}

						<!-- Placeholder for rows that have no content yet -->
						{#if row.plans.length === 0 && row.entries.length === 0}
							<span class="row-empty">—</span>
						{/if}
					</div>
				</li>
			{/each}
		</ol>
	{/if}
</div>

<style>
	.public-record {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		flex: 1;
		min-height: 0;
		overflow: hidden;
	}

	.record-heading {
		font-size: 0.8rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.08em;
		margin: 0 0 0.5rem;
		flex-shrink: 0;
	}

	.empty-record {
		font-size: 0.85rem;
		color: #555;
		font-style: italic;
	}

	.row-list {
		list-style: none;
		margin: 0;
		padding: 0;
		display: flex;
		flex-direction: column;
		gap: 0;
		overflow-y: auto;
		flex: 1;
		min-height: 0;
	}

	/* ── Engrailed divider ─────────────────────────────────────────────────── */

	.engrailed {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0;
		color: #c8a96e;
	}

	.engrailed::before,
	.engrailed::after {
		content: '';
		flex: 1;
		height: 1px;
		background: linear-gradient(to right, transparent, #5a4a2a, transparent);
	}

	.engrailed-label {
		font-size: 0.7rem;
		white-space: nowrap;
		color: #8a6a3a;
		letter-spacing: 0.05em;
	}

	/* ── Record rows ───────────────────────────────────────────────────────── */

	.record-row {
		display: flex;
		gap: 0.6rem;
		align-items: flex-start;
		padding: 0.3rem 0.4rem;
		border-radius: 4px;
		transition: background 0.15s;
	}

	.record-row.current {
		background: #2a2010;
		border-left: 2px solid #c8a96e;
		padding-left: 0.3rem;
	}

	.record-row.past {
		opacity: 0.6;
	}

	.record-row.future {
		opacity: 0.4;
	}

	/* Row number pill / button */
	.row-num {
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
		cursor: pointer;
		transition: background 0.15s, color 0.15s;
		padding: 0;
		border: none;
	}

	.record-row.current .row-num {
		background: #c8a96e;
		color: #1a1a1a;
	}

	.row-num:hover {
		background: #444;
		color: #e8e4d9;
	}

	.record-row.current .row-num:hover {
		background: #e0c080;
	}

	/* Row content area */
	.row-content {
		flex: 1;
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		min-width: 0;
	}

	/* ── Plan chips ────────────────────────────────────────────────────────── */

	.plan-chip {
		display: inline-flex;
		align-items: center;
		gap: 0.3rem;
		font-size: 0.72rem;
		padding: 0.15rem 0.45rem;
		border-radius: 10px;
		background: #2a2a2a;
		border: 1px solid #444;
		align-self: flex-start;
	}

	.plan-name { font-weight: 600; color: #e8e4d9; }

	.plan-status {
		color: #888;
		font-size: 0.65rem;
		text-transform: uppercase;
	}

	.plan-pending   { border-color: #666; }
	.plan-resolving { border-color: #e0a040; background: #2a2010; }
	.plan-resolved  { border-color: #6dbf7a; opacity: 0.7; }
	.plan-cancelled { border-color: #555; opacity: 0.4; }

	/* ── Scene entries ─────────────────────────────────────────────────────── */

	.entry-line {
		font-size: 0.82rem;
		color: #ccc;
		line-height: 1.4;
		margin: 0;
		word-break: break-word;
	}

	.entry-author {
		font-weight: 600;
		color: #c8a96e;
		margin-right: 0.35em;
	}

	/* ── Empty row ─────────────────────────────────────────────────────────── */

	.row-empty {
		font-size: 0.75rem;
		color: #444;
	}
</style>
