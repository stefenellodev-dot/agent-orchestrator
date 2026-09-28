package service_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/service"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	base := []string{"-c", "user.email=test@example.com", "-c", "user.name=Test", "-c", "commit.gpgsign=false"}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "git %v: %s", args, string(out))
	return string(out)
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o600))
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "init")
	return dir
}

func countAddedWorktrees(t *testing.T, repo string) int {
	t.Helper()
	out := runGit(t, repo, "worktree", "list", "--porcelain")
	n := strings.Count(out, "worktree ")
	return n - 1 // first entry is the main working tree
}

func TestGitWorktree_CreateOnceReuseAndCleanup(t *testing.T) {
	ctx := context.Background()
	base := initRepo(t)
	root := t.TempDir()
	mgr := service.NewGitWorktreeManager(root)

	wt, err := mgr.Create(ctx, "WI-1", base, "main")
	require.NoError(t, err)
	assert.DirExists(t, wt.Path)
	assert.Equal(t, "WI-1", wt.WorkItemID)
	assert.Equal(t, 1, countAddedWorktrees(t, base))

	// Idempotent: same WorkItem reuses the SAME worktree, no second one created.
	wt2, err := mgr.Create(ctx, "WI-1", base, "main")
	require.NoError(t, err)
	assert.Equal(t, wt.Path, wt2.Path)
	assert.Equal(t, 1, countAddedWorktrees(t, base), "must not create a second worktree")

	// Cleanup removes it and git metadata is pruned.
	require.NoError(t, mgr.Cleanup(ctx, "WI-1"))
	assert.NoDirExists(t, wt.Path)
	assert.Equal(t, 0, countAddedWorktrees(t, base))

	// Recreating after cleanup yields the same deterministic path.
	wt3, err := mgr.Create(ctx, "WI-1", base, "main")
	require.NoError(t, err)
	assert.Equal(t, wt.Path, wt3.Path)
}

func TestGitWorktree_PathIsDeterministic(t *testing.T) {
	mgr := service.NewGitWorktreeManager("/some/root")
	assert.Equal(t, filepath.Join("/some/root", "WI-abc"), mgr.Path("WI-abc"))
}

func TestGitWorktree_CleanupMissingIsNoError(t *testing.T) {
	ctx := context.Background()
	mgr := service.NewGitWorktreeManager(t.TempDir())
	assert.NoError(t, mgr.Cleanup(ctx, "WI-does-not-exist"))
}
