# redesign-server-data-7560a3

The server-side groundwork for the table-page and profile-page redesign. No UI.

## What changed and why

**1. The profile list sends what a table's title needs.** `GET /api/accounts/me/tables`
(`ListMyTables`, `MyTable` in `types.ts`) now also sends `current_row`, `plan_type`,
`prologue_ranking_step`, `ending_vote_open` and `shake_up_category`. All but `plan_type`
are plain reads of the games row that was already loaded.

`plan_type` is the plan the current Main Event row is about, and it is decided **once, on the
server, in the row state**: `RowState.TitlePlanType`, set by `titlePlan`
(`handler/table_title.go`) from the same snapshot that decides the gates. The profile list
reads it from the per-table row-state computation it already ran for `waiting_on_player_ids`
(so there is no extra query), and the table page will read `rowState.title_plan_type`, which
already arrives live over the socket. There is no client-side copy of the pick to keep in step.

I first built a client twin (`titlePlanType`) tested against a shared fixture. Review found the
rule didn't match the order the server plays a row in: `ComputeRowState`'s follow-scene gate
(step 4.5) runs before "plan pending", so with two plans on a row the table is still on plan 1's
follow-scene turn while a plan-2-first rule already names plan 2. Matching it needs the row's
scenes, which the client doesn't hold as a list, so the twin couldn't be exact. Computing it where
the scenes are made the mirror, the fixture and an extra batched query unnecessary.

The pick: the plan being resolved (a Host Festivity paused under the duel it spawned gives way
to the duel, the existing rule); else, while a resolved plan's follow-scene turn is being played,
that plan; else the next plan waiting on the row; else the last one resolved there.

Two fields beyond the three the brief named: `ending_vote_open` (the end-of-game vote is a
state the title has to cover, and it is only on the games row) and `shake_up_category`.

**2. One title formatter.** `frontend/src/lib/tableTitle.ts`: `tableTitle(t)` takes fields
shaped like `Game` and `MyTable`, so either passes straight in. It is wired into nothing; the
profile and shell pieces do that. The state-to-title table is in the PR and in
`docs/work/questions/table-title-wording.md`. **For those pieces:** the title bar gets the plan
from `rowState.title_plan_type`, not from a client-side pick over `plans`.

**3. Laws and Rumors "last seen" (migration 058, a one-way door).** Chat compares a marker
to `scene_posts.id`, which works because posts are never edited. A law or rumor is edited
(`PATCH`), and the owner wants an edit to count as new, so each carries a `rev` drawn from
one sequence on creation and again on every edit. A player's marker for each list is the
highest `rev` they have seen; new means `rev` above it.

- `PUT /api/tables/{id}/seen/laws` and `/seen/rumors`, body `{"seen_rev": N}`: monotonic,
  clamped to the game's newest `rev`, private, no broadcast. Mirrors `read-marker`.
- The viewer's markers come back as `seen_rev` on `GET .../laws` and `.../rumors`, and as
  `laws_rumors_seen: {laws, rumors}` on `GET .../state`. `Law` and `Rumor` carry `rev`.
- `frontend/src/lib/lawsRumorsSeen.ts`: `isNewEntry`, `hasNewEntries` (the tab's dot),
  `newEntryCount`, `seenThrough` (what to report once the list has been on screen).
- **Your own writing isn't new to you** (an interpretation, not a ruling:
  `docs/work/questions/own-writes-not-new.md`). Each create and edit statement takes the writer's
  id (`writer_id` / `editor_id`) and, in the same statement, moves that player's marker to the
  entry if nothing else unseen sits behind it. It is part of the insert or update (a
  data-modifying CTE), not a follow-up statement: the Prologue and some plan paths run these
  inside a transaction, where a separately failing "best effort" statement would abort the
  whole transaction and undo a player's turn. It selects from `players`, so an id that is not a
  seat here inserts nothing and cannot trip a foreign key. The id is never stored.
  Writers: Propose Decree, the preparer only (the signatory only writes the rider, and the
  amendment chain records skips like rewrites); Spread Rumors, the rumor's author; Host
  Festivity, whoever typed the text (the route's caller, which is not always the guest the effect
  lands on); the Prologue, the claimer; edits, the editor.
