-- 001_initial.sql
-- MVP schema: WorkItems, Sessions, Gates, Approvals, Events.
-- PostgreSQL is the audit trail; no message broker.

CREATE TABLE IF NOT EXISTS work_items (
    id            TEXT PRIMARY KEY,
    project       TEXT NOT NULL,
    title         TEXT NOT NULL,
    description   TEXT NOT NULL DEFAULT '',
    priority      TEXT NOT NULL DEFAULT 'medium',
    status        TEXT NOT NULL,
    current_phase TEXT NOT NULL,
    worktree_path TEXT NOT NULL DEFAULT '',
    base_branch   TEXT NOT NULL DEFAULT 'main',
    assignee      TEXT NOT NULL DEFAULT '',
    metadata      JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_work_items_project ON work_items (project);

-- Concurrency guard: at most one active WorkItem per project (MVP default).
CREATE UNIQUE INDEX IF NOT EXISTS one_active_per_project
    ON work_items (project)
    WHERE status NOT IN ('complete', 'failed', 'blocked');

CREATE TABLE IF NOT EXISTS sessions (
    id               TEXT PRIMARY KEY,
    work_item_id     TEXT NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    phase            TEXT NOT NULL,
    opencode_session TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL,
    agent            TEXT NOT NULL DEFAULT '',
    prompt           TEXT NOT NULL DEFAULT '',
    output           JSONB,
    error            TEXT NOT NULL DEFAULT '',
    exit_code        INTEGER NOT NULL DEFAULT 0,
    started_at       TIMESTAMPTZ NOT NULL,
    completed_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_sessions_work_item ON sessions (work_item_id, started_at);

CREATE TABLE IF NOT EXISTS gates (
    id                 TEXT PRIMARY KEY,
    work_item_id       TEXT NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    phase              TEXT NOT NULL,
    required_approvers JSONB NOT NULL DEFAULT '[]'::jsonb,
    status             TEXT NOT NULL,
    payload            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at         TIMESTAMPTZ NOT NULL,
    resolved_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_gates_work_item ON gates (work_item_id);

CREATE TABLE IF NOT EXISTS approvals (
    id                   BIGSERIAL PRIMARY KEY,
    gate_id              TEXT NOT NULL REFERENCES gates (id) ON DELETE CASCADE,
    username             TEXT NOT NULL,
    decision             TEXT NOT NULL,
    comment              TEXT NOT NULL DEFAULT '',
    authorization_record JSONB,
    at                   TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_approvals_gate ON approvals (gate_id, at);

CREATE TABLE IF NOT EXISTS events (
    id           TEXT PRIMARY KEY,
    work_item_id TEXT NOT NULL REFERENCES work_items (id) ON DELETE CASCADE,
    type         TEXT NOT NULL,
    payload      JSONB NOT NULL DEFAULT '{}'::jsonb,
    actor        TEXT NOT NULL,
    at           TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_work_item ON events (work_item_id, at);
