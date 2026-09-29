-- 004_workitem_capability.sql
-- R7: a first-class WorkItem capability (OpenCode agent profile). Capability
-- drives agent selection, so it is persisted (not metadata, which must never
-- drive state). Empty string preserves the R1-R6 per-phase default behavior.

ALTER TABLE work_items ADD COLUMN IF NOT EXISTS capability TEXT NOT NULL DEFAULT '';
