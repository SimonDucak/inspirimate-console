-- 0033 — owner-scoped Firebase App Distribution connections.
--
-- The service account JSON is encrypted at rest. project_id and client_email
-- are labels for the console. The private key is only inside secret_enc.
-- An app points at one connection; the APK upload reads it from there.

CREATE TABLE firebase_distribution (
  id            TEXT PRIMARY KEY,
  owner_token   TEXT NOT NULL,
  name          TEXT NOT NULL,
  app_id        TEXT NOT NULL,
  project_id    TEXT NOT NULL,
  client_email  TEXT NOT NULL,
  group_alias   TEXT NOT NULL DEFAULT '',
  tester_emails TEXT NOT NULL DEFAULT '[]',
  secret_enc    BLOB NOT NULL,
  secret_nonce  BLOB NOT NULL,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);
CREATE INDEX        idx_firebase_distribution_owner      ON firebase_distribution(owner_token);
CREATE UNIQUE INDEX idx_firebase_distribution_owner_name ON firebase_distribution(owner_token, name);

ALTER TABLE app ADD COLUMN firebase_distribution_id TEXT;
CREATE INDEX idx_app_firebase_distribution ON app(firebase_distribution_id);
