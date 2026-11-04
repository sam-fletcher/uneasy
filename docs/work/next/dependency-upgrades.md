---
title: "Upgrade to SvelteKit 3, or dismiss the five leftover Dependabot alerts?"
---

SvelteKit alerts for prerendered cache-control XSS (two) and the enhanced-form redirect, the `cookie` advisory (a kit dependency) and `deepmerge` (via vite-plugin-svelte 5) are only fixed by the SvelteKit 3 line: kit 3, vite 8, vite-plugin-svelte 7, adapter-static 4, vitest 5. None is reachable in this client-rendered, non-prerendered app that uses no enhanced forms (triage in `docs/work/notes/dep-security-10c2bf.md`). Do the major bump as its own piece of work, or dismiss the five in GitHub as unused?

## What a trial found

A throwaway install (kit 3.1.1, vite 8.3.5, vite-plugin-svelte 7.3.1,
adapter-static 4.0.1, vitest 5.0.5, TypeScript 6.0.3) clears every SvelteKit,
`cookie`, postcss-family and Vitest alert. Only `deepmerge` remains, because
vite-plugin-svelte 7 still depends on it. So even this bump leaves one alert
(build time only). It builds, and 558 of 559 unit tests pass (the one failure
was my scratch location missing `game/`, not the upgrade).

Work it needs, if you say go:

- Move `svelte.config.js` into `sveltekit({ adapter: adapter({ fallback:
  'index.html' }) })` in `vite.config.ts`; kit 3 rejects the old file.
- `$lib` is removed: add `alias: { $lib: 'src/lib' }` there (smallest change)
  or rename every import to `#lib` (a large mechanical diff across the repo).
- `tsconfig.json` must extend `$app/tsconfig`, not `.svelte-kit/tsconfig.json`.
- `svelte-check` (TypeScript 6) then reports 10 errors: `renotify` in
  `src/service-worker.ts`, a typed-route error at
  `src/routes/profile/+page.svelte:180`, and `tests/shot.mjs` typing.
- Node 22.17 or newer is required, and the Dockerfile builds on node:20, so
  its frontend stage must move to 22 or 24 (CI already uses 24).
- Verify: `make check-fast`, `make build`, integration, the full e2e suite,
  and a look in a real browser, since the bundle's runtime changes.
