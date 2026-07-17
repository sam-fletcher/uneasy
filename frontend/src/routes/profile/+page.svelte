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
	import { playerColor } from '$lib/playerColor';
	import PhaseBadge from '$lib/components/shared/PhaseBadge.svelte';
	import { TEXT_LIMITS } from '$lib/textLimits';
	import RetinueSheet from '$lib/components/RetinueSheet.svelte';
	import FeedbackForm from '$lib/components/FeedbackForm.svelte';
	import { getPushState, enablePush, disablePush, type PushState } from '$lib/push';

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
	let feedbackOpen = $state(false);

	// ── Table cards ──────────────────────────────────────────────────────────
	// Live games first; within each group keep the server's most-recently-
	// joined-first order (Array.prototype.sort is stable).
	const sortedTables = $derived(
		[...tables].sort((a, b) => Number(a.phase === 'ended') - Number(b.phase === 'ended'))
	);

	function isYourMove(t: MyTable): boolean {
		return t.phase !== 'ended' && t.waiting_on_player_ids.includes(t.player_id);
	}

	// ── Notifications ────────────────────────────────────────────────────────
	// The cadence <select> works in strings ('off' | '1' | '3' | '8' | '24' |
	// '72'); notify_cadence_hours itself is number | null.
	let cadenceDraft = $state('24');
	let cadenceSaving = $state(false);
	let pushState = $state<PushState>('off');
	let pushBusy = $state(false);

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
			cadenceDraft = acct.notify_cadence_hours == null ? 'off' : String(acct.notify_cadence_hours);
			const res = await withTimeout(listMyTables());
			tables = res.tables;
			getPushState().then((s) => { pushState = s; });
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
			me = { ...me, ...await updateMe({ username: usernameDraft.trim() }) };
			editingUsername = false;
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update player name.';
		}
	}
	async function saveEmail() {
		error = ''; notice = '';
		try {
			me = { ...me, ...await updateMe({ email: emailDraft.trim() || null }) };
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
	async function saveCadence() {
		error = ''; notice = '';
		cadenceSaving = true;
		try {
			const hours = cadenceDraft === 'off' ? null : Number(cadenceDraft);
			me = { ...me, ...await updateMe({ notify_cadence_hours: hours }) };
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update reminder cadence.';
		} finally {
			cadenceSaving = false;
		}
	}

	async function togglePush() {
		if (!me) return;
		error = '';
		pushBusy = true;
		try {
			pushState = pushState === 'on'
				? await disablePush()
				: await enablePush(me.vapid_public_key);
		} catch (e) {
			error = e instanceof Error ? e.message : 'Could not update push notifications.';
		} finally {
			pushBusy = false;
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
		<p class="wordmark">Uneasy Lies <span class="the">the</span> Head</p>

		{#if error}<p class="error-text">{error}</p>{/if}
		{#if notice}<p class="status">{notice}</p>{/if}

		{#if tables.length > 0}
			<section class="card tables-card">
				<h2>Your tables</h2>
				<ul class="table-list">
					{#each sortedTables as t (t.game_id)}
						{@const ended = t.phase === 'ended'}
						<li>
							<a class="table-card" class:your-move={isYourMove(t)} class:ended href={`/table/${t.game_id}`}>
								<span class="table-id">
									<span class="table-code">Table <span class="code-value">{t.join_code}</span></span>
									<PhaseBadge phase={t.phase} />
								</span>
								<span class="pills">
									{#each t.players as p (p.id)}
										{@const online = !ended && (p.online || p.id === t.player_id)}
										{@const waited = !ended && t.waiting_on_player_ids.includes(p.id)}
										<span
											class="pill"
											class:online
											class:waited
											style:--pill-color={playerColor(p)}
											aria-label={`${p.display_name}${p.id === t.player_id ? ' (you)' : ''}${online ? ', online' : ''}${waited ? ', game is waiting on them' : ''}`}
										>
											<span class="pill-dot" aria-hidden="true"></span>
											<span class="pill-name">{p.display_name}</span>
										</span>
									{/each}
								</span>
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
				<button class="action-btn primary" onclick={doCreate} disabled={busy}>Create a new table</button>
			</div>
		</section>

				<section class="card">
			<h2>Notifications</h2>
			<p class="hint">
				If you're on the "Waiting On" list longer than your chosen time, we'll send a reminder.
			</p>
			<div class="row">
				<span class="label">Remind me</span>
				<select aria-label="Reminder cadence" bind:value={cadenceDraft} onchange={saveCadence} disabled={cadenceSaving}>
					<option value="1">Every hour</option>
					<option value="3">Every 3 hours</option>
					<option value="8">Every 8 hours</option>
					<option value="24">Once a day</option>
					<option value="72">Every 3 days</option>
					<option value="off">Off</option>
				</select>
			</div>
			<div class="row push-row">
				<span class="label">This device</span>
				{#if me.vapid_public_key === '' && pushState !== 'unsupported' && pushState !== 'ios-needs-install'}
					<span class="muted-text small">Push isn't configured on this server yet.</span>
				{:else if pushState === 'unsupported'}
					<span class="muted-text small">Push notifications aren't supported in this browser.</span>
				{:else if pushState === 'ios-needs-install'}
					<span class="muted-text small">Add Uneasy to your Home Screen (Share → Add to Home Screen) to enable push on iPhone/iPad.</span>
				{:else if pushState === 'denied'}
					<span class="muted-text small">Blocked — allow notifications for this site in your browser settings.</span>
				{:else}
					<span>Push notifications: {pushState === 'on' ? 'On' : 'Off'}</span>
					<button class="action-btn primary" onclick={togglePush} disabled={pushBusy}>
						{pushBusy ? '…' : pushState === 'on' ? 'Turn off' : 'Turn on'}
					</button>
				{/if}
			</div>
			<p class="hint push-hint">
				The cadence above applies to your whole account; push must be turned
				on separately on each device/browser you want reminders on.
			</p>
		</section>

		<section class="card">
			<h2>Account</h2>
			<div class="row">
				<span class="label">Player name</span>
				{#if editingUsername}
					<input aria-label="Player name" bind:value={usernameDraft} maxlength={TEXT_LIMITS.USERNAME} />
					<button class="action-btn primary small" onclick={saveUsername}>Save</button>
					<button class="action-btn secondary small" onclick={() => { editingUsername = false; usernameDraft = me?.username ?? ''; }}>Cancel</button>
				{:else}
					<span>{me.username}</span>
					<button class="action-btn primary small" aria-label="Edit player name" onclick={() => { editingUsername = true; }}>Edit</button>
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
					<button class="action-btn primary small" aria-label="Edit password" onclick={() => { editingPassword = true; }}>Edit</button>
				{/if}
			</div>
		</section>

		<div class="footer-actions">
			<button class="action-btn primary feedback-btn" onclick={() => feedbackOpen = true}>Send feedback</button>
			<button class="action-btn secondary" onclick={doLogout}>Log out</button>
		</div>
	</div>

	<RetinueSheet open={feedbackOpen} onClose={() => feedbackOpen = false}>
		<div class="feedback-sheet">
			<h3>Send feedback</h3>
			<FeedbackForm />
		</div>
	</RetinueSheet>
{/if}

<style>
	/* One column, always. 750px = double the 375px design minimum: wide
	   enough for two mobile-sized table tiles side by side (the .table-list
	   auto-fill grid handles that), and forms gain nothing from going
	   wider, so every section caps there. */
	/* A content column like any other: capped at 440 and centered
	   (docs/STYLE_GUIDE.md "Layout widths"). */
	.profile { display:flex; flex-direction:column; gap:1.25rem; max-width:440px; margin: 0 auto; padding-top:1rem; }
	.wordmark {
		text-align: center;
		font-family: var(--font-display);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		font-size: clamp(1.15rem, 4.5vw, 1.6rem);
		color: var(--color-accent);
		margin-top: -0.8rem;
		margin-bottom: -0.4rem;
	}
	.wordmark .the { font-size: 0.6em; letter-spacing: 0.1em; }
	h2 { color:var(--color-accent); font-size:1.2rem; margin-bottom:0.75rem; }
	.hint { color:var(--color-text-muted); font-size:0.85rem; margin-bottom:0.6rem; }
	.card { background:var(--color-surface); border:1px solid var(--color-border); border-radius:12px; padding:1.25rem; }
	.row { display:flex; align-items:center; flex-wrap:wrap; gap:0.5rem; padding:0.5rem 0; border-bottom:1px solid var(--color-border); }
	.row:last-child { border-bottom:none; }
	.label { width:5rem; color:var(--color-text-muted); font-size:0.85rem; }
	.masked { letter-spacing:0.15em; color:var(--color-text-muted); }
	.row input, .row select { flex:1; min-width:0; min-height:44px; }
	.row select {
		background: var(--color-surface-2);
		color: var(--color-text);
		border: 1px solid var(--color-border);
		border-radius: var(--radius-sm);
		padding: 0 0.6rem;
		font: inherit;
	}
	.row span:not(.label) { flex:1; min-width:0; }
	.push-row { border-bottom: none; }
	.push-hint { margin-top: -0.3rem; }
	.load-error { display:flex; flex-direction:column; align-items:center; gap:1rem; max-width:440px; margin:0 auto; padding-top:2rem; }
	.status { color:var(--color-accent); font-size:0.9rem; }
	/* The field label sits on its own line so the value and its buttons get
	   the full width instead of cramping — the page column is phone-width
	   (≤440) at every viewport, so this is unconditional. */
	.label { width:100%; }
	ul { list-style:none; }

	/* ── Table cards ────────────────────────────────────────────────────────
	   One card per game, echoing the in-table header: table id + shared
	   PhaseBadge stacked on the left, the roster as a pill grid on the
	   right. State channels: waited pill = gold tint (the app's "current
	   actor" chip treatment), online = thin green ring, your-move = warm
	   card fill, ended = whole card muted with the other treatments
	   suppressed. */
	/* Tiles: sized to look right at the 375px viewport minimum (~330px of
	   card content), so two of them sit side by side once the section nears
	   its 750px cap. min(330px, 100%) keeps the minimum from forcing
	   overflow on phones. */
	.table-list {
		display:grid;
		grid-template-columns:repeat(auto-fill, minmax(min(330px, 100%), 1fr));
		gap:0.6rem;
	}
	.table-card {
		display:flex;
		align-items:center;
		flex-wrap:wrap;
		gap:0.6rem 1.25rem;
		padding:0.7rem 0.75rem;
		border:1px solid var(--color-border);
		border-radius:var(--radius-md);
		color:var(--color-text);
		text-decoration:none;
	}
	.table-card:hover { border-color:var(--color-accent-dim); }
	.table-card:focus-visible { outline:2px solid var(--color-accent); outline-offset:1px; }
	/* The game is blocked on you: warm emphasis fill, same semantics as the
	   in-table selected/active gold surfaces. */
	.table-card.your-move {
		background:var(--color-surface-active);
		border-color:var(--color-accent-dim);
	}
	.table-card.ended { opacity:0.55; }
	.table-card.ended:hover { opacity:0.8; }

	.table-id {
		display:flex;
		flex-direction:column;
		/* Badge centres under the (wider) table name. */
		align-items:center;
		gap:0.5rem;
		flex-shrink:0;
	}
	.table-code { font-size:0.85rem; color:var(--color-text-muted); }
	.code-value { color:var(--color-text); letter-spacing:0.12em; }

	/* Roster pills fill the space right of the table id (the card's column
	   gap keeps a healthy minimum distance) and centre in it both ways,
	   wrapping to as many rows as the names need. */
	.pills {
		flex:1;
		display:flex;
		flex-wrap:wrap;
		justify-content:center;
		align-items:center;
		align-content:center;
		gap:0.4rem;
	}
	.pill {
		display:inline-flex;
		align-items:center;
		gap:0.35rem;
		padding:0.28rem 0.6rem;
		font-size:0.8rem;
		background:var(--color-surface-2);
		border:1px solid var(--color-border);
		border-radius:999px;
	}
	/* Online = ring around the pill; its *presence* is the signal (survives
	   colour-blindness), the muted green echoes the retinue's online dot. */
	.pill.online { box-shadow:0 0 0 1px var(--color-online); }
	.pill.waited {
		background:var(--color-chip-gold-bg);
		border-color:var(--color-chip-gold-border);
	}
	.pill-dot {
		width:8px; height:8px;
		border-radius:50%;
		background:var(--pill-color, var(--color-text-muted));
		flex-shrink:0;
	}
	.pill-name {
		max-width:10ch;
		overflow:hidden;
		white-space:nowrap;
		text-overflow:ellipsis;
	}
	.join { display:flex; gap:0.5rem; }
	/* Secondary "create a table" action, set apart below the primary join row. */
	.create-row { display:flex; align-items:center; justify-content:space-between; flex-wrap:wrap; gap:0.5rem; margin-top:1rem; padding-top:0.85rem; border-top:1px solid var(--color-border); }
	.create-row .hint { margin:0; }
	.footer-actions { display:flex; justify-content:center; gap:0.75rem; flex-wrap:wrap; margin-top:0.5rem; }
	.feedback-btn { display:inline-flex; align-items:center; justify-content:center; text-decoration:none; }
	.feedback-sheet h3 { margin: 0 0 0.75rem; }
</style>
