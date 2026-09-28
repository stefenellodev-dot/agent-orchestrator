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

// fakeAdapter records phases and returns a canned result without touching disk.
type fakeAdapter struct {
	phases []domain.Phase
	result domain.RunResult
	err    error
}

func (f *fakeAdapter) ValidateCLI(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{Version: "test"}, nil
}

func (f *fakeAdapter) Run(_ context.Context, req domain.RunRequest) (*domain.RunResult, error) {
	f.phases = append(f.phases, req.Phase)
	if f.err != nil {
		return nil, f.err
	}
	res := f.result
	return &res, nil
}

func toGate(t *testing.T, adapter *fakeAdapter) (*service.Orchestrator, domain.WorkItemID) {
	t.Helper()
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, adapter)
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/tmp/repo"})
	require.NoError(t, err)
	require.NoError(t, orch.Drive(ctx, wi.ID))
	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	require.Equal(t, domain.PhaseAwaitingApproval, got.CurrentPhase, "should pause at the gate")
	return orch, wi.ID
}

func TestCannotEnterImplementationWithoutAuthorization(t *testing.T) {
	orch, id := toGate(t, &fakeAdapter{})

	err := orch.AdvanceToImplementation(context.Background(), id)
	assert.ErrorIs(t, err, service.ErrAuthorizationRequired)

	got, _ := orch.GetWorkItem(context.Background(), id)
	assert.Equal(t, domain.PhaseAwaitingApproval, got.CurrentPhase)
}

func TestApprove_RecordsAuthorizationAndAdvances(t *testing.T) {
	ctx := context.Background()
	orch, id := toGate(t, &fakeAdapter{})

	require.NoError(t, orch.Approve(ctx, id, service.ApprovalInput{User: "alice", Comment: "lgtm"}))

	got, err := orch.GetWorkItem(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseImplementation, got.CurrentPhase)

	gate, err := orch.GetGate(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.GateApproved, gate.Status)
	require.Len(t, gate.Approvals, 1)
	require.NotNil(t, gate.Approvals[0].Authorization)
	assert.True(t, gate.Approvals[0].Authorization.Granted)
	assert.Equal(t, "alice", gate.Approvals[0].Authorization.GrantedBy)

	events, _ := orch.ListEvents(ctx, id)
	assert.True(t, containsEvent(events, domain.EventAuthorizationGranted))
	assert.True(t, containsEvent(events, domain.EventGateApproved))
}

func TestRequestChanges_ReturnsToDecision(t *testing.T) {
	ctx := context.Background()
	adapter := &fakeAdapter{}
	orch, id := toGate(t, adapter)

	require.NoError(t, orch.RequestChanges(ctx, id, service.ApprovalInput{User: "bob", Comment: "rework"}))

	got, err := orch.GetWorkItem(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseDecision, got.CurrentPhase)

	gate, err := orch.GetGate(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.GateChangesRequested, gate.Status)
}

func TestReject_Blocks(t *testing.T) {
	ctx := context.Background()
	orch, id := toGate(t, &fakeAdapter{})

	require.NoError(t, orch.Reject(ctx, id, service.ApprovalInput{User: "carol", Comment: "no"}))

	got, err := orch.GetWorkItem(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseBlocked, got.CurrentPhase)
}

func TestApprove_RequiresAwaitingApprovalPhase(t *testing.T) {
	ctx := context.Background()
	orch := service.New(memory.New(), &fakeProvisioner{}, &fakeAdapter{})
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/tmp/repo"})
	require.NoError(t, err)

	// Still in discovery, not at the gate.
	err = orch.Approve(ctx, wi.ID, service.ApprovalInput{User: "alice"})
	assert.ErrorIs(t, err, service.ErrInvalidTransition)
}

func containsEvent(events []*domain.Event, typ domain.EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}
