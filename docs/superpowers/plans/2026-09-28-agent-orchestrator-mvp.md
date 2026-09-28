# Agent Orchestrator MVP — Incremental Implementation Plan

> **For agentic workers:** Execute this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Use TDD: write the failing test, run it, implement, run it, commit.

**Goal:** Build the smallest Human-in-the-Loop orchestrator that drives one WorkItem through `Discovery → Decision → awaiting_approval → Implementation → Validation → Complete` by invoking ephemeral `opencode run` executions inside a single reused Git worktree, then layer persistence, evidence, dashboard, and Piave deployment.

**Architecture:** One Go binary with four subcommands (`serve`, `web`, `opencode-run`, `migrate`) + PostgreSQL + Git worktrees under a configurable root + an OpenCode adapter boundary. Local execution uses `opencode run` directly; Piave execution swaps only the adapter's process launcher. No Kubernetes, no broker, no external auth.

**Tech Stack:** Go 1.22, PostgreSQL 16, `pgx/v5`, `echo/v4`, `cobra`, `viper`, React + Vite (embedded), systemd + Podman/Quadlet.

---

## Milestone Map

| # | Milestone | Vertical slice proven | Exit criterion |
|---|-----------|----------------------|----------------|
| 0 | Walking skeleton | API → worktree → OpenCode run → persisted Session/Event | One WorkItem reaches `complete` against a **test repo**, auto-approved |
| 1 | PostgreSQL persistence | Same as M0 but durable, restart-safe | Kill/restart process, WorkItem state survives |
| 2 | Human approval gate | `awaiting_approval` blocks; `IMPLEMENTATION=AUTHORIZED` condition | WorkItem cannot enter Implementation without recorded authorization |
| 3 | Objective evidence | Commit SHA, diff, test/lint exit codes persisted | `complete` requires exit-code-0 validation; confidence is metadata only |
| 4 | Dashboard | List, detail, approve/reject in browser | Human completes M2 flow without curl |
| 5 | Piave deployment | systemd + Podman/Quadlet worker | Same M0 flow runs via `systemd-run`-launched container |
| 6 | Controlled real run | One real WorkItem on CondoSmart | Read-only towards repo until final explicit approval |

Each milestone ends with a working, shippable binary. No milestone depends on a later one.

---

## Milestone 0 — Walking Skeleton (smallest runnable vertical slice)

**Scope:** In-memory store, real worktree, real `opencode run` for Discovery+Decision+Implementation+Validation, auto-approve stub at the gate. Proves the skeleton end-to-end before any durability.

### Task 0.1 — Initialize module, config, CLI skeleton

**Files:**
- Create: `go.mod`, `Makefile`, `.gitignore`
- Create: `cmd/orchestrator/main.go`
- Create: `internal/config/config.go`
- Create: `configs/config.example.yaml`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Write failing config test**

```go
func TestLoadConfig_AppliesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	os.WriteFile(path, []byte("server:\n  port: 9999\n"), 0o600)
	cfg, err := config.Load(path)
	require.NoError(t, err)
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "opencode", cfg.OpenCode.BinaryPath) // default applied
	assert.Equal(t, 1, cfg.Concurrency.MaxActivePerProject) // default applied
}
```

- [ ] **Step 2: Run to verify failure** → `go test ./internal/config/...` → FAIL (package missing)
- [ ] **Step 3: Implement** `config.Load` (viper, `setDefaults`), `Config` struct including `Concurrency.MaxActivePerProject` (default 1), `Worktree.RootPath`, `OpenCode.Agents` (explicit per-phase names), `Auth` (basic), `Web`.
- [ ] **Step 4: Implement CLI skeleton** with `serve`, `web`, `opencode-run`, `migrate` subcommands each returning `errors.New("not implemented")`.
- [ ] **Step 5: Run** → `go build ./... && go test ./internal/config/...` → PASS
- [ ] **Step 6: Commit** → `git commit -m "chore: module, config, CLI skeleton"`

### Task 0.2 — Domain types and state machine

**Files:**
- Create: `internal/domain/workitem.go`, `internal/domain/evidence.go`, `internal/domain/opencode.go`
- Test: `internal/domain/phase_test.go`

- [ ] **Step 1: Write failing state-machine test**

