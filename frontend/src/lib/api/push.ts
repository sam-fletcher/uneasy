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

// Returns 204 No Content. This used to be a raw fetch because apiFetch's
// res.json() threw on an empty body — and, because it also never checked
// res.ok, a failed unsubscribe reported success and the toggle flipped anyway.
// apiFetch handles 204 now.
export async function deletePushSubscription(endpoint: string): Promise<void> {
	await apiFetch<void>('/push-subscriptions', {
		method: 'DELETE',
		body: JSON.stringify({ endpoint }),
	});
}
