-- Make every game-scoped foreign key cascade on delete, so that
-- `DELETE FROM games WHERE id = $1` removes the game and all of its data in one
-- statement. Without this, deleting a game errors on FK violations from these
-- child tables; the old dev-only wipe only worked because TRUNCATE ... CASCADE
-- ignores per-FK delete rules.
--
-- This encodes "a game's rows belong to the game" at the schema level, which the
-- new per-game delete endpoint relies on and which the eventual production
-- game-deletion path will reuse.
--
-- Only direct REFERENCES games(id) constraints are listed. Grandchild tables
-- (war_*, scene_entries' own children, simultaneous_reveal entries, dice_roll
-- stages, etc.) already cascade off their parents, so they're covered
-- transitively once these parents cascade. `scenes` already had ON DELETE
-- CASCADE and is intentionally omitted.

ALTER TABLE assets DROP CONSTRAINT assets_game_id_fkey,
  ADD CONSTRAINT assets_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE banked_dice DROP CONSTRAINT banked_dice_game_id_fkey,
  ADD CONSTRAINT banked_dice_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_game_id_fkey,
  ADD CONSTRAINT dice_rolls_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE laws DROP CONSTRAINT laws_game_id_fkey,
  ADD CONSTRAINT laws_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_game_id_fkey,
  ADD CONSTRAINT pending_counter_demands_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE plan_tokens DROP CONSTRAINT plan_tokens_game_id_fkey,
  ADD CONSTRAINT plan_tokens_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE plans DROP CONSTRAINT plans_game_id_fkey,
  ADD CONSTRAINT plans_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE player_cards DROP CONSTRAINT player_cards_game_id_fkey,
  ADD CONSTRAINT player_cards_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE players DROP CONSTRAINT players_game_id_fkey,
  ADD CONSTRAINT players_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE prologue_choices DROP CONSTRAINT prologue_choices_game_id_fkey,
  ADD CONSTRAINT prologue_choices_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE prologue_committed_hearts DROP CONSTRAINT prologue_committed_hearts_game_id_fkey,
  ADD CONSTRAINT prologue_committed_hearts_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE prologue_extra_peers DROP CONSTRAINT prologue_extra_peers_game_id_fkey,
  ADD CONSTRAINT prologue_extra_peers_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE prologue_track_done DROP CONSTRAINT prologue_track_done_game_id_fkey,
  ADD CONSTRAINT prologue_track_done_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE public_record_rows DROP CONSTRAINT public_record_rows_game_id_fkey,
  ADD CONSTRAINT public_record_rows_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE rankings DROP CONSTRAINT rankings_game_id_fkey,
  ADD CONSTRAINT rankings_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE rumors DROP CONSTRAINT rumors_game_id_fkey,
  ADD CONSTRAINT rumors_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE scene_entries DROP CONSTRAINT scene_entries_game_id_fkey,
  ADD CONSTRAINT scene_entries_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE scene_posts DROP CONSTRAINT scene_posts_game_id_fkey,
  ADD CONSTRAINT scene_posts_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_game_id_fkey,
  ADD CONSTRAINT shake_up_spends_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE simultaneous_reveals DROP CONSTRAINT simultaneous_reveals_game_id_fkey,
  ADD CONSTRAINT simultaneous_reveals_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE tone_topics DROP CONSTRAINT tone_topics_game_id_fkey,
  ADD CONSTRAINT tone_topics_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
ALTER TABLE wars DROP CONSTRAINT wars_game_id_fkey,
  ADD CONSTRAINT wars_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id) ON DELETE CASCADE;
