-- Per-project agent task limit, in seconds. 0 keeps the 10 minute default.
ALTER TABLE app ADD COLUMN task_timeout_s INTEGER NOT NULL DEFAULT 0;
