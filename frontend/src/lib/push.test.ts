import { describe, it, expect } from 'vitest';
import { derivePushState, deriveBlockedHelp, urlBase64ToUint8Array } from './push';

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

describe('deriveBlockedHelp', () => {
	const CHROME_MAC =
		'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36';
	const CHROME_ANDROID =
		'Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Mobile Safari/537.36';
	const EDGE_WINDOWS =
		'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/127.0.0.0 Safari/537.36 Edg/127.0.0.0';
	const FIREFOX_WINDOWS = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0';
	const SAFARI_MAC =
		'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15';

	it('sends an installed iOS app to the iOS Settings app, not the browser', () => {
		const help = deriveBlockedHelp({ ua: 'iPhone', isIOS: true });
		expect(help.steps[0]).toMatch(/iOS Settings app/);
		// No desktop OS caveat on a phone.
		expect(help.note).toBeUndefined();
	});

	it('uses the Android address-bar path for any Android browser', () => {
		for (const ua of [CHROME_ANDROID, 'Mozilla/5.0 (Android 14; Mobile; rv:128.0) Gecko/128.0 Firefox/128.0']) {
			const help = deriveBlockedHelp({ ua, isIOS: false });
			expect(help.steps[0]).toMatch(/Permissions/);
			expect(help.note).toBeUndefined();
		}
	});

	it("points Firefox at the lock icon's × rather than an Allow toggle", () => {
		const help = deriveBlockedHelp({ ua: FIREFOX_WINDOWS, isIOS: false });
		expect(help.steps[1]).toMatch(/×/);
	});

	it('treats Edge as Chromium, not as Safari', () => {
		// Edge's UA contains "Safari/537.36" too, so ordering is what's under test.
		expect(deriveBlockedHelp({ ua: EDGE_WINDOWS, isIOS: false }).steps[0]).toMatch(/left of the web address/);
		expect(deriveBlockedHelp({ ua: SAFARI_MAC, isIOS: false }).steps[0]).toMatch(/Safari → Settings/);
	});

	it('adds the OS-level caveat per desktop platform only', () => {
		expect(deriveBlockedHelp({ ua: CHROME_MAC, isIOS: false }).note).toMatch(/System Settings/);
		expect(deriveBlockedHelp({ ua: EDGE_WINDOWS, isIOS: false }).note).toMatch(/Settings → System/);
		expect(deriveBlockedHelp({ ua: CHROME_ANDROID, isIOS: false }).note).toBeUndefined();
	});

	it('always yields actionable steps, even for an unknown browser', () => {
		const help = deriveBlockedHelp({ ua: 'SomeFutureBrowser/1.0', isIOS: false });
		expect(help.steps.length).toBeGreaterThan(0);
		expect(help.steps.every((s) => s.length > 0)).toBe(true);
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
