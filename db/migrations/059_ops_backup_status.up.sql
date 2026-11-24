-- 059_ops_backup_status.up.sql
-- One row recording when the weekly database backup last succeeded.
--
-- Why it lives in the database: GitHub switches off scheduled workflows after
-- about 60 days without repository activity, and a switched-off workflow can't
-- report that it stopped. So the backup workflow (.github/workflows/db-backup.yml)
-- writes the time here as its last step, and the server, which is always running,
-- reads it once a day and posts to Discord if it is more than 8 days old.
--
-- The table is deliberately a single row: `id` is a boolean that must be true, so
-- a second row is impossible and the workflow's write is a plain upsert. With no
-- row (nothing has been backed up since this shipped) the server stays quiet.
--
-- Nothing in the game reads or writes this table; the down migration just drops
-- it, and the next backup run would find no table and fail loudly.

CREATE TABLE ops_backup_status (
  id              BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (id),
  last_success_at TIMESTAMPTZ NOT NULL
);
