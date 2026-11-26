-- Usage read path (#18): link an assistant message to the run that produced
-- it, so its usage can be attached after a restart without recomputing from
-- the event stream. The column is nullable: canonical history written before
-- this migration simply has no usage attached.
ALTER TABLE messages ADD COLUMN run_id TEXT;
CREATE INDEX idx_messages_run ON messages(run_id);
