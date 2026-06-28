-- 037_monarch_line_of_succession.up.sql
-- ADR-007 Phase A: durable monarch role + line of succession (schema + lookup).
--
-- marginalia.title holds a stable title id (e.g. 'monarch', 'true_heir') when a
-- marginalia embodies a claimed throne-line title; NULL for an ordinary note.
-- The role lives ON the marginalia, so losing the title and tearing its
-- marginalia become the same event. The id is canonical and immutable after
-- claim — it must NOT be settable through the marginalia text-update path (a
-- later phase stamps it at claim time and rejects edits).
ALTER TABLE marginalia
  ADD COLUMN title TEXT;

-- games.throne_established trips true the first time a 'monarch' title is
-- claimed (Prologue or Shake Up "Claim a new title") and never flips back. It
-- is STORED, not inferred from a marginalia row, so a later destroy can't erase
-- the fact that the throne ever existed. currentMonarch returns "no monarch"
-- while this is false (a lone heir is a powerless pretender).
ALTER TABLE games
  ADD COLUMN throne_established BOOLEAN NOT NULL DEFAULT FALSE;
