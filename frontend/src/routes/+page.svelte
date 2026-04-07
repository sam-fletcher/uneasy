<!-- Landing page: pick a name, then create or join a table. -->
<script lang="ts">
	import { goto } from '$app/navigation';
	import { setIdentity, createTable, joinTable } from '$lib/api';

	let displayName = $state('');
	let joinCode = $state('');
	let mode = $state<'idle' | 'join'>('idle');
	let error = $state('');
	let loading = $state(false);

	async function handleCreate() {
		if (!displayName.trim()) { error = 'Enter a name first.'; return; }
		loading = true; error = '';
		try {
			await setIdentity(displayName.trim());
			const { game } = await createTable();
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Something went wrong.';
		} finally {
			loading = false;
		}
	}

	async function handleJoin() {
		if (!displayName.trim()) { error = 'Enter a name first.'; return; }
		if (!joinCode.trim()) { error = 'Enter a join code.'; return; }
		loading = true; error = '';
		try {
			await setIdentity(displayName.trim());
			const { game } = await joinTable(joinCode.trim().toUpperCase());
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Something went wrong.';
		} finally {
			loading = false;
		}
	}
</script>

<div class="landing">
	<h1>Uneasy Lies the Head</h1>
	<p class="subtitle">A play-by-post royal court drama</p>

	<div class="card">
		<label for="name">Your name</label>
		<input
			id="name"
			type="text"
			placeholder="e.g. Margaret"
			bind:value={displayName}
			maxlength={40}
			disabled={loading}
		/>

		{#if mode === 'join'}
			<label for="code" style="margin-top:1rem">Join code</label>
			<input
				id="code"
				type="text"
				placeholder="e.g. KMPX72"
				bind:value={joinCode}
				maxlength={6}
				style="text-transform:uppercase;letter-spacing:0.15em"
				disabled={loading}
			/>
		{/if}

		{#if error}
			<p class="error">{error}</p>
		{/if}

		<div class="actions">
			{#if mode === 'idle'}
				<button class="primary" onclick={handleCreate} disabled={loading}>
					{loading ? 'Creating…' : 'Create a table'}
				</button>
				<button class="secondary" onclick={() => { mode = 'join'; error = ''; }} disabled={loading}>
					Join a table
				</button>
			{:else}
				<button class="primary" onclick={handleJoin} disabled={loading}>
					{loading ? 'Joining…' : 'Join'}
				</button>
				<button class="secondary" onclick={() => { mode = 'idle'; error = ''; }} disabled={loading}>
					Back
				</button>
			{/if}
		</div>
	</div>
</div>

<style>
	.landing {
		display: flex;
		flex-direction: column;
		align-items: center;
		padding-top: 4rem;
		gap: 1rem;
	}

	h1 {
		font-size: 2rem;
		font-weight: 700;
		color: #c8a96e;
		text-align: center;
	}

	.subtitle {
		color: #999;
		text-align: center;
	}

	.card {
		width: 100%;
		max-width: 380px;
		background: #252525;
		border: 1px solid #333;
		border-radius: 12px;
		padding: 2rem;
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 1rem;
	}

	label {
		font-size: 0.85rem;
		color: #aaa;
	}

	.error {
		color: #e07070;
		font-size: 0.9rem;
		margin-top: 0.25rem;
	}

	.actions {
		display: flex;
		flex-direction: column;
		gap: 0.5rem;
		margin-top: 1.25rem;
	}

	.primary {
		background: #c8a96e;
		color: #1a1a1a;
		font-weight: 600;
	}

	.primary:hover:not(:disabled) {
		background: #d9bb80;
	}

	.secondary {
		background: #333;
		color: #e8e4d9;
	}

	.secondary:hover:not(:disabled) {
		background: #3e3e3e;
	}

	button:disabled {
		opacity: 0.5;
		cursor: not-allowed;
	}
</style>
