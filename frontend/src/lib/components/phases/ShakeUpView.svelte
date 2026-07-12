<!-- ShakeUpView.svelte
  Phase 4c — Shake-Up endgame UI.

  Two sub-modes driven by game.shake_up_step, both reverse-rank turn order
  (lowest status first):
    1 (rolling): each player rolls in turn through the real dice-roll stage
      machine (DiceRollPanel, in its shake-up mode) — the server creates the
      open roll for whoever is up next. After the last roller resolves, the
      server advances to step 2.
    2 (spending): players take turns in reverse rank order. The active
      spender announces an option (deducts 1 token immediately), other
      players post ±1 adjustments or explicitly pass (1 token each to
      adjust; passing is free), and the spender commits — once every other
      token-holding player has reacted — to lock in the final cost and
      trigger the mechanical effect.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '../plans/planPanel.css';
	import { onMount, onDestroy, untrack } from 'svelte';
	import {
		getShakeUp, shakeUpSpend, shakeUpAdjust, shakeUpCommit, shakeUpPass,
	} from '$lib/api';
	import type {
		Game, Player, Asset, AssetType, Ranking, DiceRoll, DiceRollDie, RollParticipant,
		ShakeUpOptionInfo, ShakeUpSpend, ShakeUpAdjustmentRow, ShakeUpPassRow, ShakeUpTokensRow,
		ClaimableTitle,
	} from '$lib/api';
	import AssetCardSelectable from '../AssetCardSelectable.svelte';
	import DiceRollPanel from '../DiceRollPanel.svelte';
	import CardPicker from '../plans/CardPicker.svelte';
	import Buffet, { type BuffetTab } from '../plans/shared/Buffet.svelte';
	import { playerColor, playerColorByID } from '$lib/playerColor';
	import { shakeUpWaitingOn, type WaitingOnState } from '$lib/waitingOn';
	import { TEXT_LIMITS } from '$lib/textLimits';

	interface Props {
		gameID: string;
		game: Game;
		players: Player[];
		assets: Asset[];
		rankings: Ranking[];
		currentPlayerID: number | null;
		activeRoll: DiceRoll | null;
		activeRollDice: DiceRollDie[];
		activeRollParticipants: RollParticipant[];
		waitingOn: WaitingOnState;
	}
	let {
		gameID, game, players, assets, rankings, currentPlayerID,
		activeRoll = $bindable(), activeRollDice = $bindable(), activeRollParticipants = $bindable(),
		waitingOn = $bindable(),
	}: Props = $props();

	let tokens = $state<ShakeUpTokensRow[]>([]);
	let options = $state<ShakeUpOptionInfo[]>([]);
	let claimableTitles = $state<ClaimableTitle[]>([]);
	let openSpend = $state<{
		spend: ShakeUpSpend;
		adjustments: ShakeUpAdjustmentRow[];
		passes: ShakeUpPassRow[];
		pending_reactor_ids: number[];
		commit_ready: boolean;
	} | null>(null);
	let currentActor = $state<number | null>(null);
	let currentRollerID = $state<number | null>(null);
	let error = $state('');
	let busy = $state(false);

	// Local, ephemeral: the reason shown when a tap on the (aria-disabled, not
	// dead) reduce-cost button is blocked by the floor guard — Make Demands
	// eligibility pattern (PlanPanel's ineligibleNotice). Cleared on refresh
	// so a stale reason doesn't linger onto a different spend.
	let reduceBlockedReason = $state('');

	async function refresh() {
		try {
			const data = await getShakeUp(gameID);
			tokens = data.tokens;
			options = data.options ?? [];
			claimableTitles = data.claimable_titles ?? [];
			openSpend = data.open_spend ?? null;
			currentActor = data.current_actor ?? null;
			currentRollerID = data.current_roller_id ?? null;
			reduceBlockedReason = '';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load shake-up state.';
		}
	}

	onMount(refresh);

	function onShakeUpEvent() { refresh(); }
	onMount(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.spend_abandoned',
			'shake_up.passed', 'shake_up.ended',
		]) window.addEventListener(`uneasy:${t}`, onShakeUpEvent);
	});
	onDestroy(() => {
		for (const t of [
			'shake_up.step_changed', 'shake_up.rolled', 'shake_up.spend_opened',
			'shake_up.adjusted', 'shake_up.spend_committed', 'shake_up.spend_abandoned',
			'shake_up.passed', 'shake_up.ended',
		]) window.removeEventListener(`uneasy:${t}`, onShakeUpEvent);
	});

	const myTokens = $derived(
		tokens.find(t => t.id === currentPlayerID)?.shake_up_tokens ?? 0
	);
	// Whose turn it is to announce a spend (reverse-rank order, server-driven).
	const isMyTurn = $derived(currentActor != null && currentActor === currentPlayerID);
	const playerNameMap = $derived(new Map(players.map(p => [p.id, p.display_name])));
	function playerName(id: number | null): string {
		if (id == null) return '?';
		return players.find(p => p.id === id)?.display_name ?? '?';
	}

	// ── Act tracker ──────────────────────────────────────────────────────────
	const CATEGORIES = ['esteem', 'knowledge', 'power'] as const;
	const CATEGORY_LABELS: Record<(typeof CATEGORIES)[number], string> = {
		esteem: 'Esteem', knowledge: 'Knowledge', power: 'Power',
	};
	const currentCategoryIndex = $derived(
		CATEGORIES.indexOf((game.shake_up_category ?? 'esteem') as (typeof CATEGORIES)[number])
	);
	const stepLabel = $derived(
		game.shake_up_step === 1 ? 'Rolling' : game.shake_up_step === 2 ? 'Spending' : '—'
	);

	// Phase intro: open by default only in the first act. A one-shot seed
	// (like Buffet's `open` prop / AssetCardSelectable's `defaultExpanded`) —
	// once the player toggles it, their choice sticks even as the category
	// advances.
	let introOpen = $state(untrack(() => game.shake_up_category === 'esteem'));

	// Read-only reference of every option across all three acts (Buffet).
	// Wording follows SHAKEUP_RULES.md's "List of Options" verbatim.
	const BUFFET_ALWAYS =
		'Each option costs 1 token. Others may spend their own tokens to raise or lower the cost by 1 per token. Turn order loops until everyone is out.';
	const buffetTabs: BuffetTab[] = [
		{
			key: 'esteem', label: 'Esteem', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_peer', label: 'Take a peer asset' },
				{ key: 'take_artifact', label: 'Take an artifact asset' },
				{ key: 'break_resource', label: 'Break a resource asset' },
				{ key: 'bump_knowledge', label: 'Bump up on Knowledge' },
			],
		},
		{
			key: 'knowledge', label: 'Knowledge', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_resource', label: 'Take a resource asset' },
				{ key: 'break_holding', label: 'Break a holding asset' },
				{ key: 'break_peer', label: 'Break a peer asset' },
				{ key: 'bump_power', label: 'Bump up on Power' },
			],
		},
		{
			key: 'power', label: 'Power', always: BUFFET_ALWAYS,
			opts: [
				{ key: 'take_holding', label: 'Take a holding asset' },
				{ key: 'break_artifact', label: 'Break an artifact asset' },
				{ key: 'claim_title', label: 'Claim a new title' },
				{ key: 'bump_esteem', label: 'Bump up on Esteem' },
			],
		},
	];

	// ── Turn order & token pools (shared by both steps) ─────────────────────
	// Reverse rank (lowest status first), mirroring gamepkg.ShakeUpTurnOrder.
	// Dummy slots (player_id null) don't take turns, so they're skipped.
	const turnOrder = $derived.by<number[]>(() => {
		const cat = game.shake_up_category;
		if (!cat) return [];
		return rankings
			.filter(r => r.category === cat && r.player_id !== null)
			.sort((a, b) => b.rank - a.rank)
			.map(r => r.player_id as number);
	});
	// Whoever the story is currently on: the roller in step 1, or the
	// announcer/spender in step 2 (whether they're still being reacted to or
	// already clear to commit).
	const activeTurnPlayerID = $derived(
		game.shake_up_step === 1 ? currentRollerID : (openSpend?.spend.player_id ?? currentActor)
	);

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

	// Take/break asset-type filters, keyed off the option key (matches the
	// backend's expectedTakeType/expectedBreakType in handler/shake_up.go).
	const TAKE_ASSET_TYPE: Record<string, AssetType> = {
		take_peer: 'peer', take_artifact: 'artifact', take_resource: 'resource', take_holding: 'holding',
	};
	const BREAK_ASSET_TYPE: Record<string, AssetType> = {
		break_resource: 'resource', break_holding: 'holding', break_peer: 'peer', break_artifact: 'artifact',
	};
	const isTakeOption = $derived(pickedOption in TAKE_ASSET_TYPE);
	const targetAssetType = $derived<AssetType | null>(
		TAKE_ASSET_TYPE[pickedOption] ?? BREAK_ASSET_TYPE[pickedOption] ?? null
	);
	// Take options exclude the spender's own assets (ruling 8 — a no-op);
	// break options may target any owner's asset, including the spender's own.
	const targetableAssets = $derived(
		targetAssetType == null
			? []
			: assets.filter(a =>
				a.asset_type === targetAssetType &&
				!a.is_destroyed &&
				(!isTakeOption || a.owner_id !== currentPlayerID))
	);
	function selectTargetAsset(id: number | null) {
		pickedAssetID = id ?? '';
		pickedMarginaliaID = '';
	}

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
			: (assets.find(a => a.id === pickedAssetID)?.marginalia ?? []).filter(m => !m.is_torn)
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
		busy = true; error = ''; reduceBlockedReason = '';
		try {
			await shakeUpAdjust(gameID, openSpend.spend.id, direction);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not adjust.';
		} finally {
			busy = false;
		}
	}

	// The reduce button stays clickable (aria-disabled, not native-disabled)
	// even at the cost floor, so a tap always surfaces why — mirrors the Make
	// Demands ineligible-card pattern rather than silently swallowing the tap.
	function onReduceClick() {
		if (atCostFloor) {
			reduceBlockedReason = "The cost can't go below 1 token.";
			return;
		}
		adjust(-1);
	}

	async function pass() {
		if (!openSpend || busy) return;
		busy = true; error = '';
		try {
			await shakeUpPass(gameID, openSpend.spend.id);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not pass.';
		} finally {
			busy = false;
		}
	}

	// ADR-008 "pay or abandon": intent is required once the cost was raised
	// (extra > 0); the plain no-intent form is only valid when extra === 0,
	// where the spend auto-commits regardless of what's passed.
	async function commit(intent?: 'pay' | 'abandon') {
		if (!openSpend || busy) return;
		busy = true; error = '';
		try {
			await shakeUpCommit(gameID, openSpend.spend.id, intent);
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
	// The floor guard (ADR-008 §1) keeps this at ≥ 1 server-side; Math.max
	// here just guards the brief window before a stale client catches up.
	const runningCost = $derived(openSpend ? Math.max(openSpend.spend.base_cost + adjustmentTotal, 1) : 0);
	const atCostFloor = $derived(runningCost <= 1);
	// extra = final cost owed beyond the base — always ≥ 0 given the floor
	// guard, since base_cost is always 1.
	const extra = $derived(openSpend ? Math.max(runningCost - openSpend.spend.base_cost, 0) : 0);
	const affordable = $derived(myTokens >= extra);
	const pendingReactorIDs = $derived(openSpend?.pending_reactor_ids ?? []);
	const commitReady = $derived(openSpend?.commit_ready ?? true);
	const pendingReactorNames = $derived(pendingReactorIDs.map(playerName));
	const myPassed = $derived(
		currentPlayerID != null && (openSpend?.passes ?? []).some(p => p.player_id === currentPlayerID)
	);
	const spendOptionInfo = $derived.by(() => {
		const spend = openSpend;
		if (!spend) return undefined;
		return options.find(o => o.Key === spend.spend.option_key);
	});
	// The option phrase without its trailing period, echoing the backend's
	// shakeUpOptionPhrase (handler/system_posts.go) used for the chat log.
	const spendPhrase = $derived(
		(spendOptionInfo?.Description ?? openSpend?.spend.option_key ?? '').replace(/\.$/, '')
	);
	const spendTargetAsset = $derived.by(() => {
		const spend = openSpend;
		if (!spend || spend.spend.target_asset_id == null) return undefined;
		return assets.find(a => a.id === spend.spend.target_asset_id);
	});

	// ── Waiting-on derivation ────────────────────────────────────────────────
	// Both steps are strictly sequential turn order, so the pure function
	// (lib/waitingOn.ts) always names exactly one party (or, mid-reaction-
	// window, the set of pending reactors) — see its vitest coverage.
	const computedWaitingOn = $derived(shakeUpWaitingOn({
		step: game.shake_up_step,
		currentRollerID,
		openSpend: openSpend ? {
			spend: { player_id: openSpend.spend.player_id },
			pendingReactorIDs: openSpend.pending_reactor_ids,
			commitReady: openSpend.commit_ready,
		} : null,
		currentActor,
	}));
	$effect(() => { waitingOn = computedWaitingOn; });
</script>

<div class="shake-up">
	<header>
		<h2>Shake-Up</h2>
		<div class="act-tracker" role="list" aria-label="Shake-Up acts">
			{#each CATEGORIES as cat, i (cat)}
				<div
					class="act"
					class:current={cat === game.shake_up_category}
					class:done={CATEGORIES.indexOf(cat) < currentCategoryIndex}
					role="listitem"
				>
					<span class="act-name">{CATEGORY_LABELS[cat]}</span>
					{#if cat === game.shake_up_category}
						<span class="act-step">{stepLabel}</span>
					{/if}
				</div>
				{#if i < CATEGORIES.length - 1}<span class="act-arrow" aria-hidden="true">→</span>{/if}
			{/each}
		</div>
		<div class="state">
			<span>Your tokens: <strong>{myTokens}</strong></span>
		</div>
	</header>

	<section class="intro-block" class:open={introOpen}>
		<button
			type="button"
			class="intro-toggle"
			aria-expanded={introOpen}
			onclick={() => (introOpen = !introOpen)}
		>
			<span>About the Shake-Up</span>
			<span class="intro-caret" aria-hidden="true">▾</span>
		</button>
		{#if introOpen}
			<div class="intro-body">
				<p>
					This is the finale. Over three acts — Esteem, then Knowledge, then
					Power — each player rolls to earn tokens, then spends them in turn
					to take, break, rise, and claim. Take stock of everything that's
					happened over the last thirteen rows of the public record, and
					embrace this last moment: once Power is settled, the game ends.
				</p>
			</div>
		{/if}
	</section>

	{#if error}<p class="error-text">{error}</p>{/if}

	<Buffet tabs={buffetTabs} defaultTab={game.shake_up_category ?? 'esteem'} />

	<section class="tokens-panel">
		<h3>Token pools</h3>
		<!-- <p class="muted-text small">Turn order is reverse rank — lowest status first.</p> -->
		<div class="roller-chips">
			{#each turnOrder as pid (pid)}
				<span
					class="roller-chip"
					class:active={pid === activeTurnPlayerID}
					style:border-color={playerColorByID(pid, players)}
				>
					{playerName(pid)}:&nbsp;<strong>{tokens.find(t => t.id === pid)?.shake_up_tokens ?? 0}</strong>
				</span>
			{/each}
		</div>
	</section>

	{#if game.shake_up_step === 1}
		<section class="rolling-section">
			<h3>Rolling for {game.shake_up_category ?? 'this category'}</h3>
			<p class="muted-text small">Each distinct die face earns one token.</p>
			{#if activeRoll}
				<DiceRollPanel
					bind:roll={activeRoll}
					bind:dice={activeRollDice}
					votes={[]}
					bind:participants={activeRollParticipants}
					bankedDice={[]}
					{assets}
					{currentPlayerID}
					{players}
					{playerNameMap}
				/>
			{:else}
				<p class="muted-text small">Waiting for the next roll to start…</p>
			{/if}
		</section>

	{:else if game.shake_up_step === 2}
		{#if openSpend}
			<section class="spend-panel">
				<h3>Open spend</h3>
				<p class="spend-phrase">
					<span class="spend-player" style:color={playerColorByID(openSpend.spend.player_id, players)}>
						{playerName(openSpend.spend.player_id)}
					</span>
					announced: {spendPhrase}
				</p>
				{#if spendTargetAsset}
					<AssetCardSelectable
						asset={spendTargetAsset}
						ownerColor={playerColor(players.find(pl => pl.id === spendTargetAsset.owner_id))}
						ownerLabel={spendTargetAsset.owner_id === currentPlayerID
							? 'Your own'
							: `Owned by ${playerName(spendTargetAsset.owner_id)}`}
					/>
				{/if}
				<p class="muted-text small">
					Base cost {openSpend.spend.base_cost} · running cost: <strong>{runningCost}</strong>
					{#if adjustmentTotal !== 0}
						({adjustmentTotal > 0 ? '+' : ''}{adjustmentTotal} so far)
					{/if}
				</p>
				{#if openSpend.adjustments.length > 0}
					<ul class="adjustment-history">
						{#each openSpend.adjustments as a (a.id)}
							<li>
								<span style:color={playerColorByID(a.player_id, players)}>{playerName(a.player_id)}</span>
								{a.adjustment > 0 ? 'raised' : 'lowered'} the cost by 1
							</li>
						{/each}
					</ul>
				{/if}

				{#if isMySpend}
					{#if !commitReady}
						<p class="muted-text small reactor-note">
							Others can still react — waiting on {pendingReactorNames.join(', ')} to react.
						</p>
					{:else if extra === 0}
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit()}>
								{busy ? '…' : 'Commit'}
							</button>
						</div>
					{:else if affordable}
						<!-- ADR-008: the cost was raised and the spender can afford it —
						     Pay or Abandon is their real choice, not a forced completion. -->
						<p class="muted-text small reactor-note">
							The cost was raised to {runningCost} ({extra} more than you committed to).
							Pay the extra, or abandon — either way the tokens already spent are gone.
						</p>
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit('pay')}>
								{busy ? '…' : `Pay ${extra} more token${extra === 1 ? '' : 's'}`}
							</button>
							<button class="action-btn secondary" disabled={busy} onclick={() => commit('abandon')}>
								{busy ? '…' : 'Abandon'}
							</button>
						</div>
					{:else}
						<!-- Forced abandon: the raise exceeds what's left in the spender's
						     pool, so Pay isn't offered — only Abandon. -->
						<p class="muted-text small reactor-note">
							The cost was raised to {runningCost}, {extra} more than you committed to — you only
							have {myTokens} token{myTokens === 1 ? '' : 's'} left, so you can't afford it. The
							spend must be abandoned; the tokens already spent are gone either way.
						</p>
						<div class="adjust-buttons">
							<button class="action-btn primary" disabled={busy} onclick={() => commit('abandon')}>
								{busy ? '…' : 'Abandon'}
							</button>
						</div>
					{/if}
				{:else if myTokens > 0}
					<div class="adjust-buttons">
						<button class="action-btn secondary" disabled={busy} onclick={() => adjust(1)}>+1 (costs you 1 token)</button>
						<button
							class="action-btn secondary"
							class:reduce-blocked={atCostFloor}
							disabled={busy}
							aria-disabled={atCostFloor}
							title={atCostFloor ? "The cost can't go below 1 token" : undefined}
							onclick={onReduceClick}
						>−1 (costs you 1 token)</button>
						<button class="action-btn secondary" disabled={busy || myPassed} onclick={pass}>
							{myPassed ? 'You let it stand' : 'Let it stand'}
						</button>
					</div>
					{#if reduceBlockedReason}
						<p class="muted-text small reactor-note" role="status">{reduceBlockedReason}</p>
					{/if}
				{:else}
					<p class="muted-text small">You have no tokens left to react with.</p>
				{/if}
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
						<CardPicker
							label="Target asset"
							items={targetableAssets}
							{players}
							ownerLabel={(a) => a.owner_id === currentPlayerID
								? 'Your own'
								: `Owned by ${playerName(a.owner_id)}`}
							selected={pickedAssetID === '' ? null : pickedAssetID}
							onSelect={selectTargetAsset}
						/>
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
									maxlength={TEXT_LIMITS.NAME}
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
	.state { display: flex; gap: 1.25rem; font-size: 0.9rem; color: var(--color-text-muted); flex-wrap: wrap; margin-top: 0.4rem; }

	/* Act tracker: Esteem → Knowledge → Power, current act lit, its step
	   named beneath it. */
	.act-tracker { display: flex; align-items: center; flex-wrap: wrap; gap: 0.4rem; margin-top: 0.5rem; }
	.act {
		display: flex; flex-direction: column; align-items: center; gap: 0.1rem;
		min-height: 44px; justify-content: center;
		padding: 0.3rem 0.75rem;
		border-radius: 6px;
		border: 1px solid var(--color-border);
		background: var(--color-surface-2);
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
	.act.done { color: var(--color-text-faint); }
	.act.current {
		border-color: var(--color-accent);
		background: #3a2f18;
		color: var(--color-accent);
		font-weight: 600;
	}
	.act-step { font-size: 0.68rem; text-transform: uppercase; letter-spacing: 0.05em; }
	.act-arrow { color: var(--color-text-faint); font-size: 0.9rem; }

	/* Phase intro — collapsible accordion, mirrors Buffet's own toggle. */
	.intro-block { display: flex; flex-direction: column; gap: 0; }
	.intro-toggle {
		display: flex; align-items: center; justify-content: space-between;
		width: 100%; min-height: 44px; padding: 0.55rem 0.75rem;
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
		color: var(--color-accent);
		font: inherit; text-align: left; cursor: pointer;
	}
	.intro-block.open .intro-toggle {
		border-color: var(--color-accent);
		border-bottom-color: transparent;
		border-bottom-left-radius: 0;
		border-bottom-right-radius: 0;
	}
	.intro-caret {
		color: var(--color-accent); font-size: 0.75rem;
		transform: rotate(-90deg); transition: transform 0.15s ease;
	}
	.intro-block.open .intro-caret { transform: rotate(0); }
	.intro-body {
		border: 1px solid var(--color-accent); border-top: none;
		border-bottom-left-radius: 8px; border-bottom-right-radius: 8px;
		padding: 0.6rem 0.75rem;
	}
	.intro-body p { margin: 0; font-size: 0.85rem; line-height: 1.5; color: var(--color-text-muted); }

	.rolling-section { display: flex; flex-direction: column; gap: 0.6rem; }
	.roller-chips { display: flex; flex-wrap: wrap; gap: 0.4rem; }
	.roller-chip {
		display: inline-flex; align-items: center;
		min-height: 44px; padding: 0.35rem 0.75rem;
		border-radius: 999px;
		border: 1px solid var(--color-neutral);
		background: var(--color-surface-2);
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
	.roller-chip.active {
		border-color: var(--color-accent); background: #3a2f18;
		color: var(--color-accent); font-weight: 600;
	}

	.announce-form {
		display: flex; flex-direction: column; gap: 0.5rem;
		max-width: 24rem;
		background: var(--color-surface-sunken); border: 1px solid var(--color-border); border-radius: 8px;
		padding: 0.75rem;
	}

	.su-form-row { display: flex; flex-direction: column; gap: 0.3rem; }
	.su-form-label { font-size: 0.85rem; color: var(--color-text-muted); }
	.su-chip-row { display: flex; flex-wrap: wrap; gap: 0.35rem; }
	.su-chip {
		display: inline-flex; align-items: center;
		min-height: 44px; padding: 0.35rem 0.85rem;
		border-radius: 999px;
		border: 1px solid var(--color-neutral);
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
		background: var(--color-border); color: var(--color-text); border: 1px solid var(--color-neutral);
		border-radius: 4px; padding: 0.3rem 0.5rem; font-size: 0.9rem;
	}

	.spend-panel {
		background: var(--color-surface-sunken); border: 1px solid var(--color-border-strong); border-radius: 8px; padding: 0.75rem;
		display: flex; flex-direction: column; gap: 0.5rem;
	}
	.spend-phrase { margin: 0; font-size: 0.92rem; }
	.spend-player { font-weight: 600; }
	.adjustment-history {
		list-style: none; margin: 0; padding: 0;
		display: flex; flex-direction: column; gap: 0.15rem;
		font-size: 0.8rem; color: var(--color-text-muted);
	}
	.reactor-note { color: var(--color-accent-hover); }
	.adjust-buttons { display: flex; gap: 0.5rem; margin-top: 0.25rem; flex-wrap: wrap; }

	/* Cost-floor guard (ADR-008 §1): the reduce button stays tappable
	   (aria-disabled, not native disabled) so a tap surfaces the reason —
	   mirrors PlanPanel's .ineligible treatment. */
	.action-btn.reduce-blocked { cursor: not-allowed; opacity: 0.4; }
</style>
