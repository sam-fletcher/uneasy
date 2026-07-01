<!--
	SceneDetailsPanel.svelte

	Shown in the Main Event scene panel while a scene is active. Doubles as:
	- A reminder of the location, time elapsed, and present characters.
	- The control surface for "taking over" any of the focus player's peers
	  that are currently unclaimed (controller_player_id == null).
	- The "End Scene" trigger for the focus player.

	Each present character is rendered as an expandable AssetCardSelectable
	in display-only mode so anyone can review marginalia mid-scene.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import {
		claimScenePeer,
		createRoll,
		endScene,
		type Asset,
		type DiceRoll,
		type Player,
		type Scene,
		type ScenePeerView,
		type TimeElapsed,
	} from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { useSecretCounts } from '$lib/secretCountsContext';
	import AssetCardSelectable from './AssetCardSelectable.svelte';

	interface Props {
		gameID: string | number;
		scene: Scene;
		peers: ScenePeerView[];
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		isFocusPlayer: boolean;
		/** Called once End Scene resolves so the parent can refetch state. */
		onSceneEnded: () => void;
		/** True when a dice roll is already in flight (hides the start-roll button). */
		rollActive?: boolean;
		/** Called when this panel creates a new free dice roll. */
		onRollCreated?: (roll: DiceRoll) => void;
	}

	const {
		gameID,
		scene,
		peers,
		assets,
		players,
		currentPlayerID,
		isFocusPlayer,
		onSceneEnded,
		rollActive = false,
		onRollCreated = () => {},
	}: Props = $props();

	// Per-viewer known-secret counts (undefined outside a provider → no eyes).
	const secretCounts = useSecretCounts();

	const timeLabels: Record<TimeElapsed, string> = {
		moments: 'Moments later',
		hours: 'Hours later',
		days: 'Days later',
		weeks: 'Weeks later',
		flashback: 'Flashback',
		simultaneous: 'Simultaneous',
	};

	function assetByID(id: number): Asset | undefined {
		return assets.find(a => a.id === id);
	}
	function playerByID(id: number | null | undefined): Player | undefined {
		if (id == null) return undefined;
		return players.find(p => p.id === id);
	}
	function colorFor(ownerID: number): string {
		return playerColor(playerByID(ownerID));
	}

	const locationAsset = $derived(
		scene.location_holding_id != null ? assetByID(scene.location_holding_id) : undefined
	);

	const focusPlayer = $derived(playerByID(scene.focus_player_id));

	// The focus player's main character is implicitly present and never recorded
	// as a scene_peer; surface it at the top of the Present list so it's visible
	// alongside the claimable peers. Always played by the focus player.
	const focusMainCharacter = $derived(
		assets.find(a =>
			a.asset_type === 'peer' &&
			!a.is_destroyed &&
			a.is_main_character &&
			a.owner_id === scene.focus_player_id
		) ?? null
	);
	const focusMainCharacterLabel = $derived(
		scene.focus_player_id === currentPlayerID
			? 'Played by you'
			: `Played by ${focusPlayer?.display_name ?? 'the focus player'}`
	);

	function controllerLabel(p: ScenePeerView): { text: string; claimable: boolean } {
		if (p.controller_player_id == null) {
			return { text: 'Unclaimed — open to be played', claimable: true };
		}
		const ctrl = playerByID(p.controller_player_id);
		if (p.controller_player_id === currentPlayerID) {
			return { text: 'Played by you', claimable: false };
		}
		return { text: `Played by ${ctrl?.display_name ?? 'someone'}`, claimable: false };
	}

	let busyAssetID = $state<number | null>(null);
	let claimError = $state('');
	let endingScene = $state(false);
	let endError = $state('');

	async function claim(peerAssetID: number) {
		if (busyAssetID != null || isFocusPlayer) return;
		busyAssetID = peerAssetID;
		claimError = '';
		try {
			await claimScenePeer(gameID, scene.id, peerAssetID);
			// WS will broadcast scene.peer_claimed; the parent will refetch.
		} catch (e) {
			claimError = e instanceof Error ? e.message : 'Could not take over.';
		} finally {
			busyAssetID = null;
		}
	}

	// ── Dice roll (non-focus players only) ────────────────────────────────────
	let showRollForm = $state(false);
	let rollDifficulty = $state(3);
	let rollingBusy = $state(false);
	let rollError = $state('');

	async function onStartRoll() {
		if (rollingBusy || isFocusPlayer) return;
		rollingBusy = true;
		rollError = '';
		try {
			const { roll } = await createRoll(gameID, {
				actor_id: scene.focus_player_id,
				difficulty: rollDifficulty,
				scene_id: scene.id,
			});
			onRollCreated(roll);
			showRollForm = false;
		} catch (e) {
			rollError = e instanceof Error ? e.message : 'Could not start roll.';
		} finally {
			rollingBusy = false;
		}
	}

	async function onEndScene() {
		if (endingScene || !isFocusPlayer) return;
		endingScene = true;
		endError = '';
		try {
			await endScene(gameID);
			onSceneEnded();
		} catch (e) {
			endError = e instanceof Error ? e.message : 'Could not end scene.';
		} finally {
			endingScene = false;
		}
	}
</script>