```go
func TestPhaseNext(t *testing.T) {
	assert.Equal(t, PhaseDecision, PhaseDiscovery.Next())
	assert.Equal(t, PhaseAwaitingApproval, PhaseDecision.Next())
	assert.Equal(t, PhaseImplementation, PhaseAwaitingApproval.Next())
	assert.True(t, PhaseComplete.IsTerminal())
	assert.True(t, PhaseFailed.IsTerminal())
	assert.True(t, PhaseBlocked.IsTerminal())
}
```

- [ ] **Step 2: Run to verify failure** → FAIL
- [ ] **Step 3: Implement domain types** — `WorkItem`, `Session`, `Gate`, `Approval` (with optional `AuthorizationRecord`), `Event`, `SessionOutput` (objective fields: `CommitSHA`, `Diff`, `TestResults`, `LintResults`, `ExitCode`), `RunRequest`, `RunResult`, `CLICapabilities`. `Plan.Confidence` exists but is tagged metadata-only in comments.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit**

### Task 0.3 — In-memory store + orchestrator service (no gate enforcement yet)

**Files:**
- Create: `internal/store/store.go` (interface), `internal/store/memory/store.go`
- Create: `internal/service/orchestrator.go`
- Test: `internal/service/orchestrator_test.go`

- [ ] **Step 1: Write failing test** — creating a WorkItem starts Discovery and records `workitem.created` + `phase.started` events.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement `Store` interface** (`CreateWorkItem`, `GetWorkItem`, `UpdateWorkItemPhase`, `CreateSession`, `UpdateSession`, `AppendEvent`, `ListEvents`, `CreateGate`, `UpdateGate`) and the memory impl. Implement `Orchestrator.CreateWorkItem` that provisions a worktree path and emits events.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit**

### Task 0.4 — Git worktree manager (one worktree per WorkItem, full lifecycle)

**Files:**
- Create: `internal/service/worktree.go`
- Test: `internal/service/worktree_test.go` (uses a temp bare repo fixture)
- Create: `tests/fixtures/test-repo/` (init script or committed minimal repo)

- [ ] **Step 1: Write failing test**

```go
func TestWorktree_CreateOnceReuseAndCleanup(t *testing.T) {
	// given a base repo and root dir
	wt, err := mgr.Create(ctx, "WI-1", baseRepo, "main")
	require.NoError(t, err)
	assert.DirExists(t, wt.Path)
	// second call returns SAME path, no new worktree
	wt2, _ := mgr.Create(ctx, "WI-1", baseRepo, "main")
	assert.Equal(t, wt.Path, wt2.Path)
	// phases reuse it
	require.NoError(t, mgr.Cleanup(ctx, "WI-1"))
	assert.NoDirExists(t, wt.Path)
}
```

- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** `Create` (idempotent: `git worktree add <root>/WI-<id> <base>`, reuse if exists), `Cleanup` (`git worktree remove --force`), `Path(id)`. Record `worktree.created` / `worktree.cleaned` events.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit**

### Task 0.5 — OpenCode adapter + mandatory capability probe

**Files:**
- Create: `internal/service/opencode_adapter.go`
- Create: `internal/service/opencode_capabilities.go`
- Test: `internal/service/opencode_capabilities_test.go`
- Create: `docs/opencode-capabilities.md` (findings record)

- [ ] **Step 1: Write failing probe test** — `ValidateCLI` parses `opencode --version` and `opencode run --help`, returns a `CLICapabilities` with each flag marked true/false. It MUST NOT panic on unknown output; unknown flags → false.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement probe.** Run `opencode --version`, `opencode run --help`, `opencode agent list`. Detect `--format`, `--session`, `--continue`, `--fork`, `--agent`, `--dir`, `--attach` by substring match on help text. Write findings to `docs/opencode-capabilities.md`.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Manual verification step (blocking gate for the milestone):** execute and record actual output:

```bash
opencode --version
opencode run --help
opencode agent list
```

Record in `docs/opencode-capabilities.md`. Decisions:
- If `--format json` present → parse JSON; else parse text (**fallback path must exist**).
- If `--session`/`--fork` uncertain → **run each phase independently** (correctness must not depend on session continuity).
- If `--agent` present → use configured agent names; else use default agent.

- [ ] **Step 6: Implement `Run`** using only capabilities detected. Independent-phase mode is the default. Command shape (adapt to verified flags):

```go
// independent phase, verified flags only
cmd := exec.CommandContext(ctx, bin, "run", "--dir", req.WorktreePath, req.Prompt)
cmd.Dir = req.WorktreePath
cmd.Env = append(os.Environ(), mapToEnv(req.EnvVars)...)
```

