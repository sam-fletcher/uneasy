<!-- Logged-out "locked out?" intake (adr/FEEDBACK_AND_RESET_PLAN.md). There is
  no self-service reset: the owner verifies the requester socially via the
  stated contact channel and hand-generates a reset link. Honest copy only —
  never implies an automated email is coming, since none exists. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { submitResetRequest } from '$lib/api';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	let username = $state('');
	let contact = $state('');
	let note = $state('');
	let website = $state(''); // honeypot — real users never see or fill this in

	let submitting = $state(false);
	let submitted = $state(false);
	let error = $state('');

	async function submit() {
		if (!username.trim() || !contact.trim() || submitting) return;
		submitting = true;
		error = '';
		try {
			await submitResetRequest({
				username: username.trim(),
				contact: contact.trim(),
				body: note.trim() || undefined,
				website: website || undefined,
			});
			submitted = true;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not send your request.';
		} finally {
			submitting = false;
		}
	}
</script>

<div class="screen">
	<div class="card">
		<h1>Locked out?</h1>

		{#if submitted}
			<Notice tone="success" live>
				Thanks — a person (not a bot) will verify it's really you over the contact you gave,
				then send you a link to set a new password. There's no automated email here, so hang
				tight until you hear back.
			</Notice>
			<a class="action-btn secondary" href="/">Back to login</a>
		{:else}
			<p class="muted-text">
				Password resets here are handled by a person, not a robot. Tell us who you are and
				where to reach you, and once we've verified it's really you, we'll send a reset link
				over that same channel.
			</p>

			<form onsubmit={(e) => { e.preventDefault(); submit(); }}>
				<label class="field-label" for="lo-username">Player name</label>
				<input
					id="lo-username"
					type="text"
					bind:value={username}
					maxlength={TEXT_LIMITS.USERNAME}
					disabled={submitting}
					required
				/>

				<label class="field-label" for="lo-contact">
					Where should I reach you (email or Discord)
				</label>
				<input
					id="lo-contact"
					type="text"
					bind:value={contact}
					maxlength={TEXT_LIMITS.EMAIL}
					disabled={submitting}
					required
				/>

				<label class="field-label" for="lo-note">Anything else? (optional)</label>
				<textarea
					id="lo-note"
					rows={3}
					bind:value={note}
					maxlength={TEXT_LIMITS.NARRATIVE}
					disabled={submitting}
				></textarea>

				<!-- Honeypot: hidden from sighted and screen-reader users alike; a
				     non-empty value marks the request as a bot and the server
				     silently discards it while still responding 200. -->
				<input
					type="text"
					name="website"
					bind:value={website}
					class="honeypot"
					tabindex="-1"
					autocomplete="off"
					aria-hidden="true"
				/>

				{#if error}<ErrorText message={error} />{/if}

				<button
					class="action-btn primary"
					type="submit"
					disabled={submitting || !username.trim() || !contact.trim()}
				>
					{submitting ? 'Sending…' : 'Send request'}
				</button>
			</form>
		{/if}
	</div>
</div>

<style>
	.screen {
		min-height: 100dvh;
		display: flex;
		align-items: center;
		justify-content: center;
		padding: var(--space-5) var(--space-4);
	}
	/* The form sits on the page with no box: a section gets space, and only its
	   controls are boxed. */
	.card {
		width: 100%;
		max-width: 420px;
		display: flex;
		flex-direction: column;
		gap: var(--space-3);
	}
	h1 {
		font-size: var(--text-title);
		color: var(--color-accent);
	}
	form {
		display: flex;
		flex-direction: column;
		gap: var(--space-2);
	}
	/* Each text field is a touch target, so it is at least the touch floor. The
	   honeypot is hidden from everyone, so it keeps its own size. */
	form input:not(.honeypot) {
		min-height: var(--size-tap);
	}
	.field-label {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	/* The global input rule (+layout.svelte) doesn't cover <textarea>, so the
	   same look is spelled out here. */
	textarea {
		background: var(--color-surface-2);
		color: inherit;
		border: 1px solid var(--color-border-control);
		border-radius: var(--radius-control);
		font: inherit;
		font-size: var(--text-body);
		padding: var(--space-2) var(--space-3);
		resize: vertical;
		min-height: 72px;
	}
	textarea:focus {
		outline: 2px solid var(--color-you);
		outline-offset: 2px;
	}
	.honeypot {
		position: absolute;
		left: -9999px;
		width: 1px;
		height: 1px;
		overflow: hidden;
	}
	.action-btn {
		align-self: flex-start;
		text-decoration: none;
	}
</style>
