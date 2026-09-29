package service_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

// captureAdapter records the agent requested for each phase invocation.
type captureAdapter struct {
	mu     sync.Mutex
	agents []string
}

func (a *captureAdapter) Name() string { return "capture" }
func (a *captureAdapter) Available(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{Version: "capture", SupportedAgents: []string{"explore", "plan", "build"}}, nil
}
func (a *captureAdapter) Run(_ context.Context, req domain.RunRequest) (*domain.RunResult, error) {
	a.mu.Lock()
	a.agents = append(a.agents, req.Agent)
	a.mu.Unlock()
	return &domain.RunResult{ExitCode: 0}, nil
}
func (a *captureAdapter) captured() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]string(nil), a.agents...)
}

func driveCapToTerminal(t *testing.T, orch *service.Orchestrator, id domain.WorkItemID) {
	t.Helper()
	require.NoError(t, orch.Drive(context.Background(), id))
}

func TestCapability_OverridesPerPhaseAgents(t *testing.T) {
	ctx := context.Background()
	ad := &captureAdapter{}
	orch := service.New(memory.New(), &fakeProvisioner{}, ad)
	orch.SetAutoApprove(true)
	orch.SetAgents("explore", "plan", "build", "build")

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "build",
	})
	require.NoError(t, err)
	assert.Equal(t, "build", wi.Capability, "capability must be persisted on the WorkItem")

	driveCapToTerminal(t, orch, wi.ID)

	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseComplete, got.CurrentPhase)

	agents := ad.captured()
	require.Len(t, agents, 4, "four OpenCode phases")
	for _, a := range agents {
		assert.Equal(t, "build", a, "WorkItem capability must override the per-phase agents")
	}
}

func TestCapability_DefaultsToPerPhaseAgents(t *testing.T) {
	ctx := context.Background()
	ad := &captureAdapter{}
	orch := service.New(memory.New(), &fakeProvisioner{}, ad)
	orch.SetAutoApprove(true)
	orch.SetAgents("explore", "plan", "build", "build")

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo"})
	require.NoError(t, err)
	driveCapToTerminal(t, orch, wi.ID)

	assert.Equal(t, []string{"explore", "plan", "build", "build"}, ad.captured(),
		"without a capability the per-phase agents must be used")
}

func TestCapability_AllowedAgentsRejectsAtCreate(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &captureAdapter{})
	orch.SetCapabilities([]string{"explore", "plan", "build"})
	orch.RegisterProject(service.ProjectConfig{Name: "p", AllowedAgents: []string{"build"}})

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "plan",
	})
	assert.ErrorIs(t, err, service.ErrPolicyViolation, "capability outside allowed_agents must be rejected")

	_, err = orch.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "build",
	})
	assert.NoError(t, err, "allowed capability must be accepted")
}

func TestCapability_UnknownRejectedAtCreate(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &captureAdapter{})
	orch.SetCapabilities([]string{"build"})
	orch.RegisterProject(service.ProjectConfig{Name: "p"})

	_, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "ghost",
	})
	assert.ErrorIs(t, err, service.ErrPolicyViolation, "unknown capability must be rejected")

	// With no capability probe, validation is permissive (unknown runtime).
	orch2 := service.New(memory.New(), &fakeProvisioner{}, &captureAdapter{})
	orch2.RegisterProject(service.ProjectConfig{Name: "p"})
	_, err = orch2.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "ghost",
	})
	assert.NoError(t, err, "without a capability probe, capabilities cannot be validated")
}

func TestCapability_AllowedAgentsBlocksExecution(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &captureAdapter{})
	orch.SetAutoApprove(true)
	orch.SetAgents("explore", "plan", "explore", "build") // implementation resolves to explore
	orch.RegisterProject(service.ProjectConfig{Name: "p", AllowedAgents: []string{"build"}})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "x", RepoPath: "/repo"})
	require.NoError(t, err)

	err = orch.Drive(ctx, wi.ID)
	require.Error(t, err)
	assert.ErrorIs(t, err, service.ErrPolicyViolation)

	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseBlocked, got.CurrentPhase)

	events, err := orch.ListEvents(ctx, wi.ID)
	require.NoError(t, err)
	assert.True(t, hasEvent(events, domain.EventPolicyRejected))
}

func TestCapability_UnknownFlaggedInDiagnostics(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &captureAdapter{})

	// Created while the runtime is unknown (permissive), then the runtime reports
	// its real agent set: the WorkItem capability becomes unknown.
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{
		Project: "p", Title: "x", RepoPath: "/repo", Capability: "ghost",
	})
	require.NoError(t, err)
	orch.SetCapabilities([]string{"explore", "plan", "build"})

	rep := orch.Diagnostics(ctx)
	assert.True(t, hasFinding(rep, "capabilities", service.StatusWarn),
		"unknown capability must be surfaced (workitem %s)", wi.ID)
	assert.Contains(t, rep.Capabilities, "build", "reported capabilities must be exposed")
}