After completion, collect objective evidence: `git -C <wt> rev-parse HEAD` (CommitSHA), `git -C <wt> diff <base>..HEAD` (Diff), captured `ExitCode`/stdout/stderr.

- [ ] **Step 7: Test `Run`** against a fake `opencode` shell script on `PATH` (success + nonzero exit + malformed output). → PASS
- [ ] **Step 8: Commit**

### Task 0.6 — Minimal API to create/get a WorkItem and drive the flow

**Files:**
- Create: `internal/api/routes.go`, `internal/api/handlers.go`, `internal/api/dto.go`, `internal/api/middleware.go`
- Test: `internal/api/handlers_test.go` (httptest)
- Modify: `cmd/orchestrator/main.go` (`serve` wiring)

- [ ] **Step 1: Write failing handler test** — `POST /api/workitems` then `GET /api/workitems/{id}` returns phase; `GET /api/workitems/{id}/events` returns ordered events.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement routes/handlers** with the interface `OrchestratorService`.
- [ ] **Step 4: Implement `serve`** wiring config → store → worktree mgr → adapter → service → echo server with `/healthz`.
- [ ] **Step 5: Run** → PASS
- [ ] **Step 6: Commit**

### Task 0.7 — End-to-end skeleton test (auto-approved through Complete)

**Files:**
- Test: `tests/integration/skeleton_test.go`
- Create: `configs/test.yaml`
- Create: `tests/fixtures/fake-opencode.sh`

- [ ] **Step 1: Write failing E2E test** — start orchestrator with fake opencode + temp repo, POST WorkItem, wait for `complete`, assert phase sequence events, assert worktree cleaned.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement auto-approve stub** behind `feature.auto_approve_for_tests=true` (default false) so the gate returns "approved" automatically **only in tests**.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit** → tag `v0.0.1-skeleton`

**Milestone 0 acceptance:** the full flow is reachable over HTTP against a test repo; only the gate is stubbed.

---

## Milestone 1 — PostgreSQL Persistence

**Scope:** Replace memory store with PostgreSQL; add migrations; keep the `Store` interface unchanged so the service/API layer is untouched.

### Task 1.1 — Schema and migration runner

**Files:**
- Create: `migrations/001_initial.sql`
- Create: `internal/store/postgres/migrate.go`
- Test: `internal/store/postgres/migrate_test.go`

- [ ] **Step 1: Write failing test** — running migrations twice is idempotent; tables exist.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement DDL** for `work_items`, `sessions`, `gates`, `approvals`, `events`. `events` is the audit log (no broker). `work_items` has `project` index and a partial unique index enforcing **at most 1 active WorkItem per project** (M0→M1 default), e.g.:

```sql
CREATE UNIQUE INDEX one_active_per_project
  ON work_items (project)
  WHERE status NOT IN ('complete','failed','blocked');
```

- [ ] **Step 4: Implement `migrate` command** applied via `cobra`.
- [ ] **Step 5: Run** → PASS
- [ ] **Step 6: Commit**

### Task 1.2 — PostgreSQL repos implementing `Store`

**Files:**
- Create: `internal/store/postgres/workitem_repo.go`, `session_repo.go`, `gate_repo.go`, `event_repo.go`, `store.go`
- Test: `internal/store/postgres/store_test.go` (uses `ORCHESTRATOR_TEST_DSN`, skipped if unset)

- [ ] **Step 1: Write failing repo test** — round-trip WorkItem, Session, Gate, Event.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement repos** with `pgx/v5`; JSON columns for `metadata`, `output`, `payload`.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Swap store in `serve`** by config (`store.kind: postgres|memory`).
- [ ] **Step 6: Re-run Milestone 0 E2E test against Postgres** → PASS
- [ ] **Step 7: Commit**

### Task 1.3 — Concurrency enforcement (default 1 active per project)

**Files:**
- Test: `internal/service/concurrency_test.go`
- Modify: `internal/service/orchestrator.go`

- [ ] **Step 1: Write failing test** — second WorkItem for same project returns `ErrProjectBusy` when `MaxActivePerProject=1`.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** pre-check + rely on the partial unique index as the hard guarantee (handle unique violation → `ErrProjectBusy`).
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit** → tag `v0.1.0-persistence`

---

## Milestone 2 — Human Approval Gate

**Scope:** Enforce `awaiting_approval`; record `IMPLEMENTATION = AUTHORIZED` as a condition on approval; remove the test auto-approve path from production code.

### Task 2.1 — Gate service

**Files:**
- Create: `internal/service/approval.go`
- Test: `internal/service/approval_test.go`

