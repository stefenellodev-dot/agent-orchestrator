package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/store"
	"github.com/stefenello/agent-orchestrator/internal/store/postgres"
)

func newStore(t *testing.T) *postgres.Store {
	t.Helper()
	ctx := context.Background()
	pool := testPool(t)
	resetDB(t, pool)
	require.NoError(t, postgres.Migrate(ctx, pool))
	return postgres.NewStore(pool)
}

func TestStore_WorkItemRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	wi := &domain.WorkItem{
		ID:            domain.NewWorkItemID(),
		Project:       "p",
		Title:         "t",
		Description:   "d",
		Priority:      domain.PriorityHigh,
		Status:        domain.PhaseDiscovery,
		CurrentPhase:  domain.PhaseDiscovery,
		WorktreePath:  "/wt/WI-1",
		BaseBranch:    "main",
		BaseCommitSHA: "deadbeefcafe",
		Metadata:      domain.Metadata{"k": "v"},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	require.NoError(t, st.CreateWorkItem(ctx, wi))

	got, err := st.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, wi.ID, got.ID)
	assert.Equal(t, domain.PriorityHigh, got.Priority)
	assert.Equal(t, "deadbeefcafe", got.BaseCommitSHA, "immutable base commit must persist")
	assert.Equal(t, "v", got.Metadata["k"])
	assert.WithinDuration(t, now, got.CreatedAt, time.Millisecond)

	got.CurrentPhase = domain.PhaseDecision
	got.Status = domain.PhaseDecision
	require.NoError(t, st.UpdateWorkItem(ctx, got))

	reloaded, err := st.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseDecision, reloaded.CurrentPhase)

	list, err := st.ListWorkItems(ctx, "p")
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestStore_SessionRoundTrip(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	wi := &domain.WorkItem{ID: domain.NewWorkItemID(), Project: "p", Title: "t", Status: domain.PhaseDiscovery, CurrentPhase: domain.PhaseDiscovery, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, st.CreateWorkItem(ctx, wi))

	completed := now.Add(time.Minute)
	sess := &domain.Session{
		ID: domain.NewSessionID(), WorkItemID: wi.ID, Phase: domain.PhaseImplementation,
		Status: domain.SessionCompleted, Agent: "build", Prompt: "do it", ExitCode: 0,
		StartedAt: now, CompletedAt: &completed,
		Output: &domain.SessionOutput{BaseCommitSHA: "base", CommitSHA: "abc", Diff: "diff", DiffStat: "1 file"},
	}
	require.NoError(t, st.CreateSession(ctx, sess))

	got, err := st.GetSession(ctx, sess.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Output)
	assert.Equal(t, "abc", got.Output.CommitSHA)
	assert.Equal(t, "diff", got.Output.Diff)
	require.NotNil(t, got.CompletedAt)

	list, err := st.ListSessions(ctx, wi.ID)
	require.NoError(t, err)
	assert.Len(t, list, 1)
}

func TestStore_GateWithAuthorization(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	wi := &domain.WorkItem{ID: domain.NewWorkItemID(), Project: "p", Title: "t", Status: domain.PhaseAwaitingApproval, CurrentPhase: domain.PhaseAwaitingApproval, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, st.CreateWorkItem(ctx, wi))

	gate := &domain.Gate{
		ID: domain.NewGateID(), WorkItemID: wi.ID, Phase: domain.PhaseAwaitingApproval,
		Status: domain.GatePending, CreatedAt: now,
		Payload: domain.GatePayload{RiskAssessment: domain.RiskLow, Evidence: []domain.EvidenceRef{{Type: "decision", Summary: "s"}}},
	}
	require.NoError(t, st.CreateGate(ctx, gate))

	gate.Status = domain.GateApproved
	resolved := now.Add(time.Minute)
	gate.ResolvedAt = &resolved
	gate.Approvals = append(gate.Approvals, domain.Approval{
		User: "alice", Decision: domain.ApprovalApprove, At: now,
		Authorization: &domain.AuthorizationRecord{Granted: true, GrantedBy: "alice", GrantedAt: now},
	})
	require.NoError(t, st.UpdateGate(ctx, gate))

	got, err := st.GetGateByWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.GateApproved, got.Status)
	require.Len(t, got.Approvals, 1)
	require.NotNil(t, got.Approvals[0].Authorization)
	assert.True(t, got.Approvals[0].Authorization.Granted)
	assert.Equal(t, domain.RiskLow, got.Payload.RiskAssessment)
}

func TestStore_ConcurrencyGuard(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	mk := func(project string) *domain.WorkItem {
		return &domain.WorkItem{
			ID: domain.NewWorkItemID(), Project: project, Title: "t",
			Status: domain.PhaseDiscovery, CurrentPhase: domain.PhaseDiscovery,
			CreatedAt: now, UpdatedAt: now,
		}
	}

	require.NoError(t, st.CreateWorkItem(ctx, mk("p")))
	assert.ErrorIs(t, st.CreateWorkItem(ctx, mk("p")), store.ErrProjectBusy)

	// Terminal state releases the slot.
	first, err := st.ListWorkItems(ctx, "p")
	require.NoError(t, err)
	require.Len(t, first, 1)
	first[0].Status = domain.PhaseComplete
	first[0].CurrentPhase = domain.PhaseComplete
	require.NoError(t, st.UpdateWorkItem(ctx, first[0]))

	require.NoError(t, st.CreateWorkItem(ctx, mk("p")), "completed WorkItem should free the project slot")
}

func TestStore_EventsOrderedAndNotFound(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	now := time.Now().UTC().Truncate(time.Microsecond)

	wi := &domain.WorkItem{ID: domain.NewWorkItemID(), Project: "p", Title: "t", Status: domain.PhaseDiscovery, CurrentPhase: domain.PhaseDiscovery, CreatedAt: now, UpdatedAt: now}
	require.NoError(t, st.CreateWorkItem(ctx, wi))

	for i := 0; i < 3; i++ {
		require.NoError(t, st.AppendEvent(ctx, &domain.Event{
			ID: domain.NewEventID(), WorkItemID: wi.ID, Type: domain.EventPhaseStarted,
			Payload: map[string]any{"i": float64(i)}, Actor: domain.ActorSystem, At: now.Add(time.Duration(i) * time.Second),
		}))
	}
	events, err := st.ListEvents(ctx, wi.ID)
	require.NoError(t, err)
	require.Len(t, events, 3)
	assert.Less(t, events[0].At, events[2].At)

	_, err = st.GetWorkItem(ctx, "missing")
	assert.ErrorIs(t, err, store.ErrNotFound)
}
