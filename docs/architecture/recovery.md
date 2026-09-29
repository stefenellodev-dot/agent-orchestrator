# Recovery: startup reconciliation of interrupted executions

Status: implemented (boot reconciliation + single-driver discipline).

## Problem

A server restart (deploy, crash, SIGTERM) destroys the in-flight `Drive`
goroutine while the OpenCode adapter call is blocked. `executePhase` only
writes `CompletedAt`/status/exit **after** the adapter returns, so the session
stays `running`/`completed_at=NULL` forever, the WorkItem stays non-terminal,
and the `one_active_per_project` partial unique index
(`WHERE status NOT IN ('complete','failed','blocked')`) holds the project's
concurrency slot permanently.

## Model

Single process: an in-memory driver execution cannot survive a restart. On
startup there is no live driver from a previous process, so any Session still
`running`/`pending` on a non-terminal WorkItem is an orphan.

## Lifecycle

```text
startup
→ reconciliation
→ session.failed        (status=failed, completed_at set, audited reason)
→ phase.failed          (existing markFailed semantics)
→ WorkItem.failed       (terminal → concurrency slot released)
→ retry                 (existing API; from=discovery by default, no gate needed)
```

- `failed` (not `blocked`): an interrupted session is a **system** event, not a
  human decision. Discovery legitimately has no Human Gate yet, and `Reject`
  requires a pending gate that discovery-phase items don't have. `failed` is
  directly recoverable via the existing `retry` endpoint.
- Worktrees are **retained** for debugging (consistent with all failed items).
- The concurrency guard is unchanged; the released slot follows from the
  terminal state, not from a schema change.

## Safeguards

- **Legitimate active execution is never touched.** A WorkItem currently driven
  by this process (`inflight` map, single-driver discipline) is skipped.
- **Terminal WorkItems are never touched** (complete/failed/blocked), even if a
  stray `running` session row exists.
- **Idempotent.** A second run marks nothing new: failed items are skipped at
  the top, already-failed sessions are skipped, and no events duplicate.
- **No auto-resume.** A recovered WorkItem waits for an explicit `retry`; the
  sweep never re-drives failed items.

## Events

- `session.failed` with `reason: reconciled_interrupted` (per orphaned session).
- `phase.failed` with the interruption cause (per recovered WorkItem).
- `workitem.reconciled` audit event with phase + interrupted-session count.

## Known limitation (follow-up hardening)

`exec.CommandContext` kills only the direct `opencode` child; grandchildren
that inherit the stdout/stderr pipes can in principle wedge `cmd.Run` past the
phase deadline. Process-group killing (`Setpgid` + kill-group) is recorded as
future hardening, not implemented here.
