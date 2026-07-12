// api/push.ts — client for POST/DELETE /api/push-subscriptions
// (adr/NOTIFICATIONS_PLAN.md Session 4). Registers/removes the browser
// PushSubscription objects lib/push.ts creates via pushManager.subscribe.

import { apiFetch } from './client';

export function createPushSubscription(sub: PushSubscriptionJSON): Promise<{ subscription: unknown }> {
	return apiFetch('/push-subscriptions', {
		method: 'POST',
		body: JSON.stringify({
			endpoint: sub.endpoint,
			keys: { p256dh: sub.keys?.p256dh, auth: sub.keys?.auth },
		}),
	});
}

// Raw fetch, not apiFetch: the endpoint returns 204 No Content, which
// apiFetch's res.json() would throw on.
export async function deletePushSubscription(endpoint: string): Promise<void> {
	await fetch('/api/push-subscriptions', {
		method: 'DELETE',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ endpoint }),
	});
}
