# Host Festivity: the host card keeps its draft, and End meets the roll lock

Two small Host Festivity bugs, from the items `festivity-host-card-gaps` and `festivity-end-event-roll-lock`
(both deleted by this PR).

## What changed and why

- **The host's drafts survive a duel.** The pending-Mar card (`HostPendingMar`) and the Make picker
  (`HostChoosing`) are taken off the page while a duel is pending, and their picks were component state, so a
  half-written rumor or a half-made pick went with them. It was worse than the item said: an *accepted* challenge
  becomes a nested Propose Duel, and the play area then swaps the whole festivity panel for the duel, so even a
  draft held in `HostFestivityPanel` would be lost. The draft now lives in `lib/festivityDraft.svelte.ts`, keyed by
  plan (the mar draft also by the insisting guest), the same shape as `rollDraft.svelte.ts`. A successful send
  clears it. Rejected: keeping the cards mounted through the challenge (it would not cover the nested duel, and the
  table is meant to read as paused); lifting the state into `HostFestivityPanel` (same hole).
- **A stand-in sees the host named.** The panel passes `amHost` to both cards. For a Make Demands perform-steps
  winner the Mar card reads "alice's Mar", "carol insists alice takes a Mar", "You choose for alice", and the Mar
  list itself is worded about the host (`standInMarOpts` in `options.ts`: "A rumor spreads about alice", "alice must
  accept all duels", "alice breaks themselves"), as are the rumor label and the empty-state lines. The Make picker's
  header names the host too. The host's own wording is unchanged. The list is printed to the host in the second
  person (book p.61); a stand-in answers it for them, so "you" would name the wrong person.
- **The host's name is drawn by `PlayerName`.** Wherever it sits in template text (the Mar heading, "insists
  alice takes a Mar", "You choose for alice", the rumor label, the accept-duels and nothing-to-tear notes, and the
  Make picker's header) it is `<PlayerName>` in the host's colour, through small snippets so the spaces around it
  survive. Names inside a string prop stay plain, because a string can't hold a component (see Noticed, not fixed).
- **Stale picks are dropped.** The draft now outlives a duel, which can change the board under it, so both cards
  prune a pick that no longer matches their lists when it changes: a peer taken or lost, a torn marginalia, an
  option the host can no longer carry out. A pick that is still valid stays. Without it the server would refuse
  the send ("no longer available") and the host would have to re-pick, so it is a convenience, not a safety net.
- **End the event mid-roll answers with the roll lock's message.** `hfEndEventHandler` now calls
  `hfBlockOnActiveRoll` before the endable check, like the other host actions. The refusal itself was always right.

## How it was verified

- `make check-fast` and `make test-integration` (private database) pass.
- Each fix shown failing without it: the End test (`TestHostFestivity_EndEventMidRoll_GetsTheRollLock`) fails with
  the old handler ("still in full swing"); the new e2e spec (host's Make picker and pending Mar through a guest's
  duel challenge) fails on the old components (the textbox is gone after the challenge); the four stand-in/host
  wording tests in `planPanelsC.test.ts` fail on the old components. `festivityDraft.test.ts` pins the store.
- The whole `host_festivity.spec.ts` ran against this branch's own server: 11 passed.
- 375px screenshots (in the PR): the host's Mar card and Make picker before and after a challenge, and a
  perform-steps stand-in's Mar card and Make picker.
- A render test pins that the stand-in's heading and sentences carry the host's `PlayerName`, and fails on the
  plain-text version; another pins that `standInMarOpts` has the same keys, in the same order, as `MAR_OPTS`.
- Not run: the other e2e specs; the *accepted*-challenge path (nested duel) in a browser, which uses the same store
  (only the declined path is pinned by a test); the stale-pick pruning, which is a client effect a server render
  never runs and no test covers.

## Behaviour changes to look at before merging

- A stand-in's Mar list is third-person ("alice breaks themselves"). That wording is mine, not the book's.
- The host's half-made pick now survives a page-level swap to a nested duel, within one tab session. A page reload
  still loses it (as before).

## Open questions for the owner

None.

## Noticed, not fixed

- Host names inside string props are still plain text: the stand-in's Mar list (`standInMarOpts` titles and
  descriptions, which are `OptionCard` props), the `emptyMessage` of the two `CardPicker`s, and the "alice has no
  peers" lock reason. They belong to the `inline-text-names-in-strings` item.
- The e2e helper for a stand-in (`winPerformSteps`) is copied between specs; there is no Host Festivity stand-in
  e2e spec. The stand-in wording is pinned by server-render tests only, because the panel learns who the stand-in is
  from an effect a server render never runs.
