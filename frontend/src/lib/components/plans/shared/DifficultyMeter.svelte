<!-- shared/DifficultyMeter.svelte
  A presentational "thermometer" for plans whose difficulty is built up during
  the pre-roll (Chronicle Histories: max(knowledge rank, #invoked); Make
  Introductions: 2 + peers). The 1..max ends are static; segments fill in three
  bands:
    - 1..floor      "free" floor (set by rank / base) — neutral fill
    - floor+1..value the part the player pushed past the floor ("greed") — warm
    - value+1..max   empty headroom
  An optional dashed marker flags the next segment ("next invoke → N"). Pure and
  read-only, so every player sees the same meter.
-->
<script lang="ts">
	let {
		value,
		floor,
		max = 6,
		reason = '',
		nextLabel = '',
		headline = 'Difficulty',
		lowLabel = 'easy',
		highLabel = 'hard',
	}: {
		value: number;
		floor: number;
		max?: number;
		reason?: string;
		nextLabel?: string;
		headline?: string;
		lowLabel?: string;
		highLabel?: string;
	} = $props();

	// The segment the next commitment would light up, if it's still headroom.
	const nextSeg = $derived(value < max ? value + 1 : 0);
	type Band = 'floor' | 'greed' | 'empty';
	const segments = $derived(
		Array.from({ length: max }, (_, i): { n: number; band: Band; isNext: boolean } => {
			const n = i + 1;
			const band: Band = n <= floor ? 'floor' : n <= value ? 'greed' : 'empty';
			return { n, band, isNext: nextLabel !== '' && n === nextSeg };
		}),
	);
</script>

<div class="meter">
	<div class="meter-head">
		<span class="meter-label">{headline}</span>
		<span class="meter-value">{value}</span>
	</div>
	{#if reason}<p class="meter-reason">{reason}</p>{/if}

	<div class="meter-bar" role="img" aria-label={`${headline} ${value} of ${max}`}>
		{#each segments as seg (seg.n)}
			<div class="seg {seg.band}" class:next={seg.isNext}></div>
		{/each}
	</div>

	<div class="meter-ends">
		<span>1 {lowLabel}</span>
		{#if nextLabel}<span class="meter-next">{nextLabel}</span>{/if}
		<span>{max} {highLabel}</span>
	</div>
</div>

<style>
	.meter { display: flex; flex-direction: column; gap: var(--space-1); }
	.meter-head {
		display: flex;
		align-items: baseline;
		gap: var(--space-2);
	}
	.meter-label { color: var(--color-text-secondary); font-size: var(--text-small); }
	/* The number is the one thing here a player reads at a glance: a standalone
	   counter, so it is the one bold figure. Orange, as the "greed" band below. */
	.meter-value {
		color: var(--color-leveraged);
		font-size: var(--text-title);
		font-weight: 600;
		line-height: 1;
	}
	.meter-reason {
		margin: 0;
		color: var(--color-text-faint);
		font-size: var(--text-label);
	}
	.meter-bar { display: flex; gap: var(--space-1); }
	.seg {
		flex: 1;
		height: 14px;
		border-radius: var(--radius-piece);
		background: var(--color-surface-2);
		border: 1px solid var(--color-border);
	}
	.seg.floor {
		background: var(--color-info);
		border-color: var(--color-info);
	}
	.seg.greed {
		background: var(--color-leveraged);
		border-color: var(--color-leveraged);
	}
	.seg.next {
		background: transparent;
		border-style: dashed;
		border-color: var(--color-accent);
	}
	.meter-ends {
		display: flex;
		justify-content: space-between;
		color: var(--color-text-faint);
		font-size: var(--text-label);
	}
	.meter-next { color: var(--color-accent); }
</style>
