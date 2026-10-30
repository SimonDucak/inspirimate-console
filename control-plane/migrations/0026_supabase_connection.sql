-- 0026 — owner-scoped Supabase connections.
--
-- Stores a Supabase personal access token ENCRYPTED AT REST. Only the
-- AES-GCM ciphertext and nonce are persisted. The plaintext token is never
-- stored in a readable column, returned by the API, or shown in the console.
--
-- owner_token is the API tenant. name is unique per owner. features is a
-- JSON array of the enabled MCP feature names (database, docs).
CREATE TABLE supabase_connection (
  id           TEXT PRIMARY KEY,
  owner_token  TEXT NOT NULL,
  name         TEXT NOT NULL,
  project_ref  TEXT NOT NULL,
  features     TEXT NOT NULL,
  secret_enc   BLOB NOT NULL,
  secret_nonce BLOB NOT NULL,
  created_at   INTEGER NOT NULL,
  updated_at   INTEGER NOT NULL
);
CREATE INDEX        idx_supabase_connection_owner      ON supabase_connection(owner_token);
CREATE UNIQUE INDEX idx_supabase_connection_owner_name ON supabase_connection(owner_token, name);
