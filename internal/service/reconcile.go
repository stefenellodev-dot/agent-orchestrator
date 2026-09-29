package service

import (
	"context"
	"errors"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// Reconcile performs deterministic, idempotent startup reconciliation.
//
// At startup there is no live driver from a previous process: nothing running
// in THIS process can predate boot, so any Session still in running/pending
// was interrupted by a crash/restart. Reconcile:
//
//  1. marks those orphaned Sessions as failed (audited; existing evidence is
//     preserved — it never fabricates execution evidence);
//  2. marks the owning WorkItem failed via the existing lifecycle semantics
//     (phase.failed event), so the project concurrency slot is released and
//     the existing retry API can resume it from an appropriate phase;
//  3. emits a workitem.reconciled audit event when it touched Sessions.
//
// awaiting_approval is not resumed (it waits for a human) and terminal phases
// are skipped. An item currently driven by THIS process (see the single-driver
// guard) is never touched — it is a legitimate active execution, not an orphan.
//
// Idempotent: a second run marks nothing new (failed WorkItems are terminal
// and failed Sessions are no longer running/pending), and re-driving the same
// WorkItem is prevented by the single-driver guard.
//
// Returns the number of WorkItems whose interrupted Sessions were reconciled.
func (o *Orchestrator) Reconcile(ctx context.Context) (int, error) {
	items, err := o.store.ListWorkItems(ctx, "")
	if err != nil {
		return 0, err
	}

	reconciled := 0
	for _, wi := range items {
		if wi.CurrentPhase.IsTerminal() {
			continue
		}
		if o.isInflight(wi.ID) {
			// Legitimate active execution in this process: the driver owns
			// these Sessions right now. Reconciliation must not fail them.
			continue
		}

		orphaned := o.reconcileSessions(ctx, wi)
		if orphaned == 0 {
			// No interrupted execution: resume clean non-terminal items whose
			// phase requires an OpenCode run, as before.
			if wi.CurrentPhase.RequiresOpenCodeRun() && o.autoDrive && o.adapter != nil {
				go func(id domain.WorkItemID) {
					_ = o.Drive(context.Background(), id)
				}(wi.ID)
			}
			continue
		}

		_ = o.appendEvent(ctx, wi.ID, domain.EventWorkItemReconciled, domain.ActorSystem, map[string]any{
			"phase":                string(wi.CurrentPhase),
			"interrupted_sessions": orphaned,
		})
		reconciled++

		// Fail the interrupted WorkItem through the existing lifecycle so the
		// concurrency slot is released and retry can resume it. The cause is
		// system-attributed (server restart), not a human decision, so failed
		// — not blocked — is the correct terminal state. Do NOT auto-resume:
		// recovery happens explicitly through the retry API.
		_ = o.markFailed(ctx, wi,
			errors.New("interrupted by server restart; reconciled at startup"))
	}
	return reconciled, nil
}

// reconcileSessions marks running/pending Sessions of a WorkItem as failed,
// recording that the execution was interrupted by a restart. Returns how many
// Sessions were reconciled.
func (o *Orchestrator) reconcileSessions(ctx context.Context, wi *domain.WorkItem) int {
	sessions, err := o.store.ListSessions(ctx, wi.ID)
	if err != nil {
		return 0
	}
	n := 0
	for _, s := range sessions {
		if s.Status != domain.SessionRunning && s.Status != domain.SessionPending {
			continue
		}
		now := o.now()
		s.Status = domain.SessionFailed
		s.Error = "interrupted by restart; reconciled at startup"
		s.CompletedAt = &now
		if err := o.store.UpdateSession(ctx, s); err != nil {
			continue
		}
		n++
		_ = o.appendEvent(ctx, wi.ID, domain.EventSessionFailed, domain.ActorSystem, map[string]any{
			"session_id": string(s.ID),
			"phase":      string(s.Phase),
			"reason":     "reconciled_interrupted",
		})
	}
	return n
}
