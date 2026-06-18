<!-- Festivity/SocializingTurn.svelte
  The current player's own turn during socializing: roll-or-opt-out, then
  pick a make/mar option once the outcome is known. The make-option
  `challenge_duel` issues a duel challenge instead of going through
  /guest-choice (the festivity then surfaces a ChallengeBanner).
-->
<script lang="ts">
	import { guestRoll, guestChoice, challengeDuel, getAssetSuggestions, type Asset, type Plan, type Player } from '$lib/api';
	import PlayerChips from '../PlayerChips.svelte';
	import CardPicker from '../CardPicker.svelte';
	import SuggestionPicker from '../../SuggestionPicker.svelte';
	import FormField from '../FormField.svelte';
	import { playerName } from '../shared';
	import { destructionWarning } from '$lib/assetRisk';
	import { MAKE_OPTS, MAR_OPTS, type FestRes } from './options';

	let {
		plan, fest, players, assets, currentPlayerID, myRollID,
		myEffectiveOutcome, difficulty, blockedByOtherRoll, activeRollerID, onPlansChanged,
	}: {
		plan: Plan;
		fest: FestRes;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		myRollID: number | null;
		myEffectiveOutcome: 'make' | 'mar' | null;
		/** Roll difficulty (host's esteem status), shown on the Roll button. */
		difficulty: number;
		/** True while another guest's roll-and-choice is in progress — blocks acting. */
		blockedByOtherRoll: boolean;
		/** The guest whose roll-and-choice is in progress, for the "waiting on" note. */
		activeRollerID: number | null;
		onPlansChanged: () => void;
	} = $props();

	let actionBusy = $state(false);
	let actionError = $state('');

	async function onRoll() {
		if (actionBusy) return;
		actionBusy = true; actionError = '';
		try { await guestRoll(plan.id, 'roll'); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not roll.'; }
		finally { actionBusy = false; }
	}
	async function onOptOut() {
		if (actionBusy) return;
		actionBusy = true; actionError = '';
		try { await guestRoll(plan.id, 'opt_out'); onPlansChanged(); }
		catch (e) { actionError = e instanceof Error ? e.message : 'Could not opt out.'; }
		finally { actionBusy = false; }
	}

	let pickedChoice = $state<string | null>(null);
	let rumorText = $state('');
	let peerName = $state('');
	let pickedAssetID = $state<number | null>(null);
	let pickedMargID = $state<number | null>(null);
	let pickedDuelTargetID = $state<number | null>(null);
	let pickerBusy = $state(false);
	let pickerError = $state('');

	const myCenterPeerCandidates = $derived(
		assets.filter(a => fest.centeredAssetIDs.includes(a.id) && !a.is_destroyed),
	);
	const myOwnPeers = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.asset_type === 'peer'
				&& !a.is_destroyed),
	);
	// The acting player's main character with an intact marginalia — the
	// break_self target (the breaker picks which marginalia to tear).
	const myMainCharacter = $derived(
		currentPlayerID == null
			? []
			: assets.filter(a =>
				a.owner_id === currentPlayerID
				&& a.is_main_character
				&& !a.is_destroyed
				&& (a.marginalia ?? []).some(m => !m.is_torn)),
	);
	const breakSelfWarn = $derived(destructionWarning(assets.find(a => a.id === pickedAssetID)));
	const otherGuests = $derived(
		fest.guests.filter(id => id !== currentPlayerID),
	);

	// Peer-name suggestions, fetched lazily when introduce_peer is chosen.
	let peerNameSuggestions = $state<string[]>([]);
	let peerNameSuggLoading = $state(false);
	let peerNameSuggFetched = false;

	// Reset picker when plan changes.
	let lastPlanID = $state<number | null>(null);
	$effect(() => {
		if (plan.id !== lastPlanID) {
			lastPlanID = plan.id;
			pickedChoice = null;
			rumorText = '';
			peerName = '';
			pickedAssetID = null;
			pickedMargID = null;
			pickedDuelTargetID = null;
			pickerError = '';
			peerNameSuggestions = [];
			peerNameSuggFetched = false;
		}
	});

	$effect(() => {
		if (pickedChoice !== 'introduce_peer' || peerNameSuggFetched) return;
		peerNameSuggFetched = true;
		peerNameSuggLoading = true;
		getAssetSuggestions(plan.game_id, 'peer', 'name')
			.then(res => { peerNameSuggestions = res.suggestions; })
			.catch(() => { peerNameSuggestions = []; })
			.finally(() => { peerNameSuggLoading = false; });
	});

	async function submitMyChoice() {
		if (pickerBusy || !pickedChoice) return;
		pickerBusy = true; pickerError = '';
		try {
			if (myEffectiveOutcome === 'make' && pickedChoice === 'challenge_duel') {
				if (pickedDuelTargetID == null) {
					pickerError = 'Pick a target.';
					return;
				}
				await challengeDuel(plan.id, pickedDuelTargetID);
			} else {
				const body: {
					choice: string; rumor_text?: string; peer_name?: string;
					asset_id?: number; marginalia_id?: number;
				} = { choice: pickedChoice };
				if (pickedChoice === 'spread_rumor' || pickedChoice === 'rumor_about_you') {
					body.rumor_text = rumorText.trim();
				}
				if (pickedChoice === 'introduce_peer') {
					body.peer_name = peerName.trim() || 'New peer';
				}
				if (pickedChoice === 'take_center_peer' || pickedChoice === 'disagreement') {
					if (pickedAssetID == null) { pickerError = 'Pick an asset.'; return; }
					body.asset_id = pickedAssetID;
				}
				if (pickedChoice === 'break_self') {
					if (pickedMargID == null) { pickerError = 'Pick a marginalia to tear.'; return; }
					body.asset_id = pickedAssetID ?? undefined;
					body.marginalia_id = pickedMargID;
				}
				await guestChoice(plan.id, body);
			}
			pickedChoice = null;
			rumorText = '';
			peerName = '';
			pickedAssetID = null;
			pickedDuelTargetID = null;
			onPlansChanged();
		} catch (e) {
			pickerError = e instanceof Error ? e.message : 'Could not submit choice.';
		} finally { pickerBusy = false; }
	}
