-- 058_laws_rumors_seen.up.sql
-- The redesigned table page marks Laws and Rumors "new" with a pale dot, on
-- the tab and on each entry the player hasn't looked at yet, and an edit counts
-- as new. That needs the same thing chat has (042): a per-player marker held on
-- the server so it stays right across devices. Two halves:
--
--   1. A revision number on every law and rumor. Chat can compare a marker to
--      scene_posts.id because a post is never edited. A law or rumor is edited
--      (PATCH /laws/{id}), and a created_at comparison would miss the edit, so
--      each entry carries `rev`, drawn from one sequence on creation and drawn
--      again on every edit. "Newer than your marker" then means "created or
--      edited since you looked", with the same monotonic-integer shape as
--      players.last_read_post_id. An integer rather than updated_at because the
--      client compares markers, and a JS Date holds milliseconds where Postgres
--      holds microseconds. Like chat's ids, revs are allocated in call order, not
--      commit order, so a marker can in principle pass a lower rev that commits
--      a moment later; that entry would then never show as new. The window is a
--      few milliseconds wide and the cost is one missing dot.
--
--   2. The markers themselves, in their OWN table rather than as columns on
--      players. Raw players rows are served to every player in the game (GET
--      /state, GET /tables/{id}), so a marker column there would be public. For
--      chat that is harmless; for this it is not: a hidden-source rumor can be
--      edited by its hidden author, and the editor's marker is the one thing
--      the server moves when they do, so a public marker would name them.
--      Nothing selects this table into a response except the viewer's own row.
--
-- One-way door: the new table is never read by anything older than this
-- migration, and the down migration drops it and both rev columns, losing every
-- player's markers. Dropping them only makes everything look new once.

CREATE SEQUENCE laws_rumors_rev_seq;

-- Existing rows are numbered in creation order, so the markers backfilled below
-- mean something. A nullable add, the backfill, then NOT NULL and the default:
-- ADD COLUMN ... DEFAULT nextval() would number existing rows in physical
-- order. The two lists are numbered separately (each has its own marker, so
-- they never compare across), and the sequence is then moved past the larger so
-- every later insert and edit gets a number above all of them.
ALTER TABLE laws ADD COLUMN rev BIGINT;
UPDATE laws SET rev = n.rn
FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn FROM laws) n
WHERE laws.id = n.id;

ALTER TABLE rumors ADD COLUMN rev BIGINT;
UPDATE rumors SET rev = n.rn
FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn FROM rumors) n
WHERE rumors.id = n.id;

SELECT setval('laws_rumors_rev_seq', GREATEST(m, 1), m > 0)
FROM (SELECT GREATEST(
        COALESCE((SELECT max(rev) FROM laws), 0),
        COALESCE((SELECT max(rev) FROM rumors), 0)) AS m) x;

ALTER TABLE laws ALTER COLUMN rev SET NOT NULL;
ALTER TABLE laws ALTER COLUMN rev SET DEFAULT nextval('laws_rumors_rev_seq');
ALTER TABLE rumors ALTER COLUMN rev SET NOT NULL;
ALTER TABLE rumors ALTER COLUMN rev SET DEFAULT nextval('laws_rumors_rev_seq');

-- Laws and rumors are listed per game, newest-revision checks included.
CREATE INDEX laws_game_rev_idx ON laws (game_id, rev);
CREATE INDEX rumors_game_rev_idx ON rumors (game_id, rev);

CREATE TABLE player_laws_rumors_seen (
  player_id        BIGINT PRIMARY KEY REFERENCES players(id) ON DELETE CASCADE,
  laws_seen_rev    BIGINT NOT NULL DEFAULT 0,
  rumors_seen_rev  BIGINT NOT NULL DEFAULT 0
);

-- Everyone already at a table has, as far as anyone can tell, seen what is
-- there: without this every Laws and Rumors tab in every game in progress would
-- light up on deploy. A player with no row reads as 0 (everything new), which
-- is right for a seat that joins after laws exist.
INSERT INTO player_laws_rumors_seen (player_id, laws_seen_rev, rumors_seen_rev)
SELECT p.id,
       COALESCE((SELECT max(l.rev) FROM laws l WHERE l.game_id = p.game_id), 0),
       COALESCE((SELECT max(r.rev) FROM rumors r WHERE r.game_id = p.game_id), 0)
FROM players p;
