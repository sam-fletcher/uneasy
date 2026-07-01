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
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { onDestroy } from 'svelte';
	import { createScene, type Asset, type Player, type TimeElapsed, type SceneSetupDraft } from '$lib/api';
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
		/**
		 * When true, render the form structure for non-focus players to
		 * see, but disable all inputs and hide the submit button. Selections
		 * displayed come from `draft` (the focus player's in-flight
		 * snapshot, broadcast over the WS). The waiting-on banner already
		 * names the focus player.
		 */
		readOnly?: boolean;
		/**
		 * Ephemeral mirror of the focus player's selections, broadcast over
		 * the WS. Drives the read-only render; ignored when !readOnly.
		 * Null until the focus player's first change.
		 */
		draft?: SceneSetupDraft | null;
	}

	const { gameID, assets, players, focusPlayerID, prompt, onSceneStarted, readOnly = false, draft = null }: Props = $props();

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
	// The focus player's own main character is always present. It's pinned at the
	// top of the list, locked checked, and never sent in present_peer_ids (the
	// server treats it as implicitly present and rejects it if listed). The rest
	// are selectable peers.
	const focusMainCharacter = $derived(
		assets.find(a =>
			a.asset_type === 'peer' &&
			!a.is_destroyed &&
			a.is_main_character &&
			a.owner_id === focusPlayerID
		) ?? null
	);
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

	// ── Display values ────────────────────────────────────────────────────────
	// Focus player renders from local state; read-only viewers render from the
	// draft broadcast by the focus player. Draft fields default to "empty"
	// (null / "") so the form starts blank for late joiners until the first
	// keystroke arrives.
	const displayHoldingID = $derived(
		readOnly ? (draft?.holding_id ?? null) : selectedHoldingID
	);
	const displayCustomLocation = $derived(
		readOnly ? (draft?.custom_location ?? '') : customLocation
	);
	const displayTimeElapsed = $derived<TimeElapsed | null>(
		readOnly
			? ((draft?.time_elapsed ?? '') as TimeElapsed) || null
			: timeElapsed
	);
	const displayTimeNote = $derived(
		readOnly ? (draft?.time_note ?? '') : timeNote
	);
	const displayPeerIDs = $derived<Set<number>>(
		readOnly ? new Set(draft?.present_peer_ids ?? []) : selectedPeerIDs
	);

	// ── Draft emission ────────────────────────────────────────────────────────
	// While the focus player edits, broadcast a snapshot of their current
	// selections so non-focus clients can mirror the form. Debounced for text
	// fields (which fire on every keystroke); chip/card toggles also pass
	// through the same debounce — 150ms is short enough to feel live.
	let draftTimer: ReturnType<typeof setTimeout> | null = null;
	function flushDraft() {
		draftTimer = null;
		const payload: Omit<SceneSetupDraft, 'player_id'> = {
			holding_id: selectedHoldingID,
			custom_location: customLocation,
			time_elapsed: timeElapsed ?? '',
			time_note: timeNote,
			present_peer_ids: [...selectedPeerIDs],
		};
		window.dispatchEvent(new CustomEvent('uneasy:scene_setup_draft', { detail: payload }));
	}
	$effect(() => {
		if (readOnly) return;
		// Touch every field so $effect re-runs on any change.
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		selectedHoldingID; customLocation; timeElapsed; timeNote; selectedPeerIDs;
		if (draftTimer) clearTimeout(draftTimer);
		draftTimer = setTimeout(flushDraft, 150);
	});
	onDestroy(() => {
		if (draftTimer) {
			clearTimeout(draftTimer);
			draftTimer = null;
		}
	});

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

