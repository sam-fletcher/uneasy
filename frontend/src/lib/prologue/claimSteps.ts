// claimSteps.ts — pure step-ledger derivation for ClaimChoiceModal
// (Session 3 of adr/PROLOGUE_CHOOSING_REDESIGN_PLAN.md).
//
// Given the modal's editable draft state (already-formatted titles/summaries
// so this stays decoupled from card-glyph/suit-label formatting), produces
// the ordered list of steps plus each one's completion and collapsed-summary
// text. Completion checks mirror the trim rules that fed the old flat
// `ready` boolean; take-card steps are always complete (there's nothing to
// write). Auto-advance/manual-reopen of *which* step is currently expanded
// is transient UI state, not derived data, so it stays in the component.

export type ClaimStepKind = 'asset' | 'marginalia' | 'card';

export interface ClaimCardStepInput {
	/** Stable id for this card, e.g. "H::K" — survives re-derivation. */
	key: string;
	title: string;
	isTake: boolean;
	/** The player's draft text. Ignored (should be '') when isTake. */
	text: string;
}

export interface ClaimStepsInput {
	assetTitle: string;
	assetText: string;
	assetMarginalia: string;
	/** Null when this sheet type has no second-step text field
	 *  (hailing_from). */
	marginalia: { title: string; text: string } | null;
	cards: ClaimCardStepInput[];
}

export interface ClaimStep {
	key: string;
	kind: ClaimStepKind;
	title: string;
	complete: boolean;
	/** One-line text for the collapsed view. Empty while incomplete — there's
	 *  nothing entered yet to summarize. Unused for take-card steps, which
	 *  render their own rich (italic-asset-name) line straight from
	 *  stealPreview rather than a plain-string summary. */
	summary: string;
	isTake: boolean;
}

export function deriveClaimSteps(input: ClaimStepsInput): ClaimStep[] {
	const steps: ClaimStep[] = [
		{
			key: 'asset',
			kind: 'asset',
			title: input.assetTitle,
			complete: !!input.assetText.trim() && !!input.assetMarginalia.trim(),
			summary: input.assetText.trim(),
			isTake: false,
		},
	];

	if (input.marginalia) {
		steps.push({
			key: 'marginalia',
			kind: 'marginalia',
			title: input.marginalia.title,
			complete: !!input.marginalia.text.trim(),
			summary: input.marginalia.text.trim(),
			isTake: false,
		});
	}

	for (const c of input.cards) {
		steps.push({
			key: `card:${c.key}`,
			kind: 'card',
			title: c.title,
			complete: c.isTake || !!c.text.trim(),
			summary: c.isTake ? '' : c.text.trim(),
			isTake: c.isTake,
		});
	}

	return steps;
}
