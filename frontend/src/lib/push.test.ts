import { describe, it, expect } from 'vitest';
import { derivePushState, urlBase64ToUint8Array } from './push';

describe('derivePushState', () => {
	const base = { isIOS: false, isStandalone: false, apiSupported: true, permission: 'default' as NotificationPermission, subscribed: false };

	it('flags un-installed iOS before any feature detection', () => {
		expect(derivePushState({ ...base, isIOS: true, isStandalone: false, apiSupported: false })).toBe('ios-needs-install');
	});

	it('treats an installed iOS PWA like any other supported browser', () => {
		expect(derivePushState({ ...base, isIOS: true, isStandalone: true, subscribed: true, permission: 'granted' })).toBe('on');
	});

	it('reports unsupported when the Push API is missing', () => {
		expect(derivePushState({ ...base, apiSupported: false })).toBe('unsupported');
	});

	it('reports denied once the user has refused the browser prompt', () => {
		expect(derivePushState({ ...base, permission: 'denied' })).toBe('denied');
	});

	it('is only "on" once both granted and actually subscribed', () => {
		expect(derivePushState({ ...base, permission: 'granted', subscribed: false })).toBe('off');
		expect(derivePushState({ ...base, permission: 'granted', subscribed: true })).toBe('on');
	});

	it('defaults to off when nothing has happened yet', () => {
		expect(derivePushState(base)).toBe('off');
	});
});

describe('urlBase64ToUint8Array', () => {
	it('round-trips a padded base64url VAPID-shaped key', () => {
		// "hello" base64url-encoded without padding.
		const bytes = urlBase64ToUint8Array('aGVsbG8');
		expect(new TextDecoder().decode(bytes)).toBe('hello');
	});

	it('handles the URL-safe substitutions (- and _)', () => {
		// Bytes [0xff, 0xff, 0xbe] base64-encode to "//++vg=="-ish territory;
		// pick a value whose standard base64 contains + and / to exercise the swap.
		const original = new Uint8Array([0xfb, 0xff, 0xbf]);
		const std = btoa(String.fromCharCode(...original)); // "+/+/"-shaped
		const urlSafe = std.replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
		expect(urlBase64ToUint8Array(urlSafe)).toEqual(original);
	});
});
