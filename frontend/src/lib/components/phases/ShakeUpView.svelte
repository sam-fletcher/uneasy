<!-- ShakeUpView.svelte
  Phase 4c — Shake-Up endgame UI.

  Two sub-modes driven by game.shake_up_step:
    1 (rolling): each player submits a single roll-result for the active
      category. After the last submission the server advances to step 2.
    2 (spending): players take turns in reverse rank order. The active
      spender announces an option (deducts 1 token immediately), other
      players post ±1 adjustments (1 token each), and the spender commits
      to lock in the final cost and trigger the mechanical effect.
-->
<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import {
		getShakeUp, shakeUpRoll, shakeUpSpend, shakeUpAdjust, shakeUpCommit,
	} from '$lib/api';
	import type {
		Game, Player, Asset, ShakeUpOptionInfo, ShakeUpSpend,
		ShakeUpAdjustmentRow, ShakeUpTokensRow,
	} from '$lib/api';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import { playerColor } from '$lib/playerColor';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
	}
	let { gameID, game, players, assets, currentPlayerID }: Props = $props();

	let tokens = $state<ShakeUpTokensRow[]>([]);
	let options = $state<ShakeUpOptionInfo[]>([]);
	let openSpend = $state<{ spend: ShakeUpSpend; adjustments: ShakeUpAdjustmentRow[] } | null>(null);
	let error = $state('');
	let busy = $state(false);

	async function refresh() {
		try {
			const data = await getShakeUp(gameID);
			tokens = data.tokens;
			options = data.options ?? [];
			openSpend = data.open_spend ?? null;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load shake-up state.';
		}
	}

	onMount(refresh);

	function onShakeUpEvent() { refresh(); }
	onMount(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.ended',
		]) window.addEventListener(`uneasy:${t}`, onShakeUpEvent);
	});
	onDestroy(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.ended',
		]) window.removeEventListener(`uneasy:${t}`, onShakeUpEvent);
	});

	const myTokens = $derived(
		tokens.find(t => t.id === currentPlayerID)?.shake_up_tokens ?? 0
	);
	function playerName(id: number | null): string {
		if (id == null) return '?';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	// ── Step 1: rolling ──────────────────────────────────────────────────────
	let rollResult = $state(1);
	async function submitRoll() {
		if (busy) return;
		busy = true; error = '';
		try {
			await shakeUpRoll(gameID, rollResult);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not submit roll.';
		} finally {
			busy = false;
		}
	}

	// ── Step 2: announce spend ──────────────────────────────────────────────
	let pickedOption = $state<string>('');
	let pickedAssetID = $state<number | ''>('');
	const pickedOptionInfo = $derived(options.find(o => o.Key === pickedOption));
	const myAssets = $derived(assets.filter(a => !a.is_destroyed));

	async function announce() {
		if (!pickedOption || busy) return;
		busy = true; error = '';
		try {
			const body: { option_key: string; target_asset_id?: number } = { option_key: pickedOption };
			if (pickedOptionInfo?.NeedsAsset && pickedAssetID !== '') {
				body.target_asset_id = pickedAssetID as number;
			}
			await shakeUpSpend(gameID, body);
			pickedOption = '';
			pickedAssetID = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not announce spend.';
		} finally {
			busy = false;
		}
	}

	async function adjust(direction: 1 | -1) {
		if (!openSpend || busy) return;
		busy = true; error = '';
		try {
			await shakeUpAdjust(gameID, openSpend.spend.id, direction);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not adjust.';
		} finally {
			busy = false;
		}
	}

	async function commit() {
		if (!openSpend || busy) return;
		busy = true; error = '';
		try {
			await shakeUpCommit(gameID, openSpend.spend.id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not commit.';
		} finally {
			busy = false;
		}
	}

	const isMySpend = $derived(openSpend?.spend.player_id === currentPlayerID);
	const adjustmentTotal = $derived(
		(openSpend?.adjustments ?? []).reduce((sum, a) => sum + a.adjustment, 0)
	);
</script>

<div class="shake-up">
	<header>
		<h2>Shake-Up</h2>
		<div class="state">
			<span>Category: <strong>{game.shake_up_category ?? '—'}</strong></span>
			<span>Step: <strong>{game.shake_up_step === 1 ? 'rolling' : game.shake_up_step === 2 ? 'spending' : '—'}</strong></span>
			<span>Your tokens: <strong>{myTokens}</strong></span>
		</div>
	</header>

	{#if error}<p class="local-error">{error}</p>{/if}

	<section class="tokens-panel">
		<h3>Token pools</h3>
		<ul>
			{#each tokens as t}
				<li>{playerName(t.id)}: <strong>{t.shake_up_tokens}</strong></li>
			{/each}
		</ul>
	</section>

	{#if game.shake_up_step === 1}
		<section>
			<h3>Submit your roll</h3>
			<p class="muted small">
				Roll your dice (you may leverage your own assets, but no help/interference). Enter the total result.
				It will be added to your token pool.
			</p>
			<div class="roll-form">
				<input type="number" min="1" max="60" bind:value={rollResult} />
				<button class="primary" disabled={busy || myTokens > 0} onclick={submitRoll}>
					{busy ? '…' : 'Submit roll'}
				</button>
			</div>
			{#if myTokens > 0}
				<p class="muted small">You've rolled — waiting on others.</p>
			{/if}
		</section>

	{:else if game.shake_up_step === 2}
		{#if openSpend}
			<section class="spend-panel">
				<h3>Open spend</h3>
				<p>
					<strong>{playerName(openSpend.spend.player_id)}</strong>
					announced <strong>{openSpend.spend.option_key}</strong>
					{#if openSpend.spend.target_asset_id != null}
						on asset #{openSpend.spend.target_asset_id}
					{/if}
				</p>
				<p class="muted small">
					Base cost {openSpend.spend.base_cost} · adjustments {adjustmentTotal >= 0 ? '+' : ''}{adjustmentTotal}
					· running cost: <strong>{openSpend.spend.base_cost + adjustmentTotal}</strong>
				</p>
				<div class="adjust-buttons">
					{#if !isMySpend}
						<button disabled={busy || myTokens < 1} onclick={() => adjust(1)}>+1 (costs you 1 token)</button>
						<button disabled={busy || myTokens < 1} onclick={() => adjust(-1)}>−1 (costs you 1 token)</button>
					{:else}
						<button class="primary" disabled={busy} onclick={commit}>Commit</button>
					{/if}
				</div>
			</section>
		{:else if myTokens > 0}
			<section>
				<h3>Announce a spend</h3>
				<div class="announce-form">
					<div class="su-form-row">
						<span class="su-form-label">Option:</span>
						<div class="su-chip-row">
							{#each options as opt}
								<button
									type="button"
									class="su-chip"
									class:active={pickedOption === opt.Key}
									title={opt.Description}
									onclick={() => {
										pickedOption = pickedOption === opt.Key ? '' : opt.Key;
										pickedAssetID = '';
									}}
								>{opt.Description}</button>
							{/each}
						</div>
					</div>
					{#if pickedOptionInfo?.NeedsAsset}
						<div class="su-form-row">
							<span class="su-form-label">Target asset:</span>
							{#if myAssets.length === 0}
								<p class="muted" style="margin:0;">No eligible assets.</p>
							{:else}
								<div class="su-peer-cards">
									{#each myAssets as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={pickedAssetID === a.id}
											onToggle={() => (pickedAssetID = pickedAssetID === a.id ? '' : a.id)}
										/>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
					<button class="primary" disabled={!pickedOption || busy} onclick={announce}>
						{busy ? '…' : 'Announce (cost 1 token)'}
					</button>
				</div>
			</section>
		{:else}
			<p class="muted">You have no tokens. Waiting for others to finish spending — the category advances when everyone is at zero.</p>
		{/if}
	{:else}
		<p class="muted">Shake-Up state unavailable.</p>
	{/if}
</div>

<style>
	.shake-up {
		display: flex; flex-direction: column; gap: 1rem;
		padding: 1rem 0;
		flex: 1; min-height: 0; overflow-y: auto;
	}
	.shake-up h2 { color: #c8a96e; font-size: 1.3rem; margin: 0; }
	.shake-up h3 { color: #c8a96e; font-size: 1rem; margin: 0.25rem 0; }
	.state { display: flex; gap: 1.25rem; font-size: 0.9rem; color: #ccc; flex-wrap: wrap; margin-top: 0.25rem; }
	.muted { color: #999; font-size: 0.9rem; margin: 0; }
	.muted.small { font-size: 0.8rem; }
	.local-error { color: #e07070; font-size: 0.85rem; margin: 0; }

	.tokens-panel ul { list-style: none; padding: 0; margin: 0.25rem 0 0; display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.9rem; }

	.roll-form, .announce-form {
		display: flex; flex-direction: column; gap: 0.5rem;
		max-width: 24rem;
		background: #1e1e1e; border: 1px solid #333; border-radius: 8px;
		padding: 0.75rem;
	}
	.roll-form { flex-direction: row; align-items: end; }

	.su-form-row { display: flex; flex-direction: column; gap: 0.3rem; }
	.su-form-label { font-size: 0.85rem; color: #aaa; }
	.su-chip-row { display: flex; flex-wrap: wrap; gap: 0.35rem; }
	.su-chip {
		display: inline-flex; align-items: center;
		min-height: 44px; padding: 0.35rem 0.85rem;
		border-radius: 999px;
		border: 1px solid #555;
		background: #2a2a2a;
		color: #e8e4d9;
		font-size: 0.9rem;
		cursor: pointer;
		text-align: left;
	}
	.su-chip.active { border-color: #c8a96e; background: #3a2f18; }
	.su-chip:focus-visible { outline: 2px solid #c8a96e; outline-offset: 1px; }

	.su-peer-cards { display: flex; flex-direction: column; gap: 0.4rem; }
	input {
		background: #333; color: #e8e4d9; border: 1px solid #555;
		border-radius: 4px; padding: 0.3rem 0.5rem; font-size: 0.9rem;
	}

	.spend-panel {
		background: #1e1e1e; border: 1px solid #444; border-radius: 8px; padding: 0.75rem;
	}
	.adjust-buttons { display: flex; gap: 0.5rem; margin-top: 0.5rem; flex-wrap: wrap; }

	button {
		background: #333; color: #e8e4d9; border: 1px solid #555;
		border-radius: 6px; padding: 0.4rem 0.8rem; font-weight: 600;
		cursor: pointer;
	}
	button:disabled { opacity: 0.4; cursor: not-allowed; }
	.primary { background: #c8a96e; color: #1a1a1a; border-color: #c8a96e; }
</style>
