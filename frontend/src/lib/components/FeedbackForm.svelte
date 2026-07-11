<!-- FeedbackForm.svelte
  Login-gated feedback intake (adr/FEEDBACK_AND_RESET_PLAN.md). Rendered
  inside a RetinueSheet by its callers (HelpButton, the table page's inline
  lobby help, the profile page) — this component is just the sheet content,
  mirroring how HelpContent works.

  gameId/route/phase are client-side context the server can't see on its
  own; in-game callers pass the current table/phase, the profile page passes
  none of them.
-->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { onMount } from 'svelte';
	import { getMe, submitFeedback } from '$lib/api';
	import { TEXT_LIMITS } from '$lib/textLimits';

	let { gameId, route, phase }: { gameId?: string; route?: string; phase?: string } = $props();

	let loggedIn = $state<boolean | null>(null);
	let body = $state('');
	let contact = $state('');
	let submitting = $state(false);
	let submitted = $state(false);
	let error = $state('');

	onMount(async () => {
		const me = await getMe();
		loggedIn = me !== null;
	});

	async function submit() {
		if (!body.trim() || submitting) return;
		submitting = true;
		error = '';
		try {
			await submitFeedback({
				body: body.trim(),
				contact: contact.trim() || undefined,
				game_id: gameId !== undefined ? Number(gameId) : undefined,
				route,
				phase,
			});
			submitted = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send feedback.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="feedback-form">
	{#if loggedIn === null}
		<p class="muted-text">Loading…</p>
	{:else if !loggedIn}
		<p class="muted-text">Log in to send feedback.</p>
	{:else if submitted}
		<p class="status">Thanks — it's been recorded.</p>
	{:else}
		<label class="field-label" for="feedback-body">What's on your mind?</label>
		<textarea
			id="feedback-body"
			rows={5}
			bind:value={body}
			maxlength={TEXT_LIMITS.NARRATIVE}
			placeholder="A bug, something confusing, an idea — whatever it is."
			disabled={submitting}
		></textarea>

		<label class="field-label" for="feedback-contact">How to reach you (optional)</label>
		<input
			id="feedback-contact"
			type="text"
			bind:value={contact}
			maxlength={TEXT_LIMITS.EMAIL}
			placeholder="Email, Discord, whatever's easiest"
			disabled={submitting}
		/>

		<p class="privacy-note muted-text small">Stored with your account name so we can follow up.</p>

		{#if error}<p class="error-text">{error}</p>{/if}

		<button class="action-btn primary" onclick={submit} disabled={submitting || !body.trim()}>
			{submitting ? 'Sending…' : 'Send feedback'}
		</button>
	{/if}
</div>

<style>
	.feedback-form {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.field-label {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	textarea {
		/* The app's global :global(input) dark styling (app-wide layout)
		   doesn't cover <textarea>, so it needs the same look spelled out here. */
		background: var(--color-surface-2);
		color: inherit;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		font: inherit;
		padding: 0.6rem 0.8rem;
		resize: vertical;
		min-height: 100px;
	}
	textarea:focus {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
	}
	.privacy-note {
		margin: 0;
	}
	.action-btn {
		align-self: flex-start;
	}
</style>
