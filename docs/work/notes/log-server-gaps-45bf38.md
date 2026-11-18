# Small log and server gaps (log-server-gaps-45bf38)

Item: `docs/work/next/log-and-server-gaps.md`. Every bullet was re-checked against the code
first; all of them were still current (none was already fixed). Three are left in the item
file, each with its own owner question.

## What changed and why

One commit per area.

**Server**

- **Password reset.** The "is it used?" read and the `used_at` write were separate, with a
  bcrypt hash between them, so simultaneous redemptions all passed. `MarkPasswordResetTokenUsed`
  now has `AND used_at IS NULL` and returns rows affected (`:execrows`, regenerated with
  `make sqlc`); it is the first statement of the redemption transaction, and zero rows rolls
  everything back to the same uniform "link invalid or expired" 400. No migration.
- **Feedback log.** With `DISCORD_WEBHOOK_URL` unset, `NotifyDiscord` logged the whole message
  (body and contact) at Info. It still does so when the server runs with `UNEASY_DEV=1`; otherwise
  it logs that a submission arrived (kind, username, game, route) and points at the
  `feedback_submissions` row.
- **`LoadResolutionData`.** About 140 callers, so the signature stays. `game.ParseResolutionData`
  returns the error, and separates two cases that are not alike:
  - **Not valid JSON** (`ErrResolutionDataMalformed`): nothing decoded. Every plan route (all go
    through `requirePlanAccess`, including `GET /api/plans/:id`) refuses it with a 500 naming the
    plan, before any side effect, because several routes act first and save last.
    `saveResolutionData` (the only write path) refuses a value decoded from one, as the backstop
    for writers that don't enter through `requirePlanAccess`.
  - **A field in an older shape** (`ErrResolutionDataMistyped`): saved fields have changed JSON type
    before (`make_mar_choices` was a list of strings, Spread Propaganda's `give_peer_done` a bool,
    a Make Introductions draft's `marginalia` a list). `encoding/json` skips just that field and
    decodes the rest, and the old code lived with that, so a plan resolved under an old shape must
    still load. It is not refused and not marked; the rest of the blob reads, and a later save
    writes it back in the current shape.

  Logging: a blob that isn't JSON is an Error and an older shape is a Warn. Both name the plan
  (`plan_id`, `game_id`, `plan_type`) on the paths that run again and again (the Waiting On
  computation's `ResolvingWaitees` and `reaimWaitees`, the Make Demands choosers, the esteem-lockout
  check, `requirePlanAccess`) through `loadPlanResolutionData(plan)`. The plain
  `loadResolutionData(raw)` has only the column, so it logs just the not-JSON case and can't name a
  plan; that case can't reach it through a route. Read-only callers outside the routes degrade to
  what decoded, so one bad plan doesn't take the table page down. The four typed `LoadXData`
  readers stay lenient.
- **Discarded save errors outside the plan routes** are now logged with the plan id: the Liaise
  redelay outcome (`reveals.go`), the Make Demands counter-demand marker, and the follow-up
  meeting's state.

**Action log**

- **Chronicle Histories.** `cast-roll` posts one line for the pre-roll's invoked artifacts, in
  the voice of the existing in-scene line ("X invoked A and B."); nothing invoked, no line.
- **Make Introductions.** When another player writes a newcomer, the generic "created" line now
  credits the writer, and the plan's own line says only where they landed ("... joined X's
  retinue.") instead of repeating the act as "defined the newcomer". The asset row still records
  the introducer as creator; `materializeDraftPeer` takes the credited player as an explicit
  argument and `EmitAssetCreatedBy` names them.
- **Liaise.** A redelay that overflows row 13 now says why ("... : the chosen delay landed past
  row 13.", the same words the fell-through plan post uses). The face-0 line is unchanged.
- **Spread Rumors.** The "consented" line is posted from inside `srCommitTakeConsent`, after the
  assets have changed hands and before the "took" lines. An existing test pins consent reading
  before the take, so the transfer is split into its effect (`takeRumorAsset`) and its post
  (`announceRumorTake`); `takeRumorAsset` is on the log-coverage allow-list with that reason. If a
  take of several assets fails part-way, the assets that did move are announced (consent first)
  before the error is returned: they were already broadcast as taken, and a retry lapses through
  `srStaleTakeAssets`, so nothing else would ever log them. Nothing moved, nothing posted.

**Client**

- **`SimultaneousRevealInput`.** The `GET /api/reveals/null` call comes from a submit in flight
  outliving the plan's reveal id (the plan resolves, the id is cleared, then submit's own refresh
  runs). The widget now fetches only through `loadReveal`, which makes no request without an id;
  the prop is nullable and submit won't start without one.
- **`HostFestivityPanel` and `ProposeDuelPanel`** subscribe through `useWindowEvents`.

## How it was verified

Against this worktree's own code, on a private database:

- `make check-fast`, `make test-integration` and the six Playwright specs for the plans touched
  (`host_festivity`, `propose_duel`, `clandestinely_liaise`, `chronicle_histories`,
  `make_introductions`, `spread_rumors`): see the PR for the results of the latest run.
- Every new test was shown to fail with its fix removed: password reset (conditional dropped: all
  six redemptions succeed), feedback log (gate dropped), the not-JSON guards (each of the two
  removed), the older-shape tests (old shapes treated as unreadable again: the plan view 500s),
  the Liaise overflow line, Spread Rumors consent and partial-take lines, the Make Introductions
  credit, the Chronicle invocation line, the reveal guard and wiring tests, and the window-events
  guard.

Nothing was looked at in a browser: no layout changed, and the client changes are a guard and a
listener refactor. No DOM test library exists here, so those are held by a unit test of
`loadReveal` plus source-level wiring guards, not a mounted-component test.

## Behaviour changes to look at before merging

- **A `resolution_data` blob that isn't JSON now makes plan routes 500** (including
  `GET /api/plans/:id`) instead of reading as an empty plan. It should never happen. A plan saved
  in an older field shape is unaffected, apart from one Warn line per read with its plan id.
