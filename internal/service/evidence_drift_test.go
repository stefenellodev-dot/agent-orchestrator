package service_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/service"
)

// Phase 1.1 regression: advancing the base branch independently must not be
// attributed to the WorkItem.
func TestEvidence_BaseBranchDriftDoesNotCorruptEvidence(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	root := t.TempDir()
	mgr := service.NewGitWorktreeManager(root)

	wt, err := mgr.Create(ctx, "WI-drift", repo, "main")
	require.NoError(t, err)
	base := wt.BaseCommitSHA
	require.NotEmpty(t, base)

	// The WorkItem makes its own change on its own branch.
	require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "wi-change.txt"), []byte("work\n"), 0o600))
	runGit(t, wt.Path, "add", "-A")
	runGit(t, wt.Path, "commit", "-m", "workitem change")

	// Meanwhile main advances independently in the MAIN repository.
	require.NoError(t, os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("x\n"), 0o600))
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-m", "unrelated advance")

	col := service.NewEvidenceCollector()
	out, err := col.Collect(ctx, wt.Path, "main", base, service.ProjectValidation{})
	require.NoError(t, err)

	assert.Equal(t, base, out.BaseCommitSHA, "evidence must use the immutable base")
	assert.Contains(t, out.Diff, "wi-change.txt")
	assert.NotContains(t, out.Diff, "unrelated.txt", "base-branch drift must not contaminate evidence")
}

// Without an explicit base, the collector falls back to the branch point
// (merge-base), which is also drift-safe.
func TestEvidence_FallsBackToMergeBase(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	root := t.TempDir()
	mgr := service.NewGitWorktreeManager(root)

	wt, err := mgr.Create(ctx, "WI-mb", repo, "main")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wt.Path, "wi-change.txt"), []byte("work\n"), 0o600))
	runGit(t, wt.Path, "add", "-A")
	runGit(t, wt.Path, "commit", "-m", "workitem change")

	require.NoError(t, os.WriteFile(filepath.Join(repo, "unrelated.txt"), []byte("x\n"), 0o600))
	runGit(t, repo, "add", "-A")
	runGit(t, repo, "commit", "-m", "unrelated advance")

	out, err := service.NewEvidenceCollector().Collect(ctx, wt.Path, "main", "", service.ProjectValidation{})
	require.NoError(t, err)
	assert.Contains(t, out.Diff, "wi-change.txt")
	assert.NotContains(t, out.Diff, "unrelated.txt")
}
