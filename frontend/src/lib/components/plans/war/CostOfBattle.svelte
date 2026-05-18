<!-- MakeWar/CostOfBattle.svelte
  Cost-of-battle pointer (whose turn it is), the active payer's picker,
  and the late-joiner entry-payment picker. All three are tied to the same
  refresh cycle and `BattleCostForm` sub-component.
-->
<script lang="ts">
	import {
		payBattleCost, payWarEntry, proposePeace,
		type Asset, type Player, type WarStateResponse, type WarParticipantInfo,
	} from '$lib/api';
	import BattleCostForm, { type BattleSubmission } from './BattleCostForm.svelte';
	import { assetsWithIntactMarginalia, ownerUnleveragedAssets, playerName } from '../shared';

	let { war, planID, players, assets, currentPlayerID, myPart, amFullParticipant, onChanged, setError }: {
		war: WarStateResponse;
		planID: number;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		myPart: WarParticipantInfo | null;
		amFullParticipant: boolean;
		onChanged: () => Promise<void> | void;
		setError: (msg: string) => void;
	} = $props();

	const activePayerID = $derived(war.outstanding_costs[0]?.payer_id ?? null);
	const itsMyCostTurn = $derived(activePayerID != null && activePayerID === currentPlayerID);

	const myOwedOpponents = $derived(
		war.outstanding_costs
			.filter(c => c.payer_id === currentPlayerID)
			.map(c => c.opponent_id),
	);

	const entryOpponentsOutstanding = $derived.by<number[]>(() => {
		if (myPart == null || myPart.entry_payment_complete) return [];
		const mySide = myPart.side;
		const opponents = war.participants
			.filter(p => p.entry_payment_complete && p.surrendered_at_row == null && p.side !== mySide)
			.map(p => p.player_id);
		const paid = new Set(
			war.battle_costs
				.filter(bc => bc.is_entry && bc.payer_id === currentPlayerID)
				.map(bc => bc.opponent_id),
		);
		return opponents.filter(id => !paid.has(id));
	});

	const myMarginaliaAssets = $derived(assetsWithIntactMarginalia(assets, currentPlayerID));
	const myUnleveraged = $derived(ownerUnleveragedAssets(assets, currentPlayerID));

	async function handleCostSubmit(s: BattleSubmission) {
		setError('');
		try {
			if (s.kind === 'peace') {
				await proposePeace(planID, s.terms);
			} else if (s.choice === 'break_asset') {
				await payBattleCost(planID, {
					opponent_id: s.opponent_id, choice: 'break_asset',
					marginalia_id: s.marginalia_id, surrender: s.surrender,
				});
			} else {
				await payBattleCost(planID, {
					opponent_id: s.opponent_id, choice: 'leverage_two',
					asset_id_1: s.asset_id_1, asset_id_2: s.asset_id_2,
					surrender: s.surrender,
				});
			}
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not pay cost.');
			throw e;
		}
	}

	async function handleEntrySubmit(s: BattleSubmission) {
		if (s.kind !== 'battle') return;
		setError('');
		try {
			if (s.choice === 'break_asset') {
				await payWarEntry(planID, {
					opponent_id: s.opponent_id, choice: 'break_asset',
					marginalia_id: s.marginalia_id,
				});
			} else {
				await payWarEntry(planID, {
					opponent_id: s.opponent_id, choice: 'leverage_two',
					asset_id_1: s.asset_id_1, asset_id_2: s.asset_id_2,
				});
			}
			await onChanged();
		} catch (e) {
			setError(e instanceof Error ? e.message : 'Could not pay entry.');
			throw e;
		}
	}
</script>

{#if war.status === 'active' && war.outstanding_costs.length > 0}
	<div class="choices-section">
		<p class="choices-header">Cost of battle — row {war.current_row}</p>
		{#if activePayerID != null}
			<p class="choices-note">
				{itsMyCostTurn ? 'It is YOUR turn to pay cost.'
					: `Waiting on ${playerName(players, activePayerID)} to pay cost of battle.`}
			</p>
		{/if}
		<p class="choices-note muted">
			Outstanding (payer → opponent), in reverse-power order:
			{#each war.outstanding_costs as k, i}
				{i > 0 ? '; ' : ''}{playerName(players, k.payer_id)} → {playerName(players, k.opponent_id)}
			{/each}
		</p>
	</div>
{:else if war.status === 'active'}
	<p class="choices-note muted">No outstanding cost-of-battle this row.</p>
{/if}

{#if itsMyCostTurn && amFullParticipant}
	<div class="choices-section">
		<p class="choices-header">Your cost of battle</p>
		<BattleCostForm
			mode="cost"
			formKey={planID}
			opponents={myOwedOpponents}
			{players}
			marginaliaAssets={myMarginaliaAssets}
			unleveraged={myUnleveraged}
			allowPeace
			allowSurrender
			onSubmit={handleCostSubmit}
		/>
	</div>
{/if}

{#if war.status === 'active' && myPart && !myPart.entry_payment_complete}
	<div class="choices-section">
		<p class="choices-header">War entry — owed payments</p>
		{#if entryOpponentsOutstanding.length === 0}
			<p class="choices-note muted">All entry payments recorded. The server
				will mark you a full participant momentarily.</p>
		{:else}
			<p class="choices-note">
				You joined this war late. Pay one cost of battle against each
				existing opposing opponent before you count as a full
				participant. Outstanding:
				{entryOpponentsOutstanding.map(id => playerName(players, id)).join(', ')}
			</p>
			<BattleCostForm
				mode="entry"
				formKey={planID}
				opponents={entryOpponentsOutstanding}
				{players}
				marginaliaAssets={myMarginaliaAssets}
				unleveraged={myUnleveraged}
				allowPeace={false}
				allowSurrender={false}
				onSubmit={handleEntrySubmit}
			/>
		{/if}
	</div>
{/if}
