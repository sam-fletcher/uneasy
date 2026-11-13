<!-- ErrorText.svelte
  The one way an error message reaches the screen. Wraps the two existing
  danger-text classes so every failure in the app is announced to a screen
  reader from a single place.

  Why a component rather than a class: `role="alert"` cannot come from CSS,
  and these messages are almost always *inserted* into the DOM after an
  action — exactly the case a live region exists for. Before this, none of
  the app's error renders announced at all, while non-error status text
  (PlanPanel's ineligible reason, ShakeUpView's blocked reason) already used
  role="status" correctly. See adr/ERROR_HANDLING_PLAN.md.

  Variants map to the two class namespaces, which stay distinct on purpose —
  the plans/ tree has its own unscoped `.muted`, so shared/ uses `-text`
  suffixes to avoid racing it (see statusText.css):

    "page"  → .error-text   (statusText.css)   — routes, phase views, modals
    "panel" → .res-error    (planPanel.css)    — inside a plan panel

  Callers import the matching stylesheet as they already do; this component
  only supplies the markup and the role. Render it inside an {#if}: an empty
  live region is fine, but the surrounding layout gap usually isn't.

  The look is a danger notice (docs/STYLE_GUIDE.md "Notices"): the danger mark
  beside the words, in the danger colour. The words say what went wrong, so the
  colour is never the only signal.
-->
<script lang="ts">
	import ToneMark from './ToneMark.svelte';

	interface Props {
		message: string;
		/** Which class namespace to render in. Defaults to the routes/phase
		 *  one; pass "panel" inside lib/components/plans/. */
		variant?: 'page' | 'panel';
		/** Extra classes the call site already had (e.g. the table page's
		 *  local `.error` spacing, `.inline` for the smaller size). */
		extra?: string;
	}
	let { message, variant = 'page', extra = '' }: Props = $props();

	const base = $derived(variant === 'panel' ? 'res-error' : 'error-text');
</script>

<p class="{base} error-notice{extra ? ' ' + extra : ''}" role="alert">
	<span class="error-mark" aria-hidden="true"><ToneMark tone="danger" /></span>
	<span class="error-words">{message}</span>
</p>

<style>
	/* The mark is centred on the first line however many lines the words take,
	   as in Notice: its box is exactly one line tall. The colour and size come
	   from the classes above, so each caller keeps the look it had. */
	.error-notice {
		display: flex;
		align-items: flex-start;
		gap: var(--space-2);
	}
	.error-mark {
		display: flex;
		align-items: center;
		flex: none;
		height: 1lh;
	}
	.error-words { min-width: 0; }
</style>
