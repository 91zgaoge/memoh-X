DROP INDEX IF EXISTS idx_snapshots_runtime_snapshot_name;
ALTER TABLE snapshots
  DROP COLUMN IF EXISTS runtime_snapshot_name,
  DROP COLUMN IF EXISTS source,
  DROP COLUMN IF EXISTS display_name;
