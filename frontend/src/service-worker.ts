/// <reference types="@sveltejs/kit" />
/// <reference no-default-lib="true" />
/// <reference lib="esnext" />
/// <reference lib="webworker" />

// Push-only service worker (adr/NOTIFICATIONS_PLAN.md Session 4). No fetch
// handler and no asset caching on purpose — this app is a live multiplayer
// game; a stale-cache bug here would be worse than the offline support it'd
// buy. Its only job is to show a push notification and route the click.

const sw = self as unknown as ServiceWorkerGlobalScope;

// Skip the default "waiting" lifecycle: there's nothing cached to go stale,
// so the new worker should take over immediately rather than waiting for
// every open tab to close.
sw.addEventListener('install', () => {
	sw.skipWaiting();
});
sw.addEventListener('activate', (event) => {
	event.waitUntil(sw.clients.claim());
});

interface PushPayload {
	title: string;
	body: string;
	tag: string;
	url: string;
}

// Matches handler/push_notifications.go's pushPayload struct exactly.
function parsePayload(event: PushEvent): PushPayload | null {
	if (!event.data) return null;
	try {
		return event.data.json() as PushPayload;
	} catch {
		return null;
	}
}

sw.addEventListener('push', (event) => {
	const payload = parsePayload(event);
	if (!payload) return;

	event.waitUntil(
		(async () => {
			const clientsList = await sw.clients.matchAll({ type: 'window', includeUncontrolled: true });
			// Don't interrupt a player who already has this table open and
			// focused in front of them — they don't need a system notification
			// to tell them what they're already looking at.
			const alreadyFocused = clientsList.some((c) => {
				const clientPath = new URL(c.url).pathname;
				return clientPath === payload.url && (c as WindowClient).focused;
			});
			if (alreadyFocused) return;

			await sw.registration.showNotification(payload.title, {
				body: payload.body,
				tag: payload.tag,
				// Repeat pings for the same table replace the old one instead of
				// stacking (settled decisions table, adr/NOTIFICATIONS_PLAN.md).
				renotify: true,
				data: { url: payload.url },
			});
		})()
	);
});

sw.addEventListener('notificationclick', (event) => {
	event.notification.close();
	const url = (event.notification.data as { url?: string } | undefined)?.url;
	if (!url) return;

	event.waitUntil(
		(async () => {
			const clientsList = await sw.clients.matchAll({ type: 'window', includeUncontrolled: true });
			const target = new URL(url, sw.location.origin).href;
			const existing = clientsList.find((c) => c.url === target);
			if (existing) {
				await (existing as WindowClient).focus();
				return;
			}
			await sw.clients.openWindow(target);
		})()
	);
});
