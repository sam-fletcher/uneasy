#!/usr/bin/env bash
# Keep every session on the latest main, and tell it when PRs merge.
#
# Runs as two hooks (.claude/settings.json):
#   SessionStart       sync_main.sh             — at session start
#   UserPromptSubmit   sync_main.sh --on-prompt — each time the owner writes
#
# Each run:
# 1. Fetches origin (on prompts, at most every 30s) and fast-forwards the
#    main checkout, so the dev containers (which mount it) run the latest
#    main and new worktrees branch from it. Restarts the Vite container when
#    frontend dependencies changed, because it only runs `npm install` at
#    start-up, and the Go server containers when migrations changed, because
#    Air doesn't watch db/migrations and the migrations are embedded at build.
# 2. Tells this session which PRs merged since it last looked. Each session
#    remembers the last origin/main it saw, so it hears about a merge even
#    when another session's run did the fast-forward.
# 3. In a worktree whose branch has no commits of its own yet, fast-forwards
#    the branch too, so new work never starts from a stale main; a branch
#    with its own work is told to rebase.
#
# Only ever fast-forwards, and leaves anything with local changes alone.
# What it prints is added to the session's context; on prompts it prints
# only when something changed.

set -u
on_prompt=false
[ "${1:-}" = "--on-prompt" ] && on_prompt=true
input=$(cat 2>/dev/null || true)
session_id=$(sed -n 's/.*"session_id" *: *"\([^"]*\)".*/\1/p' <<<"$input" | head -1)

here=$(git rev-parse --show-toplevel 2>/dev/null) || exit 0
common=$(cd "$here" && cd "$(git rev-parse --git-common-dir)" && pwd)
main_checkout=$(dirname "$common")
state="$common/sync_main"
mkdir -p "$state/seen"

say() { echo "sync_main: $*"; }

# One run at a time: sessions started together would otherwise race on the
# fetch and the fast-forward. A prompt run just skips; a start-up run waits.
lock="$state/lock"
locked=false
for _ in $(seq 1 20); do
  if mkdir "$lock" 2>/dev/null; then locked=true; break; fi
  # A lock left by a killed run is stale after a minute.
  if [ -n "$(find "$lock" -maxdepth 0 -mmin +1 2>/dev/null)" ]; then
    rmdir "$lock" 2>/dev/null; continue
  fi
  $on_prompt && exit 0
  sleep 0.5
done
$locked || exit 0
trap 'rmdir "$lock" 2>/dev/null' EXIT

# ── Fetch ─────────────────────────────────────────────────────────────────────
fresh=""
$on_prompt && fresh=$(find "$common/FETCH_HEAD" -mtime -30s 2>/dev/null)
if [ -z "$fresh" ]; then
  # Give up on a stalled network rather than hanging until the hook timeout.
  if ! git -C "$main_checkout" -c http.lowSpeedLimit=1000 -c http.lowSpeedTime=10 \
      fetch -q origin main 2>/dev/null; then
    $on_prompt || say "couldn't fetch origin; main may be stale."
    exit 0
  fi
fi
origin_main=$(git -C "$main_checkout" rev-parse -q --verify origin/main) || exit 0

# ── The main checkout ─────────────────────────────────────────────────────────
branch=$(git -C "$main_checkout" branch --show-current)
before=$(git -C "$main_checkout" rev-parse HEAD)
if [ "$before" = "$origin_main" ]; then
  :
elif [ "$branch" != main ]; then
  $on_prompt || say "the main checkout is on '${branch:-a detached HEAD}', not main; left alone."
elif [ -n "$(git -C "$main_checkout" status --porcelain --untracked-files=no)" ]; then
  say "the main checkout has uncommitted changes, so it wasn't updated to the latest main."
elif git -C "$main_checkout" merge -q --ff-only origin/main 2>/dev/null; then
  say "main checkout fast-forwarded to $(git -C "$main_checkout" log -1 --format='%h %s')."
  changed=$(git -C "$main_checkout" diff --name-only "$before" HEAD)
  if grep -q '^frontend/package-lock.json$' <<<"$changed"; then
    if docker restart uneasy-frontend-1 >/dev/null 2>&1; then
      say "frontend dependencies changed; restarted the Vite container."
    else
      say "frontend dependencies changed; restart the Vite container (docker restart uneasy-frontend-1)."
    fi
  fi
  if grep -q '^db/migrations/' <<<"$changed"; then
    if docker restart uneasy-server-1 uneasy-server-e2e-1 >/dev/null 2>&1; then
      say "migrations changed; restarted the Go server containers to apply them."
    else
      say "migrations changed; restart the Go server containers (docker restart uneasy-server-1 uneasy-server-e2e-1)."
    fi
  fi
  if grep -qE '^(Dockerfile\.dev|docker-compose\.yml|\.air.*\.toml)$' <<<"$changed"; then
    say "the dev stack's own config changed; tell the owner to rebuild it (docker compose up -d --build)."
  fi
else
  say "the main checkout can't fast-forward to origin/main; left alone."
fi

# ── What merged since this session last looked ────────────────────────────────
merged_news=false
if [ -n "$session_id" ]; then
  seen_file="$state/seen/$session_id"
  seen=$(cat "$seen_file" 2>/dev/null || true)
  if [ -n "$seen" ] && [ "$seen" != "$origin_main" ] &&
      git -C "$main_checkout" merge-base --is-ancestor "$seen" "$origin_main" 2>/dev/null; then
    # GitHub's merge commits read "Merge pull request #N from …", then the
    # PR title as the first line of the body.
    for c in $(git -C "$main_checkout" log --merges --reverse --format=%h "$seen..$origin_main"); do
      pr=$(git -C "$main_checkout" log -1 --format=%s "$c" | sed -n 's/^Merge pull request \(#[0-9]*\).*/\1/p')
      [ -n "$pr" ] || continue
      say "merged into main since this session last checked: $pr $(git -C "$main_checkout" log -1 --format=%b "$c" | head -1)"
      merged_news=true
    done
  fi
  echo "$origin_main" >"$seen_file"
  # Forget sessions untouched for two weeks.
  find "$state/seen" -type f -mtime +14 -delete 2>/dev/null
fi

# ── This session's worktree ───────────────────────────────────────────────────
[ "$here" = "$main_checkout" ] && exit 0
own=$(git -C "$here" rev-list --count origin/main..HEAD 2>/dev/null) || exit 0
behind=$(git -C "$here" rev-list --count HEAD..origin/main 2>/dev/null) || exit 0
[ "$behind" -eq 0 ] && exit 0
# On prompts, speak only when main has just moved, not on every message.
$on_prompt && ! $merged_news && exit 0
dirty=$(git -C "$here" status --porcelain --untracked-files=no)
if [ "$own" -eq 0 ] && [ -z "$dirty" ]; then
  if git -C "$here" merge -q --ff-only origin/main 2>/dev/null; then
    say "this branch had no commits yet; fast-forwarded it to the latest main."
  else
    say "this branch is behind main but couldn't fast-forward (an untracked file in the way?). Run git merge --ff-only origin/main and read the error."
  fi
elif [ "$own" -eq 0 ]; then
  say "this branch is behind main and has uncommitted changes. Commit them, then git rebase origin/main."
else
  say "this branch is behind main by $behind commit(s). Rebase onto it (git rebase origin/main) before opening a PR."
fi
exit 0
