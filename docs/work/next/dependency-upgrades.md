---
title: "Later: upgrade SvelteKit 3, Go and TypeScript together"
order: 200
---

No rush. The owner chose on 2026-11-03 to leave this for now, and to look at it
alongside a Go version bump and TypeScript 7.1, which may fit the stack better once
it is released.

Four low-severity Dependabot alerts stay open until then: the SvelteKit redirect and
prerender XSS alerts, `cookie`, and `deepmerge`. None is reachable in this
client-rendered, non-prerendered app that uses no enhanced forms (triage in
`docs/work/notes/dep-security-10c2bf.md`). Don't dismiss them without asking.

## What a trial found

A throwaway install (kit 3.1.1, vite 8.3.5, vite-plugin-svelte 7.3.1,
adapter-static 4.0.1, vitest 5.0.5, TypeScript 6.0.3) clears every SvelteKit,
`cookie`, postcss-family and Vitest alert. Only `deepmerge` remains, because
vite-plugin-svelte 7 still depends on it. So even this bump leaves one alert
(build time only). It builds, and 558 of 559 unit tests pass (the one failure
was the scratch location missing `game/`, not the upgrade).

Work it needs:

- Move `svelte.config.js` into `sveltekit({ adapter: adapter({ fallback:
  'index.html' }) })` in `vite.config.ts`; kit 3 rejects the old file.
- `$lib` is removed: add `alias: { $lib: 'src/lib' }` there (smallest change)
  or rename every import to `#lib` (a large mechanical diff across the repo).
- `tsconfig.json` must extend `$app/tsconfig`, not `.svelte-kit/tsconfig.json`.
- `svelte-check` (TypeScript 6) then reports 10 errors: `renotify` in
  `src/service-worker.ts`, a typed-route error at
  `src/routes/profile/+page.svelte:180`, and `tests/shot.mjs` typing.
- Node 22.17 or newer is required. The Dockerfile, the dev container and CI
  are all on Node 24 already, so nothing to do here.
- Verify: `make check-fast`, `make build`, integration, the full e2e suite,
  and a look in a real browser, since the bundle's runtime changes.
