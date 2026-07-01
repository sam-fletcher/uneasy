<!-- Duel/SetupPhase.svelte
  Setup phase: who duels (a peer; the player's main character by default) +
  the secret stake-count reveal.

  "Champions" in the rules just means which peer fights — the duellist's main
  character unless they send someone else in their stead. It's narrative only
  and gates nothing; the phase advances on the stake-count reveal alone.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import { electChampion, selectStakes, type Asset, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { playerName, assetName } from '../shared';
	import type { DuelRes } from './shared';

	let { plan, duelRes, players, assets, currentPlayerID, amParticipant, amPreparer, amTarget, myMaxStakes, myStakes, onPlansChanged, onRefresh }: {
		plan: Plan;
		duelRes: DuelRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		amParticipant: boolean;
		amPreparer: boolean;
		amTarget: boolean;
		myMaxStakes: number;
		myStakes: DuelStake[];
		onPlansChanged: () => void;
		onRefresh: () => Promise<void> | void;
	} = $props();

	// ── Who duels (a peer) ───────────────────────────────────────────────────
	const myPeers = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID && a.asset_type === 'peer' && !a.is_destroyed),
	);
	// A player's main character is their default duellist.
	function mainCharacterOf(playerID: number | null): Asset | null {
		if (playerID == null) return null;
		return assets.find(a =>
			a.owner_id === playerID && a.asset_type === 'peer'
			&& a.is_main_character && !a.is_destroyed) ?? null;
	}
	const myMC = $derived(mainCharacterOf(currentPlayerID));
	const myChampionID = $derived(amPreparer ? duelRes.prepChampID : amTarget ? duelRes.targChampID : null);
	// The peer currently set to duel for me: an elected champion, else my MC.
	const myDuelerID = $derived(myChampionID ?? myMC?.id ?? null);

	// Display name of whoever duels for a side (champion, else their MC).
	function duelerName(playerID: number | null, champID: number | null): string {
		if (champID != null) return assetName(assets, champID);
		const mc = mainCharacterOf(playerID);
		return mc ? mc.name : playerName(players, playerID);
	}

	const opponentID = $derived(amPreparer ? plan.target_player_id : plan.preparer_id);
	const opponentChampID = $derived(amPreparer ? duelRes.targChampID : duelRes.prepChampID);

	// Duel-type framing carries the narrative weight (arms vs wits).
	const heading = $derived(
		duelRes.duelType === 'wits' ? 'Trial of wits — who speaks for you?'
		: duelRes.duelType === 'arms' ? 'Duel of arms — who fights for you?'
		: 'The duel — who duels for you?',
	);
	const verbPhrase = $derived(
		myChampionID === null ? (
			duelRes.duelType === 'wits' ? 'speaks for themselves'
			: duelRes.duelType === 'arms' ? 'fights for themselves'
			: 'duels for themselves'
		) : (
			duelRes.duelType === 'wits' ? 'speaks for you'
			: duelRes.duelType === 'arms' ? 'fights for you'
			: 'duels for you'
		)
	);

	// The picker holds a *local* selection while open; nothing is persisted (and
	// no log fires) until the duellist confirms. This avoids spamming the chat
	// with a message on every card tap.
	let changing = $state(false);
	let pendingDuelerID = $state<number | null>(null);
	let championBusy = $state(false);
	let championError = $state('');
	function openPicker() {
		pendingDuelerID = myDuelerID;
		changing = true;
	}
	async function confirmDueler() {
		if (championBusy) return;
		// Selecting the main character (or clearing) returns to the default —
		// stored as a nil champion.
		const target = pendingDuelerID == null || pendingDuelerID === myMC?.id ? null : pendingDuelerID;
		championBusy = true; championError = '';
		try {
			await electChampion(plan.id, target);
			changing = false;
			onPlansChanged();
		} catch (e) {
			championError = e instanceof Error ? e.message : 'Could not set your duellist.';
		} finally { championBusy = false; }
	}

	// ── Stakes ─────────────────────────────────────────────────────────────────
	// Any non-destroyed asset I own may be staked (peer, resource, artifact),
	// leveraged or not. Count is whatever I select: 1..1+esteem status, capped by
	// how many I actually own.
	const myStakeableAssets = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a => a.owner_id === currentPlayerID && !a.is_destroyed),
	);
	const effectiveMaxStakes = $derived(Math.min(myMaxStakes, myStakeableAssets.length));
	function stakeableLabel(a: Asset): string | undefined {
		return a.is_leveraged ? 'already leveraged' : undefined;
	}

	// I've committed once my stakes exist (the server hides the opponent's stakes
	// during setup, so myStakes holds only my own here).
	const iCommitted = $derived(myStakes.length > 0);

	let stakeSelectionIDs = $state<number[]>([]);
	let stakeBusy = $state(false);
	let stakeError = $state('');
	const stakeCountValid = $derived(
		stakeSelectionIDs.length >= 1 && stakeSelectionIDs.length <= effectiveMaxStakes,
	);
	async function commitStakes() {
		if (stakeBusy || !stakeCountValid) return;
		stakeBusy = true; stakeError = '';
		try {
			await selectStakes(plan.id, stakeSelectionIDs);
			stakeSelectionIDs = [];
			onPlansChanged();
			await onRefresh();
		} catch (e) {
			stakeError = e instanceof Error ? e.message : 'Could not commit your stakes.';
		} finally { stakeBusy = false; }
	}
