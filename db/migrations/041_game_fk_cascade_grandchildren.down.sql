-- Restore the foreign keys touched by 041_game_fk_cascade_grandchildren.up.sql
-- to NO ACTION (the default), reversing that migration.

ALTER TABLE marginalia DROP CONSTRAINT marginalia_asset_id_fkey,
  ADD CONSTRAINT marginalia_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES assets(id);
ALTER TABLE secrets DROP CONSTRAINT secrets_asset_id_fkey,
  ADD CONSTRAINT secrets_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES assets(id);
ALTER TABLE secret_visibility DROP CONSTRAINT secret_visibility_secret_id_fkey,
  ADD CONSTRAINT secret_visibility_secret_id_fkey FOREIGN KEY (secret_id) REFERENCES secrets(id);
ALTER TABLE dice_roll_dice DROP CONSTRAINT dice_roll_dice_roll_id_fkey,
  ADD CONSTRAINT dice_roll_dice_roll_id_fkey FOREIGN KEY (roll_id) REFERENCES dice_rolls(id);
ALTER TABLE difficulty_votes DROP CONSTRAINT difficulty_votes_roll_id_fkey,
  ADD CONSTRAINT difficulty_votes_roll_id_fkey FOREIGN KEY (roll_id) REFERENCES dice_rolls(id);
ALTER TABLE duel_staked_assets DROP CONSTRAINT duel_staked_assets_plan_id_fkey,
  ADD CONSTRAINT duel_staked_assets_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_plan_id_fkey,
  ADD CONSTRAINT duel_bouts_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE liaise_choices DROP CONSTRAINT liaise_choices_plan_id_fkey,
  ADD CONSTRAINT liaise_choices_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE shake_up_cost_adjustments DROP CONSTRAINT shake_up_cost_adjustments_spend_id_fkey,
  ADD CONSTRAINT shake_up_cost_adjustments_spend_id_fkey FOREIGN KEY (spend_id) REFERENCES shake_up_spends(id);
ALTER TABLE dice_roll_dice DROP CONSTRAINT dice_roll_dice_leveraged_asset_id_fkey,
  ADD CONSTRAINT dice_roll_dice_leveraged_asset_id_fkey FOREIGN KEY (leveraged_asset_id) REFERENCES assets(id);
ALTER TABLE duel_staked_assets DROP CONSTRAINT duel_staked_assets_asset_id_fkey,
  ADD CONSTRAINT duel_staked_assets_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES assets(id);
ALTER TABLE liaise_choices DROP CONSTRAINT liaise_choices_target_asset_id_fkey,
  ADD CONSTRAINT liaise_choices_target_asset_id_fkey FOREIGN KEY (target_asset_id) REFERENCES assets(id);
ALTER TABLE plans DROP CONSTRAINT plans_target_asset_id_fkey,
  ADD CONSTRAINT plans_target_asset_id_fkey FOREIGN KEY (target_asset_id) REFERENCES assets(id);
ALTER TABLE prologue_extra_peers DROP CONSTRAINT prologue_extra_peers_asset_id_fkey,
  ADD CONSTRAINT prologue_extra_peers_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES assets(id);
ALTER TABLE rumors DROP CONSTRAINT rumors_target_asset_id_fkey,
  ADD CONSTRAINT rumors_target_asset_id_fkey FOREIGN KEY (target_asset_id) REFERENCES assets(id);
ALTER TABLE scene_peers DROP CONSTRAINT scene_peers_peer_asset_id_fkey,
  ADD CONSTRAINT scene_peers_peer_asset_id_fkey FOREIGN KEY (peer_asset_id) REFERENCES assets(id);
ALTER TABLE scene_posts DROP CONSTRAINT scene_posts_speaking_as_asset_id_fkey,
  ADD CONSTRAINT scene_posts_speaking_as_asset_id_fkey FOREIGN KEY (speaking_as_asset_id) REFERENCES assets(id);
ALTER TABLE scenes DROP CONSTRAINT scenes_location_holding_id_fkey,
  ADD CONSTRAINT scenes_location_holding_id_fkey FOREIGN KEY (location_holding_id) REFERENCES assets(id);
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_target_asset_id_fkey,
  ADD CONSTRAINT shake_up_spends_target_asset_id_fkey FOREIGN KEY (target_asset_id) REFERENCES assets(id);
