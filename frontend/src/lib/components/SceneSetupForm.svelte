<!--
	SceneSetupForm.svelte

	Rendered in the Main Event scene panel when the focus player needs to set
	a scene (no active scene, no plans pending or resolving on the row).

	Three sections:
	- Where: holding cards (single-select) OR a custom location panel.
	- When: time-elapsed chip + optional note.
	- Who else: peer cards (multi-select). Focus player's own main character
	  is implicit and excluded.

	The follow-on prompt (or fallback) is shown read-only at the top so the
	focus player has the rules' guidance in front of them while filling out
	the form.

	Submit posts to /scenes; on success the parent is responsible for
	loading the active scene state (the WS event will also push it).
-->
<script lang="ts">
	import { createScene, type Asset, type Player, type TimeElapsed } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import AssetCardSelectable from './AssetCardSelectable.svelte';

	interface Props {
		gameID: string | number;
		assets: Asset[];
		players: Player[];
		focusPlayerID: number;
		/**
		 * Pre-computed prompt to show above the form. Comes from the
		 * follow-on prompt of the most recently resolved plan on this row,
		 * or the free-scene fallback string. The parent computes this so
		 * the form doesn't need to know about plans.
		 */
		prompt: string;
		/** Called once the scene is created. The parent triggers a refetch. */
		onSceneStarted: () => void;
	}

	const { gameID, assets, players, focusPlayerID, prompt, onSceneStarted }: Props = $props();

	// ── Where ──────────────────────────────────────────────────────────────────
	const holdings = $derived(
		assets.filter(a => a.asset_type === 'holding' && !a.is_destroyed)
	);
	let selectedHoldingID = $state<number | null>(null);
	let customLocation = $state('');

	function selectHolding(asset: Asset) {
		// Toggle off if already selected (single-select with cancel).
		selectedHoldingID = selectedHoldingID === asset.id ? null : asset.id;
		if (selectedHoldingID != null) customLocation = '';
	}

	function onCustomInput(value: string) {
		customLocation = value;
		if (value.trim() !== '') selectedHoldingID = null;
	}

	// ── When ───────────────────────────────────────────────────────────────────
	const timeOptions: { value: TimeElapsed; label: string }[] = [
		{ value: 'moments', label: 'Moments later' },
		{ value: 'hours', label: 'Hours later' },
		{ value: 'days', label: 'Days later' },
		{ value: 'weeks', label: 'Weeks later' },
		{ value: 'flashback', label: 'Flashback' },
		{ value: 'simultaneous', label: 'Simultaneous' },
	];
	let timeElapsed = $state<TimeElapsed | null>(null);
	let timeNote = $state('');

	function selectTime(value: TimeElapsed) {
		timeElapsed = timeElapsed === value ? null : value;
		if (timeElapsed != null) timeNote = '';
	}
	function onTimeNoteInput(value: string) {
		timeNote = value;
		if (value.trim() !== '') timeElapsed = null;
	}

	// ── Who else ──────────────────────────────────────────────────────────────
	// Every peer except the focus player's own main character (implicitly present).
	const presentablePeers = $derived(
		assets.filter(a =>
			a.asset_type === 'peer' &&
			!a.is_destroyed &&
			!(a.is_main_character && a.owner_id === focusPlayerID)
		)
	);
	let selectedPeerIDs = $state<Set<number>>(new Set());

	function togglePeer(asset: Asset) {
		const next = new Set(selectedPeerIDs);
		if (next.has(asset.id)) next.delete(asset.id);
		else next.add(asset.id);
		selectedPeerIDs = next;
	}

	function colorFor(ownerID: number): string {
		return playerColor(players.find(p => p.id === ownerID));
	}

	// ── Submit ────────────────────────────────────────────────────────────────
	const hasLocation = $derived(
		selectedHoldingID != null || customLocation.trim() !== ''
	);
	const hasTime = $derived(
		timeElapsed != null || timeNote.trim() !== ''
	);
	const canSubmit = $derived(hasLocation && hasTime);
	let submitting = $state(false);
	let error = $state('');

	async function submit() {
		if (!canSubmit || submitting) return;
		submitting = true;
		error = '';
		try {
			const params: Parameters<typeof createScene>[1] = {
				time_elapsed: timeElapsed ?? 'moments',
				present_peer_ids: [...selectedPeerIDs],
			};
			if (selectedHoldingID != null) {
				params.location_holding_id = selectedHoldingID;
			} else {
				params.location_custom = customLocation.trim();
			}
			if (timeNote.trim() !== '') {
				params.time_note = timeNote.trim();
			}
			await createScene(gameID, params);
			onSceneStarted();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not start scene.';
		} finally {
			submitting = false;
		}
	}
</script>

