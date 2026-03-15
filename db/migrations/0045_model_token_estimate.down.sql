-- Remove enable_token_estimate column from models table
ALTER TABLE models DROP COLUMN IF EXISTS enable_token_estimate;
