-- Console language for this instance (en or sk). One choice for everyone who
-- opens the console. Existing rows stay English. Non-secret.
ALTER TABLE instance_settings ADD COLUMN locale TEXT NOT NULL DEFAULT 'en';