- **Production without `DISCORD_WEBHOOK_URL`** logs less: the body and contact are gone from
  the log.
- **Spread Rumors:** the order of a consented take is now: the rumor's own lines, then
  "consented", then "took". The rumor lines used to come after the consent line.
- **Make Introductions** for the two other-writer outcomes: one "created the peer ..." line
  crediting the writer, then "X joined Y's retinue." The "defined the newcomer" line is gone.

## Open questions for the owner

- `docs/work/questions/chronicle-mar-announcement.md`: should a Chronicle mar be announced when
  the dice land, and should each player's mar pick carry narration?
- `docs/work/questions/festivity-duel-type-and-place.md`: who names arms or wits, and the
  location, for a duel challenged at a Host Festival?
- `docs/work/questions/push-body-names-the-table.md`: what should a reminder name so a player at
  several tables can tell them apart? (I first built the join code, as an early plan had it, and
  dropped it: the profile page later dropped the code, and a lock screen is public.)

## Noticed, not fixed

- `srCommitTakeConsent` isn't atomic: `ApplyChoice` creates the rumor and posts its lines before
  the transfers, so a transfer that fails leaves the rumor behind with a 500 and the request
  still pending. The fix is a transaction over the whole commit, which the helpers beneath it
  don't support yet.
- Other writers swallow errors this change didn't touch: the Liaise redelay outcome and the
  counter-demand marker now log, but they still carry on after a failed save (the redelay
  schedules and logs the next meeting before it saves the liaison's state). `finalizeRoll`
  (`rolls_dice.go`) does return its error to its callers; I found no discarded save there.
- A Make Introductions delayed arrival also reads twice ("created the peer ..." then "X finally
  arrived at court."), and a broken journey posts the created line and the journey line. Neither
  was in the bullet.
- A Liaise redelay past row 13 simply schedules nothing. The Explosive Finale's collapse onto
  row 13 only applies to a plan's own delay reveal, not this one. If that's meant to apply,
  it's a rules question.
- The Prologue, Shake-Up and lobby views also subscribe to `uneasy:*` events by hand.
