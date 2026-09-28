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

func TestEvidenceCollector_CapturesGitAndCommands(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	// Commit a change on a branch based on main, like a WorkItem worktree does.
	runGit(t, repo, "checkout", "-b", "workitem/test")
	require.NoError(t, os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("new\n"), 0o600))
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feature")

	col := service.NewEvidenceCollector()
	out, err := col.Collect(ctx, repo, "main", service.ProjectValidation{
		TestCommands:      []string{"true"},
		LintCommands:      []string{"false"},
		TypecheckCommands: []string{"echo ok"},
	})
	require.NoError(t, err)

	assert.NotEmpty(t, out.CommitSHA)
	assert.NotEmpty(t, out.BaseCommitSHA)
	assert.NotEmpty(t, out.Diff, "diff against base must be captured")
	assert.Contains(t, out.Diff, "feature.txt")

	require.Len(t, out.TestCommands, 1)
	assert.Equal(t, 0, out.TestCommands[0].ExitCode)
	assert.True(t, out.TestCommands[0].Passed)

	require.Len(t, out.LintCommands, 1)
	assert.NotEqual(t, 0, out.LintCommands[0].ExitCode)

	assert.False(t, out.AllValidationPassed(), "a failing command must fail the suite")
}

func TestEvidenceCollector_NoCommandsIsNotPassed(t *testing.T) {
	ctx := context.Background()
	repo := initRepo(t)
	out, err := service.NewEvidenceCollector().Collect(ctx, repo, "main", service.ProjectValidation{})
	require.NoError(t, err)
	assert.False(t, out.AllValidationPassed(), "empty validation must not be treated as passing")
}