- [ ] **Step 1: Write failing tests**

```go
func TestCannotEnterImplementationWithoutAuthorization(t *testing.T) { /* Expect ErrAuthorizationRequired */ }
func TestApprove_RecordsAuthorizationAndAdvances(t *testing.T) { /* authorization.granted event + phase=implementation */ }
func TestRequestChanges_ReturnsToDecision(t *testing.T)
func TestReject_Blocks(t *testing.T)
```

- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** `AwaitApproval`, `Approve` (sets `AuthorizationRecord{Granted:true,...}`, emits `authorization.granted`, advances), `RequestChanges` (→ `decision`), `Reject` (→ `blocked`). `AdvanceToImplementation` returns `ErrAuthorizationRequired` unless granted.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit**

### Task 2.2 — Gate API + gate creation on Decision completion

**Files:**
- Modify: `internal/api/routes.go`, `internal/api/handlers.go`, `internal/service/orchestrator.go`
- Test: `internal/api/gate_handlers_test.go`

- [ ] **Step 1: Write failing test** — `POST /api/workitems/{id}/approve`, `/reject`, `/request-changes`; after Decision, `GET` shows gate payload (plan + evidence refs).
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** handlers; on Decision phase completion, create Gate with `payload` referencing Decision evidence and set phase `awaiting_approval`.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Remove auto-approve from production path** (keep only under test build/config flag).
- [ ] **Step 6: Commit** → tag `v0.2.0-gate`

---

## Milestone 3 — Objective Evidence

**Scope:** Make `complete` gated on reproducible validation results, not agent confidence.

### Task 3.1 — Evidence collector

**Files:**
- Create: `internal/service/evidence.go`
- Test: `internal/service/evidence_test.go`

- [ ] **Step 1: Write failing test** — given a worktree with committed changes and a passing/failing test command, collector returns `CommitSHA`, unified `Diff`, and `TestResults` with `ExitCode`.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** collection from git + configured validation commands (per project config, orchestrator-owned).
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Commit**

### Task 3.2 — Completion rule

**Files:**
- Modify: `internal/service/orchestrator.go`
- Test: `internal/service/completion_test.go`

- [ ] **Step 1: Write failing test** — Validation with non-zero exit → `failed` (not `complete`); zero exit → `complete`. `Plan.Confidence` never affects outcome.
- [ ] **Step 2: Run** → FAIL
- [ ] **Step 3: Implement** rule: `complete` iff `validation.ExitCode == 0` and required evidence fields present; else `failed` or `blocked`.
- [ ] **Step 4: Run** → PASS
- [ ] **Step 5: Cleanup policy** — on `complete`, cleanup worktree after evidence persisted; on `failed`/`blocked`, retain; `retain_failed_days` configurable.
- [ ] **Step 6: Commit** → tag `v0.3.0-evidence`

---

## Milestone 4 — Dashboard

**Scope:** Browser UI for list/detail/approve/reject; embedded in the binary.

### Task 4.1 — Project scaffolding

**Files:**
- Create: `web/package.json`, `web/vite.config.ts`, `web/index.html`, `web/src/main.tsx`, `web/src/App.tsx`

- [ ] **Step 1: Scaffold Vite + React + TypeScript**, dev proxy `/api` → `:8080`.
- [ ] **Step 2: Verify** → `npm run build` produces `web/dist`
- [ ] **Step 3: Commit**

### Task 4.2 — Pages and API client

**Files:**
- Create: `web/src/api/client.ts`
- Create: `web/src/pages/WorkItemList.tsx`, `WorkItemDetail.tsx`, `ApprovalPanel.tsx`

- [ ] **Step 1: Implement list** (project, phase badge, updated_at).
- [ ] **Step 2: Implement detail** (phase timeline, events, evidence: commit SHA/diff/tests, gate payload).
- [ ] **Step 3: Implement approval panel** (approve / request changes / reject + comment). Disable approve until evidence present.
- [ ] **Step 4: Commit**

### Task 4.3 — Embed and serve

**Files:**
- Create: `internal/web/embed.go`
- Modify: `cmd/orchestrator/main.go` (`web` command)

- [ ] **Step 1: `//go:embed all:dashboard`** and serve SPA fallback.
- [ ] **Step 2: Basic auth middleware** (configurable username/password; default on).
- [ ] **Step 3: E2E test** — approve via HTTP using simulated dashboard request; assert phase advances.
- [ ] **Step 4: Commit** → tag `v0.4.0-dashboard`

