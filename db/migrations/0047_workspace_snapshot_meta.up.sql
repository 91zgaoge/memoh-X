-- Add workspace snapshot metadata columns for versioning support.
ALTER TABLE snapshots
  ADD COLUMN IF NOT EXISTS runtime_snapshot_name TEXT NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'manual',
  ADD COLUMN IF NOT EXISTS display_name TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_snapshots_runtime_snapshot_name
  ON snapshots(runtime_snapshot_name)
  WHERE runtime_snapshot_name != '';
