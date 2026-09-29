package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stefenello/agent-orchestrator/internal/domain"
	"github.com/stefenello/agent-orchestrator/internal/service"
	"github.com/stefenello/agent-orchestrator/internal/store/memory"
)

func countEvents(t *testing.T, orch *service.Orchestrator, id domain.WorkItemID, typ domain.EventType) int {
	t.Helper()
	events, err := orch.ListEvents(context.Background(), id)
	require.NoError(t, err)
	n := 0
	for _, e := range events {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func addRunningSession(t *testing.T, st *memory.Store, wi *domain.WorkItem, phase domain.Phase) domain.SessionID {
	t.Helper()
	sess := &domain.Session{
		ID:         domain.NewSessionID(),
		WorkItemID: wi.ID,
		Phase:      phase,
		Status:     domain.SessionRunning,
		Agent:      "plan",
		Prompt:     "x",
		StartedAt:  time.Now().UTC(),
	}
	require.NoError(t, st.CreateSession(context.Background(), sess))
	return sess.ID
}

func TestReconcile_TerminalWorkItemUnchanged(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	got, _ := st.GetWorkItem(ctx, wi.ID)
	got.CurrentPhase = domain.PhaseComplete
	got.Status = domain.PhaseComplete
	require.NoError(t, st.UpdateWorkItem(ctx, got))

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "terminal WorkItems are not reconciled")
	assert.Equal(t, 0, countEvents(t, orch, wi.ID, domain.EventWorkItemReconciled))
}

func TestReconcile_MarksOrphanedSessionFailed(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{}) // autoDrive off

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	sid := addRunningSession(t, st, wi, domain.PhaseDiscovery)

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	sess, err := st.GetSession(ctx, sid)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionFailed, sess.Status)
	assert.Contains(t, sess.Error, "interrupted")
	require.NotNil(t, sess.CompletedAt)

	// Audit trail: interrupted session + reconciled WorkItem.
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventSessionFailed))
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventWorkItemReconciled))
}

func TestReconcile_Idempotent(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, wi, domain.PhaseImplementation)

	_, err = orch.Reconcile(ctx)
	require.NoError(t, err)
	_, err = orch.Reconcile(ctx) // second run must not duplicate anything
	require.NoError(t, err)

	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventSessionFailed), "no duplicate session.failed events")
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventWorkItemReconciled), "no duplicate reconciled events")
}

func TestReconcile_ResumesInterruptedWorkItem(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	adapter := &fakeAdapter{}
	orch := service.New(st, &fakeProvisioner{}, adapter)
	orch.SetAutoDrive(true)

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	// Simulate a crash: a running session persisted while the process died.
	sid := addRunningSession(t, st, wi, domain.PhaseDiscovery)

	_, err = orch.Reconcile(ctx)
	require.NoError(t, err)

	// The orphaned session is marked failed...
	sess, err := st.GetSession(ctx, sid)
	require.NoError(t, err)
	assert.Equal(t, domain.SessionFailed, sess.Status)

	// ...and the WorkItem continues to the gate without manual repair.
	require.Eventually(t, func() bool {
		got, err := orch.GetWorkItem(ctx, wi.ID)
		return err == nil && (got.CurrentPhase == domain.PhaseAwaitingApproval || got.CurrentPhase.IsTerminal())
	}, 5*time.Second, 20*time.Millisecond)
}

// blockingAdapter blocks in Run until released, to test single-driver discipline.
type blockingAdapter struct {
	started chan struct{}
	release chan struct{}
	mu      sync.Mutex
	calls   int
}

func (b *blockingAdapter) ValidateCLI(context.Context) (domain.CLICapabilities, error) {
	return domain.CLICapabilities{Version: "test"}, nil
}

func (b *blockingAdapter) Run(ctx context.Context, _ domain.RunRequest) (*domain.RunResult, error) {
	b.mu.Lock()
	b.calls++
	b.mu.Unlock()
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-b.release
	return &domain.RunResult{ExitCode: 0}, nil
}

func (b *blockingAdapter) callCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.calls
}

func TestDrive_SingleDriverDiscipline(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	ad := &blockingAdapter{started: make(chan struct{}, 4), release: make(chan struct{})}
	orch := service.New(st, &fakeProvisioner{}, ad)

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)

	go func() { _ = orch.Drive(ctx, wi.ID) }()
	<-ad.started // first driver entered Run

	// A second Drive for the same WorkItem must return immediately without
	// starting another execution.
	done := make(chan error, 1)
	go func() { done <- orch.Drive(ctx, wi.ID) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("second Drive did not return immediately (single-driver violated)")
	}

	assert.Equal(t, 1, ad.callCount(), "adapter must be invoked exactly once while the first driver holds the WorkItem")
	close(ad.release)
}
