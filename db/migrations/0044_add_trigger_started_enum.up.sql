-- 0044_add_trigger_started_enum
-- Add missing 'trigger_started' value to process_log_step enum

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'trigger_started';
EXCEPTION WHEN duplicate_object THEN null; END $$;