<section class="scene-setup" class:readonly={readOnly}>
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
						selected={displayHoldingID === h.id}
						onToggle={selectHolding}
						disabled={readOnly}
					/>
				{/each}
			</div>
		{/if}

		<div class="custom-panel" class:active={displayCustomLocation.trim() !== ''}>
			<label>
				<input
					type="text"
					placeholder="Another location"
					value={displayCustomLocation}
					oninput={(e) => onCustomInput((e.target as HTMLInputElement).value)}
					maxlength={80}
					disabled={readOnly}
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
					class:active={displayTimeElapsed === opt.value}
					onclick={() => selectTime(opt.value)}
					disabled={readOnly}
				>
					{opt.label}
				</button>
			{/each}
		</div>
		<input
			type="text"
			class="note"
			placeholder="Another time"
			value={displayTimeNote}
			oninput={(e) => onTimeNoteInput((e.target as HTMLInputElement).value)}
			maxlength={120}
			disabled={readOnly}
		/>
	</div>

	<div class="section">
		<h3>Who else is here</h3>
		<div class="cards">
			{#if focusMainCharacter}
				<AssetCardSelectable
					asset={focusMainCharacter}
					ownerColor={colorFor(focusMainCharacter.owner_id)}
					ownerLabel="Always present"
					selectable
					selected={true}
					disabled={true}
				/>
			{/if}
			{#if presentablePeers.length === 0}
				{#if !focusMainCharacter}
					<p class="hint">No other peers in play yet.</p>
				{/if}
			{:else}
				{#each presentablePeers as peer (peer.id)}
					<AssetCardSelectable
						asset={peer}
						ownerColor={colorFor(peer.owner_id)}
						selectable
						selected={displayPeerIDs.has(peer.id)}
						onToggle={togglePeer}
						disabled={readOnly}
					/>
				{/each}
			{/if}
		</div>
	</div>

	{#if error}<p class="error-text">{error}</p>{/if}

	{#if !readOnly}
		<div class="actions">
			<button
				type="button"
				class="action-btn primary"
				onclick={submit}
				disabled={!canSubmit || submitting}
			>
				{submitting ? '…' : 'Begin Scene'}
			</button>
		</div>
	{/if}
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
		border: 1px solid var(--color-border-warm);
		border-left: 3px solid var(--color-accent);
		border-radius: 5px;
		padding: 0.55rem 0.7rem;
	}
	.scene-setup.readonly .prompt {
		background: var(--color-bg);
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid #4a4a4a;
	}
	.scene-setup.readonly .prompt-label,
	.scene-setup.readonly .section h3 {
		color: var(--color-text-muted);
	}
	.prompt-label {
		display: block;
		font-size: 0.7rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
		margin-bottom: 0.2rem;
	}
	.prompt p {
		margin: 0;
		font-size: 0.92rem;
		color: var(--color-text);
		line-height: 1.4;
	}

	.section { display: flex; flex-direction: column; gap: 0.5rem; }
	.section h3 {
		margin: 0;
		font-size: 0.82rem;
		color: var(--color-accent);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}

	.hint {
		font-size: 0.82rem;
		color: var(--color-text-muted);
		margin: 0;
		font-style: italic;
	}

	.cards {
		display: flex;
		flex-direction: column;
		gap: 0.35rem;
	}

	.custom-panel {
		border: 1px solid var(--color-surface-2);
		border-radius: 5px;
		background: var(--color-surface-sunken);
		padding: 0.55rem 0.7rem;
	}
	.custom-panel.active { border-color: var(--color-accent); background: #221d10; }
	.scene-setup.readonly .custom-panel.active { border-color: var(--color-surface-2); background: var(--color-surface-sunken); }

	.custom-panel label { display: flex; flex-direction: column; gap: 0.3rem; }

	input[type='text'] {
		font-size: 0.9rem;
		padding: 0.5rem 0.6rem;
		border-radius: 5px;
		border: 1px solid var(--color-border-strong);
		background: var(--color-surface-2);
		color: inherit;
		min-height: 44px;
	}
	input[type='text']:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }

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
		border: 1px solid var(--color-border-strong);
		background: #1f1f1f;
		color: #c8c4b9;
		font-size: 0.85rem;
		cursor: pointer;
	}
	.chip.active {
		background: var(--color-accent);
		color: var(--color-bg);
		border-color: var(--color-accent);
	}
	.scene-setup.readonly .chip.active {
		background: #4a4a4a;
		color: var(--color-text);
		border-color: #4a4a4a;
	}

	.note { width: 100%; }

	.actions { display: flex; gap: 0.5rem; }
</style>
