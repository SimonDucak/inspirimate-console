-- Addresses emailed when a coding agent finishes on this project.
-- JSON array of plain email addresses. Not a secret.
ALTER TABLE app ADD COLUMN notify_emails TEXT NOT NULL DEFAULT '[]';
