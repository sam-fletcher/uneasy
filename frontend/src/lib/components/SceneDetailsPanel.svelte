<!--
	SceneDetailsPanel.svelte

	Shown in the Main Event scene panel while a scene is active. Doubles as:
	- A reminder of the location, time elapsed, and present characters.
	- The control surface for "taking over" any of the focus player's peers
	  that are currently unclaimed (controller_player_id == null).
	- The "End Scene" trigger for the focus player.

	Each present character is rendered as an expandable AssetCardSelectable
	in display-only mode so anyone can review marginalia mid-scene.

	Drawn as the redesign's play steps are: no box and no tint, just small
	headings and the scene's own words, with the step gap between groups. The
	step's heading ("Your move — the scene", "Waiting on otto — the scene") is
	drawn by the page above this panel, so none is drawn here. The only boxes
	are the game pieces (the cards) and the buttons.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import {
		claimScenePeer,
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
	import LogMark from './LogMark.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import PlayerShape from '$lib/components/shared/PlayerShape.svelte';

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
		<div class="group">
			<h3 class="label">Prompt</h3>
			<p class="prompt">{scene.prompt}</p>
		</div>
	{/if}

	<!-- Where and when, then whose scene it is. The place wears the house
	     scene mark (the quotation marks the Public Record and the chat use for
	     a scene) in place of the 📍 emoji, which fell back to a different
	     system font on every platform. -->
	<header class="scene-header">
		<p class="place">
			<span class="place-mark"><LogMark family="scene" /></span>
			<span class="place-name">
				{#if locationAsset}
					{locationAsset.name}
				{:else if scene.location_custom}
					{scene.location_custom}
				{:else}
					Unknown
				{/if}
			</span>
		</p>
		<!-- A turn-scene carries a chip XOR a note; a plan-scene neither, so
		     render nothing rather than an empty slot. Printing both is what
		     made a note-only scene read "Moments later — at night" before
		     migration 054 — a duration the player never picked. -->
		{#if scene.time_elapsed}
			<p class="time">{timeLabels[scene.time_elapsed]}</p>
		{:else if scene.time_note}
			<p class="time">{scene.time_note}</p>
		{/if}
		<p class="focus-line">
			Focus:
			<PlayerShape player={focusPlayer} />
			<span class="focus-name" style:color={colorFor(scene.focus_player_id)}>
				{focusPlayer?.display_name ?? 'Unknown'}
			</span>
		</p>
	</header>

	{#if locationAsset}
		<div class="group">
			<h3 class="label">Location</h3>
			<AssetCardSelectable
				asset={locationAsset}
				ownerColor={colorFor(locationAsset.owner_id)}
				knownSecretCount={secretCounts?.known(locationAsset.id)}
			/>
		</div>
	{/if}

	<div class="group">
		<h3 class="label">Present</h3>
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
					{#if asset && !asset.is_destroyed}
						{@const lbl = controllerLabel(p)}
						<!-- A peer wears its OWNING retinue's colour regardless of
						     who currently controls it in the scene (owner ruling,
						     2026-07-19); the label still names the controller. -->
						<div class="peer-row">
							<AssetCardSelectable
								asset={asset}
								ownerColor={colorFor(asset.owner_id)}
								ownerLabel={lbl.text}
								knownSecretCount={secretCounts?.known(asset.id)}
							/>
							{#if lbl.claimable && !isFocusPlayer && currentPlayerID != null}
								<button
									type="button"
									class="action-btn secondary claim-btn"
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
		{#if claimError}<ErrorText message={claimError} />{/if}
	</div>

	{#if isFocusPlayer}
		<!-- The step's one main button. -->
		<div class="end-bar">
			<button
				type="button"
				class="action-btn primary"
				onclick={onEndScene}
				disabled={endingScene}
			>
				{endingScene ? '…' : 'End Scene'}
			</button>
			{#if endError}<ErrorText message={endError} extra="inline" />{/if}
		</div>
	<!-- {:else if currentPlayerID != null && !rollActive}
		TODO: Decide how to handle scene rolls (heavy social component) -->
	{/if}
</section>

<style>
	/* A run of groups with the step gap between them: no frame, no tint. The
	   page's scene panel is the scroller, so this doesn't scroll on its own; the
	   bottom space keeps the last control off the tab bar. */
	.scene-details {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding-bottom: var(--space-4);
		flex-shrink: 0;
	}

	/* A group: a small heading, then what it holds, a closer gap apart. */
	.group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-width: 0;
	}

	/* The small heading over a group (Prompt, Location, Present): label size,
	   capitals, secondary. The game's gold is for the step's own heading, which
	   the page draws above this panel. */
	.label {
		margin: 0;
		font-size: var(--text-label);
		font-weight: 400;
		letter-spacing: 0.06em;
		text-transform: uppercase;
		color: var(--color-text-secondary);
	}

	/* The prompt, as step text. */
	.prompt {
		margin: 0;
		font-size: var(--text-body);
		line-height: 1.5;
		color: var(--color-text);
	}

	/* Where, when and whose: three short lines, the place at step-text size and
	   the other two a step down. */
	.scene-header {
		display: flex;
		flex-direction: column;
		gap: var(--space-1);
	}
	.scene-header p { margin: 0; }

	.place {
		display: flex;
		align-items: center;
		gap: var(--space-2);
		font-size: var(--text-body);
		color: var(--color-text);
	}
	/* The scene mark, in the same slot the record gives it: 16px, faint. */
	.place-mark {
		display: flex;
		flex: none;
		width: 16px;
		height: 16px;
		color: var(--color-text-faint);
	}
	.place-name { min-width: 0; }

	.time,
	.focus-line {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.focus-line {
		display: flex;
		align-items: center;
		gap: var(--space-1);
	}

	/* A card and its button are one row, closer together than the rows are to
	   each other, so "Take over" reads as the tail of the card above it and not
	   of the next one. */
	.peer-list {
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}

	.peer-row {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* The button sits at its card's far edge. The house secondary button, 44px. */
	.claim-btn { align-self: flex-end; }

	.end-bar {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-3);
		align-items: center;
	}
</style>
