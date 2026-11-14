<!--
  WarSheet.svelte — the war's panel, as a sheet over Play.

  The shell's sheet (RetinueSheet) with the war's own title: war's mark and
  war's colour, then one line on when the war began and what being in it costs,
  then the panel itself (MakeWarPanel through LazyPlanPanel, so the heavy panel
  is not in the table page's first bundle). Everything the old war drawer held
  is in the panel: the sides, joining one or staying out, the entry payment, and
  a cost of battle, a peace vote or a surrender claim if one is open.

  The sheet is as wide as the column it opens over and shades only that column,
  so from two columns up you can read the Public Record while it is open (the
  shell finds the column; see RetinueSheet). Phone Back closes it like any other
  sheet.

  While the war owes a cost or a claim the play area is settling, the panel here
  shows the sides and says so, and leaves the payment and the vote to the play
  area (`sheetContext.ts`).

  `plan` is null while nothing is open, and also when the war ends under an open
  sheet: the sheet then closes by itself, since there is nothing left to show.

  `children` stands in for the panel. The app never passes it: it exists so the
  style guide page can show the real sheet, title and lede included, over a
  fixture, since the real panel fetches a game's war state.
-->
<script lang="ts">
	import '$lib/components/shared/modalShell.css';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import LazyPlanPanel from '$lib/components/plans/LazyPlanPanel.svelte';
	import BarbuteMark from '$lib/components/shared/BarbuteMark.svelte';
	import type { Snippet } from 'svelte';
	import { provideSettledInPlay } from './sheetContext';
	import type { PlanContext } from '$lib/components/plans/types';
	import type { Plan, Player, WarStateResponse } from '$lib/api';
	import { warName, warLede } from '$lib/warText';

	let { plan, war, players, ctx, onClose, settledInPlay = false, children }: {
		plan: Plan | null;
		war: WarStateResponse | null;
		players: Player[];
		ctx: PlanContext;
		onClose: () => void;
		/** This war owes a cost or a claim that the play area is settling. */
		settledInPlay?: boolean;
		children?: Snippet;
	} = $props();

	// The panel inside reads this to leave its blocking forms to the play area.
	provideSettledInPlay(() => settledInPlay);
</script>

<RetinueSheet open={plan != null} {onClose}>
	{#if plan}
		<div class="war-sheet">
			<h3 class="sheet-title war-title">
				<BarbuteMark size={20} />
				<span>{warName(plan, players)}</span>
			</h3>
			<p class="war-lede">{warLede(plan, war)}</p>
			{#if children}
				{@render children()}
			{:else}
				<LazyPlanPanel planType="make_war" {ctx} {plan} mode="resolve" />
			{/if}
		</div>
	{/if}
</RetinueSheet>

<style>
	.war-sheet {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
	}

	/* The sheet title, in war's colour with war's mark before it. Nothing else
	   about it changes (modalShell.css). It sits in a column with a gap, so the
	   title's own bottom margin would double up. */
	.war-title {
		gap: var(--space-2);
		margin: 0;
		color: var(--color-danger);
	}

	.war-lede {
		margin: calc(-1 * var(--space-2)) 0 0;
		font-size: var(--text-small);
		line-height: 1.5;
		color: var(--color-text-secondary);
	}

	/* The sheet is a raised surface, so a control standing on it takes the
	   raised-twice fill (the recipe in RetinueSheet's header). The chips and the
	   secondary buttons the panel draws are the controls on it. */
	.war-sheet :global(.chip),
	.war-sheet :global(.action-btn.secondary) {
		background: var(--color-surface-2);
	}
</style>
