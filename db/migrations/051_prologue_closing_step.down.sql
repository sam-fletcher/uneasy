-- 051_prologue_closing_step.down.sql
ALTER TABLE games DROP CONSTRAINT games_prologue_ranking_step_check;

ALTER TABLE games
  ADD CONSTRAINT games_prologue_ranking_step_check
    CHECK (prologue_ranking_step IN (
      'declare_power','place_set_asides_power',
      'declare_knowledge','place_set_asides_knowledge',
      'declare_esteem','place_set_asides_esteem',
      'extra_peers'
    ));
