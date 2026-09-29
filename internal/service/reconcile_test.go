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

	// Audit trail: interrupted session + reconciled WorkItem + phase transition.
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventSessionFailed))
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventWorkItemReconciled))
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventPhaseFailed))

	// The interrupted WorkItem itself becomes failed so the project slot is
	// released and the existing retry API can resume it.
	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseFailed, got.CurrentPhase)
	assert.Equal(t, domain.PhaseFailed, got.Status)
}

func TestReconcile_Idempotent(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, wi, domain.PhaseImplementation)

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	n, err = orch.Reconcile(ctx) // second run must not duplicate anything
	require.NoError(t, err)
	assert.Equal(t, 0, n, "second run must reconcile nothing new")

	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventSessionFailed), "no duplicate session.failed events")
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventWorkItemReconciled), "no duplicate reconciled events")
	assert.Equal(t, 1, countEvents(t, orch, wi.ID, domain.EventPhaseFailed), "no duplicate phase.failed events")

	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseFailed, got.CurrentPhase, "WorkItem stays failed")
}

// TestReconcile_ReleasesConcurrencySlot proves the project slot is freed:
// after recovery, creating a second WorkItem for the same project succeeds.
func TestReconcile_ReleasesConcurrencySlot(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// The slot is free: a new WorkItem for the same project must succeed.
	wi2, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t2", RepoPath: "/repo"})
	require.NoError(t, err, "project slot must be released after recovery")
	assert.NotEqual(t, wi.ID, wi2.ID)
}

// TestReconcile_RecoveredWorkItemDoesNotAutoResume proves recovery does not
// silently re-drive execution: the failed WorkItem waits for an explicit retry,
// even with autoDrive enabled.
func TestReconcile_RecoveredWorkItemDoesNotAutoResume(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	adapter := &fakeAdapter{}
	orch := service.New(st, &fakeProvisioner{}, adapter) // autoDrive off during setup

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)

	_, err = orch.Reconcile(ctx)
	require.NoError(t, err)

	sessions, err := st.ListSessions(ctx, wi.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1, "reconciliation must not start a new execution")

	// Even with auto-drive enabled afterward, nothing resumes a failed WorkItem.
	orch.SetAutoDrive(true)
	require.Never(t, func() bool {
		got, err := orch.GetWorkItem(ctx, wi.ID)
		return err == nil && got.CurrentPhase != domain.PhaseFailed
	}, 300*time.Millisecond, 20*time.Millisecond, "failed WorkItem must stay failed without explicit retry")
	sessions, err = st.ListSessions(ctx, wi.ID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1, "no new execution may start without explicit retry")
}

// TestReconcile_RetryAfterRecoveryResumes proves the existing retry API accepts
// a recovered WorkItem and drives it forward again.
func TestReconcile_RetryAfterRecoveryResumes(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	adapter := &fakeAdapter{}
	orch := service.New(st, &fakeProvisioner{}, adapter) // autoDrive off during setup

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)
	addRunningSession(t, st, wi, domain.PhaseDiscovery)

	_, err = orch.Reconcile(ctx)
	require.NoError(t, err)

	orch.SetAutoDrive(true) // resumeDrive only launches the driver when enabled
	require.NoError(t, orch.Retry(ctx, wi.ID, service.RetryInput{User: "tester"}))

	// The fake adapter succeeds every phase; retry must drive discovery and
	// decision up to the human gate without manual repair.
	require.Eventually(t, func() bool {
		got, err := orch.GetWorkItem(ctx, wi.ID)
		return err == nil && got.CurrentPhase == domain.PhaseAwaitingApproval
	}, 5*time.Second, 20*time.Millisecond)
}

// TestReconcile_TerminalWorkItemsProtected proves completed, failed and blocked
// WorkItems are never touched — even if a stray running Session row exists.
func TestReconcile_TerminalWorkItemsProtected(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	orch := service.New(st, &fakeProvisioner{}, &fakeAdapter{})

	for _, phase := range []domain.Phase{domain.PhaseComplete, domain.PhaseFailed, domain.PhaseBlocked} {
		wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p-" + string(phase), Title: "t", RepoPath: "/repo"})
		require.NoError(t, err)
		got, err := st.GetWorkItem(ctx, wi.ID)
		require.NoError(t, err)
		got.CurrentPhase = phase
		got.Status = phase
		require.NoError(t, st.UpdateWorkItem(ctx, got))
		sid := addRunningSession(t, st, wi, domain.PhaseDiscovery)

		n, err := orch.Reconcile(ctx)
		require.NoError(t, err)
		assert.Equal(t, 0, n, "terminal WorkItems are never reconciled")

		sess, err := st.GetSession(ctx, sid)
		require.NoError(t, err)
		assert.Equal(t, domain.SessionRunning, sess.Status, "stray session on terminal WorkItem must be left alone")

		got, err = orch.GetWorkItem(ctx, wi.ID)
		require.NoError(t, err)
		assert.Equal(t, phase, got.CurrentPhase)
	}
}

// TestReconcile_SkipsLegitimateActiveExecution proves a WorkItem currently
// driven by THIS process is not failed by reconciliation: it is a legitimate
// active execution, not an orphan.
func TestReconcile_SkipsLegitimateActiveExecution(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	ad := &blockingAdapter{started: make(chan struct{}, 4), release: make(chan struct{})}
	orch := service.New(st, &fakeProvisioner{}, ad)

	wi, err := orch.CreateWorkItem(ctx, service.CreateWorkItemInput{Project: "p", Title: "t", RepoPath: "/repo"})
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- orch.Drive(ctx, wi.ID) }()
	<-ad.started // driver entered Run: the WorkItem is legitimately active here

	n, err := orch.Reconcile(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, n, "legitimate active execution must not be reconciled")

	sessions, err := st.ListSessions(ctx, wi.ID)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, domain.SessionRunning, sessions[0].Status, "in-flight session must stay running")
	assert.Equal(t, 0, countEvents(t, orch, wi.ID, domain.EventSessionFailed))
	assert.Equal(t, 0, countEvents(t, orch, wi.ID, domain.EventPhaseFailed))

	got, err := orch.GetWorkItem(ctx, wi.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PhaseDiscovery, got.CurrentPhase, "driven phase must not be failed")

	close(ad.release) // let the driver finish cleanly
	require.NoError(t, <-done)
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
