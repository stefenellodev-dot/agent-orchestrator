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

func createAndFail(t *testing.T, st *memory.Store, orch *service.Orchestrator) domain.WorkItemID {
	t.Helper()
	ctx := context.Background()
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	got, err := st.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	got.CurrentPhase = domain.PhaseFailed
	got.Status = domain.PhaseFailed
	require.NoError(t, st.UpdateWorkItem(ctx, got))
	return wi.ID
}

func TestRetry_RejectsNonFailedWorkItem(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})
	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)

	err = orch.Retry(ctx, wi.ID, service.RetryInput{User: "alice"})
	assert.ErrorIs(t, err, service.ErrNotRetryable)
}

func TestRetry_FromImplementationRequiresAuthorization(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})
	id := createAndFail(t, st, orch)

	err := orch.Retry(ctx, id, service.RetryInput{User: "alice", From: domain.PhaseImplementation})
	assert.ErrorIs(t, err, service.ErrAuthorizationRequired, "cannot re-enter implementation without authorization")

	got, _ := orch.GetWorkItem(ctx, id)
	assert.Equal(t, domain.PhaseFailed, got.CurrentPhase, "state must be unchanged on rejected retry")
}

func TestRetry_FromDiscoveryReopensAndPreservesHistory(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})
	id := createAndFail(t, st, orch)

	before, err := orch.ListEvents(ctx, id)
	require.NoError(t, err)

	require.NoError(t, orch.Retry(ctx, id, service.RetryInput{User: "alice", Comment: "re-plan", From: domain.PhaseDiscovery}))

	got, err := orch.GetWorkItem(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseDiscovery, got.CurrentPhase)

	after, err := orch.ListEvents(ctx, id)
	require.NoError(t, err)
	assert.Greater(t, len(after), len(before), "retry appends events without deleting history")
	assert.True(t, hasEventType(after, domain.EventWorkItemRetried))
	// Prior evidence is preserved verbatim.
	assert.Equal(t, before[0].ID, after[0].ID)
}

func TestRetry_FromImplementationWithValidAuthorizationProceeds(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})
	id := createAndFail(t, st, orch)

	// Record a valid authorization via the gate.
	gate := &domain.Gate{
		ID: domain.NewGateID(), WorkItemID: id, Phase: domain.PhaseAwaitingApproval,
		Status: domain.GateApproved,
		Approvals: []domain.Approval{{
			User: "alice", Decision: domain.ApprovalApprove,
			Authorization: &domain.AuthorizationRecord{Granted: true, GrantedBy: "alice"},
		}},
	}
	require.NoError(t, st.CreateGate(ctx, gate))

	require.NoError(t, orch.Retry(ctx, id, service.RetryInput{User: "alice", From: domain.PhaseImplementation}))

	got, err := orch.GetWorkItem(ctx, id)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseImplementation, got.CurrentPhase)
}

func hasEventType(events []*domain.Event, typ domain.EventType) bool {
	for _, e := range events {
		if e.Type == typ {
			return true
		}
	}
	return false
}
