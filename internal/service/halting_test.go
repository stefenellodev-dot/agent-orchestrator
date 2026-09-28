package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

// scriptedAdapter returns a per-phase exit code.
type scriptedAdapter struct{ exitByPhase map[domain.Phase]int }

func (s *scriptedAdapter) ValidateCLI(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{Version: "test"}, nil
}

func (s *scriptedAdapter) Run(_ context.Context, req domain.RunRequest) (*domain.RunResult, error) {
	code := s.exitByPhase[req.Phase]
	return &domain.RunResult{ExitCode: code, Stderr: "scripted"}, nil
}

func runWithExit(t *testing.T, exitByPhase map[domain.Phase]int) domain.Phase {
	t.Helper()
	ctx := context.Background()
	repo := initRepo(t)
	orch := service.New(memory.New(), service.NewGitWorktreeManager(t.TempDir()), &scriptedAdapter{exitByPhase: exitByPhase})
	orch.SetAutoApprove(true)
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: repo})
	require.NoError(t, err)
	_ = orch.Drive(ctx, wi.ID)
	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	return got.CurrentPhase
}

func TestPhaseFailureHalting(t *testing.T) {
	cases := []struct {
		name string
		exit map[domain.Phase]int
		want domain.Phase
	}{
		{"failed discovery cannot reach gate", map[domain.Phase]int{domain.PhaseDiscovery: 1}, domain.PhaseFailed},
		{"failed decision cannot reach gate", map[domain.Phase]int{domain.PhaseDecision: 1}, domain.PhaseFailed},
		{"failed implementation cannot reach validation", map[domain.Phase]int{domain.PhaseImplementation: 1}, domain.PhaseFailed},
		{"failed validation cannot reach complete", map[domain.Phase]int{domain.PhaseValidation: 1}, domain.PhaseFailed},
		{"happy path completes", map[domain.Phase]int{}, domain.PhaseComplete},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, runWithExit(t, tc.exit))
		})
	}
}
