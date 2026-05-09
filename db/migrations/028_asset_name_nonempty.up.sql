-- 028_asset_name_nonempty.up.sql
-- Defence in depth: the asset-creation handler already rejects empty names,
-- but the schema should enforce its own invariant so direct SQL inserts
-- (tests, dev tooling, future code paths) can't slip a blank-named asset
-- through. Names are user-supplied display strings; "" makes no sense.

ALTER TABLE assets
  ADD CONSTRAINT assets_name_nonempty CHECK (length(name) > 0);
