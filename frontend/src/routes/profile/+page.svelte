<script lang="ts">
	import '$lib/components/shared/actionButton.css';
	import '$lib/components/shared/statusText.css';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import {
		getMe, listMyTables, updateMe, logout,
		createTable, joinTable,
		type Account, type MyTable,
	} from '$lib/api';
	import { feedbackHref } from '$lib/feedback';

	let me = $state<Account | null>(null);
	let tables = $state<MyTable[]>([]);
	let loading = $state(true);
	let error = $state('');

	let editingUsername = $state(false);
	let usernameDraft = $state('');
	let editingEmail = $state(false);
	let emailDraft = $state('');
	let editingPassword = $state(false);
	let passwordDraft = $state('');

	let joinCode = $state('');
	let busy = $state(false);
	let notice = $state('');

	// Reject if a fetch hangs (e.g. a wedged dev server) so the page can show a
	// retry button instead of a permanent "Loading…".
	function withTimeout<T>(p: Promise<T>, ms = 10000): Promise<T> {
		return Promise.race([
			p,
			new Promise<T>((_, reject) =>
				setTimeout(() => reject(new Error('Timed out loading your profile.')), ms)
			),
		]);
	}

	async function load() {
		loading = true;
		error = '';
		try {
			const acct = await withTimeout(getMe());
			if (!acct) { goto('/'); return; }
			me = acct;
			usernameDraft = acct.username;
			emailDraft = acct.email ?? '';
			const res = await withTimeout(listMyTables());
			tables = res.tables;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load profile.';
		} finally {
			loading = false;
		}
	}

	onMount(load);

	async function saveUsername() {
		error = ''; notice = '';
		try {
			me = await updateMe({ username: usernameDraft.trim() });
			editingUsername = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update player name.';
		}
	}
	async function saveEmail() {
		error = ''; notice = '';
		try {
			me = await updateMe({ email: emailDraft.trim() || null });
			editingEmail = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update email.';
		}
	}
	async function savePassword() {
		if (!passwordDraft) return;
		error = ''; notice = '';
		try {
			await updateMe({ password: passwordDraft });
			passwordDraft = '';
			editingPassword = false;
			notice = 'Password updated.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update password.';
		}
	}
	async function doLogout() {
		await logout();
		goto('/');
	}
	async function doCreate() {
		busy = true; error = '';
		try {
			const { game } = await createTable();
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not create table.';
			busy = false;
		}
	}
	async function doJoin() {
		if (!joinCode.trim()) return;
		busy = true; error = '';
		try {
			const { game } = await joinTable(joinCode.trim().toUpperCase());
			goto(`/table/${game.id}`);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not join table.';
			busy = false;
		}
	}
</script>

