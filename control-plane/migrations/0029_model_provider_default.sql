-- One model connection can be the instance default, with one of its models
-- preselected in the console. Changing this does not rotate the connection
-- revision, so existing task capabilities stay valid.
ALTER TABLE model_provider ADD COLUMN is_default INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_provider ADD COLUMN default_model TEXT NOT NULL DEFAULT '';
