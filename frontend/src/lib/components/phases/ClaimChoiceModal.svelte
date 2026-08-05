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
  at any time. Navigation between steps is explicit — a Next button in the
  open step's body — never reactive: an earlier draft auto-advanced the
  moment a step's trim check passed, which collapsed steps after a single
  keystroke and made completed steps impossible to reopen.

  Submits everything in a single choosePrologue call, unchanged.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import '$lib/components/shared/modalShell.css';
	import '$lib/components/shared/trackCode.css';
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
	import { assetTypeLabel, stealPreview, trackCode, trackLabel } from '$lib/prologue/choosing';
	import { deriveClaimSteps } from '$lib/prologue/claimSteps';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';
	import { dismissOnBack } from '$lib/dismissOnBack.svelte';

	interface Props {
		gameID: string;
		sheet: PrologueSheet;
		choice: PrologueSheet['choices'][number];
		cards: PlayerCardRow[];
		assets: Asset[];
		players: Player[];
		currentPlayerID: number | null;
		onClose: () => void;
		onSubmitted: () => void;
	}

	let { gameID, sheet, choice, cards, assets, players, currentPlayerID, onClose, onSubmitted }: Props =
		$props();

	// The parent only mounts this while a claim is open, so being mounted *is*
	// the open state. Back cancels the claim — the same thing the × and the
	// backdrop already do in one tap, and far better than the alternative it
	// replaces, which was leaving the table mid-claim.
	dismissOnBack(() => true, () => onClose());

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

	/** A card you already hold. Still a take step (nothing to author), but the
	 *  claim transfers nothing — PROLOGUE_RULES.md only takes from *another*
	 *  player, and the server no-ops a self-take — so the wording must not
	 *  offer to take it from yourself. Card pairs repeat across tiles, so this
	 *  comes up routinely once you've claimed a tile or two. */
	function isCardMine(suit: string, value: string): boolean {
		return cards.some(
			c => c.card_suit === suit && c.card_value === value && c.player_id === currentPlayerID
		);
	}

	function cardStepKey(suit: string, value: string): string {
		return `card:${suit}::${value}`;
	}

	/** Display name of whoever holds this card, '' when nobody does. Titles a
	 *  take step by whose asset it is (Round 3, decision 7) — stealPreview
	 *  answers the same question but also resolves the asset, which the title
	 *  doesn't need and the summary already does. */
	function holderName(suit: string, value: string): string {
		const holder = cards.find(c => c.card_suit === suit && c.card_value === value);
		if (!holder) return '';
		return players.find(p => p.id === holder.player_id)?.display_name ?? '';
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

	// Which step is currently expanded for editing; null = all collapsed.
	let openStepKey = $state<string | null>(null);

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
		// The asset step is always first and always starts incomplete.
		openStepKey = 'asset';
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
			error = e instanceof Error ? e.message : 'Could not load name suggestions.';
		} finally {
			loadingSuggestions = false;
		}
	}

	$effect(() => {
		loadSuggestions();
	});

	// Reroll is per-slot: two card steps of the same suit draw on one pool but
	// are named independently, so refetching one must not disturb the other.
	let rerollingSlotKey = $state<string | null>(null);

	async function rerollCardSuggestions(slot: CardSlot) {
		rerollingSlotKey = cardStepKey(slot.suit, slot.value);
		try {
			const res = await getPrologueCardSuggestions(gameID, slot.suit);
			slot.suggestions = res.suggestions;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load name suggestions.';
		} finally {
			rerollingSlotKey = null;
		}
	}

	// (No local suit→type map any more: it was a second copy of SUIT_MEANINGS,
	// and choosing.ts's assetTypeLabel is the table the tiles, the legend and
	// the sheet headers all read. No suitColor either — the modal drew a
	// red/black card face, and suits have left the app entirely.)

	/** A wild ranks nothing until it is spent, so its code takes the dashed
	 *  not-yet-a-track treatment (decision 3). */
	function isWild(suit: string): boolean {
		return trackLabel(suit) === '';
	}

	const marginaliaStep = $derived(
		isTitles
			? { title: 'Your main character\'s title', text: marginaliaText }
			: isLawsRumors
				? { title: `A new ${isLaw ? 'law' : 'rumor'}`, text: lawOrRumorText }
				: null
	);

	// A card step is titled by the asset TYPE it makes ("Your new holding") and
	// chipped with the track CODE it feeds ("POW") — the markup renders the
	// chip in front of the title. Type + code is the whole of what the suit
	// used to alias, and the tiles that led here are labelled the same way, so
	// the modal is naming the slot the player just tapped.
	//
	// Three titles, not two (Round 3, decision 7). A card another player holds
	// used to read "Your new holding", which was wrong twice: the asset isn't
	// new — you're transferring one that already carries someone else's name
	// and marginalia — and the string was byte-identical to a MAKE step's, so
	// on a two-card tile the ledger's two most different rows wore the same
	// label. "dave's holding" is true, and a name where the other rows say
	// "Your new" makes the two kinds of row tell apart at a glance.
	const steps = $derived(
		deriveClaimSteps({
			assetTitle: `Your new ${sheet.choice_asset_type}`,
			assetText,
			assetMarginalia,
			marginalia: marginaliaStep,
			cards: cardSlots.map(slot => {
				const type = assetTypeLabel(slot.suit);
				// Mine first: it's a subset of isTake, and you can't take from
				// yourself. The holder-name fallback can't fire (isTake comes off
				// the same `cards` array holderName reads) but costs one `??`.
				const owner = slot.isTake ? holderName(slot.suit, slot.value) : '';
				return {
					key: `${slot.suit}::${slot.value}`,
					title: isCardMine(slot.suit, slot.value)
						? `Your ${type}`
						: slot.isTake && owner
							? `${owner}'s ${type}`
							: `Your new ${type}`,
					isTake: slot.isTake,
					text: slot.text,
				};
			}),
		})
	);

	const doneCount = $derived(steps.filter(s => s.complete).length);
	const ready = $derived(steps.length > 0 && doneCount === steps.length);

	// A stale "Still need…" note would mislead once the player keeps editing,
	// so any draft change (anything that re-derives the steps) clears it.
	$effect(() => {
		void steps;
		claimBlockedReason = '';
	});

	function openStep(key: string) {
		openStepKey = key;
	}

	/** The step the Next button should jump to from `key`: the next incomplete
	 *  step below it, wrapping back to earlier incomplete ones; null when
	 *  everything else is done (Next becomes Done and just collapses). */
	function nextIncompleteAfter(key: string) {
		const idx = steps.findIndex(x => x.key === key);
		return (
			steps.slice(idx + 1).find(x => !x.complete) ??
			steps.find(x => !x.complete && x.key !== key) ??
			null
		);
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
		{#if error}<ErrorText message={error} />{/if}

		{#if choice.description}
			<p class="choice-desc">{choice.description}</p>
		{/if}

		<div class="step-ledger">
			{#each steps as step, idx (step.key)}
				{#if step.isTake}
					{@const slot = cardSlots.find(s => cardStepKey(s.suit, s.value) === step.key)}
					{@const preview = slot ? stealPreview(slot.suit, slot.value, cards, assets, players) : null}
					<section class="step take">
						{#if slot}
							<span class="track-code" class:wild={isWild(slot.suit)}>{trackCode(slot.suit)}</span>
						{/if}
						<span class="step-title">{step.title}</span>
						<span class="step-summary">
							—
							{#if preview && preview.ownerID === currentPlayerID}
								{#if preview.assetName}
									you already hold <em>{preview.assetName}</em>; nothing to take.
								{:else}
									already yours; nothing to take.
								{/if}
							{:else if preview?.assetName}
								<!-- No "from {preview.ownerName}": the title now names the
								     holder, so the summary is free to be only the
								     transaction (Round 3, decision 7). -->
								you take <em>{preview.assetName}</em>.
							{:else if preview}
								<!-- The holder's linked asset didn't resolve (destroyed, or
								     not found), so there's no name to promise — but the
								     title has already said whose it is, and "already held
								     by X" would just repeat it. -->
								you take it with this tile.
							{/if}
						</span>
					</section>
				{:else}
					{@const isOpen = openStepKey === step.key}
					{@const cardSlot =
						step.kind === 'card'
							? cardSlots.find(s => cardStepKey(s.suit, s.value) === step.key)
							: null}
					<section class="step" class:open={isOpen} class:done={step.complete}>
						<button
							type="button"
							class="step-header"
							aria-expanded={isOpen}
							aria-controls={`step-body-${idx}`}
							onclick={() => openStep(step.key)}
						>
							<span class="step-marker">{step.complete ? '✓' : idx + 1}</span>
							{#if cardSlot}
								<span class="track-code" class:wild={isWild(cardSlot.suit)}>{trackCode(cardSlot.suit)}</span>
							{/if}
							<span class="step-title">{step.title}</span>
							{#if !isOpen && step.complete}
								<span class="step-summary">{step.summary}</span>
							{/if}
						</button>

						{#if isOpen}
							{@const nextStep = nextIncompleteAfter(step.key)}
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
											placeholder={isLaw ? 'Describe the law' : 'Describe the rumor'}
											maxlength={TEXT_LIMITS.LONG_TEXT}
										></textarea>
										<span class="hint">Whatever you write can only be disputed by another {isLaw ? 'law' : 'rumor'}.</span>
									</label>
								{:else if step.kind === 'card' && cardSlot}
									<SuggestionPicker
										suggestions={cardSlot.suggestions}
										bind:value={cardSlot.text}
										loading={loadingSuggestions}
										placeholder={`Name your ${assetTypeLabel(cardSlot.suit)}`}
										maxlength={TEXT_LIMITS.NAME}
										onReroll={() => rerollCardSuggestions(cardSlot)}
										rerolling={rerollingSlotKey === cardStepKey(cardSlot.suit, cardSlot.value)}
										disabled={submitting}
									/>
								{/if}

								<div class="step-next">
									<button
										type="button"
										class="action-btn secondary"
										onclick={() => (openStepKey = nextStep?.key ?? null)}
									>
										{nextStep ? 'Next' : 'Done'}
									</button>
								</div>
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

	.choice-desc {
		margin: 0;
		color: var(--color-text);
		font-size: 0.85rem;
		line-height: 1.4;
	}

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
	/* A shade up from the shared 0.6rem: in the tile grid the code sits among
	   0.62rem chips, here it sits beside a 0.9rem step title and a 0.8rem
	   marker, and at the shared size it read as a footnote to the row rather
	   than a label on it. */
	.step .track-code { font-size: 0.7rem; }
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

	.step-body { padding: 0 0.75rem 0.75rem; }
	.step-next {
		display: flex;
		justify-content: flex-end;
		margin-top: 0.6rem;
	}

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
