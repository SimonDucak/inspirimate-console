-- 0028 — which saved Supabase connection a project uses.
--
-- The token stays on supabase_connection. This column is only the pointer.
-- NULL means the project has no Supabase connection.
ALTER TABLE app ADD COLUMN supabase_connection_id TEXT;
CREATE INDEX idx_app_supabase_connection ON app(supabase_connection_id);