<section class="scene-details">
	{#if scene.prompt}
		<div class="prompt">
			<span class="prompt-label">Prompt</span>
			<p>{scene.prompt}</p>
		</div>
	{/if}

	<header class="scene-header">
		<div class="loc-time">
			<span class="loc">
				📍
				{#if locationAsset}
					{locationAsset.name}
				{:else if scene.location_custom}
					{scene.location_custom}
				{:else}
					Unknown
				{/if}
			</span>
			<span class="time">
				{timeLabels[scene.time_elapsed]}
				{#if scene.time_note}
					— <em>{scene.time_note}</em>
				{/if}
			</span>
		</div>
		<div class="focus-line">
			Focus:
			<span class="focus-name" style:color={colorFor(scene.focus_player_id)}>
				{focusPlayer?.display_name ?? 'Unknown'}
			</span>
		</div>
	</header>

	{#if locationAsset}
		<div class="block">
			<h3>Location</h3>
			<AssetCardSelectable
				asset={locationAsset}
				ownerColor={colorFor(locationAsset.owner_id)}
				knownSecretCount={secretCounts?.known(locationAsset.id)}
			/>
		</div>
	{/if}

	<div class="block">
		<h3>Present</h3>
		<div class="peer-list">
			{#if focusMainCharacter}
				<div class="peer-row">
					<AssetCardSelectable
						asset={focusMainCharacter}
						ownerColor={colorFor(focusMainCharacter.owner_id)}
						ownerLabel={focusMainCharacterLabel}
						knownSecretCount={secretCounts?.known(focusMainCharacter.id)}
					/>
				</div>
			{/if}
			{#if peers.length > 0}
				{#each peers as p (p.peer_asset_id)}
					{@const asset = assetByID(p.peer_asset_id)}
					{#if asset}
						{@const lbl = controllerLabel(p)}
						{@const ctrlColor = playerColor(playerByID(p.controller_player_id ?? undefined))}
						<div class="peer-row">
							<AssetCardSelectable
								asset={asset}
								ownerColor={ctrlColor}
								ownerLabel={lbl.text}
								knownSecretCount={secretCounts?.known(asset.id)}
							/>
							{#if lbl.claimable && !isFocusPlayer && currentPlayerID != null}
								<button
									type="button"
									class="claim-btn"
									onclick={() => claim(p.peer_asset_id)}
									disabled={busyAssetID === p.peer_asset_id}
								>
									{busyAssetID === p.peer_asset_id ? '…' : 'Take over'}
								</button>
							{/if}
						</div>
					{/if}
				{/each}
			{/if}
		</div>
		{#if claimError}<p class="error">{claimError}</p>{/if}
	</div>

	{#if isFocusPlayer}
		<div class="end-bar">
			<button
				type="button"
				class="action-btn primary"
				onclick={onEndScene}
				disabled={endingScene}
			>
				{endingScene ? '…' : 'End Scene'}
			</button>
			{#if endError}<span class="error inline">{endError}</span>{/if}
		</div>
	<!-- {:else if currentPlayerID != null && !rollActive}
		TODO: Decide how to handle scene rolls (heavy social component) -->
	{/if}
</section>

<style>
	.scene-details {
		display: flex;
		flex-direction: column;
		gap: 0.7rem;
		padding: 0.4rem 0.2rem 0.6rem;
		overflow-y: auto;
		min-height: 0;
	}

	.prompt {
		background: #1f1a10;
		border: 1px solid var(--color-border-warm);
		border-left: 3px solid var(--color-accent);
		border-radius: 5px;
		padding: 0.5rem 0.65rem;
	}
	.prompt-label {
		display: block;
		font-size: 0.7rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		margin-bottom: 0.15rem;
	}
	.prompt p {
		margin: 0;
		font-family: var(--font-serif);
		font-size: 1rem;
		color: var(--color-text);
		line-height: 1.5;
	}

	.scene-header {
		display: flex;
		justify-content: space-between;
		align-items: flex-start;
		gap: 0.5rem;
		flex-wrap: wrap;
	}

	.loc-time {
		display: flex;
		flex-direction: column;
		gap: 0.15rem;
		font-size: 0.88rem;
		color: #d8d4c9;
	}
	.time { font-size: 0.82rem; color: #b0a890; }

	.focus-line { font-size: 0.82rem; color: var(--color-text-muted); }

	.block { display: flex; flex-direction: column; gap: 0.4rem; }
	.block h3 {
		margin: 0;
		font-size: 0.78rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.peer-list { display: flex; flex-direction: column; gap: 0.4rem; }

	.peer-row {
		display: flex;
		flex-direction: column;
		gap: 0.3rem;
	}

	.claim-btn {
		align-self: flex-end;
		padding: 0.4rem 0.7rem;
		min-height: 36px;
		border-radius: 5px;
		border: 1px solid var(--color-accent);
		background: #2a2410;
		color: var(--color-accent);
		font-size: 0.82rem;
		cursor: pointer;
	}
	.claim-btn:hover { background: var(--color-accent); color: var(--color-bg); }
	.claim-btn:disabled { opacity: 0.4; cursor: not-allowed; }

	.end-bar {
		display: flex;
		gap: 0.6rem;
		align-items: center;
		padding-top: 0.4rem;
		border-top: 1px solid var(--color-surface-2);
	}

	.error { color: var(--color-danger); font-size: 0.82rem; margin: 0; }
	.error.inline { font-size: 0.78rem; }
</style>
