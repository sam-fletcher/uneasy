<!-- Redemption side of the operator-driven password reset
  (adr/FEEDBACK_AND_RESET_PLAN.md). Reached via a single-use link an owner
  hand-generates with cmd/resetlink after verifying the requester socially.
  No auto-login on success — the backend deliberately doesn't set a session
  here, so the user proves the new password by logging in with it. -->
<script lang="ts">
	import { page } from '$app/state';
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import Notice from '$lib/components/shared/Notice.svelte';
	import { submitPasswordReset } from '$lib/api';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import ErrorText from '$lib/components/shared/ErrorText.svelte';

	const token = $derived(page.url.searchParams.get('token') ?? '');

	let newPassword = $state('');
	let confirmPassword = $state('');
	let submitting = $state(false);
	let succeeded = $state(false);
	// Deliberately uniform — the backend returns one 400 for missing/unknown/
	// expired/used tokens and we don't try to distinguish which client-side.
	let failed = $state(false);
	let mismatchError = $state('');

	async function submit() {
		mismatchError = '';
		if (!newPassword || !confirmPassword) return;
		if (newPassword !== confirmPassword) {
			mismatchError = "Passwords don't match.";
			return;
		}
		submitting = true;
		try {
			await submitPasswordReset({ token, new_password: newPassword });
			succeeded = true;
		} catch {
			failed = true;
		} finally {
			submitting = false;
		}
	}
</script>

<div class="screen">
	<div class="card">
		<h1>Set a new password</h1>

		{#if !token}
			<ErrorText message="This link is missing its token — it may have been copied incorrectly." />
			<a class="action-btn secondary" href="/locked-out">Request a reset</a>
		{:else if succeeded}
			<Notice tone="success" live>Your password has been updated.</Notice>
			<a class="action-btn primary" href="/">Go to login</a>
		{:else if failed}
			<ErrorText message="Link invalid or expired — request a new one." />
			<a class="action-btn secondary" href="/locked-out">Request a reset</a>
		{:else}
			<form onsubmit={(e) => { e.preventDefault(); submit(); }}>
				<label class="field-label" for="rp-new">New password</label>
				<input
					id="rp-new"
					type="password"
					autocomplete="new-password"
					bind:value={newPassword}
					maxlength={TEXT_LIMITS.PASSWORD}
					disabled={submitting}
					required
				/>

				<label class="field-label" for="rp-confirm">Confirm new password</label>
				<input
					id="rp-confirm"
					type="password"
					autocomplete="new-password"
					bind:value={confirmPassword}
					maxlength={TEXT_LIMITS.PASSWORD}
					disabled={submitting}
					required
				/>

				{#if mismatchError}<ErrorText message={mismatchError} />{/if}

				<button
					class="action-btn primary"
					type="submit"
					disabled={submitting || !newPassword || !confirmPassword}
				>
					{submitting ? 'Saving…' : 'Set password'}
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
	/* Each text field is a touch target, so it is at least the touch floor. */
	form input {
		min-height: var(--size-tap);
	}
	.field-label {
		font-size: var(--text-small);
		color: var(--color-text-secondary);
	}
	.action-btn {
		align-self: flex-start;
		text-decoration: none;
	}
</style>
