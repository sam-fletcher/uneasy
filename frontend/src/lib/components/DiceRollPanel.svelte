<!-- DiceRollPanel.svelte
  Shows the active dice roll for all players.
  - Actor sees: leverage button, call-vote button, roll (close-leverage) button.
  - Others see: leverage-to-interfere button, vote buttons when voting is open.
  - Everyone sees: dice pool, interference dice, cancellations, result.
-->
<script lang="ts">
	import type { DiceRoll, DiceRollDie, DifficultyVote, Asset, Player } from '$lib/api';
	import { leverageRoll, callVote, voteOnRoll, closeLeverage } from '$lib/api';

	interface Props {
		roll: DiceRoll;
		dice: DiceRollDie[];
		votes: DifficultyVote[];
		/** Is a difficulty vote currently open (call-vote was issued)? */
		voteOpen: boolean;
		assets: Asset[];
		currentPlayerID: number | null;
		players: Player[];
		playerNameMap: Map<number, string>;
		/** True if the current player is the facilitator. */
		isFacilitator: boolean;
	}

	let {
		roll = $bindable(),
		dice = $bindable(),
		votes = $bindable(),
		voteOpen = $bindable(),
		assets,
		currentPlayerID,
		players,
		playerNameMap,
		isFacilitator,
	}: Props = $props();

	const isActor = $derived(currentPlayerID === roll.actor_id);
	const isResolved = $derived(roll.result !== null);
	const canClose = $derived((isActor || isFacilitator) && !isResolved);

	// Split dice into actor pool and interference.
	const actorDice = $derived(dice.filter(d => !d.is_interference));
	const intDice = $derived(dice.filter(d => d.is_interference));

	// Assets the current player could leverage (not destroyed, not already on this roll).
	const committedAssetIDs = $derived(new Set(dice.map(d => d.leveraged_asset_id).filter(id => id != null)));
	const leverageableAssets = $derived(
		assets.filter(a =>
			a.owner_id === currentPlayerID &&
			!a.is_destroyed &&
			!committedAssetIDs.has(a.id)
		)
	);

	// Votes the current player has cast.
	const myVote = $derived(votes.find(v => v.player_id === currentPlayerID));

	// Effective difficulty shown to player.
	const effectiveDifficulty = $derived(roll.adjusted_difficulty ?? roll.difficulty);

	let busy = $state(false);
	let error = $state('');

	// ── Leverage ──────────────────────────────────────────────────────────────
	let showLeveragePicker = $state(false);

	async function onLeverage(assetID: number) {
		if (busy) return;
		busy = true;
		error = '';
		try {
			const { die } = await leverageRoll(roll.id, assetID);
			dice = [...dice, die];
			showLeveragePicker = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not leverage asset.';
		} finally {
			busy = false;
		}
	}

	// ── Call vote ─────────────────────────────────────────────────────────────
	async function onCallVote() {
		if (busy) return;
		busy = true;
		error = '';
		try {
			await callVote(roll.id);
			voteOpen = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not call vote.';
		} finally {
			busy = false;
		}
	}

	// ── Vote ──────────────────────────────────────────────────────────────────
	async function onVote(v: 'yea' | 'nay') {
		if (busy || myVote) return;
		busy = true;
		error = '';
		try {
			const result = await voteOnRoll(roll.id, v);
			votes = [...votes.filter(vt => vt.player_id !== currentPlayerID!), {
				roll_id: roll.id,
				player_id: currentPlayerID!,
				vote: v,
				voted_at: new Date().toISOString(),
			}];
			if (result.adjusted_difficulty != null) {
				roll = { ...roll, adjusted_difficulty: result.adjusted_difficulty };
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not submit vote.';
		} finally {
			busy = false;
		}
	}

	// ── Close / Roll ──────────────────────────────────────────────────────────
	async function onRoll() {
		if (busy || !canClose) return;
		busy = true;
		error = '';
		try {
			const data = await closeLeverage(roll.id);
			roll = data.roll;
			dice = data.dice;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not roll dice.';
		} finally {
			busy = false;
		}
	}
</script>

<div class="roll-panel">
	<div class="roll-header">
		<span class="roll-title">Dice Roll</span>
		<span class="roll-meta">
			Difficulty <strong>{roll.difficulty}</strong>
			{#if roll.adjusted_difficulty != null && roll.adjusted_difficulty !== roll.difficulty}
				→ <strong class="adjusted">{roll.adjusted_difficulty}</strong>
			{/if}
		</span>
		<span class="roll-actor">
			Actor: {playerNameMap.get(roll.actor_id) ?? '?'}
		</span>
	</div>

	{#if error}
		<p class="roll-error">{error}</p>
	{/if}

	<!-- ── Dice pools ────────────────────────────────────────────────────── -->
	<div class="dice-section">
		<div class="dice-group">
			<span class="dice-label">Actor's dice</span>
			<div class="dice-row">
				{#each actorDice as die (die.id)}
					<div
						class="die"
						class:cancelled={die.is_cancelled}
						class:unrolled={die.face == null}
						title={die.leveraged_asset_id
							? `${assets.find(a => a.id === die.leveraged_asset_id)?.name ?? 'asset'} leveraged`
							: 'base die'}
					>
						{die.face ?? '?'}
					</div>
				{/each}
			</div>
		</div>

		{#if intDice.length > 0}
			<div class="dice-group interference">
				<span class="dice-label">Interference</span>
				<div class="dice-row">
					{#each intDice as die (die.id)}
						<div
							class="die int"
							title={playerNameMap.get(die.player_id) ?? '?'}
						>
							{die.face ?? '?'}
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>

	<!-- ── Result ────────────────────────────────────────────────────────── -->
	{#if isResolved}
		<div class="result-banner" class:make={roll.outcome === 'make'} class:mar={roll.outcome === 'mar'}>
			<span class="result-label">{roll.outcome === 'make' ? 'Make' : 'Mar'}</span>
			<span class="result-score">{roll.result} distinct face{roll.result === 1 ? '' : 's'} vs. difficulty {effectiveDifficulty}</span>
		</div>
	{/if}

	<!-- ── Actions ───────────────────────────────────────────────────────── -->
	{#if !isResolved}
		<div class="roll-actions">

			<!-- Leverage picker -->
			{#if leverageableAssets.length > 0}
				{#if showLeveragePicker}
					<div class="leverage-picker">
						<span class="picker-label">
							{isActor ? 'Leverage to add a die:' : 'Leverage to interfere:'}
						</span>
						{#each leverageableAssets as asset (asset.id)}
							<button
								class="leverage-item"
								onclick={() => onLeverage(asset.id)}
								disabled={busy}
							>
								<span class="lev-name">{asset.name}</span>
								<span class="lev-type">{asset.asset_type}</span>
							</button>
						{/each}
						<button class="text-btn" onclick={() => { showLeveragePicker = false; }}>Cancel</button>
					</div>
				{:else}
					<button class="action-btn secondary" onclick={() => { showLeveragePicker = true; }} disabled={busy}>
						{isActor ? 'Leverage asset (+1 die)' : 'Interfere (leverage to add opposition die)'}
					</button>
				{/if}
			{/if}

			<!-- Vote section -->
			{#if voteOpen}
				<div class="vote-section">
					<span class="vote-label">Difficulty vote — {votes.length} of {players.length} cast</span>
					{#if !myVote}
						<div class="vote-buttons">
							<button class="vote-btn yea" onclick={() => onVote('yea')} disabled={busy}>
								👍 Yea (easier)
							</button>
							<button class="vote-btn nay" onclick={() => onVote('nay')} disabled={busy}>
								👎 Nay (harder)
							</button>
						</div>
					{:else}
						<span class="vote-cast">You voted: {myVote.vote === 'yea' ? '👍 yea' : '👎 nay'}</span>
					{/if}
				</div>
			{:else if isActor && !voteOpen}
				<button class="action-btn secondary" onclick={onCallVote} disabled={busy}>
					Call difficulty vote
				</button>
			{/if}

			<!-- Roll button (actor or facilitator) -->
			{#if canClose}
				<button class="action-btn primary roll-btn" onclick={onRoll} disabled={busy}>
					{busy ? '…' : '🎲 Roll the dice'}
				</button>
			{/if}
		</div>
	{/if}
</div>

<style>
	.roll-panel {
		border: 1px solid #4a3a20;
		border-radius: 6px;
		padding: 0.75rem;
		background: #1e1a10;
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
		flex-shrink: 0;
	}

	.roll-header {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		flex-wrap: wrap;
	}

	.roll-title {
		font-weight: 700;
		color: #c8a96e;
		font-size: 0.9rem;
	}

	.roll-meta {
		font-size: 0.85rem;
		color: #aaa;
	}

	.adjusted {
		color: #e0c070;
	}

	.roll-actor {
		font-size: 0.78rem;
		color: #888;
		margin-left: auto;
	}

	.roll-error {
		color: #e07070;
		font-size: 0.82rem;
		margin: 0;
	}

	/* ── Dice ─────────────────────────────────────────────────────────────── */

	.dice-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
	}

	.dice-group {
		display: flex;
		align-items: center;
		gap: 0.5rem;
	}

	.dice-group.interference .dice-label {
		color: #e07070;
	}

	.dice-label {
		font-size: 0.75rem;
		color: #888;
		min-width: 90px;
		flex-shrink: 0;
	}

	.dice-row {
		display: flex;
		gap: 0.35rem;
		flex-wrap: wrap;
	}

	.die {
		width: 32px;
		height: 32px;
		border-radius: 5px;
		border: 2px solid #c8a96e;
		background: #2a2010;
		color: #e8e4d9;
		font-weight: 700;
		font-size: 1rem;
		display: flex;
		align-items: center;
		justify-content: center;
		transition: opacity 0.2s;
	}

	.die.int {
		border-color: #e07070;
		background: #2a1010;
		color: #f0b0b0;
	}

	.die.cancelled {
		opacity: 0.3;
		text-decoration: line-through;
		border-style: dashed;
	}

	.die.unrolled {
		color: #666;
		border-color: #555;
		border-style: dashed;
	}

	/* ── Result banner ────────────────────────────────────────────────────── */

	.result-banner {
		display: flex;
		align-items: center;
		gap: 0.75rem;
		padding: 0.5rem 0.75rem;
		border-radius: 5px;
		border: 1px solid;
	}

	.result-banner.make {
		border-color: #6dbf7a;
		background: #0a1f0a;
	}

	.result-banner.mar {
		border-color: #e07070;
		background: #1f0a0a;
	}

	.result-label {
		font-size: 1.1rem;
		font-weight: 800;
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.result-banner.make .result-label { color: #6dbf7a; }
	.result-banner.mar .result-label { color: #e07070; }

	.result-score {
		font-size: 0.82rem;
		color: #aaa;
	}

	/* ── Actions ──────────────────────────────────────────────────────────── */

	.roll-actions {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
	}

	.action-btn {
		padding: 0.4rem 0.8rem;
		border-radius: 5px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		align-self: flex-start;
	}

	.action-btn.primary {
		background: #c8a96e;
		color: #1a1a1a;
	}

	.action-btn.secondary {
		background: #333;
		color: #c8a96e;
		border: 1px solid #4a4030;
	}

	.action-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	.roll-btn {
		align-self: stretch;
		text-align: center;
		font-size: 0.95rem;
		padding: 0.5rem 0.8rem;
	}

	/* Leverage picker */

	.leverage-picker {
		display: flex;
		flex-direction: column;
		gap: 0.25rem;
		background: #252525;
		border-radius: 5px;
		padding: 0.5rem;
	}

	.picker-label {
		font-size: 0.78rem;
		color: #c8a96e;
	}

	.leverage-item {
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.3rem 0.5rem;
		border-radius: 4px;
		background: #2a2a2a;
		text-align: left;
		font-size: 0.85rem;
		border: 1px solid #444;
		cursor: pointer;
		color: #e8e4d9;
	}

	.leverage-item:hover { background: #333; border-color: #c8a96e; }
	.leverage-item:disabled { opacity: 0.4; cursor: not-allowed; }

	.lev-name { flex: 1; }
	.lev-type { font-size: 0.72rem; color: #777; text-transform: capitalize; }

	.text-btn {
		background: none;
		color: #888;
		font-size: 0.78rem;
		padding: 0.2rem 0;
		cursor: pointer;
		text-decoration: underline dotted;
		align-self: flex-start;
	}

	/* Vote */

	.vote-section {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.5rem;
		border: 1px solid #3a3a20;
		border-radius: 5px;
		background: #1a1a10;
	}

	.vote-label {
		font-size: 0.78rem;
		color: #c8a96e;
	}

	.vote-buttons {
		display: flex;
		gap: 0.5rem;
	}

	.vote-btn {
		padding: 0.35rem 0.7rem;
		border-radius: 5px;
		font-size: 0.85rem;
		font-weight: 600;
		cursor: pointer;
		border: 1px solid;
	}

	.vote-btn.yea {
		background: #0a2a0a;
		border-color: #6dbf7a;
		color: #6dbf7a;
	}

	.vote-btn.nay {
		background: #2a0a0a;
		border-color: #e07070;
		color: #e07070;
	}

	.vote-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	.vote-cast {
		font-size: 0.82rem;
		color: #aaa;
	}
</style>