</script>

<div class="choices-section">
	{#if myRollID == null}
		{#if actionError}<p class="res-error">{actionError}</p>{/if}
		{#if blockedByOtherRoll}
			<p class="choices-note muted">
				Waiting for {playerName(players, activeRollerID)} to finish their roll…
			</p>
		{/if}
		<div class="form-row">
			<button
				class="action-btn primary"
				onclick={onRoll}
				disabled={actionBusy || blockedByOtherRoll}
				title="Difficulty {difficulty} — the host's standing in esteem (higher is harder to impress)"
			>
				{actionBusy ? '…' : `Roll · difficulty ${difficulty}`}
			</button>
			<button class="action-btn" onclick={onOptOut} disabled={actionBusy || blockedByOtherRoll}>
				Opt out
			</button>
		</div>
	{:else if myEffectiveOutcome == null}
		<p class="choices-note muted">Rolling… resolve the dice above.</p>
	{:else}
		{@const opts = myEffectiveOutcome === 'make' ? MAKE_OPTS : MAR_OPTS}
		<p class="choices-header">
			Result:
			<strong class="outcome-{myEffectiveOutcome}">
				{myEffectiveOutcome === 'make' ? '✓ Make' : '✗ Mar'}
			</strong>
			— pick one option:
		</p>
		<FormField label="Pick one option">
			<div class="chip-row">
				{#each opts as o}
					<button
						type="button"
						class="chip-btn"
						class:active={pickedChoice === o.key}
						onclick={() => {
							pickedChoice = pickedChoice === o.key ? null : o.key;
							pickedAssetID = null;
							pickedMargID = null;
							pickedDuelTargetID = null;
						}}
					>{o.label}</button>
				{/each}
			</div>
		</FormField>

		{#if pickedChoice === 'spread_rumor' || pickedChoice === 'rumor_about_you'}
			<label class="form-label">
				Rumor text:
				<textarea rows={2} bind:value={rumorText} class="form-textarea"
					placeholder="What does the rumor say?"></textarea>
			</label>
		{:else if pickedChoice === 'introduce_peer'}
			<div class="form-label">
				<span>New peer's name:</span>
				<SuggestionPicker
					suggestions={peerNameSuggestions}
					bind:value={peerName}
					loading={peerNameSuggLoading}
					customPlaceholder="Name of the new peer"
					maxlength={120}
				/>
			</div>
		{:else if pickedChoice === 'take_center_peer'}
			<CardPicker
				label="Peer to take from the center"
				items={myCenterPeerCandidates}
				{players}
				emptyMessage="No peers in the center of the table."
				ownerLabel={(a) => `Owned by ${playerName(players, a.owner_id)}`}
				selected={pickedAssetID}
				onSelect={(id) => (pickedAssetID = id)}
			/>
		{:else if pickedChoice === 'disagreement'}
			<CardPicker
				label="Peer to set in the center"
				items={myOwnPeers}
				{players}
				emptyMessage="You have no peers to set in the center."
				selected={pickedAssetID}
				onSelect={(id) => (pickedAssetID = id)}
			/>
		{:else if pickedChoice === 'break_self'}
			<CardPicker
				label="Marginalia to tear on your main character"
				items={myMainCharacter}
				{players}
				emptyMessage="Your main character has no intact marginalia."
				marginaliaMode
				selectedMarginaliaID={pickedMargID}
				onSelectMarginalia={(mID, parent) => {
					pickedMargID = mID;
					pickedAssetID = parent?.id ?? null;
				}}
			/>
			{#if breakSelfWarn}<p class="res-warning">{breakSelfWarn}</p>{/if}
		{:else if pickedChoice === 'challenge_duel'}
			{@const duelTargetPlayers = otherGuests
				.map(gid => players.find(p => p.id === gid))
				.filter((p): p is typeof players[number] => p != null)}
			<FormField label="Challenge">
				<PlayerChips
					players={duelTargetPlayers}
					isActive={(p) => pickedDuelTargetID === p.id}
					onSelect={(p) => (pickedDuelTargetID = pickedDuelTargetID === p.id ? null : p.id)}
				/>
				{#if pickedDuelTargetID != null && fest.acceptDuels.includes(pickedDuelTargetID)}
					<p class="choices-note muted" style="margin:0.25rem 0 0;">
						This challenge must be accepted.
					</p>
				{/if}
			</FormField>
		{/if}

		{#if pickerError}<p class="res-error">{pickerError}</p>{/if}
		<button class="action-btn primary"
			onclick={submitMyChoice}
			disabled={pickerBusy || !pickedChoice}>
			{pickerBusy ? '…' : 'Submit choice'}
		</button>
	{/if}
</div>
