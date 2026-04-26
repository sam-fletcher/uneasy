<script lang="ts">
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import {
		getMe, listMyTables, updateMe, logout,
		createTable, joinTable,
		type Account, type MyTable,
	} from '$lib/api';

	let me = $state<Account | null>(null);
	let tables = $state<MyTable[]>([]);
	let loading = $state(true);
	let error = $state('');

	let editingUsername = $state(false);
	let usernameDraft = $state('');
	let editingEmail = $state(false);
	let emailDraft = $state('');
	let codeDraft = $state('');

	let joinCode = $state('');
	let busy = $state(false);

	onMount(async () => {
		try {
			const acct = await getMe();
			if (!acct) { goto('/login'); return; }
			me = acct;
			usernameDraft = acct.username;
			emailDraft = acct.email ?? '';
			const res = await listMyTables();
			tables = res.tables;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not load profile.';
		} finally {
			loading = false;
		}
	});

	async function saveUsername() {
		try {
			me = await updateMe({ username: usernameDraft.trim() });
			editingUsername = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update username.';
		}
	}
	async function saveEmail() {
		try {
			me = await updateMe({ email: emailDraft.trim() || null });
			editingEmail = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update email.';
		}
	}
	async function saveCode() {
		if (!codeDraft) return;
		try {
			await updateMe({ code: codeDraft });
			codeDraft = '';
			error = 'Code updated.';
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update code.';
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
	<p class="muted">Loading…</p>
{:else if me}
	<div class="profile">
		<header>
			<h1>Profile</h1>
			<button class="secondary" onclick={doLogout}>Log out</button>
		</header>

		{#if error}<p class="status">{error}</p>{/if}

		<section class="card">
			<h2>Account</h2>
			<div class="row">
				<span class="label">Username</span>
				{#if editingUsername}
					<input bind:value={usernameDraft} />
					<button class="small" onclick={saveUsername}>Save</button>
					<button class="small secondary" onclick={() => { editingUsername = false; usernameDraft = me?.username ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.username}</span>
					<button class="small secondary" onclick={() => { editingUsername = true; }}>Edit</button>
				{/if}
			</div>
			<div class="row">
				<span class="label">Email</span>
				{#if editingEmail}
					<input type="email" bind:value={emailDraft} />
					<button class="small" onclick={saveEmail}>Save</button>
					<button class="small secondary" onclick={() => { editingEmail = false; emailDraft = me?.email ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.email ?? '—'}</span>
					<button class="small secondary" onclick={() => { editingEmail = true; }}>Edit</button>
				{/if}
			</div>
			<div class="row">
				<span class="label">New code</span>
				<input type="password" bind:value={codeDraft} placeholder="Leave blank to keep current" />
				<button class="small" onclick={saveCode} disabled={!codeDraft}>Update code</button>
			</div>
		</section>

		<section class="card">
			<h2>Your tables</h2>
			{#if tables.length === 0}
				<p class="muted">You haven't joined any tables yet.</p>
			{:else}
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
			{/if}
		</section>

		<section class="card">
			<h2>New table</h2>
			<div class="actions">
				<button class="primary" onclick={doCreate} disabled={busy}>Create a new table</button>
			</div>
			<div class="join">
				<input placeholder="Join code (e.g. KMPX72)" bind:value={joinCode} maxlength={6} style="text-transform:uppercase;letter-spacing:0.15em" />
				<button class="primary" onclick={doJoin} disabled={busy || !joinCode.trim()}>Join</button>
			</div>
		</section>
	</div>
{/if}

<style>
	.profile { display:flex; flex-direction:column; gap:1.25rem; max-width:600px; margin: 0 auto; padding-top:1rem; }
	header { display:flex; align-items:center; justify-content:space-between; }
	h1 { color:#c8a96e; font-size:1.5rem; }
	h2 { color:#c8a96e; font-size:1rem; margin-bottom:0.75rem; }
	.card { background:#252525; border:1px solid #333; border-radius:12px; padding:1.25rem; }
	.row { display:flex; align-items:center; gap:0.5rem; padding:0.4rem 0; border-bottom:1px solid #2e2e2e; }
	.row:last-child { border-bottom:none; }
	.label { width:5rem; color:#999; font-size:0.85rem; }
	.row input { flex:1; }
	.row span:not(.label) { flex:1; }
	.tag { color:#c8a96e; font-size:0.75rem; margin-left:0.5rem; }
	.muted { color:#888; }
	.status { color:#c8a96e; font-size:0.9rem; }
	ul { list-style:none; }
	li a { color:#e8e4d9; display:block; padding:0.5rem 0; text-decoration:none; }
	li a:hover { color:#c8a96e; }
	.actions { display:flex; gap:0.5rem; }
	.join { display:flex; gap:0.5rem; margin-top:0.75rem; }
	.primary { background:#c8a96e; color:#1a1a1a; font-weight:600; }
	.primary:hover:not(:disabled) { background:#d9bb80; }
	.secondary { background:#333; color:#e8e4d9; }
	.secondary:hover:not(:disabled) { background:#3e3e3e; }
	.small { font-size:0.85rem; padding:0.35rem 0.7rem; }
	button:disabled { opacity:0.5; cursor:not-allowed; }
</style>