<section class="scene-setup">
	<div class="prompt">
		<span class="prompt-label">Prompt</span>
		<p>{prompt}</p>
	</div>

	<div class="section">
		<h3>Where</h3>
		{#if holdings.length === 0}
			<p class="hint">No holdings yet — use a custom location below.</p>
		{:else}
			<div class="cards">
				{#each holdings as h (h.id)}
					<AssetCardSelectable
						asset={h}
						ownerColor={colorFor(h.owner_id)}
						selectable
						selected={selectedHoldingID === h.id}
						onToggle={selectHolding}
					/>
				{/each}
			</div>
		{/if}

		<div class="custom-panel" class:active={customLocation.trim() !== ''}>
			<label>
				<input
					type="text"
					placeholder="Another location"
					value={customLocation}
					oninput={(e) => onCustomInput((e.target as HTMLInputElement).value)}
					maxlength={80}
				/>
			</label>
		</div>
	</div>

	<div class="section">
		<h3>When</h3>
		<div class="chips">
			{#each timeOptions as opt}
				<button
					type="button"
					class="chip"
					class:active={timeElapsed === opt.value}
					onclick={() => selectTime(opt.value)}
				>
					{opt.label}
				</button>
			{/each}
		</div>
		<input
			type="text"
			class="note"
			placeholder="Another time"
			value={timeNote}
			oninput={(e) => onTimeNoteInput((e.target as HTMLInputElement).value)}
			maxlength={120}
		/>
	</div>

	<div class="section">
		<h3>Who else is here</h3>
		{#if presentablePeers.length === 0}
			<p class="hint">No other peers in play yet.</p>
		{:else}
			<div class="cards">
				{#each presentablePeers as peer (peer.id)}
					<AssetCardSelectable
						asset={peer}
						ownerColor={colorFor(peer.owner_id)}
						selectable
						selected={selectedPeerIDs.has(peer.id)}
						onToggle={togglePeer}
					/>
				{/each}
			</div>
		{/if}
	</div>

	{#if error}<p class="error">{error}</p>{/if}

	<div class="actions">
		<button
			type="button"
			class="primary"
			onclick={submit}
			disabled={!canSubmit || submitting}
		>
			{submitting ? '…' : 'Begin Scene'}
		</button>
	</div>
</section>

<style>
	.scene-setup {
		display: flex;
		flex-direction: column;
		gap: 0.9rem;
		padding: 0.5rem 0.2rem 0.8rem;
		overflow-y: auto;
		min-height: 0;
	}

	.prompt {
		background: #1f1a10;
		border: 1px solid #3a3020;
		border-left: 3px solid #c8a96e;
		border-radius: 5px;
		padding: 0.55rem 0.7rem;
	}
	.prompt-label {
		display: block;
		font-size: 0.7rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		margin-bottom: 0.2rem;
	}
	.prompt p {
		margin: 0;
		font-size: 0.92rem;
		color: #e8e4d9;
		line-height: 1.4;
	}

	.section { display: flex; flex-direction: column; gap: 0.5rem; }
	.section h3 {
		margin: 0;
		font-size: 0.82rem;
		color: #c8a96e;
		text-transform: uppercase;
		letter-spacing: 0.06em;
		font-weight: 600;
	}

	.hint {
		font-size: 0.82rem;
		color: #888;
		margin: 0;
		font-style: italic;
	}

	.cards {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.custom-panel {
		border: 1px solid #2a2a2a;
		border-radius: 5px;
		background: #1d1d1d;
		padding: 0.55rem 0.7rem;
	}
	.custom-panel.active { border-color: #c8a96e; background: #221d10; }

	.custom-panel label { display: flex; flex-direction: column; gap: 0.3rem; }

	.custom-label {
		font-size: 0.72rem;
		color: #888;
		text-transform: uppercase;
		letter-spacing: 0.05em;
	}

	input[type='text'] {
		font-size: 0.9rem;
		padding: 0.5rem 0.6rem;
		border-radius: 5px;
		border: 1px solid #444;
		background: #2a2a2a;
		color: inherit;
		min-height: 44px;
	}
	input[type='text']:focus { outline: 2px solid #c8a96e; outline-offset: 1px; }

	.chips {
		display: flex;
		flex-wrap: wrap;
		justify-content: center;
		gap: 0.35rem;
	}

	.chip {
		padding: 0.45rem 0.7rem;
		min-height: 44px;
		border-radius: 999px;
		border: 1px solid #3a3a3a;
		background: #1f1f1f;
		color: #c8c4b9;
		font-size: 0.85rem;
		cursor: pointer;
	}
	.chip.active {
		background: #c8a96e;
		color: #1a1a1a;
		border-color: #c8a96e;
		font-weight: 600;
	}

	.note { width: 100%; }

	.error {
		color: #e07070;
		font-size: 0.82rem;
		margin: 0;
	}

	.actions { display: flex; gap: 0.5rem; }
	.primary {
		padding: 0.55rem 1rem;
		min-height: 44px;
		border-radius: 5px;
		border: none;
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
		font-size: 0.9rem;
		cursor: pointer;
	}
	.primary:disabled {
		opacity: 0.4;
		cursor: not-allowed;
	}
</style>
