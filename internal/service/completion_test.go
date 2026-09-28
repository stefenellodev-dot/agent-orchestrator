package service_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func driveToTerminal(t *testing.T, pv service.ProjectValidation, adapter *fakeAdapter) (*service.Orchestrator, domain.WorkItem) {
	t.Helper()
	ctx := context.Background()
	repo := initRepo(t)
	root := t.TempDir()
	orch := service.New(memory.New(), service.NewGitWorktreeManager(root), adapter)
	orch.SetAutoApprove(true)
	orch.SetProjectValidation("p", pv)

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: repo})
	require.NoError(t, err)
	_ = orch.Drive(ctx, wi.ID)
	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	return orch, *got
}

func TestCompletion_PassingCommandsComplete(t *testing.T) {
	orch, got := driveToTerminal(t, service.ProjectValidation{TestCommands: []string{"test -f README.md"}}, &fakeAdapter{})
	assert.Equal(t, domain.PhaseComplete, got.CurrentPhase)

	// Evidence is persisted on the validation session.
	sessions, err := orch.ListSessions(context.Background(), got.ID)
	require.NoError(t, err)
	var found bool
	for _, s := range sessions {
		if s.Phase == domain.PhaseValidation && s.Output != nil && len(s.Output.TestCommands) == 1 {
			found = true
			assert.Equal(t, 0, s.Output.TestCommands[0].ExitCode)
		}
	}
	assert.True(t, found, "validation command result must be persisted")
}

func TestCompletion_FailingCommandFailsAndRetainsWorktree(t *testing.T) {
	orch, got := driveToTerminal(t, service.ProjectValidation{TestCommands: []string{"false"}}, &fakeAdapter{})
	assert.Equal(t, domain.PhaseFailed, got.CurrentPhase)

	_, err := os.Stat(got.WorktreePath)
	assert.NoError(t, err, "failed WorkItem must retain its worktree for debugging")
	_ = orch
}

func TestCompletion_NoCommandsFallsBackToOpenCodeExit(t *testing.T) {
	_, ok := driveToTerminal(t, service.ProjectValidation{}, &fakeAdapter{result: domain.RunResult{ExitCode: 0}})
	assert.Equal(t, domain.PhaseComplete, ok.CurrentPhase)

	_, bad := driveToTerminal(t, service.ProjectValidation{}, &fakeAdapter{result: domain.RunResult{ExitCode: 1}})
	assert.Equal(t, domain.PhaseFailed, bad.CurrentPhase)
}