ALTER TABLE war_battle_costs DROP CONSTRAINT war_battle_costs_asset_id_2_fkey,
  ADD CONSTRAINT war_battle_costs_asset_id_2_fkey FOREIGN KEY (asset_id_2) REFERENCES assets(id);
ALTER TABLE war_battle_costs DROP CONSTRAINT war_battle_costs_asset_id_1_fkey,
  ADD CONSTRAINT war_battle_costs_asset_id_1_fkey FOREIGN KEY (asset_id_1) REFERENCES assets(id);
ALTER TABLE war_surrender_claims DROP CONSTRAINT war_surrender_claims_asset_id_fkey,
  ADD CONSTRAINT war_surrender_claims_asset_id_fkey FOREIGN KEY (asset_id) REFERENCES assets(id);
ALTER TABLE dice_roll_dice DROP CONSTRAINT dice_roll_dice_cancelled_by_die_id_fkey,
  ADD CONSTRAINT dice_roll_dice_cancelled_by_die_id_fkey FOREIGN KEY (cancelled_by_die_id) REFERENCES dice_roll_dice(id);
ALTER TABLE banked_dice DROP CONSTRAINT banked_dice_used_roll_id_fkey,
  ADD CONSTRAINT banked_dice_used_roll_id_fkey FOREIGN KEY (used_roll_id) REFERENCES dice_rolls(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_responder_stake_id_fkey,
  ADD CONSTRAINT duel_bouts_responder_stake_id_fkey FOREIGN KEY (responder_stake_id) REFERENCES duel_staked_assets(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_declarer_stake_id_fkey,
  ADD CONSTRAINT duel_bouts_declarer_stake_id_fkey FOREIGN KEY (declarer_stake_id) REFERENCES duel_staked_assets(id);
ALTER TABLE liaise_choices DROP CONSTRAINT liaise_choices_target_marginalia_id_fkey,
  ADD CONSTRAINT liaise_choices_target_marginalia_id_fkey FOREIGN KEY (target_marginalia_id) REFERENCES marginalia(id);
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_target_marginalia_id_fkey,
  ADD CONSTRAINT shake_up_spends_target_marginalia_id_fkey FOREIGN KEY (target_marginalia_id) REFERENCES marginalia(id);
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_plan_id_fkey,
  ADD CONSTRAINT dice_rolls_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE laws DROP CONSTRAINT laws_origin_plan_id_fkey,
  ADD CONSTRAINT laws_origin_plan_id_fkey FOREIGN KEY (origin_plan_id) REFERENCES plans(id);
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_origin_plan_id_fkey,
  ADD CONSTRAINT pending_counter_demands_origin_plan_id_fkey FOREIGN KEY (origin_plan_id) REFERENCES plans(id);
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_resolved_plan_id_fkey,
  ADD CONSTRAINT pending_counter_demands_resolved_plan_id_fkey FOREIGN KEY (resolved_plan_id) REFERENCES plans(id);
ALTER TABLE plan_tokens DROP CONSTRAINT plan_tokens_plan_id_fkey,
  ADD CONSTRAINT plan_tokens_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE plans DROP CONSTRAINT plans_targeted_plan_id_fkey,
  ADD CONSTRAINT plans_targeted_plan_id_fkey FOREIGN KEY (targeted_plan_id) REFERENCES plans(id);
ALTER TABLE rumors DROP CONSTRAINT rumors_origin_plan_id_fkey,
  ADD CONSTRAINT rumors_origin_plan_id_fkey FOREIGN KEY (origin_plan_id) REFERENCES plans(id);
ALTER TABLE scenes DROP CONSTRAINT scenes_resolved_plan_id_fkey,
  ADD CONSTRAINT scenes_resolved_plan_id_fkey FOREIGN KEY (resolved_plan_id) REFERENCES plans(id);
ALTER TABLE simultaneous_reveals DROP CONSTRAINT simultaneous_reveals_plan_id_fkey,
  ADD CONSTRAINT simultaneous_reveals_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES plans(id);
ALTER TABLE wars DROP CONSTRAINT wars_origin_plan_id_fkey,
  ADD CONSTRAINT wars_origin_plan_id_fkey FOREIGN KEY (origin_plan_id) REFERENCES plans(id);
ALTER TABLE assets DROP CONSTRAINT assets_creator_id_fkey,
  ADD CONSTRAINT assets_creator_id_fkey FOREIGN KEY (creator_id) REFERENCES players(id);
ALTER TABLE assets DROP CONSTRAINT assets_owner_id_fkey,
  ADD CONSTRAINT assets_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES players(id);
ALTER TABLE banked_dice DROP CONSTRAINT banked_dice_player_id_fkey,
  ADD CONSTRAINT banked_dice_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE dice_roll_dice DROP CONSTRAINT dice_roll_dice_player_id_fkey,
  ADD CONSTRAINT dice_roll_dice_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE dice_rolls DROP CONSTRAINT dice_rolls_actor_id_fkey,
  ADD CONSTRAINT dice_rolls_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES players(id);
ALTER TABLE difficulty_votes DROP CONSTRAINT difficulty_votes_player_id_fkey,
  ADD CONSTRAINT difficulty_votes_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_responder_id_fkey,
  ADD CONSTRAINT duel_bouts_responder_id_fkey FOREIGN KEY (responder_id) REFERENCES players(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_winner_id_fkey,
  ADD CONSTRAINT duel_bouts_winner_id_fkey FOREIGN KEY (winner_id) REFERENCES players(id);
ALTER TABLE duel_bouts DROP CONSTRAINT duel_bouts_declarer_id_fkey,
  ADD CONSTRAINT duel_bouts_declarer_id_fkey FOREIGN KEY (declarer_id) REFERENCES players(id);
ALTER TABLE duel_staked_assets DROP CONSTRAINT duel_staked_assets_player_id_fkey,
  ADD CONSTRAINT duel_staked_assets_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE laws DROP CONSTRAINT laws_signatory_id_fkey,
  ADD CONSTRAINT laws_signatory_id_fkey FOREIGN KEY (signatory_id) REFERENCES players(id);
ALTER TABLE liaise_choices DROP CONSTRAINT liaise_choices_player_id_fkey,
  ADD CONSTRAINT liaise_choices_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE marginalia DROP CONSTRAINT marginalia_torn_by_id_fkey,
  ADD CONSTRAINT marginalia_torn_by_id_fkey FOREIGN KEY (torn_by_id) REFERENCES players(id);
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_demanding_player_id_fkey,
  ADD CONSTRAINT pending_counter_demands_demanding_player_id_fkey FOREIGN KEY (demanding_player_id) REFERENCES players(id);
ALTER TABLE pending_counter_demands DROP CONSTRAINT pending_counter_demands_target_player_id_fkey,
  ADD CONSTRAINT pending_counter_demands_target_player_id_fkey FOREIGN KEY (target_player_id) REFERENCES players(id);
ALTER TABLE plan_tokens DROP CONSTRAINT plan_tokens_player_id_fkey,
  ADD CONSTRAINT plan_tokens_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE plans DROP CONSTRAINT plans_preparer_id_fkey,
  ADD CONSTRAINT plans_preparer_id_fkey FOREIGN KEY (preparer_id) REFERENCES players(id);
ALTER TABLE plans DROP CONSTRAINT plans_target_player_id_fkey,
  ADD CONSTRAINT plans_target_player_id_fkey FOREIGN KEY (target_player_id) REFERENCES players(id);
ALTER TABLE player_cards DROP CONSTRAINT player_cards_player_id_fkey,
  ADD CONSTRAINT player_cards_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE prologue_choices DROP CONSTRAINT prologue_choices_player_id_fkey,
  ADD CONSTRAINT prologue_choices_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE prologue_committed_hearts DROP CONSTRAINT prologue_committed_hearts_player_id_fkey,
  ADD CONSTRAINT prologue_committed_hearts_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE prologue_extra_peers DROP CONSTRAINT prologue_extra_peers_player_id_fkey,
  ADD CONSTRAINT prologue_extra_peers_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE prologue_track_done DROP CONSTRAINT prologue_track_done_player_id_fkey,
  ADD CONSTRAINT prologue_track_done_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE rankings DROP CONSTRAINT rankings_player_id_fkey,
  ADD CONSTRAINT rankings_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE rumors DROP CONSTRAINT rumors_source_player_id_fkey,
  ADD CONSTRAINT rumors_source_player_id_fkey FOREIGN KEY (source_player_id) REFERENCES players(id);
ALTER TABLE scene_entries DROP CONSTRAINT scene_entries_author_id_fkey,
  ADD CONSTRAINT scene_entries_author_id_fkey FOREIGN KEY (author_id) REFERENCES players(id);
ALTER TABLE scene_peers DROP CONSTRAINT scene_peers_controller_player_id_fkey,
  ADD CONSTRAINT scene_peers_controller_player_id_fkey FOREIGN KEY (controller_player_id) REFERENCES players(id);
ALTER TABLE scene_posts DROP CONSTRAINT scene_posts_author_id_fkey,
  ADD CONSTRAINT scene_posts_author_id_fkey FOREIGN KEY (author_id) REFERENCES players(id);
ALTER TABLE scenes DROP CONSTRAINT scenes_focus_player_id_fkey,
  ADD CONSTRAINT scenes_focus_player_id_fkey FOREIGN KEY (focus_player_id) REFERENCES players(id);
ALTER TABLE secret_visibility DROP CONSTRAINT secret_visibility_player_id_fkey,
  ADD CONSTRAINT secret_visibility_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE secrets DROP CONSTRAINT secrets_author_id_fkey,
  ADD CONSTRAINT secrets_author_id_fkey FOREIGN KEY (author_id) REFERENCES players(id);
ALTER TABLE shake_up_cost_adjustments DROP CONSTRAINT shake_up_cost_adjustments_player_id_fkey,
  ADD CONSTRAINT shake_up_cost_adjustments_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_target_player_id_fkey,
  ADD CONSTRAINT shake_up_spends_target_player_id_fkey FOREIGN KEY (target_player_id) REFERENCES players(id);
ALTER TABLE shake_up_spends DROP CONSTRAINT shake_up_spends_player_id_fkey,
  ADD CONSTRAINT shake_up_spends_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE simultaneous_reveal_entries DROP CONSTRAINT simultaneous_reveal_entries_player_id_fkey,
  ADD CONSTRAINT simultaneous_reveal_entries_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE war_battle_costs DROP CONSTRAINT war_battle_costs_payer_id_fkey,
  ADD CONSTRAINT war_battle_costs_payer_id_fkey FOREIGN KEY (payer_id) REFERENCES players(id);
ALTER TABLE war_battle_costs DROP CONSTRAINT war_battle_costs_opponent_id_fkey,
  ADD CONSTRAINT war_battle_costs_opponent_id_fkey FOREIGN KEY (opponent_id) REFERENCES players(id);
ALTER TABLE war_participants DROP CONSTRAINT war_participants_player_id_fkey,
  ADD CONSTRAINT war_participants_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE war_peace_proposals DROP CONSTRAINT war_peace_proposals_proposer_id_fkey,
  ADD CONSTRAINT war_peace_proposals_proposer_id_fkey FOREIGN KEY (proposer_id) REFERENCES players(id);
ALTER TABLE war_peace_votes DROP CONSTRAINT war_peace_votes_player_id_fkey,
  ADD CONSTRAINT war_peace_votes_player_id_fkey FOREIGN KEY (player_id) REFERENCES players(id);
ALTER TABLE war_surrender_claims DROP CONSTRAINT war_surrender_claims_surrendered_id_fkey,
  ADD CONSTRAINT war_surrender_claims_surrendered_id_fkey FOREIGN KEY (surrendered_id) REFERENCES players(id);
ALTER TABLE war_surrender_claims DROP CONSTRAINT war_surrender_claims_claimant_id_fkey,
  ADD CONSTRAINT war_surrender_claims_claimant_id_fkey FOREIGN KEY (claimant_id) REFERENCES players(id);
ALTER TABLE plans DROP CONSTRAINT plans_game_id_row_number_fkey,
  ADD CONSTRAINT plans_game_id_row_number_fkey FOREIGN KEY (game_id, row_number) REFERENCES public_record_rows(game_id, row_number);
ALTER TABLE scene_entries DROP CONSTRAINT scene_entries_game_id_row_number_fkey,
  ADD CONSTRAINT scene_entries_game_id_row_number_fkey FOREIGN KEY (game_id, row_number) REFERENCES public_record_rows(game_id, row_number);
ALTER TABLE scene_posts DROP CONSTRAINT scene_posts_game_id_row_number_fkey,
  ADD CONSTRAINT scene_posts_game_id_row_number_fkey FOREIGN KEY (game_id, row_number) REFERENCES public_record_rows(game_id, row_number);
