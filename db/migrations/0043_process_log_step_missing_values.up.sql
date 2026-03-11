-- 0043_process_log_step_missing_values
-- Add missing process_log_step enum values

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'model_selected';
EXCEPTION WHEN duplicate_object THEN null; END $$;

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'token_budget_calculated';
EXCEPTION WHEN duplicate_object THEN null; END $$;

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'container_resolved';
EXCEPTION WHEN duplicate_object THEN null; END $$;

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'resolve_completed';
EXCEPTION WHEN duplicate_object THEN null; END $$;
