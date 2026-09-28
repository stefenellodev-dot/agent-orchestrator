package service

import "context"

// Worktree describes a provisioned Git worktree for a WorkItem.
type Worktree struct {
	WorkItemID string
	Path       string
	BaseBranch string
}

// WorktreeManager owns the worktree lifecycle. One WorkItem owns exactly one
// worktree for its entire lifecycle (Discovery through Validation); it is
// never recreated between phases.
type WorktreeManager interface {
	// Create is idempotent: calling it again for the same WorkItem returns the
	// existing worktree rather than creating a second one.
	Create(ctx context.Context, workItemID, repoPath, baseBranch string) (*Worktree, error)
	// Cleanup removes the worktree. Called only when retention policy allows.
	Cleanup(ctx context.Context, workItemID string) error
	// Path returns the on-disk path for a WorkItem without creating anything.
	Path(workItemID string) string
}
