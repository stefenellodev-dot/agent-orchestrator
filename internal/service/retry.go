package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
)

// ErrNotRetryable is returned when a WorkItem is not in a retryable phase.
var ErrNotRetryable = errors.New("work item is not retryable")

// RetryInput describes a safe retry of a failed/blocked WorkItem.
type RetryInput struct {
	User    string
	Comment string
	// From is the phase to re-enter. Empty defaults to discovery (re-plan).
	From domain.Phase
	// Rebase resets the WorkItem branch to the current tip of the base branch
	// and updates the immutable base commit. Use when the plan is stale.
	Rebase bool
}

// Retry safely re-drives a failed or blocked WorkItem.
//
// Guarantees:
//   - no duplicate WorkItem is created;
//   - prior sessions and events are preserved (new ones are appended);
//   - re-entering at or past Implementation requires a valid, granted
//     authorization; otherwise the WorkItem returns to the human gate;
//   - the human gate is never bypassed: re-planning re-opens the gate.
func (o *Orchestrator) Retry(ctx context.Context, id domain.WorkItemID, in RetryInput) error {
	wi, err := o.GetWorkItem(ctx, id)
	if err != nil {
		return err
	}
	if wi.CurrentPhase != domain.PhaseFailed && wi.CurrentPhase != domain.PhaseBlocked {
		return fmt.Errorf("%w: work item is %q, not failed/blocked", ErrNotRetryable, wi.CurrentPhase)
	}

	from := in.From
	if from == "" {
		from = domain.PhaseDiscovery
	}
	switch from {
	case domain.PhaseDiscovery, domain.PhaseDecision, domain.PhaseImplementation, domain.PhaseValidation:
	default:
		return fmt.Errorf("%w: cannot retry from phase %q", ErrInvalidInput, from)
	}

	// Re-entering at or past Implementation requires the human authorization.
	needsAuthorization := from == domain.PhaseImplementation || from == domain.PhaseValidation
	if needsAuthorization && !o.hasValidAuthorization(ctx, id) {
		return ErrAuthorizationRequired
	}

	if in.Rebase {
		if err := o.rebaseWorktree(ctx, wi); err != nil {
			return err
		}
	}

	if err := o.appendEvent(ctx, id, domain.EventWorkItemRetried, domain.ActorHuman, map[string]any{
		"by":      in.User,
		"comment": in.Comment,
		"from":    string(from),
		"rebase":  in.Rebase,
	}); err != nil {
		return err
	}

	if err := o.updatePhase(ctx, wi, from); err != nil {
		if errors.Is(err, store.ErrProjectBusy) {
			return ErrProjectBusy
		}
		return err
	}

	o.resumeDrive(id)
	return nil
}

// hasValidAuthorization reports whether the WorkItem has an approved gate with a
// granted authorization record.
func (o *Orchestrator) hasValidAuthorization(ctx context.Context, id domain.WorkItemID) bool {
	gate, err := o.store.GetGateByWorkItem(ctx, id)
	if err != nil {
		return false
	}
	return gate.Status == domain.GateApproved && hasGrantedAuthorization(gate)
}

// rebaseWorktree resets the WorkItem branch to the current tip of its base
// branch and refreshes the immutable base commit. It only touches the WorkItem's
// own branch and worktree, never the base branch or the main working tree.
func (o *Orchestrator) rebaseWorktree(ctx context.Context, wi *domain.WorkItem) error {
	repoPath := o.projects[wi.Project].RepoPath
	wt, err := o.worktrees.Create(ctx, string(wi.ID), repoPath, wi.BaseBranch)
	if err != nil {
		return err
	}
	if _, err := runGitCommand(ctx, wt.Path, "reset", "--hard", wi.BaseBranch); err != nil {
		return fmt.Errorf("rebase worktree: %w", err)
	}
	wi.BaseCommitSHA = resolveBaseSHA(ctx, wt.Path, wi.BaseBranch)
	wi.UpdatedAt = o.now()
	if err := o.store.UpdateWorkItem(ctx, wi); err != nil {
		if errors.Is(err, store.ErrProjectBusy) {
			return ErrProjectBusy
		}
		return err
	}
	_ = o.appendEvent(ctx, wi.ID, domain.EventWorktreeCreated, domain.ActorSystem, map[string]any{
		"path":       wt.Path,
		"rebased_to": wi.BaseCommitSHA,
	})
	return nil
}
