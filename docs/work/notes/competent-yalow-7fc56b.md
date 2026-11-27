# Migration number guard

## What changed and why

Two open PRs (#143 render crash reports, #146 peace treaties) each added a
`060_*` migration. Each passed CI on its own. Migrations apply at server
start (embedded golang-migrate) and `main` auto-deploys, so two 060s would
have stopped production booting. golang-migrate also only moves forward, so
a 061 deployed before a 060 exists silently skips the 060.

`db/migrations_test.go` (package `db`, beside the `//go:embed` in
`db/migrate.go`) fails `make check-fast` when:

- two files share a version (two `.up.sql`, or two `.down.sql`). The message
  names the version and the clashing files, and says the PR merged second
  must renumber, rebase onto main and merge after the PR it clashes with;
- a version has no `.up.sql` or no `.down.sql`;
- an up and down of one version have different names (so they aren't one
  migration);
- a file in the directory doesn't parse as `NNN_name.{up,down}.sql`;
- the versions aren't a gap-free run from 001 (or include 000).

The checker is `migrationProblems`, a pure function over file names, so
`TestMigrationProblems` can cover each case with synthetic names.
`TestMigrationFilesAreUniqueAndGapFree` runs it over the embedded directory.

No runtime code changed.

## How it was verified

- `go test -count=1 -run TestMigration -v ./db/` on this branch (which matches
  `origin/main`): all pass. The 59 embedded versions are each one up and one down.
- Scratch duplicate: added `060_scratch_first.{up,down}.sql` and
  `060_scratch_second.{up,down}.sql` to `db/migrations/`, ran
  `TestMigrationFilesAreUniqueAndGapFree`, which failed with one clash message
  per direction naming both files. The scratch files were removed; `git status`
  shows only the new test file and this note.
- `make check-fast`: exit 0 (the run that also covers vet, golangci-lint, deadcode,
  the full Go suite, frontend check and unit tests, and the tracker check).
- **Not run:** `make test-integration`. The change adds no database code and
  nothing touches a database, so I didn't set up a private database for it.

## Behaviour changes to look at before merging

- Any open PR whose migration number is already taken on `main` will fail
  this test on its next CI run after it is rebased. That is the intent. Right
  now #143 and #146 both claim 060: whichever merges second has to renumber.
- Renumbering only works while the migration is still unmerged. Once a
  migration is on `main`, it must not be renamed.

## Open questions for the owner

- None.

## Noticed, not fixed

- Nothing outside scope.
