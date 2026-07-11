<!-- Logged-out "locked out?" intake (adr/FEEDBACK_AND_RESET_PLAN.md). There is
  no self-service reset: the owner verifies the requester socially via the
  stated contact channel and hand-generates a reset link. Honest copy only —
  never implies an automated email is coming, since none exists. -->
<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { submitResetRequest } from '$lib/api';
	import { TEXT_LIMITS } from '$lib/textLimits';

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
			<p class="status">
				Thanks — a person (not a bot) will verify it's really you over the contact you gave,
				then send you a link to set a new password. There's no automated email here, so hang
				tight until you hear back.
			</p>
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

				{#if error}<p class="error-text">{error}</p>{/if}

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
		padding: 1.5rem 1rem;
	}
	.card {
		width: 100%;
		max-width: 420px;
		background: var(--color-surface);
		border: 1px solid var(--color-border);
		border-radius: 12px;
		padding: 1.5rem;
		display: flex;
		flex-direction: column;
		gap: 0.75rem;
	}
	h1 {
		font-size: 1.4rem;
		color: var(--color-accent);
	}
	.status {
		color: var(--color-accent);
		font-size: 0.9rem;
	}
	form {
		display: flex;
		flex-direction: column;
		gap: 0.6rem;
	}
	.field-label {
		font-size: 0.85rem;
		color: var(--color-text-muted);
	}
	textarea {
		/* The app's global :global(input) dark styling doesn't cover
		   <textarea>, so it needs the same look spelled out here. */
		background: var(--color-surface-2);
		color: inherit;
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		font: inherit;
		padding: 0.6rem 0.8rem;
		resize: vertical;
		min-height: 72px;
	}
	textarea:focus {
		outline: 2px solid var(--color-accent);
		outline-offset: 1px;
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
