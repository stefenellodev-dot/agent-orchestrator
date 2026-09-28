-- 002_workitem_base_commit.sql
-- Persist the immutable base commit each WorkItem branched from. Evidence is
-- computed against this SHA so a moving base branch cannot contaminate it.

ALTER TABLE work_items ADD COLUMN IF NOT EXISTS base_commit_sha TEXT NOT NULL DEFAULT '';
