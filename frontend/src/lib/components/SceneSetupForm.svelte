<!--
	SceneSetupForm.svelte

	Rendered in the Main Event scene panel when the focus player needs to set
	a scene (no active scene, no plans pending or resolving on the row).

	Three sections:
	- Where: holding cards (single-select) OR a custom location panel.
	- When: a time-elapsed chip OR a free-text note, never both — picking one
	  clears the other. On the Main Event's very first scene (`isFirstScene`)
	  the chips are hidden, since there is no earlier scene for it to be
	  "moments/days later" than; the note is then the only way to answer, and
	  the existing submit gate makes it required without any extra rule.
	- Who else: peer cards (multi-select). Focus player's own main character
	  is implicit and excluded.

	The follow-on prompt (or fallback) is shown at the top, as a small heading
	and the prompt's words, so the focus player has the rules' guidance in front
	of them while filling out the form. It has no box and no tint, since it is
	only read: the boxes in this form are its controls and its cards.

	The step's own heading ("Your move — set the scene" or "Waiting on otto — set
	the scene") is drawn by the page above this form, so none is drawn here.

	Submit posts to /scenes; on success the parent is responsible for
	loading the active scene state (the WS event will also push it).

	A watcher sees the same form, mirrored from the focus player's draft. It is
	not faded: every control draws its own disabled look (a picked chip or card
	keeps its gold border and takes faint text), and that is what says "not
	yours". The help disclosure is not disabled with them.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { onDestroy } from 'svelte';
	import { createScene, type Asset, type Player, type TimeElapsed, type SceneSetupDraft } from '$lib/api';
	import { playerColor } from '$lib/playerColor';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import AssetCardSelectable from './AssetCardSelectable.svelte';
	import Chip from '$lib/components/shared/Chip.svelte';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import HelpDisclosure from '$lib/components/shared/HelpDisclosure.svelte';

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
		 * snapshot, broadcast over the WS). The step heading above the form
		 * already names the focus player.
		 */
		readOnly?: boolean;
		/**
		 * Ephemeral mirror of the focus player's selections, broadcast over
		 * the WS. Drives the read-only render; ignored when !readOnly.
		 * Null until the focus player's first change.
		 */
		draft?: SceneSetupDraft | null;
		/**
		 * True on the Main Event's opening scene (row 1), which has no earlier
		 * scene to be "moments/days later" than. Presentation only: it hides
		 * the chips and swaps the note's placeholder. The note then becomes
		 * the only way to satisfy the existing chip-or-note submit gate, so it
		 * is required here without a rule of its own — and the server needs no
		 * matching row check, since "a scene needs an elapsed time or a time
		 * note" already covers it.
		 */
		isFirstScene?: boolean;
	}

	const {
		gameID,
		assets,
		players,
		focusPlayerID,
		prompt,
		onSceneStarted,
		readOnly = false,
		draft = null,
		isFirstScene = false,
	}: Props = $props();

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
	// A chip or a note, never both (selectTime / onTimeNoteInput clear each
	// other). With the chips hidden on the opening scene this is what makes
	// the note required there.
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
				present_peer_ids: [...selectedPeerIDs],
			};
			if (selectedHoldingID != null) {
				params.location_holding_id = selectedHoldingID;
			} else {
				params.location_custom = customLocation.trim();
			}
			// Exactly one of the two, matching what the player actually chose.
			// Sending a `?? 'moments'` fallback beside a note is what wrote a
			// duration nobody picked into every note-only scene before 054.
			if (timeElapsed != null) {
				params.time_elapsed = timeElapsed;
			} else {
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
	<div class="group">
		<h3 class="label">Prompt</h3>
		<p class="prompt">{prompt}</p>
	</div>

	<!-- Setting a scene is the one recurring turn in the game with no rules
	     text anywhere near it: the form asks three questions and says nothing
	     about what makes a good answer to them (ALL_RULES_OVERVIEW.md "Setting
	     Scenes"). What it teaches is the disposition — describe a situation,
	     don't plan an outcome — and then hands the reader somewhere to look
	     when they have no idea at all.

	     Shown to WATCHERS TOO, not just the focus player (owner ruling,
	     2026-08-09). The lede is addressed to the whole table — everyone acts
	     the scene out in chat — and the moment a player can best absorb how
	     scenes work is while someone else sets one, with nothing to fill in
	     and no turn to take. Gating it on the actor would also have made this
	     the one disclosure in the app that hides from a player waiting their
	     turn; neither prologue one does. The "Need ideas?" prompts do read as
	     next-time advice for a watcher, which is the whole cost.

	     A watcher's form is a mirror of the focus player's draft, with each
	     control in its own disabled look. This disclosure is not one of those
	     controls: it is the one thing on this screen a watcher can actually
	     operate, so it is drawn as it is for the focus player. -->
	<HelpDisclosure title="How to set a good scene" id="scene-setup-help-body">
		<p class="help-lede">
			You're choosing the moment the table plays out next. Everyone acts it
			out in chat with the present characters, in either first or third person.
		</p>
		<p class="help-text">
			Describe a situation that interests you without thinking too hard about what its outcome may be;
			just drop characters into an interesting scenario and see what happens.
		</p>
		<h4>Need ideas?</h4>
		<ul class="help-list">
			<li>Are there any green Tones you want to explore?</li>
			<li>Are there any assets or marginalia that interest you?</li>
			<li>How is your character preparing for upcoming plans in the Public Record?</li>
			<li>Ask for help if you have an idea, but are unsure of how to show it.</li>
		</ul>
	</HelpDisclosure>

	<div class="group">
		<h3 class="label">Where</h3>
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

		<!-- A typed place and a picked holding are alternatives (picking one
		     clears the other), so there is no frame to say which is chosen: the
		     picked card wears the gold border, and typed words are their own
		     proof. -->
		<input
			type="text"
			aria-label="Another location"
			placeholder="Another location"
			value={displayCustomLocation}
			oninput={(e) => onCustomInput((e.target as HTMLInputElement).value)}
			maxlength={TEXT_LIMITS.NAME}
			disabled={readOnly}
		/>
	</div>

	<div class="group">
		<h3 class="label">When</h3>
		<!-- Six chips in a row of several, so the compact size: three to a
		     line on a phone. They are all "later than the last scene", which
		     the opening scene has nothing to be. Only the free-text note is
		     offered there,
		     and its placeholder carries the example instead of a hint line —
		     Where is required with no marker either, so an extra line here
		     would make this the one over-explained field in the form. -->
		{#if !isFirstScene}
			<div class="chips">
				{#each timeOptions as opt (opt.value)}
					<Chip
						compact
						selected={displayTimeElapsed === opt.value}
						onclick={() => selectTime(opt.value)}
						disabled={readOnly}
					>
						{opt.label}
					</Chip>
				{/each}
			</div>
		{/if}
		<input
			type="text"
			aria-label={isFirstScene ? 'When the scene opens' : 'Another time'}
			placeholder={isFirstScene ? 'At night? At the coronation?' : 'Another time'}
			value={displayTimeNote}
			oninput={(e) => onTimeNoteInput((e.target as HTMLInputElement).value)}
			maxlength={TEXT_LIMITS.SCENE_TIME_NOTE}
			disabled={readOnly}
		/>
	</div>

	<div class="group">
		<h3 class="label">Who else is here</h3>
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

	{#if error}<ErrorText message={error} />{/if}

	{#if !readOnly}
		<!-- The step's one main button. Greyed out until the form can be
		     submitted (a place and a time), which is how Send reads before there
		     is text. -->
		<button
			type="button"
			class="action-btn primary"
			onclick={submit}
			disabled={!canSubmit || submitting}
		>
			{submitting ? '…' : 'Begin Scene'}
		</button>
	{/if}
</section>

<style>
	/* The form is a run of groups with the step gap between them: no frame, no
	   tint. The page's scene panel is the scroller, so the form doesn't scroll
	   on its own; the bottom space keeps the last control off the tab bar. */
	.scene-setup {
		display: flex;
		flex-direction: column;
		gap: var(--space-4);
		padding-bottom: var(--space-4);
		flex-shrink: 0;
	}

	/* A group: a small heading, then what it asks, a closer gap apart. */
	.group {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
		min-width: 0;
	}

	/* The small heading over a group (Prompt, Where, When, Who else): label
	   size, capitals, secondary. The game's gold is for the step's own heading,
	   which the page draws above this form. */
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

	/* Help body. HelpDisclosure owns the frame and the gap between these;
	   everything here is the prose itself, one size down from step text since it
	   is reference, and secondary except for the lede. No margins: the body is a
	   flex column with a gap, so a margin here would double the space. */
	.help-lede {
		color: var(--color-text);
		font-size: var(--text-body);
		line-height: 1.5;
	}
	.help-text {
		color: var(--color-text-secondary);
		font-size: var(--text-small);
		line-height: 1.5;
	}
	h4 {
		margin: 0;
		font-size: var(--text-small);
		font-weight: 400;
		color: var(--color-text);
	}
	/* The house prose-bullet list (planPanel.css's .resolved-so-far-list): disc
	   markers indented one step, margin and padding zeroed rather than riding
	   the browser's 40px padding. Its type matches the paragraph above it. */
	.help-list {
		margin: 0 0 0 var(--space-4);
		padding: 0;
		list-style: disc;
		color: var(--color-text-secondary);
		font-size: var(--text-small);
		line-height: 1.5;
	}
	/* li + li, not a margin on every li: a leading margin on the first item
	   collapses out through the zero-padding ul and reopens the gap the line
	   above just closed. */
	.help-list li + li { margin-top: var(--space-1); }

	/* "Nothing to pick here" lines: what the game is saying, so secondary and
	   plain (italic is for asset names). */
	.hint {
		margin: 0;
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}

	.cards {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}

	/* A text field standing on the page: the raised surface and the neutral
	   control border, 16px type so iOS doesn't zoom the page when it takes
	   focus, and 44px tall. Disabled is the mirror a watcher sees: faint
	   words and the quiet border. */
	input[type='text'] {
		box-sizing: border-box;
		width: 100%;
		min-height: var(--size-tap);
		padding: 0 var(--space-3);
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		background: var(--color-surface);
		color: var(--color-text);
		font: inherit;
		font-size: var(--text-body);
	}
	input[type='text']::placeholder { color: var(--color-text-faint); }
	input[type='text']:focus-visible {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	input[type='text']:disabled {
		border-color: var(--color-border);
		color: var(--color-text-faint);
		cursor: not-allowed;
	}

	.chips {
		display: flex;
		flex-wrap: wrap;
		gap: var(--space-2);
	}
</style>
