-- Add enable_token_estimate column to models table
-- This allows per-model configuration of token estimation
-- Default is false for better performance
ALTER TABLE models ADD COLUMN IF NOT EXISTS enable_token_estimate BOOLEAN NOT NULL DEFAULT false;

-- Create index for faster lookups
CREATE INDEX IF NOT EXISTS idx_models_enable_token_estimate ON models(enable_token_estimate);
