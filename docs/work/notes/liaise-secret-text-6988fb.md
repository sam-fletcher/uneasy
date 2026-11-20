# Clandestinely Liaise: each player writes the meeting in Secrets We Keep

## What changed and why
Secrets We Keep was the one place the game's secrecy is authored, and it was a
stub: the step asked only for an asset, and the server wrote
"Clandestine meeting with <name> — <prep note>". The name was the *holder's own*
(so every secret said its holder met themself), and the note is public (it is in
the plan card and the log), so the underside held nothing the table didn't know.
The book (p.55) has each player write the meeting on the underside of their own asset.

- **Server** (`handler/plan_clandestinely_liaise_phases.go`). `keep-secret` takes
  `{asset_id, text}`. The stored secret is `clSecretHeader` + " — " + the player's
  text (trimmed, otherwise verbatim), authored by the seat (so a Make Demands winner
  typing for the preparer's seat writes the *preparer's* secret, as before). The header
  names who the holder met: `Met bob's Ser Ott`, the OTHER player and the peer they
  brought, with the peer's name as it is at the time of writing. If the peer can't be
  found (never recorded, or gone from the game) it falls back to the player alone,
  `Met bob`. A destroyed peer is still found. `text` present but blank, or over the
  narrative limit, is a 400 before anything is stored, so the seat stays open. `text`
  absent (a tab from before the box) gets the prep note in the words' place, under the
  same header, so both paths read alike.
- **Client.** `ClandestinelyLiaisePanel.svelte` has a "The meeting:" box under the
  asset picker. **Both boxes start empty.** The preparer's seat sees "Your preparation
  note: “…”" just above theirs as a reminder (worded with the preparer's name for a
  Make Demands stand-in); the partner wrote no note and sees none. Under the box, a hint
  promises the header ("“Met bob's Ser Ott —” goes in front of what you write…") and a
  privacy line says who else can read it. The header text is previewed by
  `lib/plans/liaiseSecret.ts`, which mirrors `clSecretHeader`; the server is
  authoritative. "Keep this secret" needs an asset *and* words. The box starts over when
  the seat or plan changes, keyed on a derived string, so a plan refetch can't wipe what
  is being typed.
- **The text is never in a draft.** `keepText` is local component state and goes into
  the keep-secret request and nowhere else. Nothing about it is added to `emitPrepDraft`,
  `ws.ts` or any broadcast; the existing events (`secret.created`,
  `liaise.keep_secret_submitted`, the log line) carry no text, and the tests pin that.
  This is the 2026-11-16 Architecture rule, applied to the field this piece adds.
- **No stored-state change.** The words live in the existing `secrets` table;
  `kept_secrets` entries and `resolution_data` are unchanged. No migration.

### The two rulings (owner, via the coordinator)
- **The stored secret names who the meeting was with.** A secret outlives the meeting:
  whoever later looks under, takes or breaks the asset reads it, and without the name the
  words can be meaningless. The header names the partner's meeting *peer* (the character)
  with its player, not just the player. Who met whom is already public from the
  preparation, so it reveals nothing new, and the player is told it is added so they
  don't type the name twice. The old-tab path uses the same header, so both read alike.
- **The preparer's box starts empty.** The preparation note is public, so prefilling it
  would let one tap keep a "secret" the whole table has already read, the flaw this piece
  exists to fix. The note is shown above the box as a reminder instead. My first version
  prefilled it and marked it a sketch; that was my call, and it is gone.

## How it was verified
Against this branch's code, rebased on `main` after #106, #108 and #109, on a private
database (dropped) and a private server on :8173 (stopped):
- `make check-fast`: exit 0.
- `make test-integration` (full suite, private DB): all packages ok.
- Integration tests, `handler/plan_clandestinely_liaise_secret_text_integration_test.go`:
  each player's text lands on their own asset under their own seat, behind the header;
  an old tab gets the prep note under the same header; the peer is named as it is now
  (renamed since prep) and a destroyed peer is still named; a missing meeting peer
  (never recorded; recorded but not in the game) falls back to the player's name, on both
  the written and old-tab paths; an in-flight liaison mixing one old-tab seat and one
  new-tab seat finishes; blank, whitespace and over-long text are refused and write
  nothing; a Make Demands stand-in's words are stored behind the preparer's header,
  authored by the preparer's seat, readable by the preparer and not by the winner who
  typed them; and the text reaches no bystander's socket, the partner's socket, any
  player's GET plan, the log, or the other seat.
- **Bit:** against the old handler four of these failed. Mutations of the new code, each
  caught and restored: naming the holder (six tests fail), dropping the fallback, skipping
  destroyed peers, authoring by the caller instead of the seat, and posting the text to
  the log (the no-leak test passes on the old handler only because it dropped the text).
- e2e (`clandestinely_liaise_secret_text.spec.ts`, three tests, plus the nine existing
  Liaise specs, updated to type the meeting): 12 passed at 375px as alice, bob and carol.
  The spec listens to every socket from its first frame, so it catches a draft as well as
  a broadcast. **Bit:** a mutation dispatching the typed text as a `prepare_plan.draft`
  fails it (bob's socket receives the secret); a mutation making the reset effect depend
  on the plan object fails the refetch test (the box is wiped to ""). Against `main` the
  new specs fail.
- Vitest: `liaiseSecret.test.ts` pins the header's shape.
- 375px screenshots as the preparer, the partner and a bystander, and the stored secret
  read back by its holder in her retinue (in the PR); tap targets measured in the DOM:
  asset cards 46px, box 114px, "Keep this secret" 44px.

Not run against this code: a 4-player table in the browser (the Make Demands stand-in
path is covered by the integration test and the existing e2e, but only three players
were driven in the browser); a real phone.

## Behaviour changes to look at before merging
- **Secrets We Keep now has a text box,** required, empty for both players.
- **The underside is the header, then what the player wrote** ("Met bob's Ser Ott — …").
- **Secrets already written by the old code keep the old line and the wrong name.**
  Nothing rewrites them (it can't tell which are stubs, and old text shouldn't be edited
  under a player). Only liaisons that keep a secret after the deploy get the new behaviour.
- **A tab left open across the deploy** has no box. Its keep-secret still works and
  writes the prep note under the header, so a liaison mid-phase can finish.

## Open questions for the owner
- `docs/work/questions/liaise-stand-in-secret-text.md`: when a Make Demands winner holds
  the preparer's seat they now write words on the preparer's asset, authored by the
  preparer's seat, and cannot read them back. Should they write the secret at all?
- `docs/work/questions/liaise-secret-blank-text.md`: required, or may it be left blank?
  I built required (it matches every other place the app asks for a secret). Not
  mentioned in the latest ruling, so it stays until the coordinator closes it.

## Noticed, not fixed
- The relay (`hub.go`, `CmdPreparePlanDraft`) doesn't check the sender is the focus
  player or that a prep step is open ("we don't validate"). `ctx.emitPrepDraft` guards
  on both client-side, which is why the Liaise box can't reach it through that call;
  a component dispatching the window event directly *can*. The box is therefore kept
  out of drafts by construction and by the e2e, not by a server backstop: a Liaise
  prep draft legitimately carries its public `notes`, so the hub can't strip by plan
  type the way it does for Spread Rumors.
- `clKeepSecretHandler` still carries the old TODO about destroyed assets (the UI
  hides them; the server doesn't refuse).
- The private RULINGS file's Liaise section doesn't yet say that both players write,
  that the box is required and starts empty, or that the underside carries the header;
  that is the coordinator's to record.
