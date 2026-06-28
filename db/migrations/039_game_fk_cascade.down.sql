-- Restore the game-scoped foreign keys to NO ACTION (the default), reversing
-- 039_game_fk_cascade.up.sql.

ALTER TABLE assets DROP CONSTRAINT assets_game_id_fkey,
  ADD CONSTRAINT assets_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE banked_dice DROP CONSTRAINT banked_dice_game_id_fkey,
  ADD CONSTRAINT banked_dice_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_game_id_fkey,
  ADD CONSTRAINT dice_rolls_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE laws DROP CONSTRAINT laws_game_id_fkey,
  ADD CONSTRAINT laws_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_game_id_fkey,
  ADD CONSTRAINT pending_counter_demands_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE plan_tokens DROP CONSTRAINT plan_tokens_game_id_fkey,
  ADD CONSTRAINT plan_tokens_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE plans DROP CONSTRAINT plans_game_id_fkey,
  ADD CONSTRAINT plans_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE player_cards DROP CONSTRAINT player_cards_game_id_fkey,
  ADD CONSTRAINT player_cards_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE players DROP CONSTRAINT players_game_id_fkey,
  ADD CONSTRAINT players_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE prologue_choices DROP CONSTRAINT prologue_choices_game_id_fkey,
  ADD CONSTRAINT prologue_choices_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE prologue_committed_hearts DROP CONSTRAINT prologue_committed_hearts_game_id_fkey,
  ADD CONSTRAINT prologue_committed_hearts_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE prologue_extra_peers DROP CONSTRAINT prologue_extra_peers_game_id_fkey,
  ADD CONSTRAINT prologue_extra_peers_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE prologue_track_done DROP CONSTRAINT prologue_track_done_game_id_fkey,
  ADD CONSTRAINT prologue_track_done_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE public_record_rows DROP CONSTRAINT public_record_rows_game_id_fkey,
  ADD CONSTRAINT public_record_rows_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE rankings DROP CONSTRAINT rankings_game_id_fkey,
  ADD CONSTRAINT rankings_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE rumors DROP CONSTRAINT rumors_game_id_fkey,
  ADD CONSTRAINT rumors_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE scene_entries DROP CONSTRAINT scene_entries_game_id_fkey,
  ADD CONSTRAINT scene_entries_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE scene_posts DROP CONSTRAINT scene_posts_game_id_fkey,
  ADD CONSTRAINT scene_posts_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_game_id_fkey,
  ADD CONSTRAINT shake_up_spends_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE simultaneous_reveals DROP CONSTRAINT simultaneous_reveals_game_id_fkey,
  ADD CONSTRAINT simultaneous_reveals_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE tone_topics DROP CONSTRAINT tone_topics_game_id_fkey,
  ADD CONSTRAINT tone_topics_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
ALTER TABLE wars DROP CONSTRAINT wars_game_id_fkey,
  ADD CONSTRAINT wars_game_id_fkey FOREIGN KEY (game_id) REFERENCES games(id);
