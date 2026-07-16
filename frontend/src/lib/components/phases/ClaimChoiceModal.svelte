<!--
  Step-ledger claim modal for a Prologue choice (Session 3 of
  adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md). The player drafts:
    1. The sheet-derived asset's text (blank; placeholder hints only).
    2. (titles) A title marginalia for their main character.
    2. (laws_rumors) A public-record entry.
    3. For each fresh card: a multiple-choice picker (3 unused examples + Custom).
       Cards already owned by another player become pre-completed take rows —
       no text needed.

  Each step collapses to a one-line summary once its trim checks pass; the
  rulebook allows the steps in any order, so any header can be tapped open
  at any time, and completing the open step just auto-advances to the next
  incomplete one as a navigation nicety, not a gate.

  Submits everything in a single choosePrologue call, unchanged.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/modalShell.css';
	import '$lib/components/shared/cardGlyph.css';
	import {
		choosePrologue,
		getPrologueCardSuggestions,
		type PrologueSheet,
		type PrologueSheetType,
		type PlayerCardRow,
		type AssetType,
		type Asset,
		type Player,
	} from '$lib/api';
	import SuggestionPicker from '../SuggestionPicker.svelte';
	import AssetCreationForm from '../AssetCreationForm.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import { stealPreview } from '$lib/prologue/choosing';
	import { deriveClaimSteps } from '$lib/prologue/claimSteps';

	interface Props {
		gameID: string;
		sheet: PrologueSheet;
		choice: PrologueSheet['choices'][number];
		cards: PlayerCardRow[];
		assets: Asset[];
		players: Player[];
		onClose: () => void;
		onSubmitted: () => void;
	}

	let { gameID, sheet, choice, cards, assets, players, onClose, onSubmitted }: Props = $props();

	const isTitles = $derived(sheet.type === 'titles');
	const isLawsRumors = $derived(sheet.type === 'laws_rumors');
	const isLaw = $derived(choice.name.toLowerCase().includes('law'));

	type CardSlot = {
		suit: string;
		value: string;
		isTake: boolean;
		suggestions: string[];
		/** The chosen text — a picked suggestion or a custom entry. */
		text: string;
	};

	function isCardTaken(suit: string, value: string): boolean {
		return cards.some(c => c.card_suit === suit && c.card_value === value);
	}

	function cardStepKey(suit: string, value: string): string {
		return `card:${suit}::${value}`;
	}

	// Editable form state. Initialized empty and seeded by the effect below
	// so the seed re-runs if the parent ever reuses this modal for a
	// different choice (Svelte 5 was warning that $state(propValue) only
	// captures the initial prop value, not a reactive reference).
	let assetText = $state('');
	let assetMarginalia = $state('');
	let marginaliaText = $state('');
	let lawOrRumorText = $state('');
	let cardSlots = $state<CardSlot[]>([]);

	// Which step is currently expanded for editing. Null briefly before the
	// seed effect below picks the first incomplete step.
	let openStepKey = $state<string | null>(null);
	// Plain (non-reactive) tracker read only inside the auto-advance effect,
	// to detect the moment the *currently open* step flips to complete.
	let lastOpenComplete = false;

	const choiceAssetType = $derived(sheet.choice_asset_type.toLowerCase() as AssetType);

	// Reset the form whenever the choice changes (including on first mount).
	// Tracking the choice name lets us avoid clobbering user edits on
	// unrelated re-renders that pass the same choice through.
	let seededFor = '';
	$effect(() => {
		if (seededFor === choice.name) return;
		// Start blank so the player must author a real name — the old
		// `[choice.name]` defaults persisted literal "[The Monarch]" when left
		// unedited (ADR-007 §7). Placeholders below hint without submitting.
		assetText = '';
		assetMarginalia = '';
		marginaliaText = '';
		lawOrRumorText = '';
		cardSlots = choice.cards.map(c => ({
			suit: c.suit,
			value: c.value,
			isTake: isCardTaken(c.suit, c.value),
			suggestions: [],
			text: '',
		}));
		openStepKey = null;
		lastOpenComplete = false;
		seededFor = choice.name;
	});

	let loadingSuggestions = $state(true);
	let submitting = $state(false);
	let error = $state('');
	let claimBlockedReason = $state('');

	async function loadSuggestions() {
		loadingSuggestions = true;
		try {
			for (const slot of cardSlots) {
				if (slot.isTake) continue;
				const res = await getPrologueCardSuggestions(gameID, slot.suit);
				slot.suggestions = res.suggestions;
			}
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load card suggestions.';
		} finally {
			loadingSuggestions = false;
		}
	}

	$effect(() => {
		loadSuggestions();
	});

	function cardLabel(suit: string, value: string): string {
		const s = suit === 'H' ? '♥' : suit === 'D' ? '♦' : suit === 'S' ? '♠' : '♣';
		return value + s;
	}

	function suitTypeLabel(suit: string): string {
		switch (suit) {
			case 'C': return 'holding';
			case 'D': return 'resource';
			case 'S': return 'artifact';
			case 'H': return 'peer';
			default:  return 'asset';
		}
	}

	function suitColor(suit: string): 'red' | 'black' {
		return suit === 'H' || suit === 'D' ? 'red' : 'black';
	}

	const marginaliaStep = $derived(
		isTitles
			? { title: 'A title held by your main character', text: marginaliaText }
			: isLawsRumors
				? { title: `A new ${isLaw ? 'Law' : 'Rumor'}`, text: lawOrRumorText }
				: null
	);

	const steps = $derived(
		deriveClaimSteps({
			assetTitle: `Your new ${sheet.choice_asset_type} asset`,
			assetText,
			assetMarginalia,
			marginalia: marginaliaStep,
			cards: cardSlots.map(slot => ({
				key: `${slot.suit}::${slot.value}`,
				title: cardLabel(slot.suit, slot.value),
				isTake: slot.isTake,
				text: slot.text,
			})),
		})
	);

	const doneCount = $derived(steps.filter(s => s.complete).length);
	const ready = $derived(steps.length > 0 && doneCount === steps.length);

	// Auto-advance: once the currently open step becomes complete, move to
	// the next incomplete one (or collapse everything once all are done).
	// This is pure navigation convenience — any header stays tappable at any
	// time (decision 4: the rulebook allows the steps in any order), and a
	// stale "still need…" note is cleared on every draft change.
	$effect(() => {
		const s = steps;
		claimBlockedReason = '';
		if (s.length === 0) return;
		if (openStepKey === null) {
			openStepKey = s.find(x => !x.complete)?.key ?? null;
			lastOpenComplete = s.find(x => x.key === openStepKey)?.complete ?? false;
			return;
		}
		const openStep = s.find(x => x.key === openStepKey);
		if (!openStep) return;
		if (openStep.complete && !lastOpenComplete) {
			const idx = s.indexOf(openStep);
			const next = s.slice(idx + 1).find(x => !x.complete) ?? s.find(x => !x.complete);
			openStepKey = next?.key ?? null;
			lastOpenComplete = next ? next.complete : false;
			return;
		}
		lastOpenComplete = openStep.complete;
	});

	function openStep(key: string) {
		openStepKey = key;
	}

	async function submit() {
		if (!ready || submitting) return;
		submitting = true;
		error = '';
		try {
			const card_assets = cardSlots
				.filter(s => !s.isTake)
				.map(s => ({
					suit: s.suit,
					value: s.value,
					text: s.text.trim(),
				}));
			await choosePrologue(gameID, {
				sheet_type: sheet.type as PrologueSheetType,
				choice_name: choice.name,
				asset_text: assetText.trim(),
				asset_marginalia: [assetMarginalia.trim()],
				marginalia_text: isTitles ? marginaliaText.trim() : undefined,
				law_or_rumor_text: isLawsRumors ? lawOrRumorText.trim() : undefined,
				card_assets,
			});
			onSubmitted();
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not submit your choice.';
		} finally {
			submitting = false;
		}
	}

	function onClaimClick() {
		if (submitting) return;
		if (!ready) {
			const missing = steps.filter(s => !s.complete).map(s => s.title);
			claimBlockedReason = `Still need: ${missing.join(', ')}.`;
			return;
		}
		submit();
	}
