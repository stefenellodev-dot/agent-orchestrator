package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

// unavailableAdapter runs prompts fine but reports the runtime unavailable,
// exercising the R6 execution runtime guard.
type unavailableAdapter struct{ probeAdapter }

func (a *unavailableAdapter) Available(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{}, errors.New("opencode not installed")
}

func newPolicyOrch() *service.Orchestrator {
	return service.New(memory.New(), &fakeProvisioner{}, &probeAdapter{})
}

func TestPolicy_RequireRegisteredProject_RejectsUnregistered(t *testing.T) {
	ctx := context.Background()
	orch := newPolicyOrch()
	orch.SetRequireRegisteredProject(true)
	orch.RegisterProject(service.ProjectConfig{Name: "known"})

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "ghost", Title: "x", RepoPath: "/repo"})
	assert.ErrorIs(t, err, service.ErrPolicyViolation)

	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "known", Title: "x", RepoPath: "/repo"})
	assert.NoError(t, err, "registered project must still be accepted")
}

func TestPolicy_UnregisteredAllowedByDefault(t *testing.T) {
	ctx := context.Background()
	orch := newPolicyOrch()

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "ghost", Title: "x", RepoPath: "/repo"})
	assert.NoError(t, err, "without require_registered_project, unknown projects stay allowed (R1-R5 behavior)")
}

func TestPolicy_ProtectedBaseBranchRejects(t *testing.T) {
	ctx := context.Background()
	orch := newPolicyOrch()
	orch.SetMaxActivePerProject(10)
	orch.RegisterProject(service.ProjectConfig{Name: "p", BaseBranch: "master"})

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo", BaseBranch: "dev"})
	assert.ErrorIs(t, err, service.ErrPolicyViolation, "non-protected base branch must be rejected")

	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo", BaseBranch: "master"})
	assert.NoError(t, err)

	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "y", RepoPath: "/repo"})
	assert.NoError(t, err, "empty base branch defaults to the protected branch")
}

func TestPolicy_PerProjectConcurrencyOverride(t *testing.T) {
	ctx := context.Background()
	orch := newPolicyOrch()
	orch.SetMaxActivePerProject(5)
	orch.RegisterProject(service.ProjectConfig{Name: "tight", MaxActivePerProject: 1})

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "tight", Title: "A", RepoPath: "/repo"})
	require.NoError(t, err)
	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "tight", Title: "B", RepoPath: "/repo"})
	assert.ErrorIs(t, err, service.ErrProjectBusy, "per-project limit must override the global limit")
}

func TestPolicy_ExecutionGuardBlocksUnavailableRuntime(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &unavailableAdapter{})
	orch.SetAutoApprove(true)
	orch.RegisterProject(service.ProjectConfig{Name: "p", RequireRuntime: true})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo"})
	require.NoError(t, err)

	drvErr := orch.Drive(ctx, wi.ID)
	require.Error(t, drvErr)
	assert.ErrorIs(t, drvErr, service.ErrPolicyViolation)

	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseBlocked, got.CurrentPhase, "policy rejection must block, not fail")

	events, err := orch.ListEvents(ctx, wi.ID)
	require.NoError(t, err)
	assert.True(t, hasEvent(events, domain.EventPolicyRejected), "policy rejection must be audited")
}

func TestPolicy_DefaultProjectDisablesRuntimeGuard(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &probeAdapter{})
	orch.RegisterProject(service.ProjectConfig{Name: "p"})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo"})
	require.NoError(t, err)
	require.NoError(t, orch.Drive(ctx, wi.ID))

	got, _ := orch.GetWorkItem(ctx, wi.ID)
	assert.Equal(t, domain.PhaseAwaitingApproval, got.CurrentPhase)
}

func hasEvent(events []*domain.Event, et domain.EventType) bool {
	for _, e := range events {
		if e.Type == et {
			return true
		}
	}
	return false
}
