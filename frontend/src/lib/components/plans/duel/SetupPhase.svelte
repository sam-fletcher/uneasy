<!-- Duel/SetupPhase.svelte
  Setup phase: who duels (a peer; the player's main character by default) +
  the secret stake-count reveal.

  "Champions" in the rules just means which peer fights — the duellist's main
  character unless they send someone else in their stead. It's narrative only
  and gates nothing; the phase advances on the stake-count reveal alone.
-->
<script lang="ts">
	import AssetName from '$lib/components/shared/AssetName.svelte';
	import PlayerName from '$lib/components/shared/PlayerName.svelte';
	import EndsNote from '../shared/EndsNote.svelte';
	import '$lib/components/shared/actionButton.css';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { electChampion, selectStakes, type Asset, type DuelStake, type Plan, type Player } from '$lib/api';
	import CardPicker from '../CardPicker.svelte';
	import { duelerOf, mainCharacterOf, type DuelRes } from './shared';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

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
	const myMC = $derived(mainCharacterOf(assets, currentPlayerID));
	const myChampionID = $derived(amPreparer ? duelRes.prepChampID : amTarget ? duelRes.targChampID : null);
	// The peer currently set to duel for me: an elected champion, else my MC.
	const myDuelerID = $derived(myChampionID ?? myMC?.id ?? null);

	// Whoever duels for a side: the elected champion, else their main character. It
	// is an asset, so its name is marked as one (<AssetName>, the one piece every panel uses).
	// Only a player with no peer at all falls back to their own name.

	// Peers I could send in my stead: any but the one already duelling for me. With
	// none the choice can't be made, and the panel says so instead of dropping the
	// button: a player should learn what is possible, not wonder where it went.
	const otherPeers = $derived(myPeers.filter(a => a.id !== myDuelerID));
	// Nobody to duel for me at all: only a main character lost mid-resolution gets
	// here. The duellist is always a peer, so this never reads as the player fighting.
	const iHaveNoDueler = $derived(duelerOf(assets, currentPlayerID, myChampionID) == null);

	// Duel-type framing carries the narrative weight (arms vs wits).
	const heading = $derived(
		duelRes.duelType === 'wits' ? 'Who speaks for you'
		: duelRes.duelType === 'arms' ? 'Who fights for you'
		: 'Who duels for you',
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

{#snippet duelerName(playerID: number | null, champID: number | null)}
	{@const who = duelerOf(assets, playerID, champID)}
	{#if who}<AssetName name={who.name} />{:else}<PlayerName {players} id={playerID} />{/if}
{/snippet}

<div class="step-groups">
	<!-- Who duels. The pairing (who faces whom) is the panel's set piece above, so this
	     group is only the duellist's own choice, and a bystander has none. -->
	{#if amParticipant}
		<div class="choices-section">
			<p class="choices-header">{heading}</p>
			{#if changing}
				<CardPicker
					label="Choose a peer to duel in your stead"
					items={myPeers}
					{players}
					selected={pendingDuelerID}
					onSelect={(id) => (pendingDuelerID = id)}
				/>
				{#if championError}<ErrorText message={championError} variant="panel" />{/if}
				<div class="step-actions">
					<button class="action-btn primary" onclick={confirmDueler} disabled={championBusy}>
						{championBusy ? '…' : 'Confirm'}
					</button>
					<button type="button" class="action-btn secondary" onclick={() => (changing = false)}>
						Cancel
					</button>
				</div>
			{:else if iHaveNoDueler}
				<p class="choices-note">You have no peer to put forward in this duel.</p>
			{:else}
				<p class="choices-note">
					{@render duelerName(currentPlayerID, myChampionID)} {verbPhrase}.
				</p>
				{#if otherPeers.length > 0}
					<button type="button" class="action-btn secondary" onclick={openPicker}>
						Send someone else
					</button>
				{:else}
					<p class="choices-note muted">You have no other peer to send in your stead.</p>
				{/if}
			{/if}
		</div>
	{/if}

	<!-- Stakes -->
	<div class="choices-section">
		<p class="choices-header">The stakes</p>
		<p class="choices-note">
			Each staked asset hides a rolled die for the duel and final difficulty result.
			More dice means more control, but the staked assets could be taken if you lose.
			All staked assets are leveraged when the duel ends.
		</p>
		{#if amParticipant}
			{#if iCommitted}
				<p class="choices-note">
					You've staked {myStakes.length} asset{myStakes.length === 1 ? '' : 's'}.
				</p>
			{:else if effectiveMaxStakes < 1}
				<Notice tone="warning">
					You have no assets available to stake, so you cannot answer this duel.
				</Notice>
			{:else}
				<CardPicker
					label="Pick 1–{effectiveMaxStakes} of your assets{effectiveMaxStakes < myMaxStakes ? ` (all you own; esteem allows ${myMaxStakes})` : ''}"
					items={myStakeableAssets}
					{players}
					ownerLabel={stakeableLabel}
					multi
					max={effectiveMaxStakes}
					selectedMulti={stakeSelectionIDs}
					onSelectMulti={(ids) => (stakeSelectionIDs = ids)}
				/>
				{#if stakeError}<ErrorText message={stakeError} variant="panel" />{/if}
				<button class="action-btn primary"
					onclick={commitStakes}
					disabled={stakeBusy || !stakeCountValid}>
					{stakeBusy ? '…' : `Commit ${stakeSelectionIDs.length} stake${stakeSelectionIDs.length === 1 ? '' : 's'}`}
				</button>
				<EndsNote text="The bouts begin as soon as both duellists have committed their stakes." />
			{/if}
		{:else}
			<p class="choices-note muted">The duellists are choosing their stakes.</p>
		{/if}
	</div>
</div>
