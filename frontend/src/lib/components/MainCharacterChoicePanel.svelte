<!-- MainCharacterChoicePanel.svelte
  Takeover panel for RowStateAwaitMainCharacterChoice: one or more players lost
  their main character (it was taken, traded, or destroyed) and must choose a
  replacement before play resumes.

  Every player sees the panel (the whole table is paused). Only a player who is
  in `actingPlayerIDs` gets inputs:
    • Has at least one peer  → promote one (free).
    • Has no peers left      → conscript a brand new peer as main character, at
                               the custom-rule cost of all their assets becoming
                               leveraged.
  Everyone else sees who the table is waiting on.

  The panel is a heading and its steps, with no box: the play area already
  frames it, and boxes are for things a player presses. The cost of conscripting is a
  warning notice (orange: careful), not a red tinted box, since nothing has
  been lost yet and the player is choosing it.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import type { Asset, Player } from '$lib/api';
	import { updateAsset, replaceMainCharacter } from '$lib/api';
	import CardPicker from '$lib/components/plans/CardPicker.svelte';
	import AssetCreationForm from '$lib/components/AssetCreationForm.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import Notice from '$lib/components/shared/Notice.svelte';
	import EndsNote from '$lib/components/plans/shared/EndsNote.svelte';

	interface Props {
		gameID: number;
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		/** Players who still owe a replacement choice (server-authoritative). */
		actingPlayerIDs: number[];
		playerNameMap: Map<number, string>;
	}

	let { gameID, assets, players, currentPlayerID, actingPlayerIDs, playerNameMap }: Props = $props();

	const amActing = $derived(currentPlayerID != null && actingPlayerIDs.includes(currentPlayerID));

	// My remaining peers — the promote pool. (The lost main character is already
	// gone from my retinue: taken assets change owner, destroyed ones are flagged.)
	const myPeers = $derived(
		assets.filter(
			(a) => a.owner_id === currentPlayerID && a.asset_type === 'peer' && !a.is_destroyed,
		),
	);

	const otherWaiters = $derived(
		actingPlayerIDs
			.filter((id) => id !== currentPlayerID)
			.map((id) => playerNameMap.get(id) ?? `Player ${id}`),
	);

	let busy = $state(false);
	let err = $state<string | null>(null);

	// ── Promote an existing peer ──────────────────────────────────────────────
	let selectedPeerID = $state<number | null>(null);

	async function promote() {
		if (selectedPeerID == null || busy) return;
		busy = true;
		err = null;
		try {
			// No current main character → a clean flip, no tear required.
			await updateAsset(selectedPeerID, { is_main_character: true });
			// The row-state broadcast clears the gate for everyone.
		} catch (e) {
			err = e instanceof Error ? e.message : 'Could not set main character.';
		} finally {
			busy = false;
		}
	}

	// ── Conscript a new peer (no peers left) ──────────────────────────────────
	let newName = $state('');
	let newMarginalia = $state('');

	async function conscript() {
		if (busy) return;
		const name = newName.trim();
		const marginalia = newMarginalia.trim();
		if (name === '' || marginalia === '') {
			err = 'Give your new character a name and one marginalia.';
			return;
		}
		busy = true;
		err = null;
		try {
			await replaceMainCharacter(gameID, { name, marginalia: [marginalia] });
		} catch (e) {
			err = e instanceof Error ? e.message : 'Could not create main character.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="mc-choice">
	<h3>Main character lost</h3>

	{#if amActing}
		<p class="lede">
			Your main character is no longer yours: they were taken, traded away or
			destroyed. Everyone needs one before play goes on, so the table is waiting
			on you. The log says what happened to them.
		</p>

		{#if myPeers.length > 0}
			<CardPicker
				label="Choose a peer from your retinue to become your new main character"
				items={myPeers}
				{players}
				selected={selectedPeerID}
				onSelect={(id) => (selectedPeerID = id)}
			/>
			<div class="actions">
				<button class="action-btn primary" onclick={promote} disabled={busy || selectedPeerID == null}>
					{busy ? '…' : 'Make main character'}
				</button>
			</div>
		{:else}
			<Notice tone="warning">
				You have no peers left to promote. You may conscript a brand new
				character — but the upheaval costs you: all of your assets,
				including this new one, will become leveraged.
			</Notice>
			<AssetCreationForm
				{gameID}
				assetType="peer"
				bind:name={newName}
				bind:marginalia={newMarginalia}
				disabled={busy}
			/>
			<div class="actions">
				<button class="action-btn primary" onclick={conscript}
					disabled={busy || !newName.trim() || !newMarginalia.trim()}>
					{busy ? '…' : 'Conscript new main character'}
				</button>
			</div>
		{/if}

		<EndsNote text="Play goes on once every player who lost a main character has chosen a new one." />
		{#if err}<ErrorText message={err} />{/if}
	{:else}
		<p class="lede">
			{otherWaiters.length > 0 ? otherWaiters.join(', ') : 'A player'} lost
			{otherWaiters.length > 1 ? 'their main characters' : 'their main character'} and must
			choose {otherWaiters.length > 1 ? 'replacements' : 'a replacement'} before play
			goes on. The log says what happened.
		</p>
	{/if}
</div>

<style>
	.mc-choice {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h3 {
		margin: 0;
		font-family: var(--font-serif);
		font-size: var(--text-heading);
		color: var(--color-accent);
	}
	.lede {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.4;
	}
	.actions {
		display: flex;
		justify-content: center;
		gap: var(--space-2);
		margin-top: var(--space-1);
	}
</style>
