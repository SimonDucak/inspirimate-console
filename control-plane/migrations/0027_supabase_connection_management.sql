-- 0027 — which Management API areas a Supabase connection may change.
--
-- features (0026) stays the MCP groups: database, docs, storage.
-- management is a JSON array of control-plane areas the same token may
-- change: auth, storage. The token itself stays in secret_enc.
ALTER TABLE supabase_connection ADD COLUMN management TEXT NOT NULL DEFAULT '[]';
