package service

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

// WorktreeGCResult summarises a garbage-collection pass. Paths are classified by
// ownership/lifecycle; only "completed" worktrees are removed.
type WorktreeGCResult struct {
	Cleaned  []string // complete WorkItems whose leftover worktree was removed
	Retained []string // failed/blocked WorkItems (retained for debugging)
	Active   []string // non-terminal WorkItems (must not be touched)
	Unknown  []string // no owning WorkItem → ambiguous, never auto-deleted
}

// ReconcileWorktrees performs an idempotent worktree lifecycle/GC pass.
//
// Ownership is deterministic: a worktree at <root>/<id> is owned by the
// WorkItem with that id. Classification:
//
//   - complete        → clean the worktree (leftover from a crash mid-cleanup);
//   - failed/blocked  → retain (debugging evidence);
//   - non-terminal    → active, never touched;
//   - no WorkItem     → unknown/ambiguous, never auto-deleted (reported only).
//
// Cleanup is safe (validated path + ownership) and idempotent (a removed
// worktree no longer appears in List, so no duplicate events). Lifecycle
// changes are audited.
func (o *Orchestrator) ReconcileWorktrees(ctx context.Context) (WorktreeGCResult, error) {
	var res WorktreeGCResult

	paths, err := o.worktrees.List()
	if err != nil {
		return res, err
	}

	for _, path := range paths {
		id := domain.WorkItemID(filepath.Base(path))
		if id == "" || path == o.worktrees.Path("") {
			res.Unknown = append(res.Unknown, path)
			continue
		}

		wi, err := o.store.GetWorkItem(ctx, id)
		if errors.Is(err, store.ErrNotFound) {
			// No owning WorkItem → ambiguous ownership: never auto-delete.
			res.Unknown = append(res.Unknown, path)
			continue
		}
		if err != nil {
			continue
		}

		switch {
		case wi.CurrentPhase == domain.PhaseComplete:
			if err := o.worktrees.Cleanup(ctx, string(id)); err != nil {
				// Cleanup failure must not hide the WorkItem's functional result;
				// it will be retried on the next GC pass.
				continue
			}
			res.Cleaned = append(res.Cleaned, path)
			_ = o.appendEvent(ctx, id, domain.EventWorktreeCleaned, domain.ActorSystem, map[string]any{
				"path":   path,
				"reason": "gc_completed",
			})
		case wi.CurrentPhase == domain.PhaseFailed || wi.CurrentPhase == domain.PhaseBlocked:
			res.Retained = append(res.Retained, path)
		default:
			res.Active = append(res.Active, path)
		}
	}
	return res, nil
}
