-- Ticket #21: a continue/retry is a new run attempt tied to the original run.
ALTER TABLE runs ADD COLUMN original_run_id TEXT REFERENCES runs(id);
CREATE INDEX idx_runs_original ON runs(original_run_id);
