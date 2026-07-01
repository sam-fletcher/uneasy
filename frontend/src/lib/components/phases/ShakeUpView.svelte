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
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { onMount, onDestroy } from 'svelte';
	import {
		getShakeUp, shakeUpRoll, shakeUpSpend, shakeUpAdjust, shakeUpCommit,
	} from '$lib/api';
	import type {
		Game, Player, Asset, ShakeUpOptionInfo, ShakeUpSpend,
		ShakeUpAdjustmentRow, ShakeUpTokensRow, ClaimableTitle,
	} from '$lib/api';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import { playerColor } from '$lib/playerColor';
	import type { WaitingOnState, Waitee } from '$lib/waitingOn';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		currentPlayerID: number | null;
		waitingOn: WaitingOnState;
	}
	let { gameID, game, players, assets, currentPlayerID, waitingOn = $bindable() }: Props = $props();

	let tokens = $state<ShakeUpTokensRow[]>([]);
	let options = $state<ShakeUpOptionInfo[]>([]);
	let claimableTitles = $state<ClaimableTitle[]>([]);
	let openSpend = $state<{ spend: ShakeUpSpend; adjustments: ShakeUpAdjustmentRow[] } | null>(null);
	let currentActor = $state<number | null>(null);
	let error = $state('');
	let busy = $state(false);

	async function refresh() {
		try {
			const data = await getShakeUp(gameID);
			tokens = data.tokens;
			options = data.options ?? [];
			claimableTitles = data.claimable_titles ?? [];
			openSpend = data.open_spend ?? null;
			currentActor = data.current_actor ?? null;
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
	// Whose turn it is to announce a spend (reverse-rank order, server-driven).
	const isMyTurn = $derived(currentActor != null && currentActor === currentPlayerID);
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
	// Break options tear one marginalia, so the breaker also picks which.
	let pickedMarginaliaID = $state<number | ''>('');
	// Claim-a-title picks a title id + the peer (one of mine, with a free slot)
	// that receives it + optional freeform marginalia flavor.
	let pickedTitleID = $state<string>('');
	let titleFlavor = $state<string>('');
	const pickedOptionInfo = $derived(options.find(o => o.Key === pickedOption));
	const isClaimTitle = $derived(pickedOptionInfo?.Key === 'claim_title');
	const myAssets = $derived(assets.filter(a => !a.is_destroyed));
	// My own peers with a free marginalia slot (positions 1–4; torn marginalia
	// still occupy their slot) — the only valid recipients of a new title.
	const titleablePeers = $derived(
		assets.filter(a => a.owner_id === currentPlayerID && a.asset_type === 'peer'
			&& !a.is_destroyed && a.marginalia.length < 4)
	);
	// Intact (un-torn) marginalia on the chosen asset — the breakable choices.
	const breakableMarginalia = $derived(
		pickedAssetID === ''
			? []
			: (myAssets.find(a => a.id === pickedAssetID)?.marginalia ?? []).filter(m => !m.is_torn)
	);
	// A break announce is only ready once a marginalia is chosen; a claim-title
	// announce needs both a title and one of my peers to bear it.
	const announceReady = $derived(
		!!pickedOption &&
		(!pickedOptionInfo?.NeedsAsset || pickedAssetID !== '') &&
		(!pickedOptionInfo?.NeedsMarginalia || pickedMarginaliaID !== '') &&
		(!isClaimTitle || (pickedTitleID !== '' && pickedAssetID !== ''))
	);

	async function announce() {
		if (!announceReady || busy) return;
		busy = true; error = '';
		try {
			const body: {
				option_key: string;
				target_asset_id?: number;
				target_marginalia_id?: number;
				target_title_id?: string;
				title_flavor?: string;
			} = { option_key: pickedOption };
			if (pickedOptionInfo?.NeedsAsset && pickedAssetID !== '') {
				body.target_asset_id = pickedAssetID as number;
			}
			if (pickedOptionInfo?.NeedsMarginalia && pickedMarginaliaID !== '') {
				body.target_marginalia_id = pickedMarginaliaID as number;
			}
			if (isClaimTitle) {
				body.target_title_id = pickedTitleID;
				body.target_asset_id = pickedAssetID as number;
				if (titleFlavor.trim()) body.title_flavor = titleFlavor.trim();
			}
			await shakeUpSpend(gameID, body);
			pickedOption = '';
			pickedAssetID = '';
			pickedMarginaliaID = '';
			pickedTitleID = '';
			titleFlavor = '';
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

	// ── Waiting-on derivation ────────────────────────────────────────────────
	//   Step 1 (rolling): players whose token pool is still 0 haven't rolled.
	//   Step 2 (spending): if a spend is open, waiting on the spender to
	//     commit. Otherwise waiting on whichever players still hold tokens
	//     (the rule is reverse-rank order, but token count is the visible
	//     proxy — anyone at zero has either spent down or never rolled high).
	const shakeUpWaitingOn = $derived.by<WaitingOnState>(() => {
		if (game.shake_up_step === 1) {
			const notRolled = tokens
				.filter(t => t.shake_up_tokens === 0)
				.map<Waitee>(t => ({ kind: 'player', playerID: t.id }));
			if (notRolled.length === 0) return { waitees: [] };
			const waitees: Waitee[] = notRolled.length === players.length
				? [{ kind: 'everyone' }]
				: notRolled;
			return { waitees, stepLabel: 'Roll for tokens' };
		}
		if (game.shake_up_step === 2) {
			if (openSpend) {
				return {
					waitees: [{ kind: 'player', playerID: openSpend.spend.player_id }],
					stepLabel: 'Commit the spend',
				};
			}
			// Reverse-rank turn order: waiting on the single active player.
			if (currentActor != null) {
				return {
					waitees: [{ kind: 'player', playerID: currentActor }],
					stepLabel: 'Spend tokens',
				};
			}
			return { waitees: [] };
		}
		return { waitees: [] };
	});
	$effect(() => { waitingOn = shakeUpWaitingOn; });
</script>

<div class="shake-up">
	<header>
		<h2>Shake-Up</h2>
		<div class="state">
			<span>Category: {game.shake_up_category ?? '—'}</span>
			<span>Step: {game.shake_up_step === 1 ? 'rolling' : game.shake_up_step === 2 ? 'spending' : '—'}</span>
			<span>Your tokens: <strong>{myTokens}</strong></span>
		</div>
	</header>

	{#if error}<p class="error-text">{error}</p>{/if}

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
			<p class="muted-text small">
				Roll your dice (you may leverage your own assets, but no help/interference). Enter the total result.
				It will be added to your token pool.
			</p>
			<div class="roll-form">
				<input type="number" min="1" max="60" bind:value={rollResult} />
				<button class="action-btn primary" disabled={busy || myTokens > 0} onclick={submitRoll}>
					{busy ? '…' : 'Submit roll'}
				</button>
			</div>
		</section>

	{:else if game.shake_up_step === 2}
		{#if openSpend}
			<section class="spend-panel">
				<h3>Open spend</h3>
				<p>
					{playerName(openSpend.spend.player_id)}
					announced {openSpend.spend.option_key}
					{#if openSpend.spend.target_asset_id != null}
						on asset #{openSpend.spend.target_asset_id}
					{/if}
				</p>
				<p class="muted-text small">
					Base cost {openSpend.spend.base_cost} · adjustments {adjustmentTotal >= 0 ? '+' : ''}{adjustmentTotal}
					· running cost: <strong>{openSpend.spend.base_cost + adjustmentTotal}</strong>
				</p>
				<div class="adjust-buttons">
					{#if !isMySpend}
						<button class="action-btn secondary" disabled={busy || myTokens < 1} onclick={() => adjust(1)}>+1 (costs you 1 token)</button>
						<button class="action-btn secondary" disabled={busy || myTokens < 1} onclick={() => adjust(-1)}>−1 (costs you 1 token)</button>
					{:else}
						<button class="action-btn primary" disabled={busy} onclick={commit}>Commit</button>
					{/if}
				</div>
			</section>
		{:else if isMyTurn}
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
										pickedMarginaliaID = '';
										pickedTitleID = '';
										titleFlavor = '';
									}}
								>{opt.Description}</button>
							{/each}
						</div>
					</div>
					{#if pickedOptionInfo?.NeedsAsset}
						<div class="su-form-row">
							<span class="su-form-label">Target asset:</span>
							{#if myAssets.length === 0}
								<p class="muted-text" style="margin:0;">No eligible assets.</p>
							{:else}
								<div class="su-peer-cards">
									{#each myAssets as a (a.id)}
										<AssetCardSelectable
											asset={a}
											ownerColor={playerColor(players.find(pl => pl.id === a.owner_id))}
											selectable
											selected={pickedAssetID === a.id}
											onToggle={() => {
												pickedAssetID = pickedAssetID === a.id ? '' : a.id;
												pickedMarginaliaID = '';
											}}
										/>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
					{#if pickedOptionInfo?.NeedsMarginalia && pickedAssetID !== ''}
						<div class="su-form-row">
							<span class="su-form-label">Marginalia to tear (breaking tears one):</span>
							{#if breakableMarginalia.length === 0}
								<p class="muted-text" style="margin:0;">This asset has no intact marginalia to tear.</p>
							{:else}
								<div class="su-chip-row">
									{#each breakableMarginalia as m (m.id)}
										<button
											type="button"
											class="su-chip"
											class:active={pickedMarginaliaID === m.id}
											onclick={() =>
												(pickedMarginaliaID = pickedMarginaliaID === m.id ? '' : m.id)}
										>{m.text}</button>
									{/each}
								</div>
							{/if}
						</div>
					{/if}
					{#if isClaimTitle}
						<div class="su-form-row">
							<span class="su-form-label">Title to claim (unclaimed this game):</span>
							{#if claimableTitles.length === 0}
								<p class="muted-text" style="margin:0;">Every title has already been claimed.</p>
							{:else}
								<div class="su-chip-row">
									{#each claimableTitles as t (t.id)}
										<button
											type="button"
											class="su-chip"
											class:active={pickedTitleID === t.id}
											title={t.description}
											onclick={() => (pickedTitleID = pickedTitleID === t.id ? '' : t.id)}
										>{t.name}{#if t.in_succession}&nbsp;♛{/if}</button>
									{/each}
								</div>
							{/if}
						</div>
						{#if pickedTitleID !== ''}
							<div class="su-form-row">
								<span class="su-form-label">Peer to bear the title (must have a free marginalia slot):</span>
								{#if titleablePeers.length === 0}
									<p class="muted-text" style="margin:0;">You have no peer with a free marginalia slot.</p>
								{:else}
									<div class="su-peer-cards">
										{#each titleablePeers as a (a.id)}
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
							<div class="su-form-row">
								<span class="su-form-label">Marginalia text (optional — defaults to the title name):</span>
								<input
									type="text"
									maxlength="120"
									placeholder="e.g. crowned at the Midwinter Accord"
									bind:value={titleFlavor}
								/>
							</div>
						{/if}
					{/if}
					<button class="action-btn primary" disabled={!announceReady || busy} onclick={announce}>
						{busy ? '…' : 'Announce (cost 1 token)'}
					</button>
				</div>
			</section>
		{:else if myTokens > 0}
			<p class="muted-text">
				It's not your turn yet — waiting on {playerName(currentActor)} to spend.
				Players take turns in reverse-rank order (lowest status first).
			</p>
		{:else}
			<p class="muted-text">You have no tokens. The category advances when everyone is at zero.</p>
		{/if}
	{:else}
		<p class="muted-text">Shake-Up state unavailable.</p>
	{/if}
</div>

<style>
	.shake-up {
		display: flex; flex-direction: column; gap: 1rem;
		padding: 1rem 0;
		flex: 1; min-height: 0; overflow-y: auto;
	}
	.shake-up h2 { color: var(--color-accent); font-size: 1.3rem; margin: 0; }
	.shake-up h3 { color: var(--color-accent); font-size: 1rem; margin: 0.25rem 0; }
	.state { display: flex; gap: 1.25rem; font-size: 0.9rem; color: #ccc; flex-wrap: wrap; margin-top: 0.25rem; }

	.tokens-panel ul { list-style: none; padding: 0; margin: 0.25rem 0 0; display: flex; flex-direction: column; gap: 0.2rem; font-size: 0.9rem; }

	.roll-form, .announce-form {
		display: flex; flex-direction: column; gap: 0.5rem;
		max-width: 24rem;
		background: var(--color-surface-sunken); border: 1px solid var(--color-border); border-radius: 8px;
		padding: 0.75rem;
	}
	.roll-form { flex-direction: row; align-items: end; }

	.su-form-row { display: flex; flex-direction: column; gap: 0.3rem; }
	.su-form-label { font-size: 0.85rem; color: var(--color-text-muted); }
	.su-chip-row { display: flex; flex-wrap: wrap; gap: 0.35rem; }
	.su-chip {
		display: inline-flex; align-items: center;
		min-height: 44px; padding: 0.35rem 0.85rem;
		border-radius: 999px;
		border: 1px solid #555;
		background: var(--color-surface-2);
		color: var(--color-text);
		font-size: 0.9rem;
		cursor: pointer;
		text-align: left;
	}
	.su-chip.active { border-color: var(--color-accent); background: #3a2f18; }
	.su-chip:focus-visible { outline: 2px solid var(--color-accent); outline-offset: 1px; }

	.su-peer-cards { display: flex; flex-direction: column; gap: 0.4rem; }
	input {
		background: var(--color-border); color: var(--color-text); border: 1px solid #555;
		border-radius: 4px; padding: 0.3rem 0.5rem; font-size: 0.9rem;
	}

	.spend-panel {
		background: var(--color-surface-sunken); border: 1px solid var(--color-border-strong); border-radius: 8px; padding: 0.75rem;
	}
	.adjust-buttons { display: flex; gap: 0.5rem; margin-top: 0.5rem; flex-wrap: wrap; }
</style>