{#if loading}
	<p class="muted-text">Loading…</p>
{:else if !me}
	<div class="load-error">
		<p class="error-text">{error || 'Could not load your profile.'}</p>
		<button class="action-btn primary" onclick={load}>Retry</button>
	</div>
{:else}
	<div class="profile">
		{#if error}<p class="error-text">{error}</p>{/if}
		{#if notice}<p class="status">{notice}</p>{/if}

		{#if tables.length > 0}
			<section class="card">
				<h2>Your tables</h2>
				<ul>
					{#each tables as t (t.game_id)}
						<li>
							<a href={`/table/${t.game_id}`}>
								Table {t.join_code}
								{#if t.is_facilitator}<span class="tag">facilitator</span>{/if}
							</a>
						</li>
					{/each}
				</ul>
			</section>
		{/if}

		<section class="card">
			<h2>Join a table</h2>
			<p class="hint">Have a code from your host? Enter it to take a seat.</p>
			<div class="join">
				<input aria-label="Join code" placeholder="Join code" bind:value={joinCode} maxlength={6} style="text-transform:uppercase;letter-spacing:0.15em" />
				<button class="action-btn primary" onclick={doJoin} disabled={busy || !joinCode.trim()}>Join</button>
			</div>
			<div class="create-row">
				<span class="hint">Hosting a game?</span>
				<button class="action-btn secondary" onclick={doCreate} disabled={busy}>Create a new table</button>
			</div>
		</section>

		<section class="card">
			<h2>Account</h2>
			<div class="row">
				<span class="label">Player name</span>
				{#if editingUsername}
					<input aria-label="Player name" bind:value={usernameDraft} maxlength={40} />
					<button class="action-btn primary small" onclick={saveUsername}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingUsername = false; usernameDraft = me?.username ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.username}</span>
					<button class="action-btn secondary small" aria-label="Edit player name" onclick={() => { editingUsername = true; }}>Edit</button>
				{/if}
			</div>
			<!-- TODO: Hook up backend email handling -->
			<!-- <div class="row">
				<span class="label">Email</span>
				{#if editingEmail}
					<input type="email" aria-label="Email" bind:value={emailDraft} />
					<button class="action-btn primary small" onclick={saveEmail}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingEmail = false; emailDraft = me?.email ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.email ?? 'Not set. For notifications and password recovery.'}</span>
					<button class="action-btn secondary small" aria-label="Edit email" onclick={() => { editingEmail = true; }}>Edit</button>
				{/if}
			</div> -->
			<div class="row">
				<span class="label">Password</span>
				{#if editingPassword}
					<input type="password" aria-label="New password" bind:value={passwordDraft} placeholder="Enter a new password" />
					<button class="action-btn primary small" onclick={savePassword} disabled={!passwordDraft}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingPassword = false; passwordDraft = ''; }}>Cancel</button>
				{:else}
					<span class="masked">••••••••</span>
					<button class="action-btn secondary small" aria-label="Edit password" onclick={() => { editingPassword = true; }}>Edit</button>
				{/if}
			</div>
		</section>

		<div class="footer-actions">
			<a class="action-btn secondary feedback-btn" href={feedbackHref}>Send feedback</a>
			<button class="action-btn secondary" onclick={doLogout}>Log out</button>
		</div>
	</div>
{/if}

<style>
	.profile { display:flex; flex-direction:column; gap:1.25rem; max-width:600px; margin: 0 auto; padding-top:1rem; }
	h2 { color:var(--color-accent); font-size:1.2rem; margin-bottom:0.75rem; }
	.hint { color:var(--color-text-muted); font-size:0.85rem; margin-bottom:0.6rem; }
	.card { background:var(--color-surface); border:1px solid var(--color-border); border-radius:12px; padding:1.25rem; }
	.row { display:flex; align-items:center; flex-wrap:wrap; gap:0.5rem; padding:0.5rem 0; border-bottom:1px solid var(--color-border-subtle); }
	.row:last-child { border-bottom:none; }
	.label { width:5rem; color:var(--color-text-muted); font-size:0.85rem; }
	.masked { letter-spacing:0.15em; color:var(--color-text-muted); }
	.row input { flex:1; min-width:0; min-height:44px; }
	.row span:not(.label) { flex:1; min-width:0; }
	.tag { color:var(--color-accent); font-size:0.75rem; margin-left:0.5rem; }
	.load-error { display:flex; flex-direction:column; align-items:center; gap:1rem; max-width:600px; margin:0 auto; padding-top:2rem; }
	.status { color:var(--color-accent); font-size:0.9rem; }
	/* On narrow screens, let the field label sit on its own line so the value
	   and its buttons get the full width instead of cramping. */
	@media (max-width: 460px) {
		.label { width:100%; }
	}
	ul { list-style:none; }
	li a { color:var(--color-text); display:block; padding:0.5rem 0; text-decoration:none; }
	li a:hover { color:var(--color-accent); }
	.join { display:flex; gap:0.5rem; }
	/* Secondary "create a table" action, set apart below the primary join row. */
	.create-row { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:0.5rem; margin-top:1rem; padding-top:0.85rem; border-top:1px solid var(--color-border-subtle); }
	.create-row .hint { margin:0; }
	.footer-actions { display:flex; justify-content:center; gap:0.75rem; flex-wrap:wrap; margin-top:0.5rem; }
	.feedback-btn { display:inline-flex; align-items:center; justify-content:center; text-decoration:none; }
</style>
