-- 003_parallel_workitems.sql
-- R5: allow a CONFIGURABLE number of concurrent (non-terminal) WorkItems per
-- project. The previous partial unique index enforced exactly one active
-- WorkItem per project, which cannot express a configurable limit. The limit is
-- now enforced by the orchestrator (concurrency.max_active_per_project); the
-- database keeps a non-unique index for counting and listing.

DROP INDEX IF EXISTS one_active_per_project;

CREATE INDEX IF NOT EXISTS idx_work_items_project_status ON work_items (project, status);