- The migration backfills every existing seat to the newest `rev` in its game, so nothing
  lights up on deploy.

**Hidden sources.** The trap: raw `players` rows are served to every seat (`GET /state`),
so a marker column there would be public, and a hidden-source rumor's author is the one seat
whose marker moves when they write or edit it. The markers therefore live in their own
table, `player_laws_rumors_seen`, which only the viewer's own row is ever read from; no
editor column was added to `rumors` (those rows are served whole to everyone). Two tests
hold this: an edit to a hidden-source rumor changes exactly `text` and `rev` on the wire,
and a schema tripwire fails if `rumors`, `laws` or `players` gain a column.

## How it was verified

Against this worktree's code:

- `make check-fast`: pass (vet, golangci-lint, deadcode, Go unit, svelte-check, Vitest, hook
  tests, tracker check).
- `make test-integration` on a private database (`uneasy_claude_redesign_server_data_7560a3`,
  never `uneasy_test`): pass.
- Each new test was shown to bite by breaking the code under it. Removing the writer from each of
  Propose Decree, Spread Rumors (make, hidden source, mar), Host Festivity (make, mar about the
  guest, a host's mar settled by another player) and the Prologue fails that path's test; naming the
  signatory as Propose Decree's writer, or the acting guest as Festivity's, fails it too; making the
  catch-up a free-standing insert fails the transaction test; skipping the follow-scene gate fails
  both the unit and the integration test; and the earlier ones (the rev draw on a rumor edit, the
  clamp, `>` to `>=`, the vote-over-plan precedence) fail theirs.
- The migration was run down, then up again over a database holding laws and rumors in two
  games: revs follow creation order, the sequence continues past them, existing seats are
  backfilled per game.

Not run: Playwright e2e (no UI changed) and anything against `:8080`/`:8090`, which serve
`main`. Nothing was looked at in a browser; there is no screen to look at.

## Behaviour changes to look at before merging

- **Migration 058 is a one-way door** in practice. The down script exists and was run, but
  it drops every player's markers and both `rev` columns, and nothing older reads them.
  Rolling the app back without rolling the migration back is safe; the reverse is not.
- `Law` and `Rumor` JSON gain `rev`; `GET .../laws`, `.../rumors` gain `seen_rev`;
  `GET .../state` gains `laws_rumors_seen`; `RowState` gains `title_plan_type` (so
  `row_state.changed` events do). All additive. `ListMyTables` gains five fields,
  `plan_type` and `prologue_ranking_step` and `shake_up_category` as explicit nulls.
- `GET /state` does one more read (the viewer's two markers) inside the existing fan-out.
  `ListMyTables` does no new read for titles.
- `CreateLaw`, `CreateRumor`, `UpdateLawText` and `UpdateRumorText` are now one statement each
  with a CTE, and take one more parameter (`WriterID` / `EditorID`, optional).

## Open questions for the owner

- `docs/work/questions/table-title-wording.md`: the titles outside the four ruled; whether a row
  with a scene reads differently from a row with nothing yet (proposed: no); and what a row keeps
  naming once its plans are done.
- `docs/work/questions/own-writes-not-new.md`: whether your own writing counts as new to you
  (proposed: no, as in chat) and who counts as the writer of a decree.

## Noticed, not fixed

- The `rumor.edited` and `law.edited` log lines name the editor (`handler/laws_rumors.go`), so
  for a hidden-source rumor the log names its author. That matches the ruling that hide-source
  hides the spreader from characters, never from players
  (`handler/plan_spread_rumors_routes.go`), so I left it. Worth a glance if the owner meant the
  edit line to stay anonymous like `rumor.created`.
- `make deadcode` prints `middleware/auth.go: unreachable func: AccountContext` (only integration
  tests call it). It is a report, not a failure, and was there before this piece.
- `svelte-check` warns about an unused `.type-note` selector in `HelpContent.svelte`. Also
  pre-existing.
- `laws.is_active` and `rumors.is_active` are never set false anywhere, but the queries and my new
  ones all filter on them. Harmless; if they are ever used, the markers already agree with the lists.
