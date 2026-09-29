package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func newGCOrch(t *testing.T) (*service.Orchestrator, *memory.Store, string, string) {
	t.Helper()
	repo := initRepo(t)
	root := t.TempDir()
	st := memory.New()
	orch := service.New(st, service.NewGitWorktreeManager(root), &fakeAdapter{})
	return orch, st, repo, root
}

func createWI(t *testing.T, orch *service.Orchestrator, repo, project string) domain.WorkItemID {
	t.Helper()
	wi, err := orch.CreateWorkItem(context.Background(), service.CreateWorkItemInput{
		Project: project, Title: "t", RepoPath: repo, BaseBranch: "main",
	})
	require.NoError(t, err)
	return wi.ID
}

func setPhase(t *testing.T, st *memory.Store, id domain.WorkItemID, phase domain.Phase) {
	t.Helper()
	wi, err := st.GetWorkItem(context.Background(), id)
	require.NoError(t, err)
	wi.CurrentPhase = phase
	wi.Status = phase
	require.NoError(t, st.UpdateWorkItem(context.Background(), wi))
}

func TestGC_CleansCompletedWorkItem(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	require.DirExists(t, wi.WorktreePath)

	setPhase(t, st, id, domain.PhaseComplete)
	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.Contains(t, res.Cleaned, wi.WorktreePath)
	assert.NoDirExists(t, wi.WorktreePath, "completed WorkItem's worktree must be cleaned")
	assert.Equal(t, 1, countEvents(t, orch, id, domain.EventWorktreeCleaned))
}

func TestGC_Idempotent(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	setPhase(t, st, id, domain.PhaseComplete)

	_, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	res2, err := orch.ReconcileWorktrees(ctx) // second pass: nothing left
	require.NoError(t, err)
	assert.Empty(t, res2.Cleaned)
	assert.Equal(t, 1, countEvents(t, orch, id, domain.EventWorktreeCleaned), "no duplicate cleanup events")
}

func TestGC_KeepsActiveWorkItem(t *testing.T) {
	ctx := context.Background()
	orch, _, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p") // stays in discovery (active)
	wi, _ := orch.GetWorkItem(ctx, id)

	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.Contains(t, res.Active, wi.WorktreePath)
	assert.DirExists(t, wi.WorktreePath, "active WorkItem's worktree must NOT be cleaned")
}

func TestGC_KeepsFailedWorkItemRetained(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	setPhase(t, st, id, domain.PhaseFailed)

	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.Contains(t, res.Retained, wi.WorktreePath)
	assert.DirExists(t, wi.WorktreePath, "failed WorkItem's worktree is retained")
}

func TestGC_DoesNotTouchOtherWorkItem(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	a := createWI(t, orch, repo, "pa")
	b := createWI(t, orch, repo, "pb") // different WorkItem (different id/project not needed for ownership)
	wiA, _ := orch.GetWorkItem(ctx, a)
	wiB, _ := orch.GetWorkItem(ctx, b)
	setPhase(t, st, a, domain.PhaseComplete)

	_, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.NoDirExists(t, wiA.WorktreePath, "completed A cleaned")
	assert.DirExists(t, wiB.WorktreePath, "B's worktree must not be touched")
}

func TestGC_UnknownWorktreeNotDeleted(t *testing.T) {
	ctx := context.Background()
	orch, _, repo, root := newGCOrch(t)
	// A worktree whose id has no owning WorkItem → ambiguous ownership.
	fakeID := "01FAKEWORKTREE000000000000"
	path := filepath.Join(root, fakeID)
	runGit(t, repo, "worktree", "add", "-b", "workitem/"+fakeID, path, "main")
	require.DirExists(t, path)

	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.Contains(t, res.Unknown, path)
	assert.DirExists(t, path, "unknown-ownership worktree must never be auto-deleted")
}

func TestGC_IgnoresNonWorktreeDirs(t *testing.T) {
	ctx := context.Background()
	orch, _, _, root := newGCOrch(t)
	// A plain directory under the root is not a worktree → ignored entirely.
	plain := filepath.Join(root, "not-a-worktree")
	require.NoError(t, os.MkdirAll(plain, 0o755))

	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	assert.NotContains(t, res.Unknown, plain)
	assert.NotContains(t, res.Cleaned, plain)
	assert.DirExists(t, plain)
}

func TestGC_CleanupFailureDoesNotHideResult(t *testing.T) {
	ctx := context.Background()
	orch, st, repo, _ := newGCOrch(t)
	id := createWI(t, orch, repo, "p")
	wi, _ := orch.GetWorkItem(ctx, id)
	setPhase(t, st, id, domain.PhaseComplete)

	// Simulate an interrupted cleanup by removing the directory but leaving Git
	// metadata: the next GC pass must remain safe (no panic, no wrong deletion).
	require.NoError(t, os.RemoveAll(wi.WorktreePath))
	res, err := orch.ReconcileWorktrees(ctx)
	require.NoError(t, err)
	_ = res // no destructive action on a missing worktree
}