---

## Milestone 5 — Piave Deployment (Podman + Quadlet + systemd)

**Scope:** Swap the adapter's process launcher; no competing architecture.

### Task 5.1 — Verify Piave environment

**Files:**
- Create: `docs/piave-environment.md`

- [ ] **Step 1: Record actual environment** (blocking gate):

```bash
podman --version
systemctl --version
loginctl show-user $USER | grep Linger
cat /etc/os-release
getenforce 2>/dev/null || echo "no selinux"
```

- [ ] **Step 2: Decide rootless vs rootful, SELinux relabel strategy (`:Z`), linger.** Record results.
- [ ] **Step 3: Commit**

### Task 5.2 — systemd units and Quadlet worker

**Files:**
- Create: `configs/systemd/orchestrator.service`, `orchestrator-web.service`
- Create: `configs/quadlet/opencode-worker@.container`

- [ ] **Step 1: Implement units** with `EnvironmentFile=/etc/agent-orchestrator/secrets.env` (0600).
- [ ] **Step 2: Implement adapter "remote" mode** — `opencode-run` supports launching via `systemd-run --unit=opencode-WI-<id>-<phase> ... podman run ...`; worktree bind-mounted read-write.
- [ ] **Step 3: Verify locally** with `systemd-run --user` against the test repo.
- [ ] **Step 4: Commit** → tag `v0.5.0-piave`

### Task 5.3 — Deployment runbook

**Files:**
- Create: `docs/deployment-piave.md`

- [ ] **Step 1: Document** install layout (`/opt/agent-orchestrator`, `/var/lib/...`), user creation, secrets, `systemctl daemon-reload/enable/start`, log inspection via `journalctl -u`.
- [ ] **Step 2: Commit** → tag `v0.5.1-deployment`

---

## Milestone 6 — Controlled Real Run (CondoSmart, then fin-engine)

### Task 6.1 — One real WorkItem on CondoSmart

- [ ] **Step 1:** Configure a CondoSmart project entry (repo path, base branch, agent names, validation commands) in orchestrator config only.
- [ ] **Step 2:** Create ONE low-risk WorkItem; run through to `awaiting_approval`.
- [ ] **Step 3:** Inspect evidence (diff, test results); human decides.
- [ ] **Step 4:** If approved and validation passes → `complete`; otherwise `blocked`/`failed` and retain worktree.
- [ ] **Step 5:** Record findings; **no push/PR/merge performed**.
- [ ] **Step 6:** Only after this succeeds, repeat for fin-engine config.

---

## Cross-cutting constraints (locked)

- One worktree per WorkItem for its entire lifecycle; never recreated between phases.
- `awaiting_approval` is a phase. `IMPLEMENTATION = AUTHORIZED` is an authorization condition recorded on approval, not a state.
- MVP correctness MUST NOT depend on OpenCode session continuity/`--fork`/persistent sessions.
- No push, PR, merge, or automatic GitHub operation.
- Project config is orchestrator-owned; no `.opencode/orchestrator.yaml` required in consumer repos.
- Basic local auth only.
- PostgreSQL `events` table is the audit trail; no broker/event-sourcing infra.
- Evidence is objective (commit SHA, diff, exit codes, test/lint output); confidence is optional metadata and never decides `COMPLETE`.
- Local and Piave differ only in the adapter's process launcher.

---

## Verification checkpoints

| Checkpoint | Command |
|-----------|---------|
| Build | `make build` |
| Unit + integration | `go test -race ./...` (Postgres tests use `ORCHESTRATOR_TEST_DSN`) |
| Lint | `golangci-lint run ./...` |
| E2E skeleton | `go test ./tests/integration/... -run TestSkeleton` |
| Capability probe | `opencode --version && opencode run --help` (recorded) |
| Piave env | `podman --version && systemctl --version` (recorded) |

---

## Open questions carried forward (non-blocking, resolve at the named task)

1. Exact OpenCode JSON schema for `--format json` → resolve in Task 0.5; adapter must tolerate absence.
2. Whether OpenCode returns a session ID on `run` → resolve in Task 0.5; if absent, independent phases are authoritative.
3. Whether forks preserve useful context across phases → affects only prompt design, not correctness.
4. Git identity inside worker (user.name/email, commit signing) → resolve in Task 5.2.
5. Validation command conventions per project → orchestrator-owned list; resolve during Task 6.1.
6. Worktree backup/recovery on Piave reboot → resolve in Task 5.3.
