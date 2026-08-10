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
	import { TEXT_LIMITS } from '$lib/textLimits';
	import AssetCardSelectable from './AssetCardSelectable.svelte';
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

<section class="scene-setup" class:readonly={readOnly}>
	<div class="prompt">
		<span class="prompt-label">Prompt</span>
		<p>{prompt}</p>
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

	     It keeps full brightness inside the greyed read-only panel, unlike
	     every other block here: it is the one thing on this screen a watcher
	     can actually operate, and dimming it would say otherwise. -->
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
					maxlength={TEXT_LIMITS.NAME}
					disabled={readOnly}
				/>
			</label>
		</div>
	</div>

	<div class="section">
		<h3>When</h3>
		<!-- The chips are all "later than the last scene", which the opening
		     scene has nothing to be. Only the free-text note is offered there,
		     and its placeholder carries the example instead of a hint line —
		     Where is required with no marker either, so an extra line here
		     would make this the one over-explained field in the form. -->
		{#if !isFirstScene}
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
		{/if}
		<input
			type="text"
			class="note"
			aria-label={isFirstScene ? 'When the scene opens' : 'Another time'}
			placeholder={isFirstScene ? 'At night? At the coronation?' : 'Another time'}
			value={displayTimeNote}
			oninput={(e) => onTimeNoteInput((e.target as HTMLInputElement).value)}
			maxlength={TEXT_LIMITS.SCENE_TIME_NOTE}
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

	{#if error}<ErrorText message={error} />{/if}

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
		background: var(--color-surface-active);
		border: 1px solid var(--color-border-warm);
		border-left: 3px solid var(--color-accent);
		border-radius: 5px;
		padding: 0.55rem 0.7rem;
	}
	.scene-setup.readonly .prompt {
		background: var(--color-bg);
		border: 1px solid var(--color-surface-2);
		border-left: 3px solid var(--color-border-strong);
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

	/* Help body. The sub-head wears the section h3's SHAPE — same uppercase,
	   same tracking, a size down — so it reads as a heading in the form's own
	   voice. It does not wear its GOLD: with the panel open, gold uppercase
	   has to keep meaning "this is one of the three fields you are filling
	   in", and a heading inside the help is commentary, not structure. The
	   accent stays the form's. */
	.help-lede {
		color: var(--color-text);
		font-size: 0.95rem;
		line-height: 1.45;
	}
	.help-text {
		color: var(--color-text-secondary);
		font-size: 0.88rem;
		line-height: 1.4;
	}
	/* No margins on either paragraph: HelpDisclosure's body flexes with a gap
	   and zeroes the browser default, so a margin here would double the space. */
	h4 {
		margin: 0.25rem 0 0;
		font-size: 0.72rem;
		color: var(--color-text-muted);
		text-transform: uppercase;
		letter-spacing: 0.06em;
	}
	/* The house prose-bullet list (planPanel.css's .resolved-so-far-list and
	   ChoicesApplied's .choices-summary-list): disc markers, indented to
	   1.1rem with margin-left and padding zeroed, rather than riding the
	   browser's 40px padding — which is what made this list sit a whole
	   indent step right of every other block in the panel.

	   Two deviations from those two, both because this list is prose rather
	   than a subordinate summary of a form's state: it takes .help-text's
	   0.88rem/secondary instead of their 0.82rem/muted, since it is peer to
	   the paragraphs above it; and its top margin is 0, because the
	   disclosure body is a flex column whose gap already spaces it off the
	   heading (theirs sit in normal flow under a label). */
	.help-list {
		margin: 0 0 0 1.1rem;
		padding: 0;
		list-style: disc;
		color: var(--color-text-secondary);
		font-size: 0.88rem;
		line-height: 1.4;
	}
	/* li + li, not a margin on every li: a leading margin on the first item
	   collapses out through the zero-padding ul and reopens the gap the line
	   above just closed. Same reason PushBlockedHelp's steps do it this way. */
	.help-list li + li { margin-top: 0.3rem; }

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
	.custom-panel.active { border-color: var(--color-accent); background: var(--color-surface-active); }
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
		background: var(--color-surface-sunken);
		color: var(--color-text-secondary);
		font-size: 0.85rem;
		cursor: pointer;
		box-sizing: border-box;
	}

	/* Squeezed column (a 360 phone gives record-phase content exactly 300;
	   docs/STYLE_GUIDE.md "Layout widths"): pack the time chips three per
	   row. Container query, not viewport — the column is what matters. */
	@container column (max-width: 300px) {
		.chips {
			gap: 0.3rem;
		}

		.chip {
			flex: 0 0 calc(33.333% - 0.2rem);
			min-width: 0;
			padding: 0.35rem 0.45rem;
			font-size: 0.74rem;
			line-height: 1.1;
			white-space: nowrap;
		}
	}
	.chip.active {
		background: var(--color-accent);
		color: var(--color-bg);
		border-color: var(--color-accent);
	}
	/* Read-only: drop the chips to label greys and lose the pointer, so they
	   read as a record of the focus player's choices, not as buttons. The
	   active one stays a step brighter — that is the only thing marking it. */
	.scene-setup.readonly .chip {
		color: var(--color-text-muted);
		cursor: default;
	}
	.scene-setup.readonly .chip.active {
		background: var(--color-border-strong);
		color: var(--color-text-secondary);
		border-color: var(--color-border-strong);
	}

	.note { width: 100%; }

	.actions { display: flex; gap: 0.5rem; }
</style>
