<!-- Duel/RollPhase.svelte
  Final-roll phase: the bouts feed accumulated dice into the plan's dice
  roll (handled by the parent's <DiceRollPanel>). Once it resolves, the
  winner picks N stakes from the loser:
    make → preparer wins, takes `result` stakes from target
    mar  → target wins,   takes `adjusted_difficulty` stakes from preparer

  On a make the pick is only offered when it is one: if the preparer's budget
  covers every stake the loser put up (or is zero) the server applies the claim
  itself as the roll resolves, and the duel completes without a click.
  On a mar the opponent always decides, because the book says they MAY take: they
  take the stakes (all of them, or a pick when the difficulty is smaller than the
  pile) or spare the loser, all or nothing. The count is never negotiable.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { makeChoice, type Asset, type DiceRoll, type DuelBout, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName } from '../shared';
	import { computeAccumulated, type DuelRes } from './shared';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import EndsNote from '../shared/EndsNote.svelte';

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
	// This phase only exists once the bouts are over, so anything still set aside
	// can no longer be won.
	const lostDice = $derived(accumulated.pending.length);

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
	// The take operates on the loser's staked Asset records; CardPicker wants
	// Asset objects, so resolve each stake's asset (dropping any that have since
	// been destroyed: a destroyed asset can't be taken).
	const loserStakeAssets = $derived(
		loserStakes
			.map(s => assets.find(a => a.id === s.asset_id))
			.filter((a): a is Asset => a != null && !a.is_destroyed),
	);
	const effectiveTake = $derived(Math.min(takeCount, loserStakeAssets.length));
	const choicesApplied = $derived(duelRes.phase === 'done');
	// The winner is only asked when the claim is a real subset pick. When the
	// budget covers every stake the loser put up, "take all" is the one legal
	// answer, and when it's zero there is nothing to take — the server settles
	// both the moment the roll lands (pduelResolveClaim), so no picker here.
	// DESIGN_PRINCIPLES.md principle 3: if you can't name the decision, it
	// isn't a step.
	const claimIsAChoice = $derived(effectiveTake > 0 && effectiveTake < loserStakeAssets.length);
	// A mar's opponent may spare the loser, so with anything to take they decide
	// even when "take all" is the only way to take (principle 6).
	const marMayBeSpared = $derived(rollOutcome === 'mar' && loserStakeAssets.length > 0);
	const needsDecision = $derived(claimIsAChoice || marMayBeSpared);

	let takeSelectionIDs = $state<number[]>([]);
	let takeBusy = $state(false);
	let takeError = $state('');

	async function submitSpare() {
		if (takeBusy) return;
		takeBusy = true; takeError = '';
		try {
			await makeChoice(plan.id, rollOutcome!, ['spare']);
			onPlansChanged();
		} catch (e) {
			takeError = e instanceof Error ? e.message : 'Could not apply duel result.';
		} finally { takeBusy = false; }
	}

	async function submitTake() {
		if (takeBusy) return;
		if (!claimIsAChoice) {
			// Everything the loser staked changes hands: send it all, so the
			// server can tell a take from a spare.
			takeBusy = true; takeError = '';
			try {
				await makeChoice(plan.id, rollOutcome!, loserStakeAssets.map(a => String(a.id)));
				onPlansChanged();
			} catch (e) {
				takeError = e instanceof Error ? e.message : 'Could not apply duel result.';
			} finally { takeBusy = false; }
			return;
		}
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
	{#if lostDice > 0}
		<!-- Tied dice wait for the next bout's winner (p.60); a tie on the final
		     bout has no next bout, so they're won by nobody. Say so, or the short
		     pool looks like a bug. -->
		<p class="choices-note">
			The final bout was a tie, so the {lostDice} {lostDice === 1 ? 'die' : 'dice'} set aside
			{lostDice === 1 ? 'was' : 'were'} won by nobody and
			{lostDice === 1 ? 'drops' : 'drop'} out of the roll.
			{#if accumulated.prep.length + accumulated.targ.length === 0}
				Neither side won a die, so the roll has none to count.
			{/if}
		</p>
	{/if}
	{#if rollActive}
		<p class="choices-note muted">Dice roll in progress — resolve above.</p>
	{:else if rollOutcome != null && !choicesApplied}
		<p class="choices-header">
			Result:
			<span class="outcome-{rollOutcome}">
				{rollOutcome === 'make' ? '✓ Make' : '✗ Mar'}
			</span>
		</p>
		<p class="choices-note">
			{#if marMayBeSpared}
				{playerName(players, winnerID)} may take {effectiveTake}
				{effectiveTake === 1 ? 'stake' : 'stakes'} from
				{playerName(players, loserID)}, or spare them.
			{:else}
				{playerName(players, winnerID)} takes {effectiveTake}
				{effectiveTake === 1 ? 'stake' : 'stakes'} from
				{playerName(players, loserID)}.
			{/if}
			Afterwards, <em>every</em> staked asset on both sides is leveraged.
		</p>
		{#if !needsDecision}
			<p class="choices-note muted">
				{effectiveTake === 0
					? 'Nothing to take — settling the duel.'
					: 'Nothing to choose — settling the duel.'}
			</p>
		{:else if amWinner && !claimIsAChoice}
			<!-- A mar where the take is every stake: take them all, or spare. -->
			{#if takeError}<ErrorText message={takeError} variant="panel" />{/if}
			<button class="action-btn primary" onclick={submitTake} disabled={takeBusy}>
				{takeBusy ? '…' : `Take ${effectiveTake === 1 ? 'the stake' : `all ${effectiveTake} stakes`}`}
			</button>
			<button class="action-btn secondary" onclick={submitSpare} disabled={takeBusy}>
				Spare {playerName(players, loserID)}
			</button>
			<EndsNote text="Either choice is the last step: Propose Duel resolves when you submit." />
		{:else if amWinner}
			<p class="choices-note">
				Choose {effectiveTake} of {playerName(players, loserID)}'s
				staked {effectiveTake === 1 ? 'asset' : 'assets'} to take:
			</p>
			<CardPicker
				label="Check the assets you'll take"
				items={loserStakeAssets}
				{players}
				multi
				max={effectiveTake}
				selectedMulti={takeSelectionIDs}
				onSelectMulti={(ids) => (takeSelectionIDs = ids)}
			/>
			{#if takeError}<ErrorText message={takeError} variant="panel" />{/if}
			<button class="action-btn primary"
				onclick={submitTake}
				disabled={takeBusy || takeSelectionIDs.length !== effectiveTake}>
				{takeBusy ? '…' : `Take ${takeSelectionIDs.length}/${effectiveTake}`}
			</button>
			{#if !marMayBeSpared}<EndsNote text="This is the last step: Propose Duel resolves when you submit." />{/if}
			{#if marMayBeSpared}
				<button class="action-btn secondary" onclick={submitSpare} disabled={takeBusy}>
					Spare {playerName(players, loserID)} instead
				</button>
				<EndsNote text="Either choice is the last step: Propose Duel resolves when you submit." />
			{/if}
		{:else}
			<p class="choices-note muted">
				Waiting for {playerName(players, winnerID)} to {marMayBeSpared ? 'take stakes or spare' : 'claim stakes'}…
			</p>
		{/if}
	{/if}
</div>