</script>

<div class="modal-backdrop backdrop" onclick={onClose} role="presentation"></div>
<div class="modal-sheet" role="dialog" aria-modal="true" aria-labelledby="claim-modal-heading">
	<header>
		<h3 id="claim-modal-heading">{choice.name}</h3>
		<button class="modal-close" onclick={onClose} aria-label="Cancel">×</button>
	</header>

	<div class="modal-sheet-scroll">
		{#if error}<p class="error-text">{error}</p>{/if}

		{#if choice.description}
			<p class="choice-desc">{choice.description}</p>
		{/if}

		<div class="summary-chips">
			{#each choice.cards as c}
				{@const preview = stealPreview(c.suit, c.value, cards, assets, players)}
				<div class="summary-chip">
					<span class="card-glyph" data-color={suitColor(c.suit)}>{cardLabel(c.suit, c.value)}</span>
					<span class="summary-chip-text">
						{#if !preview}
							make a new {suitTypeLabel(c.suit)}
						{:else if preview.assetName}
							takes <em>{preview.assetName}</em> from {preview.ownerName}
						{:else}
							already held by {preview.ownerName}
						{/if}
					</span>
				</div>
			{/each}
		</div>

		<div class="step-ledger">
			{#each steps as step, idx (step.key)}
				{#if step.isTake}
					{@const slot = cardSlots.find(s => cardStepKey(s.suit, s.value) === step.key)}
					{@const preview = slot ? stealPreview(slot.suit, slot.value, cards, assets, players) : null}
					<section class="step take">
						<span class="step-title">{step.title}</span>
						<span class="step-summary">
							—
							{#if preview?.assetName}
								you take <em>{preview.assetName}</em> from {preview.ownerName}.
							{:else if preview}
								already held by {preview.ownerName}.
							{/if}
							Nothing to write.
						</span>
					</section>
				{:else}
					{@const isOpen = openStepKey === step.key}
					<section class="step" class:open={isOpen} class:done={step.complete}>
						<button
							type="button"
							class="step-header"
							aria-expanded={isOpen}
							aria-controls={`step-body-${idx}`}
							onclick={() => openStep(step.key)}
						>
							<span class="step-marker">{step.complete ? '✓' : idx + 1}</span>
							<span class="step-title">{step.title}</span>
							{#if !isOpen && step.complete}
								<span class="step-summary">{step.summary}</span>
								<span class="step-edit">Edit</span>
							{/if}
						</button>

						{#if isOpen}
							<div class="step-body" id={`step-body-${idx}`}>
								{#if step.kind === 'asset'}
									<AssetCreationForm
										{gameID}
										assetType={choiceAssetType}
										bind:name={assetText}
										bind:marginalia={assetMarginalia}
										disabled={submitting}
									/>
								{:else if step.kind === 'marginalia' && isTitles}
									<label class="field">
										<textarea
											rows="1"
											bind:value={marginaliaText}
											placeholder={choice.name}
											maxlength={TEXT_LIMITS.MARGINALIA}
										></textarea>
										<span class="hint">Adds 1 marginalia to your main character.</span>
									</label>
								{:else if step.kind === 'marginalia' && isLawsRumors}
									<label class="field">
										<textarea
											rows="2"
											bind:value={lawOrRumorText}
											placeholder={isLaw ? 'State the new law' : 'State the new rumor'}
											maxlength={TEXT_LIMITS.LONG_TEXT}
										></textarea>
										<span class="hint">Describe the {isLaw ? 'law' : 'rumor'}.</span>
									</label>
								{:else if step.kind === 'card'}
									{@const slot = cardSlots.find(s => cardStepKey(s.suit, s.value) === step.key)}
									{#if slot}
										<SuggestionPicker
											suggestions={slot.suggestions}
											bind:value={slot.text}
											loading={loadingSuggestions}
											customPlaceholder={`Name your ${suitTypeLabel(slot.suit)}`}
											disabled={submitting}
										/>
									{/if}
								{/if}
							</div>
						{/if}
					</section>
				{/if}
			{/each}
		</div>
	</div>

	<footer class="modal-sheet-footer">
		{#if claimBlockedReason}
			<p class="muted-text small" role="status">{claimBlockedReason}</p>
		{/if}
		<div class="footer-buttons">
			<button class="action-btn secondary" onclick={onClose} disabled={submitting}>Cancel</button>
			<button
				type="button"
				class="action-btn primary"
				class:claim-blocked={!ready}
				aria-disabled={!ready || submitting}
				onclick={onClaimClick}
			>
				{submitting ? '…' : `Claim (${doneCount} of ${steps.length} done)`}
			</button>
		</div>
	</footer>
</div>

<style>
	.backdrop {
		z-index: 95;
	}
	header {
		flex-shrink: 0;
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 0.5rem;
		padding: 1rem 1.25rem 0;
	}
	h3 { color: var(--color-accent); margin: 0; font-size: 1.1rem; }

	.error-text { margin: 0; }

	.choice-desc {
		margin: 0;
		color: var(--color-text);
		font-size: 0.85rem;
		line-height: 1.4;
	}

	.summary-chips {
		display: flex;
		flex-direction: column;
		gap: 0.4rem;
		padding: 0.6rem 0.75rem;
		background: var(--color-surface-2);
		border: 1px solid var(--color-border);
		border-radius: 8px;
	}
	.summary-chip { display: flex; align-items: center; gap: 0.5rem; }
	.summary-chip-text { font-size: 0.85rem; color: var(--color-text); }
	.summary-chip-text :global(em) { font-style: italic; }

	.step-ledger { display: flex; flex-direction: column; gap: 0.5rem; }

	.step {
		background: var(--color-surface-sunken);
		border: 1px solid var(--color-border);
		border-radius: 8px;
	}
	.step.take {
		padding: 0.65rem 0.75rem;
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 0.35rem;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
	.step.take .step-title { color: var(--color-text); font-weight: 600; }

	.step-header {
		width: 100%;
		display: flex;
		align-items: center;
		gap: 0.5rem;
		padding: 0.65rem 0.75rem;
		min-height: 44px;
		background: none;
		border: none;
		text-align: left;
		color: var(--color-text);
		font-family: inherit;
		font-size: 0.9rem;
		cursor: pointer;
	}
	.step.done:not(.open) .step-header { color: var(--color-text-muted); }
	.step-marker {
		flex-shrink: 0;
		display: inline-flex;
		align-items: center;
		justify-content: center;
		width: 1.4em;
		height: 1.4em;
		border-radius: 50%;
		background: var(--color-surface-2);
		color: var(--color-accent);
		font-size: 0.8rem;
	}
	.step.done .step-marker { background: var(--color-accent); color: var(--color-bg); }
	.step-title { color: var(--color-accent); flex-shrink: 0; }
	.step.done:not(.open) .step-title { color: var(--color-text); }
	.step-summary {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		color: var(--color-text-muted);
		font-size: 0.85rem;
	}
	.step-summary :global(em) { font-style: italic; }
	.step.take .step-summary { white-space: normal; }
	.step-edit {
		flex-shrink: 0;
		color: var(--color-accent);
		font-size: 0.8rem;
		text-decoration: underline;
	}

	.step-body { padding: 0 0.75rem 0.75rem; }

	.field { display: flex; flex-direction: column; gap: 0.3rem; }
	.hint { font-size: 0.75rem; color: var(--color-text-muted); }

	textarea {
		background: var(--color-surface-2); color: var(--color-text);
		border: 1px solid var(--color-border-strong); border-radius: 4px;
		padding: 0.4rem 0.5rem; font-size: 0.9rem;
		font-family: inherit;
	}

	.footer-buttons { display: flex; gap: 0.6rem; justify-content: flex-end; }

	/* Disabled-but-tappable (style guide): the Claim button stays clickable
	   past the point where `ready` is false so a tap can explain which steps
	   remain, rather than silently swallowing the tap — mirrors ShakeUpView's
	   cost-floor reduce button and PlanPanel's ineligible-card treatment. */
	.action-btn.claim-blocked { cursor: not-allowed; opacity: 0.4; }
</style>
