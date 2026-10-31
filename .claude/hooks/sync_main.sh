#!/usr/bin/env bash
# SessionStart hook: start every session from the latest main.
#
# 1. Fetches origin and fast-forwards the main checkout, so the dev
#    containers (which mount it) run the latest main and new worktrees
#    branch from it. Restarts the Vite container when frontend
#    dependencies changed, because it only runs `npm install` at start-up,
#    and the Go server containers when migrations changed, because Air
#    doesn't watch db/migrations and the migrations are embedded at build.
# 2. In a worktree whose branch has no commits of its own yet, fast-forwards
#    the branch too, so new work never starts from a stale main.
#
# Only ever fast-forwards, and leaves anything with local changes alone.
# What it prints is added to the session's context. The coordinator also
# runs it by hand after the owner merges a PR mid-session.

set -u
here=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
common=$(cd "$here" && cd "$(git rev-parse --git-common-dir)" && pwd)
main_checkout=$(dirname "$common")

# Give up on a stalled network rather than hanging until the hook timeout.
if ! git -C "$main_checkout" -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
    fetch -q origin main 2>/dev/null; then
  echo "sync_main: couldn't fetch origin; main may be stale."
  exit 0
fi

# ── The main checkout ─────────────────────────────────────────────────────────
branch=$(git -C "$main_checkout" branch --show-current)
before=$(git -C "$main_checkout" rev-parse HEAD)
if [ "$branch" != main ]; then
  echo "sync_main: the main checkout is on '$branch', not main; left alone."
elif [ -n "$(git -C "$main_checkout" status --porcelain --untracked-files=no)" ]; then
  echo "sync_main: the main checkout has uncommitted changes; not updated."
elif git -C "$main_checkout" merge -q --ff-only origin/main 2>/dev/null; then
  after=$(git -C "$main_checkout" rev-parse HEAD)
  if [ "$before" != "$after" ]; then
    echo "sync_main: main checkout fast-forwarded to $(git -C "$main_checkout" log -1 --format='%h %s')."
    changed=$(git -C "$main_checkout" diff --name-only "$before" "$after")
    if grep -q '^frontend/package-lock.json$' <<<"$changed"; then
      if docker restart uneasy-frontend-1 >/dev/null 2>&1; then
        echo "sync_main: frontend dependencies changed; restarted the Vite container."
      else
        echo "sync_main: frontend dependencies changed; restart the Vite container (docker restart uneasy-frontend-1)."
      fi
    fi
    if grep -q '^db/migrations/' <<<"$changed"; then
      if docker restart uneasy-server-1 uneasy-server-e2e-1 >/dev/null 2>&1; then
        echo "sync_main: migrations changed; restarted the Go server containers to apply them."
      else
        echo "sync_main: migrations changed; restart the Go server containers (docker restart uneasy-server-1 uneasy-server-e2e-1)."
      fi
    fi
    if grep -qE '^(Dockerfile\.dev|docker-compose\.yml|\.air.*\.toml)$' <<<"$changed"; then
      echo "sync_main: the dev stack's own config changed; tell the owner to rebuild it (docker compose up -d --build)."
    fi
  fi
else
  echo "sync_main: the main checkout can't fast-forward to origin/main; left alone."
fi

# ── This session's worktree ───────────────────────────────────────────────────
[ "$here" = "$main_checkout" ] && exit 0
own=$(git -C "$here" rev-list --count origin/main..HEAD 2>/dev/null) || exit 0
behind=$(git -C "$here" rev-list --count HEAD..origin/main 2>/dev/null) || exit 0
[ "$behind" -eq 0 ] && exit 0
dirty=$(git -C "$here" status --porcelain --untracked-files=no)
if [ "$own" -eq 0 ] && [ -z "$dirty" ]; then
  if git -C "$here" merge -q --ff-only origin/main 2>/dev/null; then
    echo "sync_main: this branch had no commits yet; fast-forwarded it to the latest main."
  else
    echo "sync_main: this branch is behind main but couldn't fast-forward (an untracked file in the way?). Run git merge --ff-only origin/main and read the error."
  fi
elif [ "$own" -eq 0 ]; then
  echo "sync_main: this branch is behind main and has uncommitted changes. Commit them, then git rebase origin/main."
else
  echo "sync_main: this branch is behind main by $behind commit(s). Rebase onto it (git rebase origin/main) before opening a PR."
fi
exit 0
