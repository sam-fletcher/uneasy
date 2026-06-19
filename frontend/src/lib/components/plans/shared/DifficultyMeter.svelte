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
	.meter { margin: 0 0 0.6rem; }
	.meter-head {
		display: flex;
		align-items: baseline;
		gap: 0.5rem;
	}
	.meter-label { color: var(--color-text-muted); font-size: 0.85rem; }
	.meter-value {
		color: var(--color-leveraged);
		font-size: 1.5rem;
		font-weight: 600;
		line-height: 1;
	}
	.meter-reason {
		margin: 0.15rem 0 0.4rem;
		color: var(--color-text-faint);
		font-size: 0.72rem;
	}
	.meter-bar { display: flex; gap: 4px; }
	.seg {
		flex: 1;
		height: 14px;
		border-radius: 3px;
		background: var(--color-surface-sunken);
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
		margin-top: 0.25rem;
		color: var(--color-text-faint);
		font-size: 0.68rem;
	}
	.meter-next { color: var(--color-accent); }
</style>
