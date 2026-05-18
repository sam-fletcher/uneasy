<!-- Duel/RollPhase.svelte
  Final-roll phase: the bouts feed accumulated dice into the plan's dice
  roll (handled by the parent's <DiceRollPanel>). Once it resolves, the
  winner picks N stakes from the loser:
    make → preparer wins, takes `result` stakes from target
    mar  → target wins,   takes `adjusted_difficulty` stakes from preparer
-->
<script lang="ts">
	import { makeChoice, type Asset, type DiceRoll, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
	import { playerName, assetName } from '../shared';
	import { computeAccumulated, type DuelRes } from './shared';

	let { plan, duelRes, players, assets, currentPlayerID, stakes, bouts, activeRoll, rollActive, rollOutcome, onPlansChanged }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		stakes: DuelStake[];
		bouts: DuelBout[];
		activeRoll: DiceRoll | null;
		rollActive: boolean;
		rollOutcome: 'make' | 'mar' | null;
		onPlansChanged: () => void;
	} = $props();

	const accumulated = $derived(computeAccumulated(bouts, plan.preparer_id));

	const takeCount = $derived.by(() => {
		if (!activeRoll || rollOutcome == null) return 0;
		if (rollOutcome === 'make') return activeRoll.result ?? 0;
		return activeRoll.adjusted_difficulty ?? activeRoll.difficulty;
	});
	const winnerID = $derived(
		rollOutcome == null ? null
			: rollOutcome === 'make' ? plan.preparer_id : plan.target_player_id,
	);
	const loserID = $derived(
		rollOutcome == null ? null
			: rollOutcome === 'make' ? plan.target_player_id : plan.preparer_id,
	);
	const amWinner = $derived(winnerID != null && currentPlayerID === winnerID);
	const loserStakes = $derived(stakes.filter(s => s.player_id === loserID));
	const effectiveTake = $derived(Math.min(takeCount, loserStakes.length));
	const choicesApplied = $derived(duelRes.phase === 'done');

	let takeSelectionIDs = $state<number[]>([]);
	let takeBusy = $state(false);
	let takeError = $state('');

	function toggleTakeSelection(assetID: number) {
		takeSelectionIDs = takeSelectionIDs.includes(assetID)
			? takeSelectionIDs.filter(x => x !== assetID)
			: [...takeSelectionIDs, assetID];
	}

	async function submitTake() {
		if (takeBusy) return;
		if (takeSelectionIDs.length !== effectiveTake) {
			takeError = `Pick exactly ${effectiveTake} asset${effectiveTake === 1 ? '' : 's'}.`;
			return;
		}
		takeBusy = true; takeError = '';
		try {
			await makeChoice(
				plan.id,
				rollOutcome!,
				takeSelectionIDs.map(id => String(id)),
			);
			takeSelectionIDs = [];
			onPlansChanged();
		} catch (e) {
			takeError = e instanceof Error ? e.message : 'Could not apply duel result.';
		} finally { takeBusy = false; }
	}
</script>

<div class="choices-section">
	<p class="choices-header">The final roll</p>
	<p class="choices-note">
		Accumulated dice from the bouts feed into the plan's dice roll.
		{playerName(players, plan.preparer_id)}'s {accumulated.prep.length}
		{accumulated.prep.length === 1 ? 'die' : 'dice'} form the actor pool;
		{playerName(players, plan.target_player_id)}'s {accumulated.targ.length}
		{accumulated.targ.length === 1 ? 'die' : 'dice'} form interference.
	</p>
	{#if rollActive}
		<p class="choices-note muted">Dice roll in progress — resolve above.</p>
	{:else if rollOutcome != null && !choicesApplied}
		<p class="choices-header">
			Result:
			<strong class="outcome-{rollOutcome}">
				{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
			</strong>
		</p>
		<p class="choices-note">
			{playerName(players, winnerID)} takes {effectiveTake}
			{effectiveTake === 1 ? 'stake' : 'stakes'} from
			{playerName(players, loserID)}.
		</p>
		{#if amWinner}
			{#if effectiveTake === 0}
				<p class="choices-note muted">
					Nothing to take. Applying result automatically.
				</p>
				<button class="action-btn primary"
					onclick={() => { takeSelectionIDs = []; submitTake(); }}
					disabled={takeBusy}>
					{takeBusy ? '…' : 'Apply result'}
				</button>
			{:else}
				<div class="choice-list">
					{#each loserStakes as s}
						<label class="choice-item" style="display:flex;align-items:center;gap:0.5rem;">
							<input type="checkbox"
								checked={takeSelectionIDs.includes(s.asset_id)}
								onchange={() => toggleTakeSelection(s.asset_id)} />
							<span>{assetName(assets, s.asset_id)}</span>
						</label>
					{/each}
				</div>
				{#if takeError}<p class="res-error">{takeError}</p>{/if}
				<button class="action-btn primary"
					onclick={submitTake}
					disabled={takeBusy || takeSelectionIDs.length !== effectiveTake}>
					{takeBusy ? '…' : `Take ${takeSelectionIDs.length}/${effectiveTake}`}
				</button>
			{/if}
		{:else}
			<p class="choices-note muted">
				Waiting for {playerName(players, winnerID)} to claim stakes…
			</p>
		{/if}
	{/if}
</div>