</script>

<!-- Who duels -->
<div class="choices-section">
	<p class="choices-header">{heading}</p>
	{#if amParticipant}
		{#if myPeers.length === 0}
			<p class="choices-note muted">You have no peer to send — you fight in person.</p>
		{:else if changing}
			<CardPicker
				label="Choose a peer to duel in your stead"
				items={myPeers}
				{players}
				selected={pendingDuelerID}
				onSelect={(id) => (pendingDuelerID = id)}
			/>
			{#if championError}<p class="res-error">{championError}</p>{/if}
			<div class="form-row">
				<button class="action-btn primary" onclick={confirmDueler} disabled={championBusy}>
					{championBusy ? '…' : 'Confirm'}
				</button>
				<button type="button" class="inline-link" onclick={() => (changing = false)}>
					Cancel
				</button>
			</div>
		{:else}
			<p class="choices-note">
				{duelerName(currentPlayerID, myChampionID)} {verbPhrase}.
				<button type="button" class="inline-link" onclick={openPicker}>
					Send someone else
				</button>
			</p>
		{/if}
		<p class="choices-note muted" style="margin:0;">
			Facing {duelerName(opponentID, opponentChampID)}.
		</p>
	{:else}
		<p class="choices-note muted">
			{duelerName(plan.preparer_id, duelRes.prepChampID)}
			vs {duelerName(plan.target_player_id, duelRes.targChampID)}.
		</p>
	{/if}
</div>

<!-- Stakes -->
<div class="choices-section">
	<p class="choices-header">
		Choose assets to stake
		{#if amParticipant && !iCommitted && effectiveMaxStakes >= 1}
			<span class="muted">&mdash; pick 1&ndash;{effectiveMaxStakes} {#if effectiveMaxStakes < myMaxStakes}(all you own; esteem allows {myMaxStakes}){/if}</span>
		{/if}
	</p>
	<p class="choices-note">
		Each staked asset hides a rolled die for the duel and final difficulty result.
		More dice means more control, but the staked assets could be taken if you lose.
		All staked assets are leveraged when the duel ends.
	</p>
	{#if amParticipant}
		{#if iCommitted}
			<p class="choices-note">
				You've staked {myStakes.length} asset{myStakes.length === 1 ? '' : 's'}.
				Waiting for your opponent…
			</p>
		{:else if effectiveMaxStakes < 1}
			<p class="res-warning">
				You have no assets available to stake, so you cannot answer this duel.
			</p>
		{:else}
			<CardPicker
				label="Check the assets you'll stake"
				items={myStakeableAssets}
				{players}
				ownerLabel={stakeableLabel}
				multi
				max={effectiveMaxStakes}
				selectedMulti={stakeSelectionIDs}
				onSelectMulti={(ids) => (stakeSelectionIDs = ids)}
			/>
			{#if stakeError}<p class="res-error">{stakeError}</p>{/if}
			<button class="action-btn primary"
				onclick={commitStakes}
				disabled={stakeBusy || !stakeCountValid}>
				{stakeBusy ? '…' : `Commit ${stakeSelectionIDs.length} stake${stakeSelectionIDs.length === 1 ? '' : 's'}`}
			</button>
		{/if}
	{:else}
		<p class="choices-note muted">The duellists are choosing their stakes.</p>
	{/if}
</div>

<style>
	.inline-link {
		background: none;
		border: none;
		padding: 0;
		margin-left: 0.35rem;
		color: var(--color-accent);
		font: inherit;
		font-size: 0.8rem;
		text-decoration: underline;
		cursor: pointer;
	}
	.inline-link:focus-visible {
		outline: 2px solid var(--color-accent);
		outline-offset: 2px;
	}
</style>
