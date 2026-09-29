package service

import (
	"context"

	"github.com/stefenello/agent-orchestrator/internal/domain"
)

// Reconcile performs deterministic, idempotent startup reconciliation.
//
// At startup there is no live driver from a previous process, so any Session
// still in running/pending was interrupted by a crash/restart. Reconcile:
//
//  1. marks those orphaned Sessions as failed (audited; existing evidence is
//     preserved — it never fabricates execution evidence);
//  2. emits a workitem.reconciled audit event when it touched Sessions;
//  3. resumes non-terminal WorkItems whose phase requires an OpenCode run, via
//     the guarded Drive, so they continue without manual repair.
//
// awaiting_approval is not resumed (it waits for a human) and terminal phases
// are skipped. Idempotent: a second run marks nothing new, and re-driving the
// same WorkItem is prevented by the single-driver guard.
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

		orphaned := o.reconcileSessions(ctx, wi)
		if orphaned > 0 {
			reconciled++
			_ = o.appendEvent(ctx, wi.ID, domain.EventWorkItemReconciled, domain.ActorSystem, map[string]any{
				"phase":                string(wi.CurrentPhase),
				"interrupted_sessions": orphaned,
			})
		}

		// Resume execution for phases that run OpenCode. awaiting_approval does
		// not (it waits for a human); terminal phases are skipped above.
		if wi.CurrentPhase.RequiresOpenCodeRun() && o.autoDrive && o.adapter != nil {
			go func(id domain.WorkItemID) {
				_ = o.Drive(context.Background(), id)
			}(wi.ID)
		}
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
