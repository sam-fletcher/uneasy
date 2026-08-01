-- Migration 053: a target plan holds ONE demand slot for good.
--
-- Migration 017 excluded resolved demands from the uniqueness predicate, so a
-- target could pick up a second demand once the first had resolved. That is
-- reachable: two plans sharing a row have a full focus-player turn between them
-- (the rulebook's own step 7 / followSceneGate), during which a third player can
-- prepare another demand against the same still-pending target. Both end up
-- resolved+made, and DemandWinnersForTargetPlan — first match in id order —
-- honours the OLDER one, silently discarding four draft picks
-- (adr/MAKE_DEMANDS_AUDIT.md, D4). Design decision #5 ("one demand per target
-- plan") is now literal.
--
-- 'cancelled' stays excluded: it means the plan never came together, so it never
-- really claimed the slot. That branch is unreachable for a demand today (only an
-- overflowing Make War / Clandestinely Liaise delay reveal cancels, and a demand
-- always has a row), but it is the behaviour we want if that ever changes.
--
-- Safe to apply to live data: creating this index would fail if any target
-- already carried two non-cancelled demands, but that state cannot exist. It
-- requires a demand to have reached 'resolved', and until the D1 fix landed in
-- this same change no demand could be completed at all — CanComplete gated on
-- plans.result, which is only ever written together with status='resolved'.

DROP INDEX IF EXISTS uq_one_demand_per_target;

CREATE UNIQUE INDEX uq_one_demand_per_target
  ON plans (targeted_plan_id)
  WHERE targeted_plan_id IS NOT NULL
    AND status <> 'cancelled';
