# Dependabot alerts: triage and upgrades (tracker item 9)

## What changed and why

The app is a client-side-rendered SPA (`ssr = false`, `prerender = false`,
adapter-static; no `use:enhance`, remote functions or `$app/server` in `src`).
So Svelte and SvelteKit's *client* runtime ship in the bundle, SvelteKit's
*server* code and all SSR/prerender paths never run, and Go serves static files.

### Triage (23 open alerts, 2026-10-31)

| # | Package (alert) | Reaches | Why | Status |
|---|---|---|---|---|
| 45 | go-chi/chi (client IP spoof, XFF) | **Live Go server** | feeds the credential rate limiter | Fixed: 5.4.0 |
| 8 | svelte (DOM clobbering of internal state) | **Browser bundle** | Svelte client runtime | Fixed: 5.57.3 |
| 5, 9 | svelte (SSR spread XSS; SSR promise serialization) | Nothing deployed | SSR only; we never server-render | Fixed: 5.57.3 |
| 43 | devalue (ArrayBuffer revivers) | Bundle (kit client runtime) in principle | we ship no devalue-serialised data; no reachable input | Fixed: 5.9.4 |
| 10, 29 | kit (query.batch, remote-function forms) | Nothing | features unused | Fixed: kit 2.70.3 |
| 46, 48 | kit (prerendered cache-control XSS) | Nothing | `prerender = false` | **Open**: only fixed in kit 3 |
| 47 | kit (enhanced-form redirect) | Nothing | no `use:enhance` | **Open**: only fixed in kit 3 |
| 1 | cookie <0.7 (kit dependency) | Nothing deployed | kit server code only; never runs, only `build/` ships | **Open**: kit 2 pins cookie ^0.6; fixed with kit 3 |
| 4, 30, 31, 35 | postcss | Build time | processes our own CSS | Fixed: 8.5.32 |
| 37 | nanoid (via postcss) | Build time | | Fixed: 3.3.20 |
| 42 | source-map-js (via postcss) | Build time | | Fixed: 1.2.2 |
| 11, 12 | vite (Windows fs.deny / launch-editor) | Dev server only | | Fixed: 6.4.5 |
| 38, 39, 40, 44 | vitest, @vitest/mocker | Test tooling | | Fixed: vitest 4.1.12 |

New since the alert list (found by `npm audit`, not yet a Dependabot alert):
`deepmerge` (via @sveltejs/vite-plugin-svelte 5, build time only). Fixed only by
plugin-svelte 7, which needs vite 8 / kit 3.

Changes: chi 5.3.2 → 5.4.0 (one commit; code forced, see below); npm packages
bumped within their current majors in one commit, with every devDependency
now an exact pin (adapter-static, svelte-check, typescript were still carets).

**Forced code change:** chi 5.4.0 made the no-argument `ClientIPFromXFF()` panic
at startup. `clientIPResolver` now uses `ClientIPFromXFFTrustedProxies(1)`,
which also takes the rightmost X-Forwarded-For entry (one trusted hop). The
existing `client_ip_test.go` cases pass unchanged.

## How it was verified

Against this worktree's code: `make check-fast` (after `make frontend` — the
cmd/server tests need an embedded build), `make check-frontend` (0 errors; 1
pre-existing unused-CSS warning in HelpContent), `npm ci` from scratch,
`make build`, `make test-integration` on a private DB (all pass), Playwright
full suite (incl. chat specs, after #23) against a private server on :8193: 25 passed.
All re-run after rebasing on main.

## Behaviour changes to look at before merging

Svelte 5.55 → 5.57, kit 2.56 → 2.70, vite 6.4.2 → 6.4.5: no code needed
changes, but it is a real runtime bump; a quick play-through on the preview is
worthwhile. Behind Railway's edge, check that rate-limited login still works
(client IP resolves to the real address, not "no trusted client IP" 503s).

## Open questions for the owner

Upgrade to SvelteKit 3 (+ vite 8, vite-plugin-svelte 7, adapter-static 4,
vitest 5)? It is the only fix for alerts 1, 46, 47, 48 and deepmerge. All are
unreachable in this app today (see table), and a major bump may mean real
changes, so I stopped there. Or dismiss those four as "not used" in GitHub.

## Noticed, not fixed

Nothing.
