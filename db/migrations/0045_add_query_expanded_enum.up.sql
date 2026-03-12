-- 0045_add_query_expanded_enum
-- Add missing 'query_expanded' and 'memory_filtered' values to process_log_step enum

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'query_expanded';
EXCEPTION WHEN duplicate_object THEN null; END $$;

DO $$ BEGIN
  ALTER TYPE process_log_step ADD VALUE IF NOT EXISTS 'memory_filtered';
EXCEPTION WHEN duplicate_object THEN null; END $$;
