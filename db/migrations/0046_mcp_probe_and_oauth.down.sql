-- 0046_mcp_probe_and_oauth (down)
DROP TABLE IF EXISTS mcp_oauth_tokens;

ALTER TABLE mcp_connections
  DROP COLUMN IF EXISTS status,
  DROP COLUMN IF EXISTS tools_cache,
  DROP COLUMN IF EXISTS last_probed_at,
  DROP COLUMN IF EXISTS status_message,
  DROP COLUMN IF EXISTS auth_type;
