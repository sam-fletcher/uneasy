-- Make plans.row_number nullable.
--
-- Variable-delay plans (Make War, Clandestinely Liaise) create a
-- simultaneous die-face reveal at prep time and only learn their actual
-- row when the reveal closes. The FK to public_record_rows still applies;
-- under Postgres MATCH SIMPLE semantics a composite FK with a NULL column
-- is not checked, which is exactly what we want during the pending-reveal
-- window. SetPlanRowNumber writes the real row once the reveal resolves.

ALTER TABLE plans ALTER COLUMN row_number DROP NOT NULL;
