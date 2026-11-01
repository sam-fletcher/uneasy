---
title: "Upgrade to SvelteKit 3, or dismiss the five leftover Dependabot alerts?"
---

SvelteKit alerts for prerendered cache-control XSS (two) and the enhanced-form redirect, the `cookie` advisory (a kit dependency) and `deepmerge` (via vite-plugin-svelte 5) are only fixed by the SvelteKit 3 line: kit 3, vite 8, vite-plugin-svelte 7, adapter-static 4, vitest 5. None is reachable in this client-rendered, non-prerendered app that uses no enhanced forms (triage in `docs/work/notes/dep-security-10c2bf.md`). Do the major bump as its own piece of work, or dismiss the five in GitHub as unused?
