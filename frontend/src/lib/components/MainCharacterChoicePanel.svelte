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
-->
<script lang="ts">
	import type { Asset, Player } from '$lib/api';
	import { updateAsset, replaceMainCharacter } from '$lib/api';
	import CardPicker from '$lib/components/plans/CardPicker.svelte';

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
	let newMargs = $state<string[]>(['', '']);

	function addMargField() {
		if (newMargs.length < 4) newMargs = [...newMargs, ''];
	}

	async function conscript() {
		if (busy) return;
		const name = newName.trim();
		const margs = newMargs.map((m) => m.trim()).filter((m) => m !== '');
		if (name === '') {
			err = 'Give your new character a name.';
			return;
		}
		if (margs.length < 2) {
			err = 'Write at least 2 marginalia.';
			return;
		}
		busy = true;
		err = null;
		try {
			await replaceMainCharacter(gameID, { name, marginalia: margs });
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
			Your main character is no longer yours.
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
				<button class="btn primary" onclick={promote} disabled={busy || selectedPeerID == null}>
					{busy ? '…' : 'Make main character'}
				</button>
			</div>
		{:else}
			<p class="warn">
				You have no peers left to promote. You may conscript a brand new
				character — but the upheaval costs you: <strong>all of your assets,
				including this new one, will become leveraged.</strong>
			</p>
			<label class="field">
				<span>Name</span>
				<input type="text" bind:value={newName} maxlength="80" placeholder="Who steps forward?" />
			</label>
			{#each newMargs as _, i (i)}
				<label class="field">
					<span>Marginalia {i + 1}</span>
					<input type="text" bind:value={newMargs[i]} maxlength="120" placeholder="A trait, tie, or detail" />
				</label>
			{/each}
			{#if newMargs.length < 4}
				<button class="btn ghost" onclick={addMargField} disabled={busy}>+ Add marginalia</button>
			{/if}
			<div class="actions">
				<button class="btn primary" onclick={conscript} disabled={busy}>
					{busy ? '…' : 'Conscript new main character'}
				</button>
			</div>
		{/if}

		{#if err}<p class="error">{err}</p>{/if}
	{:else}
		<p class="lede">
			Waiting for {otherWaiters.length > 0 ? otherWaiters.join(', ') : 'a player'} to
			choose a new main character before play resumes.
		</p>
	{/if}
</div>

<style>
	.mc-choice {
		border: 1px solid var(--color-border-warm);
		border-radius: 8px;
		padding: 1rem;
		background: var(--color-surface, var(--color-bg));
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	h3 {
		margin: 0;
		font-family: var(--font-serif);
		color: var(--color-accent);
	}
	.lede {
		margin: 0;
		font-size: 0.9rem;
		line-height: 1.4;
	}
	.warn {
		margin: 0;
		font-size: 0.88rem;
		line-height: 1.4;
		padding: 0.5rem 0.6rem;
		border-left: 3px solid var(--color-danger);
		background: color-mix(in srgb, var(--color-danger) 8%, transparent);
		border-radius: 4px;
	}
	.field {
		display: flex;
		flex-direction: column;
		gap: 0.2rem;
		font-size: 0.82rem;
	}
	.field input {
		min-height: 44px;
		padding: 0.4rem 0.6rem;
		border: 1px solid var(--color-border-warm);
		border-radius: 5px;
		font-size: 0.9rem;
		background: var(--color-bg);
		color: inherit;
	}
	.actions {
		display: flex;
		justify-content: center;
		gap: 0.5rem;
		margin-top: 0.3rem;
	}
	.btn {
		min-height: 44px;
		padding: 0.45rem 1rem;
		border-radius: 5px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		border: 1px solid var(--color-border-warm);
		background: transparent;
		color: inherit;
	}
	.btn.primary {
		background: var(--color-accent);
		color: var(--color-bg);
		border-color: var(--color-accent);
	}
	.btn.ghost {
		align-self: flex-start;
		font-weight: 500;
	}
	.btn:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
	.error {
		margin: 0;
		color: var(--color-danger);
		font-size: 0.82rem;
	}
</style>
